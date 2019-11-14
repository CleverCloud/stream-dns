package main

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"hash"
	"strings"
	"time"

	a "stream-dns/agent"
	ms "stream-dns/metrics"
	"stream-dns/utils"

	"github.com/Shopify/sarama"
	cluster "github.com/bsm/sarama-cluster"
	"github.com/getsentry/raven-go"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"github.com/xdg/scram"
	bolt "go.etcd.io/bbolt"
)

type consumer interface {
	Run(db *bolt.DB, metricsService a.MetricsService) error
}

type KafkaConsumer struct {
	db             *bolt.DB
	config         KafkaConfig
	configConsumer *cluster.Config
	consumer       *cluster.Consumer
	ms             *a.MetricsService
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

func NewKafkaConsumer(config KafkaConfig, db *bolt.DB, metricsService *a.MetricsService) (*KafkaConsumer, error) {
	brokers := config.Address
	topics := config.Topics
	consumerGroup := "stream-dns" + utils.RandString(10)

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

	return &KafkaConsumer{
		db:             db,
		config:         config,
		configConsumer: configConsumer,
		consumer:       consumer,
		ms:             metricsService,
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
func (c *KafkaConsumer) Run(disallowCnameOnApex bool) error {
	log.WithFields(log.Fields{
		"address": c.config.Address,
	}).Infof("Kafka consumer connected to the kafka nodes and ready to consume")

	for {
		select {
		case m, ok := <-c.consumer.Messages():
			c.treatKafkaMessage(m.Key, m.Value, disallowCnameOnApex) //TODO:print the timestamp
			if !ok {
				continue
			}

		case err := <-c.consumer.Errors():
			c.ms.GetOrCreateAggregator("kafka-consumer-error", ms.Counter, false).(a.AggregatorCounter).Inc(1)
			log.WithError(err).Error("Kafka consumer error")
		}
	}

	c.consumer.Close()
	return nil
}

func (c *KafkaConsumer) treatKafkaMessage(key []byte, payload []byte, disallowCnameOnApex bool) {
	log.WithField("domain", string(key)).Info("Got a new record")
	c.ms.GetOrCreateAggregator("nb-record", ms.Counter, false).(a.AggregatorCounter).Inc(1)

	rrs, err := c.transformRecordIntoRR(payload)

	if err != nil {
		log.WithField("key", string(key)).Error("Malformated record, unable to convert it into RR structure")
		return
	}

	c.logRecordDiffIfTheRecordWasAlreayHere(key, rrs)

	err = c.registerRecordAsBytesWithTheKeyInDB(key, rrs, disallowCnameOnApex)

	if err != nil {
		log.Error(err)
		raven.CaptureError(err, nil)
		c.ms.GetOrCreateAggregator("bad-record", ms.Counter, false).(a.AggregatorCounter).Inc(1)
	} else {
		c.ms.GetOrCreateAggregator("nb-record-saved", ms.Counter, false).(a.AggregatorCounter).Inc(1)
	}
}

func (c *KafkaConsumer) checkGuardsOnRRsRegistration(domain string, qtype uint16, disallowCnameOnApex bool) (err error) {
	if disallowCnameOnApex && c.isCnameOnApexDomain(domain, qtype) {
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

func (c *KafkaConsumer) transformRecordIntoRR(rawRecord []byte) (rrs []dns.RR, err error) {
	record, err := c.tryUnmarshalRecord(rawRecord)

	if err != nil {
		return
	}

	rrs, err = MapRecordsIntoRRs(record)

	if err != nil {
		return
	}

	return
}

// Register a record from a consumer message e.g: kafka in the Bolt database
func (c *KafkaConsumer) registerRecordAsBytesWithTheKeyInDB(key []byte, rrs []dns.RR, disallowCnameOnApex bool) error {
	domain, qtype := utils.ExtractQnameAndQtypeFromKey(key)
	isSubDomain := utils.IsSubdomain(domain)

	err := c.checkGuardsOnRRsRegistration(domain, qtype, disallowCnameOnApex)

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

func (c *KafkaConsumer) isCnameOnApexDomain(domain string, qtype uint16) bool {
	return utils.IsApexDomain(domain) && dns.TypeCNAME == qtype
}

func (c *KafkaConsumer) tryUnmarshalRecord(rawRecord []byte) (records []Record, err error) {
	err = json.Unmarshal(rawRecord, &records)
	return
}

// Look in the DB if the RR already exist
// If yes, we print the diff between the two RR
func (c *KafkaConsumer) logRecordDiffIfTheRecordWasAlreayHere(key []byte, rrs []dns.RR) {
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
