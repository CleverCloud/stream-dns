/*
* Client for DNS lookup which support multiple questions
* USAGE: client [[qname] [qtype]]...
*
* Example: client yolo.com A foo.bar AAAA
 */

package main

import (
	"fmt"
	"github.com/miekg/dns"
	"os"
	log "github.com/sirupsen/logrus"
)

func main() {
	argsWithoutProg := os.Args[1:]

	if len(argsWithoutProg)%2 != 0 || len(argsWithoutProg) == 0 {
		fmt.Printf("USAGE: client [[qname] [qtype]]...")
		os.Exit(1)
	}

	client := new(dns.Client)
	m := new(dns.Msg)

	for i, j := 0, 0; i <= len(argsWithoutProg)/2; i, j = i+2, j+1 {
		m.Question = append(m.Question, dns.Question{argsWithoutProg[i], dns.StringToType[argsWithoutProg[i+1]], dns.ClassINET})
	}

	m.RecursionDesired = true

	r, _, err := client.Exchange(m, "localhost:8053")
	if r == nil {
		log.Fatalf("*** error: %s\n", err.Error())
	}

	if r.Rcode != dns.RcodeSuccess {
		log.Fatalf(" *** invalid answer name")
	}

	for _, a := range r.Answer {
		fmt.Printf("%v\n", a)
	}
}
