package main

import (
	"os"
	"net"
	"fmt"
	"testing"
	"encoding/json"
	"time"

	"kafka-dns/metrics"

	log "github.com/sirupsen/logrus"
	dns "github.com/miekg/dns"
	bolt "go.etcd.io/bbolt"
	"github.com/stretchr/testify/suite"
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
func (suite *DnsQuerySuite) TestShouldHandleQuery() {
	seedDBwithRecords(suite.DB, defaultSeedRecords)

	mockAgent := MockAgent{}
	go mockAgent.Run()
	go serve(suite.DB, defaultDnsConfig, mockAgent.Input)

	// avoid connection refused because the DNS server is not ready
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

//NOTE: this test depends on defaultSeedRecords
func (suite *DnsQuerySuite) TestShouldGetNxDomainCodeWhenDomainIsNotRegister() {
	// Run with a empty DB
	mockAgent := MockAgent{}
	go serve(suite.DB, defaultDnsConfig, mockAgent.Input)

	client := new(dns.Client)
	m := new(dns.Msg)

	m.Question = append(m.Question, dns.Question{"google.com.", dns.TypeA, dns.ClassINET})
	m.RecursionDesired = true

	r, _, err := client.Exchange(m, "localhost:8053")

	if err != nil {
		fmt.Print(err)
		suite.Fail("error on exchange")
	}

	if r.Rcode != dns.RcodeNameError {
		suite.Fail(" *** invalid answer code: should get NXDOMAIN")
	}
}

func TestDnsQueryTestSuite(t *testing.T) {
	suite.Run(t, new(DnsQuerySuite))
}

// =======================
// Deadletters all the message metrics
type MockAgent struct {
	Input  chan metrics.Metric
}

func NewAgent() MockAgent {
	a := MockAgent {
		Input:  make(chan metrics.Metric),
	}

	return a
}

func (a *MockAgent) Run() {
	for {
		log.Print("here agent")
		<-a.Input
		fmt.Print("consumed agent")
	}
}
