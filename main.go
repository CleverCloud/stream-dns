package main

import (
	"context"
	"os"
	"os/signal"
	s "strings"
	"syscall"
	"time"

	"stream-dns/agent"
	ms "stream-dns/metrics"
	"stream-dns/output"

	"github.com/getsentry/raven-go"
	dns "github.com/miekg/dns"
	"github.com/segmentio/kafka-go"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	bolt "go.etcd.io/bbolt"
)

func launchReader(db *bolt.DB, config KafkaConfig, metrics chan ms.Metric) {
	log.Info("Read from kafka topic")

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   config.Address,
		Topic:     config.Topic,
		Partition: 0,
		MinBytes:  10e3, // 10KB
		MaxBytes:  10e6, // 10MB
	})

	for {
		m, errm := r.ReadMessage(context.Background())
		if errm != nil {
			break
		}
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
	}

	r.Close()
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
			viper.GetStringSlice("kafka_address"),
			viper.GetString("kafka_topic"),
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
