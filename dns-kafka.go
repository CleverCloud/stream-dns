package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/boltdb/bolt"
	"github.com/segmentio/kafka-go"

	"github.com/miekg/dns"
)

type Record struct {
	Name     string
	Type     string
	Content  string
	Ttl      int
	Priority int
}

func launchReader(db *bolt.DB) {
	log.Print("Read from kafka topic")

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   []string{"localhost:9092"},
		Topic:     "compressed-domains",
		Partition: 0,
		MinBytes:  10e3, // 10KB
		MaxBytes:  10e6, // 10MB
	})

	for {
		m, errm := r.ReadMessage(context.Background())
		if errm != nil {
			break
		}
		log.Printf("message at topic/partition/offset %v/%v/%v: %s = %s\n", m.Topic, m.Partition, m.Offset, string(m.Key), string(m.Value))
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

func serve(db *bolt.DB) {

	dns.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {

		log.Printf(r.Question[0].Name)
		/*
			t := &dns.TXT{
				Hdr: dns.RR_Header{Name: "clever-example.com", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0},
				Txt: []string{"CATCH PLOP"},
			}
		*/
		m := new(dns.Msg)
		m.SetReply(r)
		/*
			//m.Authoritative = true
			//m.Ns = []dns.RR{NewRR("$ORIGIN plopitude.forta.\n")}
			m.Extra = append(m.Extra, t)*/

		db.View(func(tx *bolt.Tx) error {
			// Assume bucket exists and has keys
			c := tx.Bucket([]byte("records")).Cursor()

			prefix := []byte(r.Question[0].Name)
			for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
				var r []Record
				json.Unmarshal([]byte(v), &r)
				log.Printf("records: %+v", r)

			}

			return nil
		})
		m.Answer = append(m.Answer, A("zenaton-rabbitmq-c1-n3.services.clever-cloud.com. IN A 127.0.0.1"))

		w.WriteMsg(m)
	})

	serverudp := &dns.Server{Addr: ":8053", Net: "udp", TsigSecret: nil}
	if err := serverudp.ListenAndServe(); err != nil {
		fmt.Printf("Failed to setup the udp server: %s\n", err.Error())

	}

	servertcp := &dns.Server{Addr: ":8053", Net: "tcp", TsigSecret: nil}
	if err := servertcp.ListenAndServe(); err != nil {
		fmt.Printf("Failed to setup the tcp server: %s\n", err.Error())

	}
}

func main() {
	sig := make(chan os.Signal)

	db, err := bolt.Open("/tmp/my.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	//go launchReader(db)

	go serve(db)

	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig

	fmt.Printf("Signal (%s) received, stopping\n", s)
}
