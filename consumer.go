package main

import (
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"strings"
	"time"

	ms "stream-dns/metrics"
	u "stream-dns/utils"

	"github.com/Shopify/sarama"
	cluster "github.com/bsm/sarama-cluster"
	"github.com/getsentry/raven-go"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"github.com/xdg/scram"
	bolt "go.etcd.io/bbolt"
)

type consumer interface {
	Run(db *bolt.DB, metrics chan ms.Metric) error
}

type KafkaConsumer struct {
	config         KafkaConfig
	configConsumer *cluster.Config
	consumer       *cluster.Consumer
}

var SHA256 scram.HashGeneratorFcn = func() hash.Hash { return sha256.New() }
var SHA512 scram.HashGeneratorFcn = func() hash.Hash { return sha512.New() }

type XDGSCRAMClient struct {
	*scram.Client
	*scram.ClientConversation
	scram.HashGeneratorFcn
}

func (x *XDGSCRAMClient) Begin(userName, password, authzID string) (err error) {
	x.Client, err = x.HashGeneratorFcn.NewClient(userName, password, authzID)
	if err != nil {
		return err
	}
	x.ClientConversation = x.Client.NewConversation()
	return nil
}

func (x *XDGSCRAMClient) Step(challenge string) (response string, err error) {
	response, err = x.ClientConversation.Step(challenge)
	return
}

func (x *XDGSCRAMClient) Done() bool {
	return x.ClientConversation.Done()
}

func NewKafkaConsumer(config KafkaConfig) (*KafkaConsumer, error) {
	configConsumer := cluster.NewConfig()
	configConsumer.Consumer.Return.Errors = true
	configConsumer.Net.SASL.Enable = false
	configConsumer.Net.TLS.Enable = false

	configConsumer.Config.Metadata.Retry.Max = 10
	configConsumer.Config.Metadata.Retry.Backoff = 10 * time.Second

	if config.SaslEnable {
		log.Info("SASL enabled for the consumer: ", config.Address)
		configConsumer.Net.SASL.Enable = true
		configConsumer.Net.SASL.User = config.User
		configConsumer.Net.SASL.Password = config.Password
		configConsumer.Net.SASL.Mechanism = sarama.SASLMechanism(config.Mechanism)

		if strings.Contains(config.Mechanism, "sha512") || strings.Contains(config.Mechanism, "SHA-512") {
			configConsumer.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient { return &XDGSCRAMClient{HashGeneratorFcn: SHA512} }
			configConsumer.Net.SASL.Mechanism = sarama.SASLMechanism(sarama.SASLTypeSCRAMSHA512)
		} else if strings.Contains(config.Mechanism, "sha256") || strings.Contains(config.Mechanism, "SHA-256") {
			configConsumer.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient { return &XDGSCRAMClient{HashGeneratorFcn: SHA256} }
			configConsumer.Net.SASL.Mechanism = sarama.SASLMechanism(sarama.SASLTypeSCRAMSHA256)

		} else {
			log.Fatalf("invalid SHA algorithm \"%s\": can be either \"sha256\" or \"sha512\"", config.Mechanism)
		}
	}

	if config.TlsEnable {
		log.Info("TLS enabled for the consumer: ", config.Address)
		configConsumer.Net.TLS.Enable = true
	}

	configConsumer.ClientID = "stream-dns.consumer"
	configConsumer.Consumer.Offsets.Initial = sarama.OffsetOldest
	configConsumer.Consumer.Offsets.CommitInterval = 10 * time.Second

	brokers := config.Address
	topics := config.Topics
	consumerGroup := "stream-dns" + u.RandString(10)

	consumer, err := cluster.NewConsumer(brokers, consumerGroup, topics, configConsumer)
	if err != nil {
		return nil, err
	}

	return &KafkaConsumer{config, configConsumer, consumer}, nil
}

// Run the kafka agent consumer which read all the records from the Kafka topics
// Blocking call
func (k *KafkaConsumer) Run(db *bolt.DB, metrics chan ms.Metric, disallowCnameOnApex bool) error {
	log.Info("Kafka consumer connected to the kafka nodes: ", k.config.Address, " and ready to consume")

	for {
		select {
		case m, ok := <-k.consumer.Messages():
			log.Info("Got record for domain: ", string(m.Key))
			metrics <- ms.NewMetric("nb-record", nil, nil, time.Now(), ms.Counter)

			err := registerRecordAsBytesWithTheKeyInDB(db, m.Key, m.Value, disallowCnameOnApex)

			if err != nil {
				log.Error(err)
				raven.CaptureError(err, nil)
			}

			if !ok {
				continue
			}

		case err := <-k.consumer.Errors():
			metrics <- ms.NewMetric("kafka-consumer", nil, nil, time.Now(), ms.Counter)
			log.WithError(err).Error("Kafka consumer error")
		}
	}

	k.consumer.Close()
	return nil
}

// Register a record from a consumer message e.g: kafka in the Bolt database
func registerRecordAsBytesWithTheKeyInDB(db *bolt.DB, key []byte, record []byte, disallowCnameOnApex bool) error {
	domain, qtype := u.ExtractQnameAndQtypeFromConsumerKey(key)

	if disallowCnameOnApex && isCnameOnApexDomain(key) {
		return fmt.Errorf("Can't register the domain: %s \tCNAME on APEX domain are disallow.\nYou must define at true the env variable DISALLOW_CNAME_ON_APEX to allow it", domain)
	}

	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("records"))

		if err != nil {
			return nil
		}

		if u.IsSubdomain(domain) {
			// On subdomains: when CNAME already exists: allow only new CNAME.
			if b.Get([]byte(domain+".|CNAME")) != nil && qtype != dns.TypeCNAME {
				return fmt.Errorf("can't update the domain: %s a CNAME already exists", domain)
			}

			// On subdomains: when a CNAME comes, remove all previous records and replace with CNAME.
			if qtype == dns.TypeCNAME {
				// Keep the values in cache to rollback in case of error during register the CNAME
				// FIXME: register the value in a backup before delete it to recover them in the case of the PUT fail
				b.Delete([]byte(domain + ".|" + dns.TypeToString[dns.TypeA]))
				b.Delete([]byte(domain + ".|" + dns.TypeToString[dns.TypeAAAA]))
				b.Delete([]byte(domain + ".|" + dns.TypeToString[dns.TypeTXT]))
				b.Delete([]byte(domain + ".|" + dns.TypeToString[dns.TypePTR]))
				b.Delete([]byte(domain + ".|" + dns.TypeToString[dns.TypeMX]))
			}
		}

		err = b.Put(key, record)

		if err != nil {
			return err
		}

		return nil
	})
}

func isCnameOnApexDomain(key []byte) bool {
	domain, qtype := u.ExtractQnameAndQtypeFromConsumerKey(key)
	return u.IsApexDomain(domain) && dns.TypeCNAME == qtype
}
