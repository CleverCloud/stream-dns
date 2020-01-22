package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	a "stream-dns/agent"
	"stream-dns/output"
	"stream-dns/utils"
	"strings"
	"syscall"
	"time"

	"github.com/getsentry/raven-go"
	"github.com/google/uuid"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	bolt "go.etcd.io/bbolt"
)

func init() {
	viper.AutomaticEnv()
	viper.AllowEmptyEnv(false)

	switch viper.GetString("LOG_FORMAT") {
	case "json", "JSON":
		log.SetFormatter(&log.JSONFormatter{})
	default:
		log.SetFormatter(&log.TextFormatter{})
	}

	switch viper.GetString("LOG_LEVEL") {
	case "trace", "Trace", "TRACE":
		log.SetLevel(log.TraceLevel)
	case "debug", "Debug", "DEBUG":
		log.SetLevel(log.DebugLevel)
	case "info", "Info", "INFO":
		log.SetLevel(log.InfoLevel)
	case "warn", "Warn", "WARN":
		log.SetLevel(log.WarnLevel)
	case "error", "Error", "ERROR":
		log.SetLevel(log.ErrorLevel)
	case "fatal", "Fatal", "FATAL":
		log.SetLevel(log.FatalLevel)
	case "panic", "Panic", "PANIC":
		log.SetLevel(log.PanicLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	viper.SetEnvPrefix("DNS") // Avoid collisions with others env variables
}

var RecordBucket = []byte("records")

func main() {
	config := getConfiguration()

	instanceID := setupInstanceID(config.InstanceId)

	raven.SetDSN(config.sentryDSN)

	db := setupRecordsDatabase(config.PathDB)

	defer db.Close()

	setupLocalRecords(db, config.LocalRecords, config.Dns.Zones)

	agent := setupMetricAgent(config.Agent, config.Statsd, instanceID)

	metricsService := a.NewMetricsService(agent.Input, config.Agent.FlushInterval)

	err := setupConsumer(db, config.Consumer, &metricsService)

	if err != nil {
		log.Error(err)
		return
	}

	setupDNSserveDNSr(db, config.Dns, &metricsService)

	setupHTTPAdministratorserveDNSr(db, config.Administrator)

	// Setup OS signal to stop this service
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	s := <-sig

	log.WithFields(log.Fields{"signal": s}).Info("Signal received, stopping")
}

func setupRecordsDatabase(path string) (db *bolt.DB) {
	db, err := bolt.Open(path, 0600, nil)

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(RecordBucket))

		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		log.Panic(err.Error())
	}

	return db
}

func setupInstanceID(id string) string {
	if id == "" {
		id = uuid.New().String()
		log.Warn("The configuration DNS_INSTANCE_ID was not set, we generate one by default.")
	}

	log.WithField("id", id).Info("Starting stream-dns")
	return id
}

func getConfiguration() Config {
	return Config{
		ConsumerConfig{
			Kafka: KafkaConfig{
				Address:    viper.GetStringSlice("kafka_address"),
				Topics:     viper.GetStringSlice("kafka_topics"),
				SaslEnable: viper.GetBool("kafka_sasl_enable"),
				TlsEnable:  viper.GetBool("kafka_tls_enable"),
				User:       viper.GetString("kafka_user"),
				Password:   viper.GetString("kafka_password"),
				Mechanism:  viper.GetString("kafka_sasl_mechanism"),
			},
			Pulsar: PulsarConfig{
				Address:          viper.GetString("pulsar_address"),
				Topic:            viper.GetString("pulsar_topic"),
				JWT:              viper.GetString("pulsar_jwt"),
				SubscriptionName: viper.GetString("pulsar_subscription_name"),
			},
			DisallowCnameOnAPEX: viper.GetBool("disallow_cname_on_apex"),
		},
		DnsConfig{
			viper.GetString("address"),
			viper.GetBool("udp"),
			viper.GetBool("tcp"),
			viper.GetStringSlice("zones"),
		},
		AgentConfig{
			viper.GetInt("metrics_buffer_size"),
			viper.GetDuration("metrics_flush_interval") * time.Millisecond,
		},
		StatsdConfig{
			viper.GetString("statsd_address"),
			viper.GetString("statsd_prefix"),
		},
		AdministratorConfig{
			viper.GetString("admin_username"),
			viper.GetString("admin_password"),
			viper.GetString("admin_address"),
			viper.GetString("admin_jwtsecret"),
		},
		viper.GetString("pathdb"),
		viper.GetString("sentry_dsn"),
		viper.GetString("instance_id"),
		viper.GetString("local_records"),
	}
}

func setupLocalRecords(db *bolt.DB, rawLocalRecords string, zones []string) {
	if rawLocalRecords != "" {
		localRecords, err := localARecordsRawIntoRecords(rawLocalRecords, zones)

		if err == nil {
			err = registerLocalRecords(db, localRecords)

			if err != nil {
				log.Error(err)
			} else {
				log.Info("Local records from configuration has been saved")
			}
		} else {
			log.Error("Local records", err)
			os.Exit(1)
		}
	}
}

func setupMetricAgent(cfg AgentConfig, statsdCfg StatsdConfig, instanceID string) (agent a.Agent) {
	agent = a.NewAgent(a.Config{cfg.BufferSize, cfg.FlushInterval})
	agent.AddOutput(output.StdoutOutput{})

	// Setup Statsd is config exist
	if statsdCfg.Address != "" {
		statsdOutput := output.NewStatsdOutput(statsdCfg.Address, statsdCfg.Prefix, "id", instanceID)
		agent.AddOutput(statsdOutput)
	}

	go agent.Run()

	return
}

func setupConsumer(db *bolt.DB, cfg ConsumerConfig, metricsService *a.MetricsService) error {
	var typeConsumer string

	if len(cfg.Kafka.Address) != 0 {
		typeConsumer = KafkaType
	}
	if cfg.Pulsar.Address != "" {
		typeConsumer = PulsarType
	}

	if typeConsumer == "" {
		return fmt.Errorf("dan't find the type of consumer, please verify your configuration")
	}

	consumer, err := NewConsumer(typeConsumer, cfg, CommonConsumer{db, metricsService, cfg.DisallowCnameOnAPEX})

	if err != nil {
		log.Panic(err)
		raven.CaptureError(err, nil)
	}

	go consumer.Run()

	return nil
}

func setupDNSserveDNSr(db *bolt.DB, cfg DnsConfig, metricsService *a.MetricsService) {
	go serveDNS(db, cfg, metricsService)
}

func serveDNS(db *bolt.DB, config DnsConfig, metricsService *a.MetricsService) {
	handler := NewQuestionResolverHandler(db, config, metricsService)

	if config.Udp {
		serverudp := &dns.Server{Addr: config.Address, Net: "udp", TsigSecret: nil}
		serverudp.Handler = &handler
		go serverudp.ListenAndServe()
		log.WithField("address", config.Address).Info("UDP serveDNS listening")
	}

	if config.Tcp {
		servertcp := &dns.Server{Addr: config.Address, Net: "tcp", TsigSecret: nil}
		servertcp.Handler = &handler
		go servertcp.ListenAndServe()
		log.WithField("address", config.Address).Info("TCP serveDNS listening")
	}
}

func setupHTTPAdministratorserveDNSr(db *bolt.DB, cfg AdministratorConfig) {
	httpAdministrator := NewHttpAdministrator(db, cfg)
	go httpAdministrator.StartHttpAdministrator()
}

// FIXME: For now, the local-records can only accept A Rtype DNS record
// we can improve this to accept any type of Rtype and improve the parsing.
func registerLocalRecords(db *bolt.DB, records []dns.RR) error {
	tmp := make(map[string][]dns.RR)

	for _, rr := range records {
		tmp[fmt.Sprintf("%s|%s", dns.Fqdn(rr.Header().Name), dns.TypeToString[rr.Header().Rrtype])] = []dns.RR{rr}
	}

	for key, r := range tmp {
		recordRaw, err := json.Marshal(r)

		if err != nil {
			return err
		}

		keyRaw := []byte(key)

		err = db.Update(func(tx *bolt.Tx) error {
			b, _ := tx.CreateBucketIfNotExists(RecordBucket)

			err := b.Put(keyRaw, recordRaw)

			if err != nil {
				return err
			}

			return nil
		})

		log.WithField("rr", r[0].Header().Name).Info("saved local record")

		if err != nil {
			return err
		}
	}

	return nil
}

// The local records are read through the env variable as a string
// This method take a string and transform it into a slice of Record
// Example: DNS_LOCAL_RECORDS="foo.internal. 3600 IN A 1.2.3.4 \n tar.internal. 3600 IN A 1.2.3.5"
func localARecordsRawIntoRecords(rawRecords string, zones []string) ([]dns.RR, error) {
	rrs := []dns.RR{}

	if rawRecords != "" {
		rawRecordsSlice := strings.Split(rawRecords, "\n")

		for _, rrRaw := range rawRecordsSlice {
			rr, err := dns.NewRR(strings.TrimSpace(rrRaw))

			if err != nil {
				return nil, err
			}

			log.WithField("rr", rr.String()).Info("loaded a local record")

			if utils.IsALocalRR(rr.Header().Name, zones) {
				rrs = append(rrs, rr)
			} else {
				log.WithField("rr", rr.Header().Name).Warn("can't load the record for a non-managed zones")
			}
		}
	}

	return rrs, nil
}
