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
				if len(records) > 0 {
					for _, subRecords := range records {
						filteredSubRecords := filterByQtypeAndCname(subRecords, qtype)
						tmp := RecordsToAnswer(filteredSubRecords)

						for _, record := range tmp {
							if isSameQtypeOrItsCname(qtype, record.Header().Rrtype) {
								m.Answer = append(m.Answer, record)
							}
						}
					}
				} else {
					m.SetRcode(r, dns.RcodeNameError) // return NXDOMAIN
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
	viper.SetEnvPrefix("DNS") // Avoid collisions with others env variables
	viper.AllowEmptyEnv(false)
	viper.AutomaticEnv()

	config := Config{
		KafkaConfig{
			viper.GetStringSlice("kafka_address"),
			viper.GetString("kafka_topic"),
		},
		DnsConfig{
			viper.GetString("address"),
			viper.GetBool("udp"),
			viper.GetBool("tcp"),
		},
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
