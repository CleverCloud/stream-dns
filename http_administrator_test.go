package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	bolt "go.etcd.io/bbolt"
)

type HttpAdministratorSuite struct {
	suite.Suite
	DB *bolt.DB
}

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

func (suite *HttpAdministratorSuite) TestSearchRecords() {
	var records = [][]Record{
		[]Record{Record{"www.example.com.", "A", "1.1.1.1", 3600, 0}},
		[]Record{Record{"test.foo.bar.io.", "A", "2.2.2.2", 1200, 0}},
		[]Record{Record{"test.foo.io.", "A", "4.4.4.4", 3600, 0}},
		[]Record{Record{"test.bar.io.", "A", "4.4.4.4", 3600, 0}},
	}

	seedDBwithRecords(suite.DB, records)

	pattern := "foo"
	
	httpAdministrator := NewHttpAdministrator(suite.DB, "127.0.0.1:8081")
	go httpAdministrator.StartHttpAdministrator()

	time.Sleep(100 * time.Millisecond)

	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:8081/tools/dnsrecords/search?pattern=%s", pattern))
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

	suite.Equal(2, len(recordsRes))
	suite.Equal(records[1], recordsRes[0])
	suite.Equal(records[2], recordsRes[1])
}

func (suite *HttpAdministratorSuite) TestSearchRecordsAndShouldFindNothing() {
	var records = [][]Record{
		[]Record{Record{"www.example.com.", "A", "1.1.1.1", 3600, 0}},
		[]Record{Record{"test.foo.bar.io.", "A", "2.2.2.2", 1200, 0}},
		[]Record{Record{"test.foo.io.", "A", "4.4.4.4", 3600, 0}},
		[]Record{Record{"test.bar.io.", "A", "4.4.4.4", 3600, 0}},
	}

	seedDBwithRecords(suite.DB, records)

	pattern := "shoudlmatchnothing"

	httpAdministrator := NewHttpAdministrator(suite.DB, "127.0.0.1:8082")
	go httpAdministrator.StartHttpAdministrator()

	time.Sleep(100 * time.Millisecond)

	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:8081/tools/dnsrecords/search?pattern=%s", pattern))
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
