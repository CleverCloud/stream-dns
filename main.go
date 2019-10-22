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
	dns "github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	bolt "go.etcd.io/bbolt"
)

func serve(db *bolt.DB, config DnsConfig, metricsService *a.MetricsService) {
	registerHandlerForResolver(".", db, config.ResolverAddress, metricsService)
	registerHandlerForZones(config.Zones, db, metricsService)

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
	config := Config{
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

	if config.InstanceId == "" {
		config.InstanceId = uuid.New().String()
		log.Warn("The configuration DNS_INSTANCE_ID was not set, we generate one by default.")
	}

	log.WithField("id", config.InstanceId).Info("Starting stream-dns")

	// Sentry
	raven.SetDSN(config.sentryDSN)

	// Setup os signal to stop this service
	sig := make(chan os.Signal)

	db, err := bolt.Open(config.PathDB, 0600, nil)
	if err != nil {
		raven.CaptureError(err, map[string]string{"step": "init"})
		log.Fatal("database ", config.PathDB, err.Error(), "\nSet the environment variable: DNS_PATHDB")
	}

	// Local Records
	localRecords, err := localARecordsRawIntoRecords(config.LocalRecords)
	if err == nil {
		registerLocalRecords(db, localRecords)
		log.Info("Local records from configuration has been saved")
	} else {
		log.Error("Local records", err)
		os.Exit(1)
	}

	// Metrics
	agent := a.NewAgent(a.Config{config.Agent.BufferSize, config.Agent.FlushInterval})

	// Outputs metrics

	// Setup Statsd is config exist
	if config.Statsd.Address != "" {
		statsdOutput := output.NewStatsdOutput(config.Statsd.Address, config.Statsd.Prefix, "id", config.InstanceId)
		agent.AddOutput(statsdOutput)
	}

	agent.AddOutput(output.StdoutOutput{})

	go agent.Run()

	// Run goroutines service
	kafkaConsumer, err := NewKafkaConsumer(config.Kafka)

	if err != nil {
		log.Panic(err)
		raven.CaptureError(err, nil)
	}

	metricsService := a.NewMetricsService(agent.Input, config.Agent.FlushInterval)

	go kafkaConsumer.Run(db, &metricsService, config.DisallowCNAMEonAPEX)

	go serve(db, config.Dns, &metricsService)

	// Run HTTP Administrator
	if config.Administrator.Address != "" {
		httpAdministrator := NewHttpAdministrator(db, config.Administrator)
		go httpAdministrator.StartHttpAdministrator()
	}

	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig

	db.Close()
	log.WithFields(log.Fields{"signal": s}).Info("Signal received, stopping")
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

	return records, nil
}
