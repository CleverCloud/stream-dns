package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	s "strings"
	"time"

	"github.com/getsentry/raven-go"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	bolt "go.etcd.io/bbolt"

	ms "stream-dns/metrics"
	u "stream-dns/utils"
)

func intoWildcardQName(qname string) string {
	return fmt.Sprintf("*%s", qname[s.Index(qname, "."):len(qname)-1])
}

func isNotWildcardName(qname string) bool {
	return qname[0] != '*'
}

func isSameQtypeOrItsCname(qtypeQuestion uint16, qtypeRecord uint16) bool {
	return qtypeRecord == qtypeQuestion || qtypeRecord == dns.TypeCNAME
}

func filterByQtypeAndCname(records []Record, qtype uint16) []Record {
	var filteredRecords []Record

	for _, record := range records {
		rrTypeRecord := dns.StringToType[record.Type]

		if isSameQtypeOrItsCname(qtype, rrTypeRecord) {
			filteredRecords = append(filteredRecords, record)
		}
	}

	return filteredRecords
}

func getRecordsFromBucket(bucket *bolt.Bucket, qname string) ([][]Record, error) {
	var records [][]Record = [][]Record{}
	c := bucket.Cursor()

	prefix := []byte(qname)
	for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
		var record []Record
		err := json.Unmarshal([]byte(v), &record)

		if err != nil {
			raven.CaptureError(err, map[string]string{"unit": "dns"})
			return nil, err
		} else {
			records = append(records, record)
		}
	}

	if isNotWildcardName(qname) && len(records) == 0 {
		c.First()

		prefixWildcard := []byte(intoWildcardQName(qname))
		for k, v := c.Seek(prefixWildcard); k != nil && bytes.HasPrefix(k, prefixWildcard); k, v = c.Next() {
			var record []Record

			err := json.Unmarshal([]byte(v), &record)
			if err != nil {
				raven.CaptureError(err, map[string]string{"unit": "dns"})
				return nil, err
			} else {
				records = append(records, record)
			}
		}
	}

	return records, nil
}

func registerHandlerForResolver(pattern string, db *bolt.DB, address string, metrics chan ms.Metric) {
	dns.HandleFunc(pattern, func(w dns.ResponseWriter, r *dns.Msg) {
		qname := r.Question[0].Name
		qtype := r.Question[0].Qtype
		remoteAddr := w.RemoteAddr()

		log.Infof("[DNS] Got a request from %s::%s for unsupported zone: %s for the type %s", remoteAddr.Network(), remoteAddr.String(), qname, dns.TypeToString[qtype])
		metrics <- ms.NewMetric("resolver", nil, nil, time.Now(), ms.Counter)

		m := new(dns.Msg)
		m.SetReply(r)

		if qtype == dns.TypeAXFR {
			// If a server is not authoritative for the queried zone, the server SHOULD set the value to NotAuth(9).
			// The query has reached the resolver with an AXFR type then the query ask for a not authoritative zone.
			// More info: IETF RFC-5936 https://tools.ietf.org/html/rfc5936#section-2.2.1
			m.SetRcode(r, dns.RcodeNotAuth) // Server Not Authoritative for zone
		} else {
			answers, err := resolverLookup(address, qname, qtype)

			if err != nil {
				metrics <- ms.NewMetric("resolver-error", nil, nil, time.Now(), ms.Counter)
				raven.CaptureError(err, map[string]string{"unit": "dns"})
				log.Errorf("Resolver: %s for %s %s", err, qname, dns.TypeToString[qtype])
				m.SetRcode(r, dns.RcodeServerFailure)
			} else {
				for _, answer := range answers {
					m.Answer = append(m.Answer, answer)
				}
				m.SetRcode(r, dns.RcodeSuccess)
			}
		}

		w.WriteMsg(m)
		answersfmt := u.FormatAnswers(m.Answer)
		log.Infof("[DNS] Answered to %s::%s request: %s with the type %s - found %d answer(s): \n %s",
			remoteAddr.Network(), remoteAddr.String(), qname, dns.TypeToString[qtype], len(m.Answer), answersfmt)
	})
}

func resolverLookup(address string, qname string, qtype uint16) ([]dns.RR, error) {
	query := NewQuery(qname, QueryType(qtype))
	resolver := NewResolver(address, query, 2, 4)

	records, err := resolver.Lookup()

	if err != nil {
		raven.CaptureError(err, map[string]string{"unit": "dns", "action": "lookup"})
		return nil, err
	}

	answers := RecordsToAnswer(records[QueryType(qtype)])
	return answers, nil
}

func registerHandlerForZones(zones []string, db *bolt.DB, metrics chan ms.Metric) {
	for _, z := range zones {
		registerHandlerForZone(z, db, metrics)
	}
}

// Register a zone e.g: foo.com with the default handler
func registerHandlerForZone(zone string, db *bolt.DB, metrics chan ms.Metric) {
	dns.HandleFunc(zone, func(w dns.ResponseWriter, r *dns.Msg) {
		qname := r.Question[0].Name
		qtype := r.Question[0].Qtype

		remoteAddr := w.RemoteAddr()
		log.Infof("[DNS] Got a request from %s::%s for %s with the type %s ", remoteAddr.Network(), remoteAddr.String(), r.Question[0].Name, dns.TypeToString[qtype])

		m := new(dns.Msg)
		m.SetReply(r)

		if qtype == dns.TypeAXFR {
			if viper.GetBool(ALLOW_AXFR) {
				handlerZoneTransfer(qname, db, m, r, w, metrics)
			} else {
				log.Errorf("[CONFIG] AXFR request is not allowed unless you turn option %s on.", formatConfig(ALLOW_AXFR))
				w.WriteMsg(m)
			}
		} else {
			findRecordsAndSetAsAnswersInMessage(qname, qtype, db, m, r, metrics)
			w.WriteMsg(m)
			answersfmt := u.FormatAnswers(m.Answer)
			log.Infof("[DNS] Answered to %s::%s request: %s with the type %s - found %d answer(s): \n%s",
				remoteAddr.Network(), remoteAddr.String(), qname, dns.TypeToString[qtype], len(m.Answer), answersfmt)
		}
	})
}

//FIXME: This algorithme is not optimal...We should improve this
//First, we recurse on all CNAMEs until we Get a last domain wich is not a CNAME
//We get all records prefixed by this last domain
func recursionOnCname(b *bolt.Bucket, record Record) [][]Record {
	var records [][]Record
	tmp := record

	// Recurse on chain of CNAME
	for tmp.Type == dns.TypeToString[dns.TypeCNAME] {
		v := b.Get([]byte(tmp.Content + "|" + dns.TypeToString[dns.TypeCNAME]))
		if v != nil {
			var recordTmp []Record
			err := json.Unmarshal([]byte(v), &recordTmp)

			if err != nil {
				break // just abort and return what we have
			}

			records = append(records, recordTmp)

			// We doesn't allow CNAME to point on multiples CNAMES
			if len(recordTmp) == 1 && recordTmp[0].Type == dns.TypeToString[dns.TypeCNAME] {
				//continue the recursion but register this CNAME
				tmp = recordTmp[0]
			}
		} else {
			// We don't have a concrete record in the DB
			break
		}
	}

	// Found the last concrete record: A, AAAA
	c := b.Cursor()
	prefix := []byte(tmp.Content)

	for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
		var res []Record
		err := json.Unmarshal([]byte(v), &res)

		if err != nil {
			log.Error("json.unmarshal on record", string(k))
		}

		records = append(records, res)
	}

	return records
}

func findRecordsAndSetAsAnswersInMessage(qname string, qtype uint16, db *bolt.DB, m *dns.Msg, r *dns.Msg, metrics chan ms.Metric) {
	metrics <- ms.NewMetric("queries", nil, nil, time.Now(), ms.Counter)

	err := db.View(func(tx *bolt.Tx) error {
		// TODO: check if the bucket already exists and has keys
		recordsBucket := tx.Bucket([]byte("records"))

		records, err := getRecordsFromBucket(recordsBucket, qname)

		// recursion on CNAME
		// We do a recursion only if we request a domain and we got only a CNAME has answer
		if len(records) == 1 && len(records[0]) == 1 && records[0][0].Type == dns.TypeToString[dns.TypeCNAME] {
			recordsFoundByRecursionOnCNAME := recursionOnCname(recordsBucket, records[0][0])
			for _, r := range recordsFoundByRecursionOnCNAME {
				records = append(records, r)
			}
		}

		if err != nil {
			raven.CaptureError(err, map[string]string{"unit": "dns", "action": "find records"})
			return err
		} else {
			if len(records) > 0 {
				for _, subRecords := range records {
					if qtype != dns.TypeCNAME {
						subRecords = filterByQtypeAndCname(subRecords, qtype)
					}
					answers := RecordsToAnswer(subRecords)

					for _, record := range answers {
						m.Answer = append(m.Answer, record)
					}
				}
			} else {
				m.SetRcode(r, dns.RcodeNameError) // return NXDOMAIN
			}
		}

		m.SetRcode(r, dns.RcodeSuccess)
		return nil
	})

	if err != nil {
		metrics <- ms.NewMetric("err-queries", nil, nil, time.Now(), ms.Counter)
		raven.CaptureError(err, map[string]string{"unit": "dns", "action": "find records"})
		log.Errorf("[dns]: %s for %s %s", err, qname, dns.TypeToString[qtype])
	}
}

// e.g: domai.com.|A -> domain.com.
func extractDomainFromKey(key []byte) []byte {
	tmp := key[:bytes.Index(key, []byte("|"))]
	return tmp
}

// Answer to AXFR request
// The AXFR protocol treats the zone contents as an unordered set of RRs.
// Except for the requirement that the transfer must begin and end with the SOA RR,
// there is no requirement to send the RRs in any particular order or
// grouped into response messages in any particular way.
//
// More info RFC5936: https://tools.ietf.org/html/rfc5936#section-2.2
func handlerZoneTransfer(qname string, db *bolt.DB, m *dns.Msg, r *dns.Msg, w dns.ResponseWriter, metrics chan ms.Metric) {
	log.Info("[DNS] request a transfer zone for ", qname)
	metrics <- ms.NewMetric("queries-axfr", nil, nil, time.Now(), ms.Counter)

	soa, records, err := findRecordsAndSOAForAXFRInDB(db, qname)

	if err != nil {
		metrics <- ms.NewMetric("err-queries-axfr", nil, nil, time.Now(), ms.Counter)
		raven.CaptureError(err, map[string]string{"unit": "dns", "action": "axfr"})
		log.Fatal("[AXFR] ", err)
		m.SetRcode(r, dns.RcodeServerFailure)
		w.WriteMsg(m)
		return
	}

	// push SOA RR at the begin of the answer
	if soa != nil {
		soaAnswer := RecordsToAnswer(soa)
		m.Answer = append(m.Answer, soaAnswer[0])
	}

	recordsLen := len(records)

	if recordsLen > 0 {
		// NOTE: We have to chunk the records payload to avoid the error: message too large
		// create a sliding window
		if recordsLen < 500 {
			for _, recordValues := range records {
				for _, answer := range RecordsToAnswer(recordValues) {
					m.Answer = append(m.Answer, answer)
				}
			}
		} else {
			sendRecordsByChunk(records, 500, m, w)
		}
	}

	// push SOA RR at the end of the answer
	if soa != nil {
		soaAnswer := RecordsToAnswer(soa)
		m.Answer = append(m.Answer, soaAnswer[0])
		w.WriteMsg(m)
	}
}

func findRecordsAndSOAForAXFRInDB(db *bolt.DB, qname string) ([]Record, [][]Record, error) {
	var soa []Record
	var records = [][]Record{}

	err := db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("records"))
		c := bucket.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			if bytes.HasSuffix(extractDomainFromKey(k), []byte(qname)) {
				var record []Record
				err := json.Unmarshal([]byte(v), &record)

				if err != nil {
					return err
				} else {
					if dns.StringToType[record[0].Type] == dns.TypeSOA {
						soa = record
					} else {
						records = append(records, record)
					}
				}
			}
		}

		return nil
	})

	return soa, records, err
}

func sendRecordsByChunk(records [][]Record, sizeChunk int, m *dns.Msg, w dns.ResponseWriter) {
	begin := 0
	end := sizeChunk // arbitrary value found by manual testing
	lenRecords := len(records)

	for begin < lenRecords {
		for _, recordValues := range records[begin:u.Min(end, lenRecords)] {
			for _, answer := range RecordsToAnswer(recordValues) {
				m.Answer = append(m.Answer, answer)
			}
		}

		w.WriteMsg(m)
		m.Answer = nil

		begin = begin + sizeChunk
		end = end + sizeChunk
	}

	m.Answer = nil
}
