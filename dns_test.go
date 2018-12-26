package main

import (
	"fmt"
	dns "github.com/miekg/dns"
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
		records, err := getRecordsFromBucket(recordsBucket, "yolo.com.")

		if err != nil {
			suite.Fail("Can't get records from bucket: %s", err)
		}

		suite.Equal(0, len(records))

		return nil
	})
}

func (suite *DnsTestSuite) TestShouldFindARecord() {
	bucketName := "records"
	key := []byte("foo.bar.services.com.|A")
	value := []byte(`[{"name":"foo.bar.services.com","type":"A","content":"163.172.233.56","ttl":3600,"priority":0}]`)

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
		records, err := getRecordsFromBucket(recordsBucket, "foo.bar.services.com")

		if err != nil {
			suite.Fail("Can't get records from bucket: %s", err)
		}

		suite.Equal(1, len(records))

		suite.Equal("foo.bar.services.com", records[0][0].Name, "not the same name")
		suite.Equal("A", records[0][0].Type, "not the same type")

		return nil
	})
}

func (suite *DnsTestSuite) TestShouldNotFindARecordWhenItDoesntExist() {
	bucketName := "records"
	key := []byte("foo.bar.services.com.|A")
	value := []byte(`[{"name":"foo.bar.services.com","type":"A","content":"163.172.233.56","ttl":3600,"priority":0}]`)

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
		records, err := getRecordsFromBucket(recordsBucket, "unknow.services.com.")

		if err != nil {
			suite.Fail("Can't get records from bucket: %s", err)
		}

		suite.Equal(0, len(records))

		return nil
	})
}

func (suite *DnsTestSuite) TestShouldReturnOnlyTheRecordWithoutTheWildcardWhenThePlainRecordExist() {
	bucketName := "records"

	key := []byte("yds.yolo.io.|A")
	value := []byte(`[{"name":"yds.yolo.io","type":"A","content":"123.172.154.22","ttl":3600,"priority":0}]`)

	key_wildcard := []byte("*.yolo.io.|A")
	value_wildcard := []byte(`[{"name":"*.yolo.io","type":"A","content":"163.172.235.156","ttl":3600,"priority":0},{"name":"*.yolo.io","type":"A","content":"163.172.235.159","ttl":3600,"priority":0}]`)

	suite.DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucketName))

		if err != nil {
			suite.Fail("Can't seed the database")
		}

		b.Put(key, value)
		b.Put(key_wildcard, value_wildcard)
		return nil
	})

	suite.DB.View(func(tx *bolt.Tx) error {
		recordsBucket := tx.Bucket([]byte(bucketName))
		records, err := getRecordsFromBucket(recordsBucket, "yds.yolo.io")

		if err != nil {
			suite.Fail("Can't get records from bucket: %s", err)
		}

		// We should not retrieve the wildcard records
		suite.Equal(1, len(records))
		suite.Equal(1, len(records[0]))

		suite.Equal("yds.yolo.io", records[0][0].Name, "not the same name")
		suite.Equal("A", records[0][0].Type, "not the same type")
		suite.Equal("123.172.154.22", records[0][0].Content, "not the same content")
		return nil
	})
}

func (suite *DnsTestSuite) TestShouldFindARecordWithWildcardPrefix() {
	bucketName := "records"
	key := []byte("*.apps.io.|A")
	value := []byte(`[{"name":"*.apps.io","type":"A","content":"163.172.235.156","ttl":3600,"priority":0},{"name":"*.apps.io","type":"A","content":"163.172.235.159","ttl":3600,"priority":0}]`)

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
		records, err := getRecordsFromBucket(recordsBucket, "*.apps.io.")

		if err != nil {
			suite.Fail("Can't get records from bucket: %s", err)
		}

		suite.Equal(1, len(records))
		suite.Equal(2, len(records[0]))

		suite.Equal("*.apps.io", records[0][0].Name, "not the same name")
		suite.Equal("163.172.235.156", records[0][0].Content)

		suite.Equal("*.apps.io", records[0][1].Name, "not the same type")
		suite.Equal("163.172.235.159", records[0][1].Content)

		return nil
	})
}

func (suite *DnsTestSuite) TestShouldFilterRecordByQtypeOrCname() {
	var records = []Record{
		Record{"a.com", "A", "163.172.235.159", 3600, 0},
		Record{"txt.com", "TXT", "163.172.235.159", 3600, 0},
		Record{"aaaa.com", "AAAA", "163.172.235.159", 3600, 0},
		Record{"cname.com", "CNAME", "163.172.235.159", 3600, 0},
	}

	recordsFiltered := filterByQtypeAndCname(records, dns.TypeA)
	suite.Equal(2, len(recordsFiltered))
	suite.Equal("A", recordsFiltered[0].Type)
	suite.Equal("CNAME", recordsFiltered[1].Type)

	cnameRecords := filterByQtypeAndCname(records, dns.TypeCNAME)
	suite.Equal(1, len(cnameRecords))
	suite.Equal("CNAME", cnameRecords[0].Type)
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
