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

type KafkaConfig struct {
	Address []string
	Topic   string
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
