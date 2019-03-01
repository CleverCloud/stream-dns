package main

import (
	"time"

	ms "stream-dns/metrics"

	"github.com/Shopify/sarama"
	"github.com/getsentry/raven-go"
	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

type consumer interface {
	Run(db *bolt.DB, metrics chan ms.Metric) error
}

type KafkaConsumer struct {
	config         KafkaConfig
	configConsumer *sarama.Config
	consumer       sarama.Consumer
}

func NewKafkaConsumer(config KafkaConfig) (*KafkaConsumer, error) {
	configConsumer := sarama.NewConfig()
	configConsumer.Consumer.Return.Errors = true
	configConsumer.Net.SASL.Enable = false
	configConsumer.Net.TLS.Enable = false

	if config.SaslEnable {
		log.Info("SASL enabled for the consumer: ", config.Address)
		configConsumer.Net.SASL.Enable = true
		configConsumer.Net.SASL.User = config.User
		configConsumer.Net.SASL.Password = config.Password
		configConsumer.Net.SASL.Mechanism = sarama.SASLMechanism(config.Mechanism)
	}

	if config.TlsEnable {
		log.Info("TLS enabled for the consumer: ", config.Address)
		configConsumer.Net.TLS.Enable = true
	}

	configConsumer.ClientID = "stream-dns.consumer"
	configConsumer.Consumer.Offsets.CommitInterval = 10 * time.Second

	brokers := config.Address

	consumer, err := sarama.NewConsumer(brokers, configConsumer)

	if err != nil {
		return nil, err
	}

	return &KafkaConsumer{config, configConsumer, consumer}, nil
}

// Run the kafka agent consumer which read all the records from the Kafka topics
//Blocking call
func (k *KafkaConsumer) Run(db *bolt.DB, metrics chan ms.Metric) error {
	partitionConsumer, err := k.consumer.ConsumePartition(k.config.Topic, 0, sarama.OffsetOldest)
	if err != nil {
		return err
	}

	log.Info("Kafka consumer connected to the kafka nodes: ", k.config.Address, " and ready to consume")

	for {
		select {
		case m, _ := <-partitionConsumer.Messages():
			log.Info("Got record for domain ", string(m.Key))
			metrics <- ms.NewMetric("nb-record", nil, nil, time.Now(), ms.Counter)

			err := registerRecordAsBytesWithTheKeyInDB(db, m.Key, m.Value)

			if err != nil {
				log.WithError(err).Error(err)
				raven.CaptureError(err, nil)
			}

		case err := <-partitionConsumer.Errors():
			metrics <- ms.NewMetric("kafka-consumer", nil, nil, time.Now(), ms.Counter)
			log.WithError(err).Error("Kafka consumer error")
		}
	}

	return nil
}

// Register a record from a consumer message e.g: kafka in the Bolt database
func registerRecordAsBytesWithTheKeyInDB(db *bolt.DB, key []byte, record []byte) error {
	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("records"))

		if err != nil {
			return nil
		}

		err = b.Put(key, record)

		if err != nil {
			return err
		}

		return nil
	})
}
