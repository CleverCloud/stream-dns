package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestShouldDetectACorrectApexDomain(t *testing.T) {	
	assert.True(t, isApexDomain("example.com"))
}

func TestShouldDetectThatDomainWithSubdomainOrNotApexDomain(t *testing.T) {
	assert.False(t, isApexDomain("www.example.com"))
}
