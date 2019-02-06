package main

import (
	"log"
	"net"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
)

func TestLookup(t *testing.T) {
	// got
	mockServer := MockServer{}
	go mockServer.Listen()

	query := NewQuery("google.com", QueryType(dns.TypeA), QueryType(dns.TypeAAAA), QueryType(dns.TypeMX))
	resolver := NewResolver("localhost:8054", query, 2, 4)

	// do
	res, err := resolver.Lookup()

	if err != nil {
		log.Fatal(err)
		t.Fail()
	}

	// want
	aaaaRecord := res[QueryType(dns.TypeAAAA)][0]
	assert.Equal(t, "2a01:7e00::f03c:91ff:fe79:234c", aaaaRecord.Content)
	
	aRecord := res[QueryType(dns.TypeA)][0]
	assert.Equal(t, "1.2.3.4", aRecord.Content)

	mxRecord := res[QueryType(dns.TypeMX)][0]
	assert.Equal(t, "aspmx.l.google.com.", mxRecord.Content)
}

// ==== Mock DNS server ====

type MockServer struct{}

func handleRequestMock(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	switch r.Question[0].Qtype {
	case dns.TypeA:
		domain := m.Question[0].Name
		m.Answer = append(m.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
			A:   net.ParseIP("1.2.3.4"),
		})
	case dns.TypeAAAA:
		domain := m.Question[0].Name
		m.Answer = append(m.Answer, &dns.AAAA{
			Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 60},
			AAAA:   net.ParseIP("2a01:7e00::f03c:91ff:fe79:234c"),
		})
	case dns.TypeMX:
		domain := m.Question[0].Name

		m.Answer = append(m.Answer, &dns.MX{
			Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: 60},
			Preference: 10,
			Mx: "aspmx.l.google.com.",
		})
	}

	w.WriteMsg(m)
}

func (m MockServer) Listen() {
	dns.HandleFunc(".", handleRequestMock)

	serverMock := &dns.Server{Addr: ":8054", Net: "udp", TsigSecret: nil}

	log.Printf("Launch the dns mock server")
	if err := serverMock.ListenAndServe(); err != nil {
		log.Fatalf("Failed to setup the mock server: %s\n", err.Error())
	}

}
