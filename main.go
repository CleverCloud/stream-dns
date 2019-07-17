package main

import (
	"os"
	"os/signal"
	a "stream-dns/agent"
	"stream-dns/output"
	"syscall"
	"time"

	"github.com/getsentry/raven-go"
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

func main() {
	viper.SetEnvPrefix("DNS") // Avoid collisions with others env variables
	viper.AutomaticEnv()
	viper.AllowEmptyEnv(false)

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
			viper.GetDuration("metrics_flush_interval"),
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
	}

	// Sentry
	raven.SetDSN(config.sentryDSN)

	// Setup os signal to stop this service
	sig := make(chan os.Signal)

	db, err := bolt.Open(config.PathDB, 0600, nil)
	if err != nil {
		raven.CaptureError(err, map[string]string{"step": "init"})
		log.Fatal("database ", config.PathDB, err.Error(), "\nSet the environment variable: DNS_PATHDB")
	}

	// Metrics
	agent := a.NewAgent(a.Config{config.Agent.BufferSize, config.Agent.FlushInterval})

	// Outputs metrics

	// Setup Statsd is config exist
	if config.Statsd.Address != "" {
		statsdOutput := output.NewStatsdOutput(config.Statsd.Address, config.Statsd.Prefix)
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

	metricsService := a.NewMetricsService(agent.Input, config.Agent.FlushInterval*time.Millisecond)

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
