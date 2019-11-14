package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/suite"
	bolt "go.etcd.io/bbolt"
)

type DnsTestSuite struct {
	suite.Suite
	handler QuestionResolverHandler
}

// testRR is a helper that wraps a call to NewRR and panics if the error is non-nil.
func testRR(s string) dns.RR {
	r, err := dns.NewRR(s)
	if err != nil {
		panic(err)
	}

	return r
}

func testMarshalRR(rr []dns.RR) []byte {
	rrRaw, err := json.Marshal(rr)

	if err != nil {
		panic(err)
	}

	return rrRaw
}

func (suite *DnsTestSuite) SetupTest() {
	var err error
	dbPath := fmt.Sprintf("/tmp/%s.db", randSeq(10))
	db, err := bolt.Open(dbPath, 0600, nil)

	suite.handler = NewQuestionResolverHandler(db, DnsConfig{Zones: []string{".bar.services.com.", ".internal."}}, nil)

	if err != nil {
		suite.Fail("Can't create the bbolt database in /tmp/")
	}

}

func (suite *DnsTestSuite) TearDownTest() {
	suite.handler.db.Close()
}

func (suite *DnsTestSuite) TestShouldDetectCNAMEResponse() {
	suite.True(IsCnameRes([]dns.RR{testRR("mx.miek.nl. 3600 IN CNAME miek.nl.")}))
	suite.False(IsNotCNAMERes([]dns.RR{testRR("mx.miek.nl. 3600 IN CNAME miek.nl.")}))

	// A CNAME response cannot have multiple value
	suite.False(IsCnameRes([]dns.RR{testRR("mx.miek.nl. 3600 IN CNAME miek.nl."), testRR("a.miek.nl. 3600 IN CNAME miek.nl.")}))
	suite.True(IsNotCNAMERes([]dns.RR{testRR("mx.miek.nl. 3600 IN CNAME miek.nl."), testRR("a.miek.nl. 3600 IN CNAME miek.nl.")}))

	suite.False(IsCnameRes([]dns.RR{testRR("www.example.org. 2700 IN A 127.0.0.1")}))
	suite.True(IsNotCNAMERes([]dns.RR{testRR("www.example.org. 2700 IN A 127.0.0.1")}))

	suite.False(IsCnameRes([]dns.RR{testRR("www.example.org. 2700 IN A 127.0.0.1"), testRR("mx.miek.nl. 3600 IN CNAME miek.nl.")}))
	suite.True(IsNotCNAMERes([]dns.RR{testRR("www.example.org. 2700 IN A 127.0.0.1"), testRR("mx.miek.nl. 3600 IN CNAME miek.nl.")}))
}

func (suite *DnsTestSuite) TestShouldNotFindRecordsWhenBucketIsEmpty() {
	suite.handler.db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(RecordBucket); err != nil {
			suite.Fail("Can't seed the database")
		}
		return nil
	})

	rrs, err := suite.handler.lookupRecord("yolo.internal.", dns.TypeA, true, 0)
	suite.Equal(0, len(rrs))
	suite.Nil(err)
}

func (suite *DnsTestSuite) TestShouldFindARecord() {
	qname := "foo.bar.services.com."
	qtype := dns.TypeA
	key := []byte(qname + "|" + dns.TypeToString[qtype])
	rrExpected := []dns.RR{testRR(qname + " 2700 IN A 163.172.233.56")}
	rrRaw, _ := json.Marshal(rrExpected)

	suite.handler.db.Update(func(tx *bolt.Tx) error {
		if b, err := tx.CreateBucketIfNotExists(RecordBucket); err != nil {
			suite.Fail("Can't seed the database")
		} else {
			b.Put(key, rrRaw)
		}

		return nil
	})

	rrs, err := suite.handler.lookupRecord(qname, qtype, true, 0)
	suite.Equal(len(rrExpected), len(rrs))
	suite.True(dns.IsDuplicate(rrExpected[0], rrs[0]))
	suite.Nil(err)
}

func (suite *DnsTestSuite) TestShouldFindARecordWithMultipleValue() {
	qname := "foo.bar.services.com."
	qtype := dns.TypeA
	key := []byte(qname + "|" + dns.TypeToString[qtype])

	rrExpected := []dns.RR{
		testRR(qname + " 2700 IN A 163.172.233.54"),
		testRR(qname + " 2700 IN A 163.172.233.55"),
		testRR(qname + " 2700 IN A 163.172.233.56"),
	}
	rrRaw, _ := json.Marshal(rrExpected)

	suite.handler.db.Update(func(tx *bolt.Tx) error {
		if b, err := tx.CreateBucketIfNotExists(RecordBucket); err != nil {
			suite.Fail("Can't seed the database")
		} else {
			b.Put(key, rrRaw)
		}

		return nil
	})

	rrs, err := suite.handler.lookupRecord(qname, qtype, true, 0)
	suite.Equal(len(rrExpected), len(rrs))
	suite.Nil(err)
}

func (suite *DnsTestSuite) TestShouldRecurseOnCname() {
	rrsExpected := make(map[string][]dns.RR)

	rrsExpected["foo.bar.services.com.|CNAME"] = []dns.RR{testRR("foo.bar.services.com. 3600 IN CNAME toto.bar.services.com.")}
	rrsExpected["toto.bar.services.com.|CNAME"] = []dns.RR{testRR("toto.bar.services.com. 3600 IN CNAME rock.bar.services.com.")}
	rrsExpected["rock.bar.services.com.|CNAME"] = []dns.RR{testRR("rock.bar.services.com. 3600 IN CNAME off.bar.services.com.")}
	rrsExpected["off.bar.services.com.|CNAME"] = []dns.RR{testRR("off.bar.services.com. 3600 IN CNAME plain.bar.services.com.")}
	rrsExpected["plain.bar.services.com.|A"] = []dns.RR{testRR("plain.bar.services.com. 2700 IN A 127.0.0.1")}

	suite.handler.db.Update(func(tx *bolt.Tx) error {
		if b, err := tx.CreateBucketIfNotExists(RecordBucket); err != nil {
			suite.Fail("Can't seed the database")
		} else {
			for k, v := range rrsExpected {
				b.Put([]byte(k), testMarshalRR(v))
			}
		}

		return nil
	})

	rrs, err := suite.handler.lookupRecord("foo.bar.services.com.", dns.TypeA, true, 0)
	suite.Equal(5, len(rrs))
	suite.Nil(err)

	for _, v := range rrs {
		res := rrsExpected[v.Header().Name+"|"+dns.TypeToString[v.Header().Rrtype]]
		if res == nil {
			suite.Failf("Can't find %s in the answer section", v.String())
		} else {
			dns.IsDuplicate(res[0], v)
		}
	}
}

func (suite *DnsTestSuite) TestShouldNotRecurseOnCnameWhenFlagRecursionDesiredIsNotSet() {
	recursionNotDesired := false
	key := "foo.bar.services.com.|CNAME"
	rrsExpected := make(map[string][]dns.RR)

	rrsExpected[key] = []dns.RR{testRR("foo.bar.services.com. 3600 IN CNAME toto.bar.services.com.")}
	rrsExpected["toto.bar.services.com.|CNAME"] = []dns.RR{testRR("toto.bar.services.com. 3600 IN CNAME rock.bar.services.com.")}
	rrsExpected["rock.bar.services.com.|CNAME"] = []dns.RR{testRR("rock.bar.services.com. 3600 IN CNAME off.bar.services.com.")}
	rrsExpected["off.bar.services.com.|CNAME"] = []dns.RR{testRR("off.bar.services.com. 3600 IN CNAME plain.bar.services.com.")}
	rrsExpected["plain.bar.services.com.|A"] = []dns.RR{testRR("plain.bar.services.com. 2700 IN A 127.0.0.1")}

	suite.handler.db.Update(func(tx *bolt.Tx) error {
		if b, err := tx.CreateBucketIfNotExists(RecordBucket); err != nil {
			suite.Fail("Can't seed the database")
		} else {
			for k, v := range rrsExpected {
				b.Put([]byte(k), testMarshalRR(v))
			}
		}

		return nil
	})

	rrs, err := suite.handler.lookupRecord("foo.bar.services.com.", dns.TypeA, recursionNotDesired, 0)
	suite.Equal(1, len(rrs))
	suite.True(dns.IsDuplicate(rrsExpected[key][0], rrs[0]), fmt.Errorf("Expected %s but got %s", rrsExpected[key][0], rrs[0]))
	suite.Nil(err)
}

func (suite *DnsTestSuite) TestShouldHitMaxRecursionCnameOnRecursion() {
	rrsExpected := make(map[string][]dns.RR)

	rrsExpected["foo.bar.services.com.|CNAME"] = []dns.RR{testRR("foo.bar.services.com. 3600 IN CNAME toto.bar.services.com.")}
	rrsExpected["toto.bar.services.com.|CNAME"] = []dns.RR{testRR("toto.bar.services.com. 3600 IN CNAME rock.bar.services.com.")}
	rrsExpected["rock.bar.services.com.|CNAME"] = []dns.RR{testRR("rock.bar.services.com. 3600 IN CNAME off.bar.services.com.")}
	rrsExpected["off.bar.services.com.|CNAME"] = []dns.RR{testRR("off.bar.services.com. 3600 IN CNAME boom.bar.services.com.")}
	rrsExpected["boom.bar.services.com.|CNAME"] = []dns.RR{testRR("boom.bar.services.com. 3600 IN CNAME explose.bar.services.com.")} // Should not continue after this depth
	rrsExpected["explose.bar.services.com.|CNAME"] = []dns.RR{testRR("explose.bar.services.com. 3600 IN CNAME shouldnotbehit.bar.services.com.")}
	rrsExpected["shouldnotbehit.bar.services.com.|A"] = []dns.RR{testRR("shouldnotbehit.bar.services.com. 2700 IN A 127.0.0.1")}

	suite.handler.db.Update(func(tx *bolt.Tx) error {
		if b, err := tx.CreateBucketIfNotExists(RecordBucket); err != nil {
			suite.Fail("Can't seed the database")
		} else {
			for k, v := range rrsExpected {
				b.Put([]byte(k), testMarshalRR(v))
			}
		}

		return nil
	})

	rrs, err := suite.handler.lookupRecord("foo.bar.services.com.", dns.TypeA, true, 0)
	suite.Equal(0, len(rrs))
	suite.NotNil(err)
	suite.Equal(ErrMaxRecursion, err)
}

func (suite *DnsTestSuite) TestShouldFindAWildcardRecord() {
	qname := "foo.bar.services.com."
	wildcard := "*.bar.services.com."
	qtype := dns.TypeA
	key := []byte(wildcard + "|" + dns.TypeToString[qtype])
	rrExpected := []dns.RR{
		testRR(wildcard + " 2700 IN A 163.172.233.56"),
		testRR(wildcard + " 2700 IN A 163.172.233.57"),
	}
	rrRaw, _ := json.Marshal(rrExpected)

	suite.handler.db.Update(func(tx *bolt.Tx) error {
		if b, err := tx.CreateBucketIfNotExists(RecordBucket); err != nil {
			suite.Fail("Can't seed the database")
		} else {
			b.Put(key, rrRaw)
		}

		return nil
	})

	rrs, err := suite.handler.lookupRecord(qname, qtype, true, 0)
	suite.Equal(len(rrExpected), len(rrs))
	suite.Equal(qname, rrs[0].Header().Name)
	suite.Nil(err)
}

func (suite *DnsTestSuite) TestShouldReturnOnlyTheRecordWithoutTheWildcardWhenThePlainRecordExist() {
	rrsExpected := make(map[string][]dns.RR)
	key := "foo.bar.services.com.|A"
	rrsExpected[key] = []dns.RR{
		testRR("foo.bar.services.com. 2700 IN A 163.172.233.57"),
	}
	rrsExpected["*.bar.services.com.|A"] = []dns.RR{testRR("*.bar.services.com. 2700 IN A 163.172.233.54")}

	suite.handler.db.Update(func(tx *bolt.Tx) error {
		if b, err := tx.CreateBucketIfNotExists(RecordBucket); err != nil {
			suite.Fail("Can't seed the database")
		} else {
			for k, v := range rrsExpected {
				b.Put([]byte(k), testMarshalRR(v))
			}
		}

		return nil
	})

	rrs, err := suite.handler.lookupRecord("foo.bar.services.com.", dns.TypeA, true, 0)
	suite.Equal(1, len(rrs))
	suite.True(dns.IsDuplicate(rrsExpected[key][0], rrs[0]), fmt.Errorf("Expected %s \t but got \t %s", rrsExpected[key][0], rrs[0])) // Normaly the insert order should be respected
	suite.Nil(err)
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
