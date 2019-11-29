package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	a "stream-dns/agent"
	"stream-dns/utils"
	"time"

	"github.com/google/uuid"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

// DNS Resolution configuration.
const (
	Timeout             = 2000 * time.Millisecond
	TypicalResponseTime = 100 * time.Millisecond
	MaxRecursion        = 5
	MaxNameservers      = 4
)

// Resolver errors.
var (
	NXDOMAIN = fmt.Errorf("NXDOMAIN")

	ErrMaxRecursion = fmt.Errorf("maximum recursion depth reached: %d", MaxRecursion)
	ErrTimeout      = fmt.Errorf("timeout expired")
)

// AllDomain is a constant to match all the domain by using the QNAME
const AllDomain = "."

type PairKeyRRraw struct {
	key    []byte
	rrsRaw []byte
}

// QuestionResolverHandler handler to answer to DNS question
type QuestionResolverHandler struct {
	db             *bolt.DB
	config         DnsConfig
	metricsService *a.MetricsService
	resolver       *Resolver
}

// NewQuestionResolverHandler create a new QuestionResolverHandler
func NewQuestionResolverHandler(db *bolt.DB, config DnsConfig, ms *a.MetricsService) QuestionResolverHandler {
	return QuestionResolverHandler{
		db:             db,
		config:         config,
		metricsService: ms,
		resolver:       NewResolver(), //TODO: make timeout configurable
	}
}

// ServeDNS is the handler registered in the dns.Server.Handler
func (h *QuestionResolverHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	requestID := uuid.New().String()
	remoteAddr := w.RemoteAddr().String()

	var rcode int
	msg := dns.Msg{}
	msg.SetReply(r)
	question := msg.Question[0] // we only support one question

	if h.isALocalRecord(dns.Fqdn(question.Name)) {
		msg.Authoritative = true
		msg.RecursionAvailable = true
	}

	log.WithFields(log.Fields{
		"ip":         remoteAddr,
		"request-id": requestID,
		"domain":     question.Name,
		"qtype":      dns.TypeToString[question.Qtype],
		"qclass":     dns.ClassToString[question.Qclass],
	}).Info("Got a new DNS question")

	rcode, msg.Answer = h.resolveQuestion(question, msg.RecursionDesired)

	log.WithFields(log.Fields{
		"ip":         remoteAddr,
		"request-id": requestID,
		"domain":     question.Name,
		"qtype":      dns.TypeToString[question.Qtype],
		"qclass":     dns.ClassToString[question.Qclass],
		"answers":    msg.Answer,
	}).Info("Find answer for the question")

	// May optionally carry the SOA RR for the authoritative data in the answer section. c.f RFC 1034
	if len(msg.Answer) == 0 && h.isALocalRecord(question.Name) {
		msg.Authoritative = true
		zone := utils.GetZoneFromQname(question.Name)
		soa := h.getSOAForTheZone(zone)

		if soa != nil {
			msg.Ns = append(msg.Ns, soa)
		}
	}

	msg.SetRcode(r, rcode)
	err := w.WriteMsg(&msg)

	if err != nil {
		log.WithFields(log.Fields{
			"ip":         w.RemoteAddr,
			"request-id": requestID,
			"domain":     question.Name,
			"qtype":      dns.TypeToString[question.Qtype],
			"qclass":     dns.ClassToString[question.Qclass],
		}).Error(err)
	}
}

// Main point to resolve a question
// That call the method lookupRecord to get the RRs.
// The Rcode depend on the RRs got and the error from the call of the submethod lookupRecord
func (h *QuestionResolverHandler) resolveQuestion(question dns.Question, recursionDesired bool) (rcode int, rrs []dns.RR) {
	rrs, err := h.lookupRecord(dns.Fqdn(question.Name), question.Qtype, recursionDesired, 0)
	replaceWildcardByQnameInRRsIfThereAre(rrs, question.Name)
	rcode = dns.RcodeSuccess

	if err != nil {
		rcode = dns.RcodeServerFailure
	}

	if len(rrs) == 0 {
		rcode = dns.RcodeNameError
	}

	//TODO: truncate the response if the payload (rrs) is over 512 bytes and set the TC header flag.

	return
}

// lookupRecord find DNS records of type qtype for the domain qname.
// For nonexistent domains (NXDOMAIN), it will return an empty, non-nil slice.
// Currently supported qtype: A, AAAA, NS, CNAME, SOA, and TXT
func (h *QuestionResolverHandler) lookupRecord(qname string, qtype uint16, recursionDesired bool, depth int) (rrs []dns.RR, err error) {
	log.Debugf("Lookup for the record %s %s", qname, dns.TypeToString[qtype])
	if depth++; depth > MaxRecursion {
		return nil, ErrMaxRecursion
	}

	if h.isALocalRecord(qname) {
		rrs, err = h.lookupRecordInLocalDB(qname, qtype)
	} else {
		rrs = h.resolver.Resolve(qname, qtype)
	}

	// Change the wildcard canonical name for the owner, the qname of the last record.
	replaceWildcardByQnameInRRsIfThereAre(rrs, qname)

	// If the response contains a CNAME, the search is restarted at the CNAME
	// unless the response has the data for the canonical name or if the CNAME
	// is the answer itself.
	if recursionDesired && IsCnameRes(rrs) {
		tmp := rrs[0].(*dns.CNAME)
		rrstmp, err := h.lookupRecord(tmp.Target, qtype, recursionDesired, depth)

		if err != nil {
			return []dns.RR{}, err
		}

		// copy the CNAME RR into the answer section of the response.
		return append(rrs, rrstmp...), err
	}

	// copy all RRs which match QTYPE into the answer section
	return rrs, err
}

// lookupRecordInLocalDB look for a record in the local db and map the raw result into a RRs result
func (h *QuestionResolverHandler) lookupRecordInLocalDB(qname string, qtype uint16) (rrs []dns.RR, err error) {
	rawRRs, err := h.selectRawRecordInLocalDb([]byte(dns.Fqdn(qname)))
	rrs, err = mapPairKeyRawRRsIntoRR(rawRRs)

	if len(rrs) == 0 {
		// If at some label, a match is impossible (i.e., the
		// corresponding label does not exist), look to see if a
		// the "*" label exists.
		wildcardQname := utils.IntoWildcardQname(dns.Fqdn(qname))
		rawRRs, err = h.selectRawRecordInLocalDb([]byte(wildcardQname))
		rrs, err = mapPairKeyRawRRsIntoRR(rawRRs)
	}

	return
}

// selectRawRecordInLocalDb find a record in the bbolt DB and return a raw result
func (h *QuestionResolverHandler) selectRawRecordInLocalDb(prefix []byte) (rawRecords PairKeyRRraw, err error) {
	err = h.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(RecordBucket)

		c := bucket.Cursor()

		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			rawRecords = PairKeyRRraw{key: k, rrsRaw: v}
		}

		return nil
	})

	return
}

// Check if the Qname of a question is a local record related to the managed zones set in the config
func (h *QuestionResolverHandler) isALocalRecord(qname string) (isLocal bool) {
	return utils.IsALocalRR(qname, h.config.Zones)
}

// getSOAForTheZone return the SOA for a specific zone
// The Authority section of the response may optionally carry the
// SOA RR for the authoritative data in the answer section.
func (h *QuestionResolverHandler) getSOAForTheZone(zone string) (soa dns.RR) {
	log.Info("looking SOA for the zone ", zone)

	h.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(RecordBucket)
		soaRaw := bucket.Get(utils.Key(zone, dns.TypeSOA))

		var soatmp []dns.SOA
		err := json.Unmarshal(soaRaw, &soatmp)

		if err != nil {
			log.WithField("zone", zone).Error(err)
			return nil
		}

		soa, err = dns.NewRR(soatmp[0].String())

		if err != nil {
			log.Error(err)
			return nil
		}

		return nil
	})

	return
}

// smapPairKeyRawRRsIntoRR serialize a slice of RR get from bbolt into a dns.RR slice.
// Yeah Go doesn't need generics...
func mapPairKeyRawRRsIntoRR(pair PairKeyRRraw) (rrs []dns.RR, err error) {
	if pair.key == nil || pair.rrsRaw == nil {
		return []dns.RR{}, nil
	}

	qname, qtype := utils.ExtractQnameAndQtypeFromKey(pair.key)

	switch qtype {
	case dns.TypeA:
		var tmp []dns.A
		err = json.Unmarshal(pair.rrsRaw, &tmp)

		if err != nil {
			return
		}

		for _, val := range tmp {
			rr, err := dns.NewRR(val.String())
			if err != nil {
				return nil, err
			}
			rrs = append(rrs, rr)
		}
		break
	case dns.TypeAAAA:
		var tmp []dns.AAAA
		err = json.Unmarshal(pair.rrsRaw, &tmp)

		if err != nil {
			return
		}

		for _, val := range tmp {
			rr, err := dns.NewRR(val.String())
			if err != nil {
				return nil, err
			}
			rrs = append(rrs, rr)
		}
		break
	case dns.TypeCNAME:
		var tmp []dns.CNAME
		err = json.Unmarshal(pair.rrsRaw, &tmp)

		if err != nil {
			return
		}

		for _, val := range tmp {
			rr, err := dns.NewRR(val.String())
			if err != nil {
				return nil, err
			}
			rrs = append(rrs, rr)
		}
		break
	case dns.TypeTXT:
		var tmp []dns.TXT
		err = json.Unmarshal(pair.rrsRaw, &tmp)

		if err != nil {
			return
		}

		for _, val := range tmp {
			rr, err := dns.NewRR(val.String())
			if err != nil {
				return nil, err
			}
			rrs = append(rrs, rr)
		}
		break
	case dns.TypeSOA:
		var tmp []dns.SOA
		err = json.Unmarshal(pair.rrsRaw, &tmp)

		if err != nil {
			return
		}

		for _, val := range tmp {
			rr, err := dns.NewRR(val.String())
			if err != nil {
				return nil, err
			}
			rrs = append(rrs, rr)
		}
		break
	case dns.TypePTR:
		var tmp []dns.PTR
		err = json.Unmarshal(pair.rrsRaw, &tmp)

		if err != nil {
			return
		}

		for _, val := range tmp {
			rr, err := dns.NewRR(val.String())
			if err != nil {
				return nil, err
			}
			rrs = append(rrs, rr)
		}
		break
	case dns.TypeNS:
		var tmp []dns.NS
		err = json.Unmarshal(pair.rrsRaw, &tmp)

		if err != nil {
			return
		}

		for _, val := range tmp {
			rr, err := dns.NewRR(val.String())
			if err != nil {
				return nil, err
			}
			rrs = append(rrs, rr)
		}
		break
	default:
		return nil, fmt.Errorf("Can't unmarshall %s, his QTYPE doesn't exist or isn't supported yet", qname)
	}

	return
}

// Check if this slice is not the content of a CNAME response after a lookup
func IsNotCNAMERes(rrs []dns.RR) bool {
	return !IsCnameRes(rrs)
}

// Check if this slice is the content of a CNAME response after a lookup
// A CNAME response has only one RR in it with the qtype: CNAME
func IsCnameRes(rrs []dns.RR) bool {
	return len(rrs) == 1 && rrs[0].Header().Rrtype == dns.TypeCNAME
}

// replaceWildcardByQnameInRRsIfThereAre return a list of RR where all the wildcard domain has been
// change for the qname. This dont't have an effect in the qname is already a wildcard domain.
func replaceWildcardByQnameInRRsIfThereAre(rrs []dns.RR, qname string) []dns.RR {
	for _, rr := range rrs {
		if utils.IsAWildcardRR(rr) {
			rr.Header().Name = qname
		}
	}

	return rrs
}
