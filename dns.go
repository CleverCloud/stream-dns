package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	s "strings"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
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

func registerHandlerForResolver(pattern string, db *bolt.DB, address string) {
	dns.HandleFunc(pattern, func(w dns.ResponseWriter, r *dns.Msg) {
		log.Info("Got a request for unsupported zone ", r.Question[0].Name)

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

func registerHandlerForZones(zones []string, db *bolt.DB) {
	for _, z := range zones {
		registerHandlerForZone(z, db)
	}
}

// Register a zone e.g: foo.com with the default handler
func registerHandlerForZone(zone string, db *bolt.DB) {
	dns.HandleFunc(zone, func(w dns.ResponseWriter, r *dns.Msg) {
		log.Info("Got a request for ", r.Question[0].Name)

		qname := r.Question[0].Name
		qtype := r.Question[0].Qtype

		m := new(dns.Msg)
		m.SetReply(r)

		if qtype != dns.TypeAXFR {
			findRecordsAndSetAsAnswersInMessage(qname, qtype, db, m, r)
		} else {
			findAllRecordsForTheZoneTransferAndSetAsAnswersInMessage(qname, db, m, r)
		}

		m.SetRcode(r, dns.RcodeSuccess)
		w.WriteMsg(m)
	})
}

func findRecordsAndSetAsAnswersInMessage(qname string, qtype uint16, db *bolt.DB, m *dns.Msg, r *dns.Msg) {
	db.View(func(tx *bolt.Tx) error {
		// TODO: check if the bucket already exists and has keys
		recordsBucket := tx.Bucket([]byte("records"))

		records, err := getRecordsFromBucket(recordsBucket, qname)

		if err != nil {
			log.Fatal(err)
			return nil
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

		return nil
	})
}

// NOTE unimplemented
func findAllRecordsForTheZoneTransferAndSetAsAnswersInMessage(qname string, db *bolt.DB, m *dns.Msg, r *dns.Msg) {
	m.SetRcode(r, dns.RcodeSuccess)
}
