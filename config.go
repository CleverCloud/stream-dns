package main

import (
	"time"
)

type Config struct {
	Pulsar              PulsarConfig
	Kafka               KafkaConfig
	Dns                 DnsConfig
	Agent               AgentConfig
	Statsd              StatsdConfig
	PathDB              string
	sentryDSN           string
	DisallowCNAMEonAPEX bool
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
