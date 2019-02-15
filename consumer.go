package main

import (
//	"github.com/apache/pulsar/pulsar-client-go/pulsar"
	"github.com/segmentio/kafka-go"
)

type consumer interface {
	runConsumer()
}

type KafkaConsumer struct {
	config KafkaConfig
	client *kafka.Reader
}

func NewKafkaConsumer(config KafkaConfig) *KafkaConsumer {
	client := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   config.Address,
		Topic:     config.Topic,
		Partition: 0,
		MinBytes:  10e3, // 10KB
		MaxBytes:  10e6, // 10MB
	})

	return &KafkaConsumer{
		config,
		client,
	}
}

func (s KafkaConsumer) runConsumer() {

}
/*
type PulsarConsumer struct {
	config PulsarConfig
	client pulsar.Client
}

func NewPulsarConsumer(config PulsarConfig) *PulsaConsumer {
	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL: config.Address,
		OperationTimeoutSeconds: 5,
		MessageListenerThreads: runtime.NumCPU(),
	})

	return &PulsarConsumer{
		config,
		client,
	}
}

func (s PulsarConsumer) runConsumer() {

}
*/
