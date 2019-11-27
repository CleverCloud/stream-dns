package utils

import (
	"bytes"
	"fmt"
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

func Parent(name string) (string, bool) {
	labels := dns.SplitDomainName(name)
	if labels == nil {
		return "", false
	}
	return ToLowerFQDN(strings.Join(labels[1:], ".")), true
}

func ToLowerFQDN(name string) string {
	return dns.Fqdn(strings.ToLower(name))
}

// Extract the qname and qtype from the key
// e.g: key has the format: <qname>.|<qtype>
// NOTE: the qname will keep the trailing dot, use the method TrimTrailingDotsInDomain to remove it
func ExtractQnameAndQtypeFromKey(key []byte) (string, uint16) {
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

func RRsIntoString(rrs []dns.RR) (res string) {
	var rrsStrings []string

	for _, rr := range rrs {
		rrsStrings = append(rrsStrings, rr.String())
	}

	res = strings.Join(rrsStrings, ", ")

	return
}

// IntoWildcardQname return the
func IntoWildcardQname(qname string) string {
	return fmt.Sprintf("*%s", qname[strings.Index(qname, "."):len(qname)-1])
}

// IsAWildcardRR look if the first subdomain in the qname is the wildcard token: *
// The contents of the wildcard RRs follows the usual rules and formats for
// RRs. The wildcards in the zone have an owner name that controls the
// query names they will match.  The owner name of the wildcard RRs is of
// the form "*.<anydomain>", where <anydomain> is any domain name.
// c.f: RFC-1034 section 4.3.3.
func IsAWildcardRR(rr dns.RR) bool {
	splitedDomainName := dns.SplitDomainName(rr.Header().Name)
	return splitedDomainName[0] == "*"
}

// GetZoneFromQname return the parent domain of a subdomain
// e.g: giving foo.bar.com. return bar.com.
func GetZoneFromQname(qname string) string {
	tmp := dns.SplitDomainName(qname)
	return dns.Fqdn(strings.Join(tmp[1:], "."))
}

// Key return a consumer key with the format <qname.|qtype> from a qname and qtype
// e.g: foo.com, A -> foo.com.|A
func Key(qname string, qtype uint16) []byte {
	return []byte(dns.Fqdn(qname) + "|" + dns.TypeToString[qtype])
}

// IsALocalRR look if the qname is a domain of one of this domain zones
// Example
// test.io, (.io, .com) -> true
// test.fr, (.io, .com) -> false
func IsALocalRR(qname string, zones []string) (isLocal bool) {
	isLocal = false

	for _, zone := range zones {
		if strings.HasSuffix(qname, zone) {
			isLocal = true
		}
	}

	return
}
