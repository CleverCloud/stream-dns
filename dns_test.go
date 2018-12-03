package main

import (
	"fmt"
	"github.com/stretchr/testify/suite"
	bolt "go.etcd.io/bbolt"
	"math/rand"
	"testing"
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

func (suite *DnsTestSuite) TestShouldNotFindRecordsWhenBucketIsEmpty() {
	bucketName := "records"

	suite.DB.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))

		if err != nil {
			suite.Fail("Can't seed the database")
		}

		return nil
	})

	suite.DB.View(func(tx *bolt.Tx) error {
		recordsBucket := tx.Bucket([]byte(bucketName))
		records, err := getRecordsFromBucket(recordsBucket, "zenaton-rabbitmq-c1-n3.services.clever-cloud.com.")

		if err != nil {
			suite.Fail("Can't get records from bucket: %s", err)
		}

		suite.Equal(0, len(records))

		return nil
	})
}

func (suite *DnsTestSuite) TestShouldFindARecord() {
	bucketName := "records"
	key := []byte("zenaton-rabbitmq-c1-n3.services.clever-cloud.com.|A")
	value := []byte(`[{"name":"zenaton-rabbitmq-c1-n3.services.clever-cloud.com","type":"A","content":"163.172.233.56","ttl":3600,"priority":0}]`)

	suite.DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucketName))

		if err != nil {
			suite.Fail("Can't seed the database")
		}

		b.Put(key, value)
		return nil
	})

	suite.DB.View(func(tx *bolt.Tx) error {
		recordsBucket := tx.Bucket([]byte(bucketName))
		records, err := getRecordsFromBucket(recordsBucket, "zenaton-rabbitmq-c1-n3.services.clever-cloud.com.")

		if err != nil {
			suite.Fail("Can't get records from bucket: %s", err)
		}

		suite.Equal(1, len(records))

		suite.Equal("zenaton-rabbitmq-c1-n3.services.clever-cloud.com", records[0][0].Name, "not the same name")
		suite.Equal("A", records[0][0].Type, "not the same type")

		return nil
	})
}

func (suite *DnsTestSuite) TestShouldNotFindARecordWhenItDoesntExist() {
	bucketName := "records"
	key := []byte("zenaton-rabbitmq-c1-n3.services.clever-cloud.com.|A")
	value := []byte(`[{"name":"zenaton-rabbitmq-c1-n3.services.clever-cloud.com","type":"A","content":"163.172.233.56","ttl":3600,"priority":0}]`)

	suite.DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucketName))

		if err != nil {
			suite.Fail("Can't seed the database")
		}

		b.Put(key, value)
		return nil
	})

	suite.DB.View(func(tx *bolt.Tx) error {
		recordsBucket := tx.Bucket([]byte(bucketName))
		records, err := getRecordsFromBucket(recordsBucket, "wlpm3ahirq-jenkins.services.clever-cloud.com.")

		if err != nil {
			suite.Fail("Can't get records from bucket: %s", err)
		}

		suite.Equal(0, len(records))

		return nil
	})
}


func (suite *DnsTestSuite) TestShouldFindARecordWithWildcardPrefix() {
	bucketName := "records"
	key := []byte("*.cleverapps.io.|A")
	value := []byte(`[{"name":"*.cleverapps.io","type":"A","content":"163.172.235.156","ttl":3600,"priority":0},{"name":"*.cleverapps.io","type":"A","content":"163.172.235.159","ttl":3600,"priority":0}]`)

	suite.DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucketName))

		if err != nil {
			suite.Fail("Can't seed the database")
		}

		b.Put(key, value)
		return nil
	})

	suite.DB.View(func(tx *bolt.Tx) error {
		recordsBucket := tx.Bucket([]byte(bucketName))
		records, err := getRecordsFromBucket(recordsBucket, "*.cleverapps.io.")

		if err != nil {
			suite.Fail("Can't get records from bucket: %s", err)
		}

		suite.Equal(1, len(records))
		suite.Equal(2, len(records[0]))

		suite.Equal("*.cleverapps.io", records[0][0].Name, "not the same name")
		suite.Equal("163.172.235.156", records[0][0].Content)

		suite.Equal("*.cleverapps.io", records[0][1].Name, "not the same type")
		suite.Equal("163.172.235.159", records[0][1].Content)

		return nil
	})
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
