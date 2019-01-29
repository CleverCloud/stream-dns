package main

type Config struct {
	Kafka  KafkaConfig
	Dns    DnsConfig
	PathDB string
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
