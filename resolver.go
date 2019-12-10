package main

import (
	"github.com/domainr/dnsr"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
)

const defaultDNSPort = ":53"
const defaultCacheSize = 10000

// Resolver is baded on the Resolver provide by the library dnsr.
// The resolver caches responses for queries, and liberally!
// returns DNS records for a given name, not waiting for slow or broken name servers.
type Resolver struct {
	resolver *dnsr.Resolver
}

// NewResolver create a new instance Resolver
func NewResolver() *Resolver {
	return &Resolver{
		resolver: dnsr.New(defaultCacheSize),
	}
}

// Resolve find DNS records of type qtype for the domain qname.
// For nonexistent domains (NXDOMAIN), it will return an empty, non-nil slice.
// The Resolve method is based on dnsr.Resolver, which queries DNS for given name and type (A, NS, CNAME, etc.).
func (r *Resolver) Resolve(qname string, qtype uint16) []dns.RR {
	log.WithFields(log.Fields{
		"qname": qname,
		"qtype": dns.TypeToString[qtype],
	}).Info("starting a resolver request")

	tmpRRs := r.resolver.Resolve(dns.Fqdn(qname), dns.TypeToString[qtype])

	log.WithFields(log.Fields{
		"qname": qname,
		"qtype": dns.TypeToString[qtype],
	}).Trace(tmpRRs)

	rrs := r.mapRRFromDnsrIntoRR(tmpRRs)

	log.WithFields(log.Fields{
		"qname":   qname,
		"qtype":   dns.TypeToString[qtype],
		"answers": rrs,
	}).Info("get the answer for the resolve query")

	return rrs
}

// mapRRFromDnsrIntoRR map the RR from dnsr into the RR of the miekg/dns library.
func (r *Resolver) mapRRFromDnsrIntoRR(rrs dnsr.RRs) []dns.RR {
	buf := []dns.RR{}

	for _, rr := range rrs {
		tmp, err := dns.NewRR(rr.String())

		if err != nil {
			log.WithField("rr", rr.String()).Error("can't map dnsr into dns.rr")
		} else {
			buf = append(buf, tmp)
		}
	}

	return buf
}
