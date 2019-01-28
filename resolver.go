package main

import (
	"log"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type QueryType uint16

type Query struct {
	target string
	types  []QueryType
}

func NewQuery(target string, types ...QueryType) Query {
	return Query{target, types}
}

type Resolver struct {
	server     string
	query      Query
	timeout    time.Duration
	retryTimes uint
}

func NewResolver(server string, query Query, retryTimes time.Duration, timeout uint) Resolver {
	server = AddDefaultDNSPortIfIsNotDefine(server)

	return Resolver{server, query, retryTimes, timeout}
}

func (r *Resolver) Lookup() (map[QueryType][]Record, error) {
	recordsMap := map[QueryType][]Record{}

	nbTypes := len(r.query.types)
	resultsChan := make(chan struct {
		records []Record
		QueryType
		error
	})

	// A dns question can only support one query type.
	// So to improve the performance, we have to spawn exchanges in goroutine
	for _, t := range r.query.types {
		go r.goExchange(r.query.target, r.server, t, resultsChan)
	}

	for i := 0; i < nbTypes; i++ {
		res := <-resultsChan

		if res.error != nil {
			// FIXME We should found a way to manage the errors.
			// We spawn a goroutine for each QueryType and we get all the answers through a channel.
			// If we stop at the first error encountered, we will have zombie goroutine because they
			// 'll block on the send: resultsChan <- records.
			// Maybe we can do some best efforts and return both Records (for types that have succeeded)
			// and error for others (not sure the API'll be great).
			log.Fatal(res.error)
		}

		if res.error == nil && len(res.records) > 0 {
			recordsMap[res.QueryType] = res.records
		}
	}

	return recordsMap, nil
}

// Method to track the exchange value and forward the result through the channel.
// This method should be use with a goroutine.
func (r *Resolver) goExchange(target string, server string, queryType QueryType, resultsChan chan struct {
	records []Record
	QueryType
	error
}) {
	records, err := r.exchange(target, server, queryType)
	resultsChan <- struct {
		records []Record
		QueryType
		error
	}{records, queryType, err}
}

func (r *Resolver) exchange(target string, server string, queryType QueryType) ([]Record, error) {
	records := []Record{}

	msg := &dns.Msg{}
	target = AddFinalDotIfIsNotSet(target)
	msg.SetQuestion(target, uint16(queryType))

	client := &dns.Client{DialTimeout: r.timeout * time.Millisecond}

	res, _, err := client.Exchange(msg, server)

	if err == nil && len(res.Answer) > 0 {
		records = AnswersToRecord(target, queryType, res.Answer)
	}

	return records, err
}

// www.yolo.com -> www.yolo.com.
func AddFinalDotIfIsNotSet(target string) string {
	if target[len(target)-1:] != "." {
		target = target + "."
	}

	return target
}

// 1.1.1.1 -> 1.1.1.1:53
// We do nothing if the port is already define
// 1.1.1.1:53 -> 1.1.1.1:53
// 1.1.1.1:8053 -> 1.1.1.1:8053
func AddDefaultDNSPortIfIsNotDefine(server string) string {
	if server[len(server)-3:] != ":53" && !strings.Contains(server, ":") {
		return server + ":53"
	}

	return server
}

func AnswersToRecord(target string, queryType QueryType, answers []dns.RR) []Record {
	records := []Record{}

	for _, answer := range answers {
		record := AnswerToRecord(target, queryType, answer)
		records = append(records, record)
	}

	return records
}

func AnswerToRecord(name string, queryType QueryType, answer dns.RR) Record {
	record := Record{
		Name: name,
		Type: dns.TypeToString[uint16(queryType)],
		Ttl:  int(answer.Header().Ttl),
	}

	// TODO support more type
	switch uint16(queryType) {
	case dns.TypeA:
		if a, ok := answer.(*dns.A); ok {
			record.Content = a.A.String()
		}
	case dns.TypeAAAA:
		if a4, ok := answer.(*dns.AAAA); ok {
			record.Content = a4.AAAA.String()
		}
	case dns.TypeCNAME:
		if cname, ok := answer.(*dns.CNAME); ok {
			record.Content = cname.Target
		}
	case dns.TypeMX:
		if mx, ok := answer.(*dns.MX); ok {
			record.Content = mx.Mx
			record.Priority = int(mx.Preference)
		}
	case dns.TypeNS:
		if ns, ok := answer.(*dns.NS); ok {
			record.Content = ns.Ns
		}
	case dns.TypeTXT:
		if txt, ok := answer.(*dns.TXT); ok {
			record.Content = strings.Join(txt.Txt, " ")
		}
	}

	return record
}
