package utils

import (
	"bytes"
	"math/rand"
	"strings"

	dns "github.com/miekg/dns"
)

// Generate a random string of a fixed length in Go
// Use this to generate the bolt database name
func RandString(length int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	b := make([]rune, length)

	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}

// An Apex domain, is a root domain that does not contain a subdomain.
// Our (simple) strategy to detect if it's an APEX is to count the number of dot.
func IsApexDomain(domain string) bool {
	// We must just have more than one dot if we have subdomains
	return strings.Count(domain, ".") == 1
}

func IsSubdomain(domain string) bool {
	return strings.Count(domain, ".") > 1
}

// Extract the qname and qtype from the key
// e.g: key has the format: <qname>.|<qtype>
// NOTE: the qname will keep the trailing dot, use the method TrimTrailingDotsInDomain to remove it
func ExtractQnameAndQtypeFromConsumerKey(key []byte) (string, uint16) {
	res := bytes.Split(key, []byte(".|"))
	return string(res[0]), dns.StringToType[string(res[1])]
}

// Remove the trailing dot in a domain
// www.example.com. -> www.example.com
// www.example.com -> www.example.com
func TrimTrailingDotInDomain(domain string) string {
	return strings.TrimSuffix(domain, ".")
}

// Format a list of answers
func FormatAnswers(answers []dns.RR) string {
	var buff bytes.Buffer

	for _, a := range answers {
		s := FormatAnswer(a) + "\n"
		buff.WriteString(s)
	}

	return buff.String()
}

// Format an answer in a string with this format:
// Name. TTL Class Type
// The default String method put \t between each elements. This method replace this all \t by a whitespace
func FormatAnswer(answer dns.RR) string {
	h := answer.String()
	return strings.Replace(h, "\t", " ", -1) // ReplaceAll is in tip, but not in latest release of go 1.11.1
}
