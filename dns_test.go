package main

import (
	//"github.com/stretchr/testify/assert"
	"fmt"
	"github.com/stretchr/testify/suite"
	"math/rand"
	"testing"
	bolt "go.etcd.io/bbolt"
)

type DnsTestSuite struct {
	suite.Suite
	DB *bolt.DB
}

func (suite *DnsTestSuite) SetupTest() {
	var err error
	dbPath := fmt.Sprintf("/tmp/%s.db", randSeq(10))
	suite.DB, err = bolt.Open(dbPath, 0600, nil)

	if err != nil {
		suite.Fail("Can't create the bbolt database in /tmp/")
	}

}

func (suite *DnsTestSuite) TearDownTest() {
	suite.DB.Close()
}


func (suite *DnsTestSuite) TestShouldFoundACname() {

}


func TestDnsTestSuite(t *testing.T) {
	suite.Run(t, new(DnsTestSuite))
}

// Generate a random string of a fixed length in Go
// Use this to generate the bolt database name
func randSeq(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	b := make([]rune, n)

	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}
