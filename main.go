package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	a "stream-dns/agent"
	"stream-dns/output"
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

func main() {
	config := getConfiguration()

	instanceID := setupInstanceID(config.InstanceId)

	raven.SetDSN(config.sentryDSN)

	db := setupRecordsDatabase(config.PathDB)
	setupLocalRecords(db, config.LocalRecords)

	agent := setupMetricAgent(config.Agent, config.Statsd, instanceID)

	metricsService := a.NewMetricsService(agent.Input, config.Agent.FlushInterval)

	setupKafkaConsumer(db, config.Kafka, &metricsService, config.DisallowCNAMEonAPEX)

	setupDNSserveDNSr(db, config.Dns, &metricsService)

	setupHTTPAdministratorserveDNSr(db, config.Administrator)

	// Setup OS signal to stop this service
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	s := <-sig

	db.Close()
	log.WithFields(log.Fields{"signal": s}).Info("Signal received, stopping")
}

func setupRecordsDatabase(path string) (db *bolt.DB) {
	db, err := bolt.Open(path, 0600, nil)

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("records"))

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
		KafkaConfig{
			Address:    viper.GetStringSlice("kafka_address"),
			Topics:     viper.GetStringSlice("kafka_topics"),
			SaslEnable: viper.GetBool("kafka_sasl_enable"),
			TlsEnable:  viper.GetBool("kafka_tls_enable"),
			User:       viper.GetString("kafka_user"),
			Password:   viper.GetString("kafka_password"),
			Mechanism:  viper.GetString("kafka_sasl_mechanism"),
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
		viper.GetBool("disallow_cname_on_apex"),
		viper.GetString("instance_id"),
		viper.GetString("local_records"),
	}
}

func setupLocalRecords(db *bolt.DB, rawLocalRecords string) {
	if rawLocalRecords != "" {
		localRecords, err := localARecordsRawIntoRecords(rawLocalRecords)

		if err == nil {
			registerLocalRecords(db, localRecords)
			log.Info("Local records from configuration has been saved")
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

func setupKafkaConsumer(db *bolt.DB, cfg KafkaConfig, metricsService *a.MetricsService, disallowCnameOnAPEX bool) {
	kafkaConsumer, err := NewKafkaConsumer(cfg)

	if err != nil {
		log.Panic(err)
		raven.CaptureError(err, nil)
	}

	go kafkaConsumer.Run(db, metricsService, disallowCnameOnAPEX)
}

func setupDNSserveDNSr(db *bolt.DB, cfg DnsConfig, metricsService *a.MetricsService) {
	go serveDNS(db, cfg, metricsService)
}

func serveDNS(db *bolt.DB, config DnsConfig, metricsService *a.MetricsService) {
	registerHandlerForResolver(".", db, config.ResolverAddress, metricsService)
	registerHandlerForZones(config.Zones, db, metricsService)

	if config.Udp {
		serverudp := &dns.Server{Addr: config.Address, Net: "udp", TsigSecret: nil}
		go serverudp.ListenAndServe()
		log.Info("UDP serveDNSr listening on: ", config.Address)
	}

	if config.Tcp {
		servertcp := &dns.Server{Addr: config.Address, Net: "tcp", TsigSecret: nil}
		go servertcp.ListenAndServe()
		log.Info("TCP serveDNSr listening on: ", config.Address)
	}
}

func setupHTTPAdministratorserveDNSr(db *bolt.DB, cfg AdministratorConfig) {
	httpAdministrator := NewHttpAdministrator(db, cfg)
	go httpAdministrator.StartHttpAdministrator()
}

// FIXME: For now, the local-records can only accept A Rtype DNS record
// we can improve this to accept any type of Rtype and improve the parsing.
func registerLocalRecords(db *bolt.DB, records []Record) error {
	tmp := make(map[string][]Record)

	for _, r := range records {
		tmp[fmt.Sprintf("%s|A", r.Name)] = []Record{r}
	}

	for key, r := range tmp {
		recordRaw, err := json.Marshal(r)

		if err != nil {
			return err
		}

		keyRaw := []byte(key)

		err = db.Update(func(tx *bolt.Tx) error {
			b, _ := tx.CreateBucketIfNotExists([]byte("records"))

			err := b.Put(keyRaw, recordRaw)

			if err != nil {
				return err
			}

			return nil
		})

		return err
	}

	return nil
}

// The local records are read through the env variable as a string
// This method take a string and transform it into a slice of Record
// FIXME: For now, the local-records can only accept A Rtype DNS record
// we can improve this to accept any type of Rtype and improve the parsing.
func localARecordsRawIntoRecords(rawRecords string) ([]Record, error) {
	records := []Record{}

	if rawRecords != "" {
		rawRecordsSlice := strings.Split(rawRecords, "\n")

		for _, r := range rawRecordsSlice {
			var name string
			var ttl int
			var content string
			_, err := fmt.Sscanf(r, "%s %d IN A %s", &name, &ttl, &content)

			if err != nil {
				return nil, fmt.Errorf("Malformated A local record, expected: \"[NAME]. [TTL] IN A [CONTENT]\" \tgot the error: %s", err)
			}

			record := Record{
				Name:     name,
				Type:     "A",
				Content:  content,
				Ttl:      ttl,
				Priority: 0,
			}

			records = append(records, record)
		}

	}

	return records, nil
}
