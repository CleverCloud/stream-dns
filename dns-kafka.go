package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	s "strings"
	"syscall"

	dns "github.com/miekg/dns"
	"github.com/segmentio/kafka-go"
	"github.com/spf13/viper"
	bolt "go.etcd.io/bbolt"
)

type Record struct {
	Name     string
	Type     string
	Content  string
	Ttl      int
	Priority int
}

func launchReader(db *bolt.DB, config KafkaConfig) {
	log.Print("Read from kafka topic")

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   config.Address,
		Topic:     config.Topic,
		Partition: 0,
		MinBytes:  10e3, // 10KB
		MaxBytes:  10e6, // 10MB
	})

	for {
		m, errm := r.ReadMessage(context.Background())
		if errm != nil {
			break
		}
		log.Printf("%s\n", string(m.Key))

		if s.Index(string(m.Key), "*") != -1 {
			log.Printf("message at topic/partition/offset %v/%v/%v: %s = %s\n", m.Topic, m.Partition, m.Offset, string(m.Key), string(m.Value))
		}
		db.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists([]byte("records"))
			if err != nil {
				log.Fatal(err)
			}
			err2 := b.Put(m.Key, m.Value)
			if err2 != nil {
				log.Fatal(err2)

				return err2
			}
			return nil
		})
	}

	r.Close()
	defer db.Close()

}

func A(rr string) *dns.A { r, _ := dns.NewRR(rr); return r.(*dns.A) }

func AAAA(rr string) *dns.AAAA { r, _ := dns.NewRR(rr); return r.(*dns.AAAA) }

func CNAME(rr string) *dns.CNAME { r, _ := dns.NewRR(rr); return r.(*dns.CNAME) }

func SOA(rr string) *dns.SOA { r, _ := dns.NewRR(rr); return r.(*dns.SOA) }

func MX(rr string) *dns.MX { r, _ := dns.NewRR(rr); return r.(*dns.MX) }

func NS(rr string) *dns.NS { r, _ := dns.NewRR(rr); return r.(*dns.NS) }

func TXT(rr string) *dns.TXT { r, _ := dns.NewRR(rr); return r.(*dns.TXT) }

func PTR(rr string) *dns.PTR { r, _ := dns.NewRR(rr); return r.(*dns.PTR) }

func recordToString(record Record) string {
	if record.Priority > 0 {
		return fmt.Sprintf("%s %d IN %s %d %s", record.Name, record.Ttl, record.Type, record.Priority, record.Content)
	} else {
		return fmt.Sprintf("%s %d IN %s %s", record.Name, record.Ttl, record.Type, record.Content)
	}
}

func recordToAnswer(record Record) dns.RR {
	var rr dns.RR
	rtype := dns.StringToType[record.Type]
	recordstr := recordToString(record)

	switch rtype {
	case dns.TypeA:
		rr = A(recordstr)
	case dns.TypeAAAA:
		rr = AAAA(recordstr)
	case dns.TypeCNAME:
		rr = CNAME(recordstr)
	case dns.TypeSOA:
		rr = SOA(recordstr)
	case dns.TypeMX:
		rr = MX(recordstr)
	case dns.TypeNS:
		rr = NS(recordstr)
	case dns.TypeTXT:
		rr = TXT(recordstr)
	case dns.TypePTR:
		rr = PTR(recordstr)
	default:
		log.Fatalf("Incorrect or unsupported record type: %s", recordstr)
		return nil
	}

	return rr
}

func recordsToAnswer(records []Record) []dns.RR {
	var rrs []dns.RR

	for _, record := range records {
		rrs = append(rrs, recordToAnswer(record))
	}

	return rrs
}

func intoWildcardQName(qname string) string {
	return fmt.Sprintf("*%s", qname[s.Index(qname, "."):len(qname)-1])
}

func isNotWildcardName(qname string) bool {
	return qname[0] != '*'
}

func getRecordsFromBucket(bucket *bolt.Bucket, qname string) ([][]Record, error) {
	var records [][]Record = [][]Record{}
	c := bucket.Cursor()

	prefix := []byte(qname)
	for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
		var record []Record
		err := json.Unmarshal([]byte(v), &record)

		if err != nil {
			return nil, err
		} else {
			records = append(records, record)
		}
	}

	if isNotWildcardName(qname) && len(records) == 0 {
		log.Printf("here %d", len(records))
		c.First()

		prefixWildcard := []byte(intoWildcardQName(qname))
		for k, v := c.Seek(prefixWildcard); k != nil && bytes.HasPrefix(k, prefixWildcard); k, v = c.Next() {
			var record []Record

			err := json.Unmarshal([]byte(v), &record)
			if err != nil {
				return nil, err
			} else {
				records = append(records, record)
			}
		}
	}

	return records, nil
}

func isSameQtypeOrItsCname(qtypeQuestion uint16, qtypeRecord uint16) bool {
	return qtypeRecord == qtypeQuestion || qtypeRecord == dns.TypeCNAME
}

func filterByQtypeAndCname(records []Record, qtype uint16) []Record {
	var filteredRecords []Record

	for _, record := range records {
		rrTypeRecord := dns.StringToType[record.Type]

		if isSameQtypeOrItsCname(qtype, rrTypeRecord) {
			filteredRecords = append(filteredRecords, record)
		}
	}

	return filteredRecords
}

func serve(db *bolt.DB, config DnsConfig) {
	dns.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		log.Printf("Got a request for %s", r.Question[0].Name)

		qname := r.Question[0].Name
		qtype := r.Question[0].Qtype

		m := new(dns.Msg)
		m.SetReply(r)

		db.View(func(tx *bolt.Tx) error {
			// TODO: check if the bucket already exists and has keys
			recordsBucket := tx.Bucket([]byte("records"))

			records, err := getRecordsFromBucket(recordsBucket, qname)

			if err != nil {
				log.Fatal(err)
				return nil
			} else {
				for _, subRecords := range records {
					filteredSubRecords := filterByQtypeAndCname(subRecords, qtype)
					tmp := recordsToAnswer(filteredSubRecords)

					for _, record := range tmp {
						if isSameQtypeOrItsCname(qtype, record.Header().Rrtype) {
							m.Answer = append(m.Answer, record)
						}
					}
				}
			}
			return nil
		})

		w.WriteMsg(m)
	})

	if config.Udp {
		serverudp := &dns.Server{Addr: config.Address, Net: "udp", TsigSecret: nil}
		if err := serverudp.ListenAndServe(); err != nil {
			fmt.Printf("Failed to setup the udp server: %s\n", err.Error())
		}
	}

	if config.Tcp {
		servertcp := &dns.Server{Addr: config.Address, Net: "tcp", TsigSecret: nil}
		if err := servertcp.ListenAndServe(); err != nil {
			fmt.Printf("Failed to setup the tcp server: %s\n", err.Error())
		}
	}
}

func main() {
	// Get configuration
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/dns-kafka/")

	// Default values target a dev mode configuration
	viper.SetDefault("kafka.address", "localhost:9092")
	viper.SetDefault("kafka.topic", "compressed-domains")

	viper.SetDefault("dns.address", ":8053")
	viper.SetDefault("dns.udp", true)
	viper.SetDefault("dns.tcp", true)

	var config Config

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file, %s", err)
	}

	err := viper.Unmarshal(&config)
	if err != nil {
		log.Fatalf("unable to decode into struct, %v", err)
	}

	// Setup os signal to stop this service
	sig := make(chan os.Signal)

	db, err := bolt.Open("/tmp/my.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Run goroutines service
	go launchReader(db, config.Kafka)
	go serve(db, config.Dns)

	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig

	fmt.Printf("Signal (%s) received, stopping\n", s)
}
