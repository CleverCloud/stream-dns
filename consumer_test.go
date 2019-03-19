package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckIsNotCnameOnApexDomain(t *testing.T) {
	assert.True(t, isCnameOnApexDomain([]byte("example.com.|CNAME")))
	assert.False(t, isCnameOnApexDomain([]byte("example.com.|A")))

	assert.False(t, isCnameOnApexDomain([]byte("www.example.com.|CNAME")))
	assert.False(t, isCnameOnApexDomain([]byte("www.example.com.|A")))
}
