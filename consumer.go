package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	a "stream-dns/agent"
	ms "stream-dns/metrics"
	"stream-dns/utils"

	"github.com/Shopify/sarama"
	"github.com/apache/pulsar-client-go/pulsar"
	cluster "github.com/bsm/sarama-cluster"
	"github.com/google/uuid"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

const (
	KafkaType  = "kafka"
	PulsarType = "pulsar"
)

type Consumer interface {
	Run() error
	DB() *bolt.DB
	MetricsService() *a.MetricsService
}

// CommonConsumer is the common configuration infos for Consumer
type CommonConsumer struct {
	db                  *bolt.DB
	ms                  *a.MetricsService
	disallowCnameOnApex bool
}

type pulsarConsumer struct {
	common CommonConsumer
	config PulsarConfig
	client pulsar.Client
}

type kafkaConsumer struct {
	common         CommonConsumer
	config         KafkaConfig
	configConsumer *cluster.Config
	consumer       *cluster.Consumer
}

func NewConsumer(typeConsumer string, config ConsumerConfig, common CommonConsumer) (Consumer, error) {
	var consumer Consumer
	var err error

	switch typeConsumer {
	case KafkaType:
		consumer, err = NewKafkaConsumer(config.Kafka, common)
	case PulsarType:
		consumer, err = NewPulsarConsumer(config.Pulsar, common)
	default:
		return nil, fmt.Errorf("Unknow consumer type: %s", typeConsumer)
	}

	if err != nil {
		return nil, err
	} else {
		return consumer, nil
	}
}

func NewKafkaConsumer(config KafkaConfig, common CommonConsumer) (*kafkaConsumer, error) {
	brokers := config.Address
	topics := config.Topics
	consumerGroup := "stream-dns-" + uuid.New().String()

	configConsumer, err := SetUpConsumerKafkaConfig(config)

	if err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"address":   config.Address,
		"sasl":      config.SaslEnable,
		"tls":       config.TlsEnable,
		"mechanism": config.Mechanism,
	}).Info("Trying to connect to kafka brokers...")

	consumer, err := cluster.NewConsumer(brokers, consumerGroup, topics, configConsumer)
	if err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"consumer-group": consumerGroup,
		"topics":         topics,
		"address":        config.Address,
		"sasl":           config.SaslEnable,
		"tls":            config.TlsEnable,
		"mechanism":      config.Mechanism,
	}).Info("Consumer created and connected to kafka brokers")

	return &kafkaConsumer{
		common:         common,
		config:         config,
		configConsumer: configConsumer,
		consumer:       consumer,
	}, nil
}

func SetUpConsumerKafkaConfig(config KafkaConfig) (*cluster.Config, error) {
	configConsumer := cluster.NewConfig()
	configConsumer.Consumer.Return.Errors = true
	configConsumer.Net.SASL.Enable = false
	configConsumer.Net.TLS.Enable = false

	configConsumer.Config.Metadata.Retry.Max = 10
	configConsumer.Config.Metadata.Retry.Backoff = 10 * time.Second

	if config.SaslEnable {
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
			return nil, fmt.Errorf("Invalid SHA algorithm \"%s\": can be either \"sha256\" or \"sha512\"", config.Mechanism)
		}
	}

	if config.TlsEnable {
		configConsumer.Net.TLS.Enable = true
	}

	configConsumer.ClientID = "stream-dns.consumer"
	configConsumer.Consumer.Offsets.Initial = sarama.OffsetOldest
	configConsumer.Consumer.Offsets.CommitInterval = 10 * time.Second

	return configConsumer, nil
}

// Run the kafka agent consumer which read all the records from the Kafka topics
// Blocking call
func (c *kafkaConsumer) Run() error {
	log.WithFields(log.Fields{
		"address": c.config.Address,
	}).Infof("Kafka consumer connected to the kafka nodes and ready to consume")

	for {
		select {
		case m, ok := <-c.consumer.Messages():
			c.common.treatMessage(m.Key, m.Value)

			if !ok {
				continue
			}

		case err := <-c.consumer.Errors():
			c.MetricsService().GetOrCreateAggregator("kafka-consumer-error", ms.Counter, false).(a.AggregatorCounter).Inc(1)
			log.WithError(err).Error("Kafka consumer error")
		}
	}

	c.consumer.Close()
	return nil
}

func (c *kafkaConsumer) DB() *bolt.DB {
	return c.common.db
}

func (c *kafkaConsumer) MetricsService() *a.MetricsService {
	return c.common.ms
}

func NewPulsarConsumer(config PulsarConfig, common CommonConsumer) (*pulsarConsumer, error) {
	var auth pulsar.Authentication

	if config.JWT != "" {
		auth = pulsar.NewAuthenticationToken(config.JWT)
	}

	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL:            config.Address,
		Authentication: auth,
	})

	if err != nil {
		return nil, err
	}

	return &pulsarConsumer{
		common: common,
		config: config,
		client: client,
	}, nil
}

func (c *pulsarConsumer) Run() error {
	consumer, err := c.client.Subscribe(pulsar.ConsumerOptions{
		Topic:            c.config.Topic,
		SubscriptionName: c.config.SubscriptionName,
		Type:             pulsar.Shared,
	})

	if err != nil {
		return err
	}

	defer consumer.Close()

	consumerChan := consumer.Chan()

	for {
		msg := <-consumerChan
		c.common.treatMessage([]byte(msg.Key()), msg.Payload())
		consumer.Ack(msg)
	}
}

func (c *pulsarConsumer) DB() *bolt.DB {
	return c.common.db
}

func (c *pulsarConsumer) MetricsService() *a.MetricsService {
	return c.common.ms
}

func (c CommonConsumer) treatMessage(key []byte, payload []byte) {
	log.WithField("domain", string(key)).Info("Got a new record")

	records, err := c.tryUnmarshalRecord(payload)

	if err != nil {
		log.WithField("key", string(key)).Error("Malformated record, unable to convert JSON into Records")
		return
	}

	c.printMetadatas(records)

	rrs, err := MapRecordsIntoRRs(records)

	if err != nil {
		log.WithField("key", string(key)).Error("Malformated record, unable to convert it into RR structure")
		return
	}

	c.logRecordDiffIfTheRecordWasAlreayHere(key, rrs)

	err = c.registerRecordAsBytesWithTheKeyInDB(key, rrs)
}

func (c CommonConsumer) checkGuardsOnRRsRegistration(domain string, qtype uint16) (err error) {
	if c.disallowCnameOnApex && c.isCnameOnApexDomain(domain, qtype) {
		return fmt.Errorf("Can't register the domain: %s \tCNAME on APEX domain are disallow.\nYou must define at true the env variable DISALLOW_CNAME_ON_APEX to allow it", domain)
	}

	if utils.IsSubdomain(domain) {
		err = c.db.View(func(tx *bolt.Tx) (err error) {
			b := tx.Bucket([]byte(RecordBucket))

			// On subdomains: when CNAME already exists: allow only new CNAME.
			if b.Get([]byte(domain+".|CNAME")) != nil && qtype != dns.TypeCNAME {
				return fmt.Errorf("Can't update the domain: %s a CNAME already exists", domain)
			}

			return
		})
	}

	return
}

// Register a record from a consumer message e.g: kafka in the Bolt database
func (c CommonConsumer) registerRecordAsBytesWithTheKeyInDB(key []byte, rrs []dns.RR) error {
	domain, qtype := utils.ExtractQnameAndQtypeFromKey(key)
	isSubDomain := utils.IsSubdomain(domain)

	err := c.checkGuardsOnRRsRegistration(domain, qtype)

	if err != nil {
		return err
	}

	rrsRaw, err := json.Marshal(rrs)

	if err != nil {
		return err
	}

	err = c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(RecordBucket))

		if isSubDomain {
			// On subdomains: when a CNAME comes, remove all previous records and replace with CNAME.
			if qtype == dns.TypeCNAME {
				// Keep the values in cache to rollback in case of error during register the CNAME
				// FIXME: register the value in a backup before delete it to recover them in the case of the PUT fail
				b.Delete([]byte(domain + ".|" + dns.TypeToString[dns.TypeA]))
				b.Delete([]byte(domain + ".|" + dns.TypeToString[dns.TypeAAAA]))
				b.Delete([]byte(domain + ".|" + dns.TypeToString[dns.TypeTXT]))
				b.Delete([]byte(domain + ".|" + dns.TypeToString[dns.TypePTR]))
				b.Delete([]byte(domain + ".|" + dns.TypeToString[dns.TypeMX]))
				//FIXME update the nb of record in the metrics
			}
		}

		err := b.Put(key, rrsRaw)

		if err != nil {
			return err
		}

		return nil
	})

	log.WithField("rr", utils.RRsIntoString(rrs)).Infof("Saved a new record in DB")
	return err
}

func (c CommonConsumer) isCnameOnApexDomain(domain string, qtype uint16) bool {
	return utils.IsApexDomain(domain) && dns.TypeCNAME == qtype
}

func (c CommonConsumer) tryUnmarshalRecord(rawRecord []byte) (records []Record, err error) {
	err = json.Unmarshal(rawRecord, &records)
	return
}

// Look in the DB if the RR already exist
// If yes, we print the diff between the two RR
func (c CommonConsumer) logRecordDiffIfTheRecordWasAlreayHere(key []byte, rrs []dns.RR) {
	var previousRRraw []byte
	var previousRR []dns.RR

	c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(RecordBucket))
		previousRRraw = b.Get(key)
		return nil
	})

	if previousRRraw != nil {
		err := json.Unmarshal(previousRRraw, &previousRR)

		if err == nil {
			diffContent := false

			//check the difference in the content
			if len(previousRR) == len(rrs) {
				for i, previousRR := range previousRR {
					if !dns.IsDuplicate(previousRR, rrs[i]) {
						diffContent = true
					}
				}
			}

			if len(previousRR) != len(rrs) || diffContent {
				log.WithFields(log.Fields{
					"before": previousRR,
					"after":  rrs,
				}).Infof("The record %s has changed", string(key))
			}
		}
	}
}

func (c CommonConsumer) printMetadatas(rrs []Record) {
	for _, record := range rrs {
		if record.Metadatas.Producer != "" && record.Metadatas.CreatedAt != 0 {
			log.WithFields(log.Fields{
				"name":       record.Name,
				"created-at": time.Unix(int64(record.Metadatas.CreatedAt), 0),
				"producer":   record.Metadatas.Producer,
			}).Info("record metadatas")
		}
	}
}
