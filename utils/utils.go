package utils

import (
	"strings"
	"math/rand"
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
func isApexDomain(domain string) bool {
	// We must just have more than one dot if we have subdomains
	return strings.Count(domain, ".") == 1
}
