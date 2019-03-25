package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"stream-dns/metrics"

	dns "github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	bolt "go.etcd.io/bbolt"
)

// Use this list of records if your test is not a specific case
var defaultSeedRecords = [][]Record{
	[]Record{Record{"a.rock.", "A", "1.1.1.1", 3600, 0}},
	[]Record{Record{"b.rock.", "AAAA", "2.2.2.2", 1200, 0}, Record{"b.rock.", "AAAA", "3.3.3.3", 3600, 0}},
	[]Record{Record{"c.rock.", "MX", "4.4.4.4", 3600, 0}},
}

var defaultDnsConfig = DnsConfig{":8053", true, false, []string{"rock.", "services.cloud."}, "9.9.9.9"}

type DnsQuerySuite struct {
	suite.Suite
	DB *bolt.DB
}

func (suite *DnsQuerySuite) SetupTest() {
	var err error

	dbPath := fmt.Sprintf("/tmp/%s.db", randSeq(10))
	suite.DB, err = bolt.Open(dbPath, 0600, nil)

	suite.DB.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("records"))

		if err != nil {
			return fmt.Errorf("can't create bucket: %s", err)
		}

		return nil
	})

	if err != nil {
		suite.Fail("Can't create the bbolt database in /tmp/")
	}
}

// NOTE: this method use the name in Record for the key and add the final dot to it.
func seedDBwithRecords(db *bolt.DB, records [][]Record) error {
	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("records"))

		if err != nil {
			return fmt.Errorf("can't create bucket: %s", err)
		}

		for _, rs := range records {
			var err error

			recordAsJson, err := json.Marshal(rs)

			if err != nil {
				return err
			}
			
			err = b.Put([]byte(rs[0].Name + "|" + rs[0].Type), []byte(recordAsJson))

			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (suite *DnsQuerySuite) TearDownTest() {
	os.Remove(suite.DB.Path())
	suite.DB.Close()
}

//NOTE: this test depends on defaultSeedRecords
func (suite *DnsQuerySuite) TestShouldHandleQueryForManagedZone() {
	seedDBwithRecords(suite.DB, defaultSeedRecords)

	mockAgent := NewMockAgent()
	go mockAgent.Run()

	go serve(suite.DB, defaultDnsConfig, mockAgent.Input)

	// Avoid connection refused because the DNS server is not ready
	// FIXME: I tried to set the Timeout + Dialtimeout for the client
	// but that seem's to have no effect
	time.Sleep(100 * time.Millisecond)

	client := new(dns.Client)
	m := new(dns.Msg)

	m.Question = append(m.Question, dns.Question{"a.rock.", dns.TypeA, dns.ClassINET})
	m.RecursionDesired = true

	r, _, err := client.Exchange(m, "127.0.0.1:8053")

	if err != nil {
		fmt.Printf("%s", err.Error())
		suite.Fail("error on exchange")
	}

	if r.Rcode != dns.RcodeSuccess {
		suite.Fail(" *** invalid answer name")
	}

	suite.Equal(len(defaultSeedRecords[0]), len(r.Answer))

	if answer, ok := r.Answer[0].(*dns.A); ok {
		suite.True(answer.A.Equal(net.ParseIP(defaultSeedRecords[0][0].Content)))
	} else {
		suite.Fail("Invalid dns answer type: requested a type A")
	}
}

func (suite *DnsQuerySuite) TestShouldHandleTheQueryWithTheResolver() {
	mockAgent := NewMockAgent()
	go mockAgent.Run()

	go serve(suite.DB, defaultDnsConfig, mockAgent.Input)
	time.Sleep(100 * time.Millisecond)

	client := new(dns.Client)
	m := new(dns.Msg)

	m.Question = append(m.Question, dns.Question{"www.example.com.", dns.TypeA, dns.ClassINET})
	m.RecursionDesired = true

	r, _, err := client.Exchange(m, "localhost:8053")

	if err != nil {
		log.Error(err.Error())
		suite.Fail("error on exchange")
	}

	suite.Equal(1, len(r.Answer))
	answer := r.Answer[0].Header()

	suite.Equal(dns.TypeA, answer.Rrtype)
	suite.Equal("www.example.com.", answer.Name)
}

func (suite *DnsQuerySuite) TestShouldHandleAxfrQuery() {
	var axfrRecords = [][]Record{
		[]Record{Record{"zonetransfer.me.", "SOA", "1.1.1.1 2017042001 172800 900 1209600 3600", 3600, 0}},
		[]Record{Record{"zonetransfer.me.", "NS", "nsztm1.digi.ninja.", 1200, 0}},
		[]Record{Record{"foo.zonetransfer.me.", "A", "202.14.81.230", 1200, 0}},
		[]Record{Record{"bar.zonetransfer.me.", "AAAA", "2001:db8:0:85a3:0:0:ac1f:8001", 1200, 0}},
		[]Record{Record{"unknown.me.", "AAAA", "2001:db8:0:85a3:0:0:ac1f:8001", 1200, 0}},
	}

	seedDBwithRecords(suite.DB, axfrRecords)

	mockAgent := NewMockAgent()
	go mockAgent.Run()

	axfrConfig := DnsConfig{":8053", true, false, []string{"zonetransfer.me.", "me."}, "9.9.9.9"}
	go serve(suite.DB, axfrConfig, mockAgent.Input)
	time.Sleep(100 * time.Millisecond) // Avoid connection refused because the DNS server is not ready 

	client := new(dns.Client)
	m := new(dns.Msg)

	m.SetAxfr("zonetransfer.me.")
	r, _, err := client.Exchange(m, "localhost:8053")

	if err != nil {
		log.Error(err.Error())
		suite.Fail("error on exchange")
	}

	suite.Equal(5, len(r.Answer))

	suite.True(answerEqualToRecord(r.Answer[0], axfrRecords[0][0])) // SOA
	suite.True(answerEqualToRecord(r.Answer[1], axfrRecords[3][0])) // NS
	suite.True(answerEqualToRecord(r.Answer[2], axfrRecords[2][0])) // A
	suite.True(answerEqualToRecord(r.Answer[3], axfrRecords[1][0])) // AAAA
	suite.True(answerEqualToRecord(r.Answer[4], axfrRecords[0][0])) // SOA
}

func (suite *DnsQuerySuite) TestShouldHandleAxfrQueryForUnsupportedZone() {
	seedDBwithRecords(suite.DB, defaultSeedRecords)
	
	mockAgent := NewMockAgent()
	go mockAgent.Run()

	axfrConfig := DnsConfig{":8053", true, false, []string{"zonetransfer.me."}, "9.9.9.9"}
	go serve(suite.DB, axfrConfig, mockAgent.Input)
	time.Sleep(100 * time.Millisecond) // Avoid connection refused because the DNS server is not ready 

	client := new(dns.Client)
	m := new(dns.Msg)

	m.SetAxfr("not.ok.")
	r, _, err := client.Exchange(m, "localhost:8053")

	if err != nil {
		log.Error(err.Error())
		suite.Fail("error on exchange")
	}

	suite.Equal(r.Rcode, dns.RcodeNotAuth)
}

func TestDnsQueryTestSuite(t *testing.T) {
	suite.Run(t, new(DnsQuerySuite))
}


func answerEqualToRecord(answer dns.RR, record Record) bool {
	header := answer.Header()
	return header.Rrtype == dns.StringToType[record.Type] && record.Name == header.Name
}

// =======================
type MockAgent struct {
	Input chan metrics.Metric
}

func NewMockAgent() MockAgent {
	a := MockAgent{
		Input: make(chan metrics.Metric),
	}

	return a
}

// Deadletter all the metrics messages
func (a *MockAgent) Run() {
	for {
		<-a.Input // consume message to unblock Sender
	}
}
