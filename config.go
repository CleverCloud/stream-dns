package main

import (
	"time"
)

type Config struct {
	Kafka     KafkaConfig
	Dns       DnsConfig
	Agent     AgentConfig
	PathDB    string
	sentryDSN string
}

type PulsarConfig struct {
	Address string
	Topic   string
}

type KafkaConfig struct {
	Address    []string
	Topic      string
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
