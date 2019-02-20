package main

import (
	"fmt"
	"os"
	"os/signal"
	s "strings"
	"syscall"
	"time"

	"stream-dns/agent"
	ms "stream-dns/metrics"
	"stream-dns/output"

	cluster "github.com/bsm/sarama-cluster"
	"github.com/getsentry/raven-go"
	dns "github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	bolt "go.etcd.io/bbolt"
)

func launchReader(db *bolt.DB, config KafkaConfig, metrics chan ms.Metric) {
	log.Info("Read from kafka topic")

	configConsumer := cluster.NewConfig()
	configConsumer.Consumer.Return.Errors = true
	configConsumer.Group.Return.Notifications = true
	configConsumer.Config.Net.SASL.Enable = false
	configConsumer.Config.Net.TLS.Enable = false

	if config.SaslEnable {
		configConsumer.Config.Net.SASL.Enable = true
		configConsumer.Config.Net.SASL.User = config.User
		configConsumer.Config.Net.SASL.Password = config.Password
	}

	if config.TlsEnable {
		configConsumer.Config.Net.TLS.Enable = true
	}

	configConsumer.ClientID = "stream-dns.consumer"
	configConsumer.Consumer.Offsets.CommitInterval = 10 * time.Second

	brokers := config.Address
	topics := []string{config.Topic}

	consumer, err := cluster.NewConsumer(brokers, "", topics, configConsumer)
	if err != nil {
		log.Panic(err)
	}

	for {
		select {
		case m, _ := <-consumer.Messages():
			log.Debug("Got record for domain ", string(m.Key))
			metrics <- ms.NewMetric("nb-record", nil, nil, time.Now(), ms.Counter)

			if s.Index(string(m.Key), "*") != -1 {
				log.Printf("message at topic/partition/offset %v/%v/%v: %s = %s\n", m.Topic, m.Partition, m.Offset, string(m.Key), string(m.Value))
			}
			db.Update(func(tx *bolt.Tx) error {
				b, err := tx.CreateBucketIfNotExists([]byte("records"))

				if err != nil {
					raven.CaptureError(err, nil)
					log.Fatal(err)
				}

				err = b.Put(m.Key, m.Value)

				if err != nil {
					raven.CaptureError(err, nil)
					log.Fatal(err)

					return err
				}

				return nil
			})

		case err := <-consumer.Errors():
			metrics <- ms.NewMetric("kafka-consumer", nil, nil, time.Now(), ms.Counter)
			log.WithError(err).Error("Kafka consumer error")

		case notif := <-consumer.Notifications():
			log.Info(fmt.Sprintf("%+v", notif))

		}
	}

	defer db.Close()
}

func serve(db *bolt.DB, config DnsConfig, metrics chan ms.Metric) {
	registerHandlerForResolver(".", db, config.ResolverAddress, metrics)
	registerHandlerForZones(config.Zones, db, metrics)

	if config.Udp {
		serverudp := &dns.Server{Addr: config.Address, Net: "udp", TsigSecret: nil}
		go serverudp.ListenAndServe()
		log.Info("UDP server listening on: ", config.Address)
	}

	if config.Tcp {
		servertcp := &dns.Server{Addr: config.Address, Net: "tcp", TsigSecret: nil}
		go servertcp.ListenAndServe()
		log.Info("TCP server listening on: ", config.Address)
	}
}

func main() {
	viper.SetEnvPrefix("DNS") // Avoid collisions with others env variables
	viper.AllowEmptyEnv(false)
	viper.AutomaticEnv()

	config := Config{
		KafkaConfig{
			Address: viper.GetStringSlice("kafka_address"),
			Topic: viper.GetString("kafka_topic"),
			SaslEnable: viper.GetBool("sasl_enable"),
			TlsEnable: viper.GetBool("tls_enable"),
			User: viper.GetString("kafka_user"),
			Password: viper.GetString("kafka_password"),
		},
		DnsConfig{
			viper.GetString("address"),
			viper.GetBool("udp"),
			viper.GetBool("tcp"),
			viper.GetStringSlice("zones"),
			viper.GetString("resolver_address"),
		},
		AgentConfig{
			viper.GetInt("metrics_buffer_size"),
			viper.GetDuration("metrics_flush_interval"),
		},
		viper.GetString("pathdb"),
		viper.GetString("sentry_dsn"),
	}

	// Sentry
	raven.SetDSN(config.sentryDSN)

	// Setup os signal to stop this service
	sig := make(chan os.Signal)

	db, err := bolt.Open(config.PathDB, 0600, nil)
	if err != nil {
		raven.CaptureError(err, map[string]string{"step": "init"})
		log.Fatal(err)
	}

	// Metrics
	agent := agent.NewAgent(agent.Config{config.Agent.BufferSize, config.Agent.FlushInterval})
	agent.AddOutput(output.StdoutOutput{})

	go agent.Run()

	// Run goroutines service
	go launchReader(db, config.Kafka, agent.Input)
	go serve(db, config.Dns, agent.Input)

	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig

	log.WithFields(log.Fields{"signal": s}).Info("Signal received, stopping")
}
