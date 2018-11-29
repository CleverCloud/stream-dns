package main

type Config struct {
	Kafka KafkaConfig
	Dns   DnsConfig
}

type KafkaConfig struct {
	Address []string
	Topic   string
}

type DnsConfig struct {
	Address string
	Udp     bool
	Tcp     bool
}
