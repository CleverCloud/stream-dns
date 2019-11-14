package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/suite"
	bolt "go.etcd.io/bbolt"
)

type HttpAdministratorSuite struct {
	suite.Suite
	DB *bolt.DB
}

var creds = Credentials{Username: "test", Password: "test"}

var adminConfig = AdministratorConfig{Username: "test", Password: "test", Address: "127.0.0.1:9001", JwtSecret: "a-secret"}

func (suite *HttpAdministratorSuite) SetupTest() {
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

func (suite *HttpAdministratorSuite) TestShouldNotBeSignedIfSendInCorrectCreds() {
	username := "test"
	creds := Credentials{Username: username, Password: "anuncorrectpassword"}
	adminConfig := AdministratorConfig{Username: username, Password: "acorrectpassword", Address: "127.0.0.1:9001", JwtSecret: "a-secret"}

	httpAdministrator := NewHttpAdministrator(suite.DB, adminConfig)
	go httpAdministrator.StartHttpAdministrator()

	time.Sleep(100 * time.Millisecond)

	credsMarshal, err := json.Marshal(creds)
	res, err := http.Post("http://127.0.0.1:9001/signin", "application/json", bytes.NewBuffer(credsMarshal))

	if err != nil {
		suite.Fail(err.Error())
	}

	suite.Equal(401, res.StatusCode)

	cookies := res.Cookies()
	suite.Equal(0, len(cookies))
}

func (suite *HttpAdministratorSuite) TestShouldGetBadRequestIfCredsAreMissing() {
	username := "test"
	adminConfig := AdministratorConfig{Username: username, Password: "acorrectpassword", Address: "127.0.0.1:9002", JwtSecret: "a-secret"}

	httpAdministrator := NewHttpAdministrator(suite.DB, adminConfig)
	go httpAdministrator.StartHttpAdministrator()

	time.Sleep(100 * time.Millisecond)

	res, err := http.Post("http://127.0.0.1:9002/signin", "application/json", bytes.NewBuffer([]byte{}))

	if err != nil {
		suite.Fail(err.Error())
	}

	suite.Equal(400, res.StatusCode)

	cookies := res.Cookies()
	suite.Equal(0, len(cookies))
}

func (suite *HttpAdministratorSuite) TestShouldBeSignedAndGetJWTIfSendCorrectCreds() {
	username := "test"
	password := "test"
	creds := Credentials{Username: username, Password: password}
	adminConfig := AdministratorConfig{Username: username, Password: password, Address: "127.0.0.1:9003", JwtSecret: "a-secret"}

	httpAdministrator := NewHttpAdministrator(suite.DB, adminConfig)
	go httpAdministrator.StartHttpAdministrator()

	time.Sleep(100 * time.Millisecond)

	credsMarshal, err := json.Marshal(creds)
	res, err := http.Post("http://127.0.0.1:9003/signin", "application/json", bytes.NewBuffer(credsMarshal))

	if err != nil {
		suite.Fail(err.Error())
	}

	suite.Equal(200, res.StatusCode)

	cookies := res.Cookies()
	suite.Equal(1, len(cookies))
	suite.Equal("token", cookies[0].Name)
	suite.Assert().True(cookies[0].Value != "")
}

func (suite *HttpAdministratorSuite) TestSearchRecords() {
	rrs := [][]dns.RR{
		[]dns.RR{testRR("www.example.com. 3600 IN A 1.1.1.1")},
		[]dns.RR{testRR("test.foo.bar.io. 3600 IN A 2.2.2.2")},
		[]dns.RR{testRR("test.foo.io. 3600 IN A 4.4.4.4")},
		[]dns.RR{testRR("test.bar.io. 3600 IN A 4.4.4.4")},
	}

	suite.DB.Update(func(tx *bolt.Tx) error {
		if b, err := tx.CreateBucketIfNotExists(RecordBucket); err != nil {
			suite.Fail("Can't seed the database")
		} else {
			for _, rr := range rrs {
				rrRaw, err := json.Marshal(rr)
				if err != nil {
					suite.Fail(err.Error())
				}
				b.Put([]byte(rr[0].Header().Name+"|"+dns.TypeToString[rr[0].Header().Rrtype]), rrRaw)
			}
		}

		return nil
	})

	pattern := "foo"

	adminConfigWithNoAuth := AdministratorConfig{Username: "", Password: "", Address: "127.0.0.1:8081", JwtSecret: "a-secret"}
	httpAdministrator := NewHttpAdministrator(suite.DB, adminConfigWithNoAuth)
	go httpAdministrator.StartHttpAdministrator()

	time.Sleep(100 * time.Millisecond)

	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:8081/search?pattern=%s", pattern))
	if err != nil {
		suite.Fail(err.Error())
	}

	defer res.Body.Close()

	suite.Equal(http.StatusOK, res.StatusCode)
	//TODO: check the content
}

func (suite *HttpAdministratorSuite) TestSearchRecordsAndShouldFindNothing() {
	rrs := [][]dns.RR{
		[]dns.RR{testRR("www.example.com. 3600 IN A 1.1.1.1")},
		[]dns.RR{testRR("test.foo.bar.io. 3600 IN A 2.2.2.2")},
		[]dns.RR{testRR("test.foo.io. 3600 IN A 4.4.4.4")},
		[]dns.RR{testRR("test.bar.io. 3600 IN A 4.4.4.4")},
	}

	suite.DB.Update(func(tx *bolt.Tx) error {
		if b, err := tx.CreateBucketIfNotExists(RecordBucket); err != nil {
			suite.Fail("Can't seed the database")
		} else {
			for _, rr := range rrs {
				rrRaw, err := json.Marshal(rr)
				if err != nil {
					suite.Fail(err.Error())
				}
				b.Put([]byte(rr[0].Header().Name+"|"+dns.TypeToString[rr[0].Header().Rrtype]), rrRaw)
			}
		}

		return nil
	})

	pattern := "shoudlmatchnothing"

	adminConfigWithNoAuth := AdministratorConfig{Username: "", Password: "", Address: "127.0.0.1:8081", JwtSecret: "a-secret"}
	httpAdministrator := NewHttpAdministrator(suite.DB, adminConfigWithNoAuth)
	go httpAdministrator.StartHttpAdministrator()

	time.Sleep(100 * time.Millisecond)

	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:8081/search?pattern=%s", pattern))
	if err != nil {
		suite.Fail(err.Error())
	}

	defer res.Body.Close()

	suite.Equal(http.StatusOK, res.StatusCode)
	var recordsRes [][]Record

	decoder := json.NewDecoder(res.Body)
	err = decoder.Decode(&recordsRes)
	if err != nil {
		panic(err)
	}

	suite.Equal(0, len(recordsRes))
}

func TestHttpAdministratorSuite(t *testing.T) {
	suite.Run(t, new(HttpAdministratorSuite))
}
