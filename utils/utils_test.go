package utils

import (
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestShouldDetectACorrectApexDomain(t *testing.T) {
	assert.True(t, IsApexDomain("example.com"))
}

func TestShouldDetectThatDomainWithSubdomainOrNotApexDomain(t *testing.T) {
	assert.False(t, IsApexDomain("www.example.com"))
}

func TestExtractQnameAndQtypeFromConsumerKey(t *testing.T) {
	qname, qtype := ExtractQnameAndQtypeFromConsumerKey([]byte("www.example.com.|CNAME"))
	assert.Equal(t, "www.example.com", qname)
	assert.Equal(t, dns.TypeCNAME, qtype)
}

func TestTrimRemoveTrailingDotInDomain(t *testing.T) {
	assert.Equal(t, "www.example.com", TrimTrailingDotInDomain("www.example.com."))
	assert.Equal(t, "www.example.com", TrimTrailingDotInDomain("www.example.com"))
}
