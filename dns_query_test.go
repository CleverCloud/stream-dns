package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"kafka-dns/metrics"

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

			err = b.Put([]byte(rs[0].Name+"."), []byte(recordAsJson))

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

func TestDnsQueryTestSuite(t *testing.T) {
	suite.Run(t, new(DnsQuerySuite))
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
