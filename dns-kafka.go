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

/*
func handleReflect(w dns.ResponseWriter, r *dns.Msg) {
	var (
		v4  bool
		rr  dns.RR
		str string
		a   net.IP
	)
	m := new(dns.Msg)
	m.SetReply(r)
	if ip, ok := w.RemoteAddr().(*net.UDPAddr); ok {
		str = "Port: " + strconv.Itoa(ip.Port) + " (udp)"
		a = ip.IP
		v4 = a.To4() != nil
	}
	if ip, ok := w.RemoteAddr().(*net.TCPAddr); ok {
		str = "Port: " + strconv.Itoa(ip.Port) + " (tcp)"
		a = ip.IP
		v4 = a.To4() != nil
	}

	/*
		if v4 {
			rr = &dns.A{
				Hdr: dns.RR_Header{Name: dom, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0},
				A:   a.To4(),
			}
		} else {
			rr = &dns.AAAA{
				Hdr:  dns.RR_Header{Name: dom, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 0},
				AAAA: a,
			}
		}
*/ /*
	t := &dns.TXT{
		Hdr: dns.RR_Header{Name: "example.com", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0},
		Txt: []string{str},
	}

	switch r.Question[0].Qtype {
	case dns.TypeTXT:
		m.Answer = append(m.Answer, t)
		m.Extra = append(m.Extra, rr)
	default:
		fallthrough
	case dns.TypeAAAA, dns.TypeA:
		m.Answer = append(m.Answer, rr)
		m.Extra = append(m.Extra, t)
	case dns.TypeAXFR, dns.TypeIXFR:
		c := make(chan *dns.Envelope)
		tr := new(dns.Transfer)
		defer close(c)
		if err := tr.Out(w, r, c); err != nil {
			return
		}
		soa, _ := dns.NewRR(`whoami.miek.nl. 0 IN SOA linode.atoom.net. miek.miek.nl. 2009032802 21600 7200 604800 3600`)
		c <- &dns.Envelope{RR: []dns.RR{soa, t, rr, soa}}
		w.Hijack()
		// w.Close() // Client closes connection
		return
	}

	if r.IsTsig() != nil {
		if w.TsigStatus() == nil {
			m.SetTsig(r.Extra[len(r.Extra)-1].(*dns.TSIG).Hdr.Name, dns.HmacMD5, 300, time.Now().Unix())
		} else {
			println("Status", w.TsigStatus().Error())
		}
	}

	// set TC when question is tc.miek.nl.
	if m.Question[0].Name == "tc.miek.nl." {
		m.Truncated = true
		// send half a message
		buf, _ := m.Pack()
		w.Write(buf[:len(buf)/2])
		return
	}
	w.WriteMsg(m)
}
*/

func A(rr string) *dns.A { r, _ := dns.NewRR(rr); return r.(*dns.A) }

func AAAA(rr string) *dns.AAAA { r, _ := dns.NewRR(rr); return r.(*dns.AAAA) }

func CNAME(rr string) *dns.CNAME { r, _ := dns.NewRR(rr); return r.(*dns.CNAME) }

func recordToA(record Record) *dns.A {
	var Astr string

	if record.Priority > 0 {
		Astr = fmt.Sprintf("%s %d IN A %d %s", record.Name, record.Ttl, record.Priority, record.Content)
	} else {
		Astr = fmt.Sprintf("%s %d IN A %s", record.Name, record.Ttl, record.Content)
	}

	return A(Astr)
}

func recordToAAAA(record Record) *dns.AAAA {
	var AAAAstr string
	if record.Priority > 0 {
		AAAAstr = fmt.Sprintf("%s %d IN AAAA %d %s", record.Name, record.Ttl, record.Priority, record.Content)
	} else {
		AAAAstr = fmt.Sprintf("%s %d IN AAAA %s", record.Name, record.Ttl, record.Content)
	}
	return AAAA(AAAAstr)
}

func recordToCNAME(record Record) *dns.CNAME {
	var cnameStr string
	if record.Priority > 0 {
		cnameStr = fmt.Sprintf("%s %d IN CNAME %d %s", record.Name, record.Ttl, record.Priority, record.Content)
	} else {
		cnameStr = fmt.Sprintf("%s %d IN CNAME %s", record.Name, record.Ttl, record.Content)
	}
	return CNAME(cnameStr)
}

//TODO: add the support for other types
func recordToAnswer(record Record) dns.RR {
	var rr dns.RR
	// Pray for pattern matchin in Go 2 considered harmful
	if record.Type == dns.TypeToString[dns.TypeA] {
		rr = recordToA(record)
	}

	if record.Type == dns.TypeToString[dns.TypeAAAA] {
		rr = recordToAAAA(record)
	}

	if record.Type == dns.TypeToString[dns.TypeCNAME] {
		rr = recordToCNAME(record)
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
