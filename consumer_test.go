package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	bolt "go.etcd.io/bbolt"
)

type ConsumerQuerySuite struct {
	suite.Suite
	DB *bolt.DB
}

func (suite *ConsumerQuerySuite) SetupTest() {
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

func (suite *ConsumerQuerySuite) TestCheckIsNotCnameOnApexDomain() {
	assert.True(suite.T(), isCnameOnApexDomain([]byte("example.com.|CNAME")))
	assert.False(suite.T(), isCnameOnApexDomain([]byte("example.com.|A")))

	assert.False(suite.T(), isCnameOnApexDomain([]byte("www.example.com.|CNAME")))
	assert.False(suite.T(), isCnameOnApexDomain([]byte("www.example.com.|A")))
}

// Disallow CNAME on APEX when DISALLOW_CNAME_ON_APEX is disactivated
func (suite *ConsumerQuerySuite) TestAllowCNAMEonAPEX() {
	key := []byte("apex.com.|CNAME")
	record, _ := json.Marshal([]Record{Record{"apex.com.", dns.TypeCNAME, "foo.bar.com.", 3600, 0}})

	// disallow DISALLOW_CNAME_ON_APEX flag
	err := registerRecordAsBytesWithTheKeyInDB(suite.DB, key, record, false)
	assert.True(suite.T(), err == nil)

	// check the result
	var v []byte
	suite.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("records"))
		v = b.Get(key)
		return nil
	})

	// The CNAME record should exist
	suite.Equal(record, v)
}

// Allow CNAME on APEX when DISALLOW_CNAME_ON_APEX is activated
func (suite *ConsumerQuerySuite) TestDisallowCNAMEonAPEX() {
	key := []byte("apex.com.|CNAME")
	record, _ := json.Marshal([]Record{Record{"apex.com.", dns.TypeCNAME, "foo.bar.com.", 3600, 0}})

	err := registerRecordAsBytesWithTheKeyInDB(suite.DB, key, record, true)

	assert.True(suite.T(), err != nil)

	var v []byte

	suite.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("records"))
		v = b.Get(key)
		return nil
	})

	assert.True(suite.T(), v == nil)
}

// On subdomains: when a CNAME comes, remove all previous records and replace with CNAME.
func (suite *ConsumerQuerySuite) TestOnSubdomainWhenCNAMEcomesRemoveAllPreviousRecordsAndReplaceWithCNAME() {
	var previousRecords = [][]Record{
		[]Record{Record{"www.example.com.", dns.TypeA, "1.1.1.1", 3600, 0}},
		[]Record{Record{"www.example.com.", dns.TypeAAAA, "2001:db8::1", 3600, 0}},
	}
	seedDBwithRecords(suite.DB, previousRecords)

	// Trying to register a CNAME record
	key := []byte("www.example.com.|CNAME")
	record, _ := json.Marshal([]Record{Record{"www.example.com.", dns.TypeCNAME, "foo.bar.com.", 3600, 0}})

	err := registerRecordAsBytesWithTheKeyInDB(suite.DB, key, record, true)
	assert.True(suite.T(), err == nil)

	// check the result
	var a, aaaa, res []byte
	suite.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("records"))
		a = b.Get([]byte("www.example.com.|A"))
		aaaa = b.Get([]byte("www.example.com.|AAAA"))
		res = b.Get(key)
		return nil
	})

	// previous A, AAAA records should be deleted
	assert.True(suite.T(), a == nil)
	assert.True(suite.T(), aaaa == nil)
	// The new CNAME record should be registered
	suite.Equal(res, record)
}

// On subdomains: when CNAME already exists: allow only new CNAME.
func (suite *ConsumerQuerySuite) TestOnsubdomainWhenCNAMEalreadyExistsAllowOnlyNewCNAME() {
	var previousRecords = [][]Record{
		[]Record{Record{"www.example.com.", dns.TypeCNAME, "foo.bar.com", 3600, 0}},
	}
	seedDBwithRecords(suite.DB, previousRecords)

	// trying to register a CNAME record
	key := []byte("www.example.com.|CNAME")
	record, _ := json.Marshal([]Record{Record{"www.example.com.", dns.TypeCNAME, "new.bar.com.", 3600, 0}})

	err := registerRecordAsBytesWithTheKeyInDB(suite.DB, key, record, true)
	assert.True(suite.T(), err == nil)

	// check the result
	var res []byte
	suite.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("records"))
		res = b.Get(key)
		return nil
	})

	suite.Equal(res, record) // The record should be updated
}

// On subdomains: when CNAME already exists: allow only new CNAME. Sending a A, AAAA, etc should not be allowed
func (suite *ConsumerQuerySuite) TestOnsubdomainWhenCNAMEalreadyExistsShouldNotAllowA() {
	var previousRecords = [][]Record{
		[]Record{Record{"www.example.com.", dns.TypeCNAME, "foo.bar.com", 3600, 0}},
	}
	seedDBwithRecords(suite.DB, previousRecords)

	key := []byte("www.example.com.|A")
	record, _ := json.Marshal([]Record{Record{"www.example.com.", dns.TypeA, "1.1.1.1", 3600, 0}})

	err := registerRecordAsBytesWithTheKeyInDB(suite.DB, key, record, true)
	assert.True(suite.T(), err != nil)

	var res, cname []byte
	suite.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("records"))
		res = b.Get(key)
		cname = b.Get([]byte("www.example.com.|CNAME"))
		return nil
	})

	previousRecord, _ := json.Marshal(previousRecords[0])
	suite.Equal(cname, previousRecord) // The CNAME should not be deleted
	assert.True(suite.T(), res == nil) // The A record should not be registered
}

// On APEX when CNAME already exists we  allow not only CNAME
func (suite *ConsumerQuerySuite) TestOnAPEXdomainWhenCNAMEalreadyExistsWheShouldAllowAnything() {
	var previousRecords = [][]Record{
		[]Record{Record{"example.com.", dns.TypeCNAME, "foo.bar.com", 3600, 0}},
	}
	seedDBwithRecords(suite.DB, previousRecords)

	// trying to save A APEX record
	key := []byte("example.com.|A")
	record, _ := json.Marshal([]Record{Record{"example.com.", dns.TypeA, "1.1.1.1", 3600, 0}})

	err := registerRecordAsBytesWithTheKeyInDB(suite.DB, key, record, true)
	suite.Equal(nil, err)

	// check the result
	var res, cname []byte
	suite.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("records"))
		res = b.Get(key)
		cname = b.Get([]byte("example.com.|CNAME"))
		return nil
	})

	previousRecord, _ := json.Marshal(previousRecords[0])
	suite.Equal(cname, previousRecord) // The CNAME should not be deleted
	suite.Equal(res, record)           // The A record should be registered
}

func TestDnsConsumerTestSuite(t *testing.T) {
	suite.Run(t, new(ConsumerQuerySuite))
}
