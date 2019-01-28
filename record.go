package main

import (
	"fmt"
	dns "github.com/miekg/dns"
	"log"
)

type Record struct {
	Name     string
	Type     string
	Content  string
	Ttl      int
	Priority int
}

func A(rr string) *dns.A { r, _ := dns.NewRR(rr); return r.(*dns.A) }

func AAAA(rr string) *dns.AAAA { r, _ := dns.NewRR(rr); return r.(*dns.AAAA) }

func CNAME(rr string) *dns.CNAME { r, _ := dns.NewRR(rr); return r.(*dns.CNAME) }

func SOA(rr string) *dns.SOA { r, _ := dns.NewRR(rr); return r.(*dns.SOA) }

func MX(rr string) *dns.MX { r, _ := dns.NewRR(rr); return r.(*dns.MX) }

func NS(rr string) *dns.NS { r, _ := dns.NewRR(rr); return r.(*dns.NS) }

func TXT(rr string) *dns.TXT { r, _ := dns.NewRR(rr); return r.(*dns.TXT) }

func PTR(rr string) *dns.PTR { r, _ := dns.NewRR(rr); return r.(*dns.PTR) }

func recordToString(record Record) string {
	if record.Priority > 0 {
		return fmt.Sprintf("%s %d IN %s %d %s", record.Name, record.Ttl, record.Type, record.Priority, record.Content)
	} else {
		return fmt.Sprintf("%s %d IN %s %s", record.Name, record.Ttl, record.Type, record.Content)
	}
}

func RecordToAnswer(record Record) dns.RR {
	var rr dns.RR
	rtype := dns.StringToType[record.Type]
	recordstr := recordToString(record)

	switch rtype {
	case dns.TypeA:
		rr = A(recordstr)
	case dns.TypeAAAA:
		rr = AAAA(recordstr)
	case dns.TypeCNAME:
		rr = CNAME(recordstr)
	case dns.TypeSOA:
		rr = SOA(recordstr)
	case dns.TypeMX:
		rr = MX(recordstr)
	case dns.TypeNS:
		rr = NS(recordstr)
	case dns.TypeTXT:
		rr = TXT(recordstr)
	case dns.TypePTR:
		rr = PTR(recordstr)
	default:
		log.Fatalf("Incorrect or unsupported record type: %s", recordstr)
		return nil
	}

	return rr
}

func RecordsToAnswer(records []Record) []dns.RR {
	var rrs []dns.RR

	for _, record := range records {
		rrs = append(rrs, RecordToAnswer(record))
	}

	return rrs
}
