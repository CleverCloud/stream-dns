package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/suite"
	bolt "go.etcd.io/bbolt"
)

type DnsTestSuite struct {
	suite.Suite
	config DnsConfig
}

func (suite *DnsTestSuite) SetupTest() {
	dbPath := fmt.Sprintf("/tmp/%s.db", randSeq(10))
	db, err := bolt.Open(dbPath, 0600, nil)

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(RecordBucket))

		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		suite.Fail(err.Error())
	}

	config := DnsConfig{
		Address: "127.0.0.1:8053",
		Udp:     true,
		Tcp:     false,
		Zones:   []string{"foo.", "bar."},
	}

	suite.config = config

	if err != nil {
		suite.Fail(err.Error())
	}

	go serveDNS(db, suite.config, nil)
	time.Sleep(200 * time.Millisecond)
}

func (suite *DnsTestSuite) TearDownTest() {
}

func (suite *DnsTestSuite) TestShouldUseResolverForNoneAuthoritativeZone() {
	c := new(dns.Client)

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn("google.com"), dns.TypeA)
	m.RecursionDesired = true

	r, _, err := c.Exchange(m, suite.config.Address)

	suite.NotNil(r.Answer)
	suite.NotNil(r.Ns)
	suite.Nil(err)
	suite.Equal(dns.RcodeSuccess, r.Rcode, fmt.Sprintf("Expected %s but got %s", dns.RcodeToString[dns.RcodeSuccess], dns.RcodeToString[r.Rcode]))
}

func (suite *DnsTestSuite) TestShouldGetNxDomain() {
	c := new(dns.Client)

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn("test.foo."), dns.TypeA)
	m.RecursionDesired = true

	r, _, err := c.Exchange(m, suite.config.Address)

	suite.Nil(r.Answer)
	suite.Nil(r.Ns)
	suite.Nil(r.Extra)
	suite.Nil(err)
	suite.Equal(
		dns.RcodeNameError,
		r.Rcode,
		fmt.Sprintf("Expected %s but got %s", dns.RcodeToString[dns.RcodeNameError], dns.RcodeToString[r.Rcode]),
	)
}

func TestDnsTestSuite(t *testing.T) {
	suite.Run(t, new(DnsTestSuite))
}
