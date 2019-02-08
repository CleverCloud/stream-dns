package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	s "strings"
	"time"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
	ms "kafka-dns/metrics"
	u "kafka-dns/utils"
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
		log.Info("[DNS] Got a request for unsupported zone ", r.Question[0].Name)
		metrics <- ms.NewMetric("resolver", nil, nil, time.Now(), ms.Counter)

		qname := r.Question[0].Name
		qtype := r.Question[0].Qtype

		m := new(dns.Msg)
		m.SetReply(r)

		query := NewQuery(qname, QueryType(qtype))
		resolver := NewResolver(address, query, 2, 4)

		records, err := resolver.Lookup()

		if err != nil {
			log.Fatal(err)
			m.SetRcode(r, dns.RcodeServerFailure)
		}

		if len(records) > 0 {
			answers := RecordsToAnswer(records[QueryType(qtype)])

			for _, answer := range answers {
				m.Answer = append(m.Answer, answer)
			}
		}

		m.SetRcode(r, dns.RcodeSuccess)
		w.WriteMsg(m)
	})
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

		log.Info("[DNS] Got a request for ", r.Question[0].Name, " with the type ", dns.TypeToString[qtype])

		m := new(dns.Msg)
		m.SetReply(r)

		if qtype != dns.TypeAXFR {
			findRecordsAndSetAsAnswersInMessage(qname, qtype, db, m, r, metrics)
			w.WriteMsg(m)
		} else {
			handlerZoneTransfer(qname, db, m, r, w, metrics)
		}
	})
}

func findRecordsAndSetAsAnswersInMessage(qname string, qtype uint16, db *bolt.DB, m *dns.Msg, r *dns.Msg, metrics chan ms.Metric) {
	metrics <- ms.NewMetric("queries", nil, nil, time.Now(), ms.Counter)

	err := db.View(func(tx *bolt.Tx) error {
		// TODO: check if the bucket already exists and has keys
		recordsBucket := tx.Bucket([]byte("records"))

		records, err := getRecordsFromBucket(recordsBucket, qname)

		if err != nil {
			return err
		} else {
			if len(records) > 0 {
				for _, subRecords := range records {
					filteredSubRecords := filterByQtypeAndCname(subRecords, qtype)
					tmp := RecordsToAnswer(filteredSubRecords)

					for _, record := range tmp {
						if isSameQtypeOrItsCname(qtype, record.Header().Rrtype) {
							m.Answer = append(m.Answer, record)
						}
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
		log.Fatal(err)
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
		log.Fatal(err)
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
		for _, recordValues := range records[begin : u.Min(end, lenRecords)] {
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
