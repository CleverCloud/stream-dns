package main

import (
	"strings"
	"time"
)

type Config struct {
	Kafka               KafkaConfig
	Dns                 DnsConfig
	Agent               AgentConfig
	Statsd              StatsdConfig
	Administrator       AdministratorConfig
	PathDB              string
	sentryDSN           string
	DisallowCNAMEonAPEX bool
	InstanceId          string
}

type StatsdConfig struct {
	Address string
	Prefix  string // can be empty
}

type PulsarConfig struct {
	Address string
	Topic   string
}

type KafkaConfig struct {
	Address    []string
	Topics     []string
	SaslEnable bool
	TlsEnable  bool
	User       string
	Password   string
	Mechanism  string
}

type DnsConfig struct {
	Address         string
	Udp             bool
	Tcp             bool
	Zones           []string
	ResolverAddress string
}

type AgentConfig struct {
	BufferSize    int
	FlushInterval time.Duration
}

type AdministratorConfig struct {
	Username  string
	Password  string
	Address   string
	JwtSecret string
}

const ENV_PREFIX = "DNS"

// Allow zone transfer
// Turned off by default because DNS zone transfer AXFR Requests may leak domain information
const ALLOW_AXFR = "ALLOW_AXFR"

// All env variables are prefixed by DNS to avoid naming collision.
// Sometime we want to prevent that an env variable must be set to allow a feature.
// This method format the tag for a better UX and remove ambiguity
// ex: "allow_axfr" -> "DNS_ALLOW_AXFR"
// which is the true configuration param and not "allow_axfr".
func formatConfig(tag string) string {
	return strings.ToUpper(ENV_PREFIX + "_" + ALLOW_AXFR)
}
