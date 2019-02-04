// Prefer to use this Ouput for development only
package output

import (
	log "github.com/sirupsen/logrus"
	ms "kafka-dns/metrics"
)

type StdoutOutput struct{}

func (a StdoutOutput) Name() string {
	return "stdout"
}

func (a StdoutOutput) Connect() error {
	return nil
}

func (a StdoutOutput) Write(metrics []ms.Metric) {
	for _, m := range metrics {
		log.WithFields(log.Fields{
			"type": ms.TypeToString[m.Type()],
			"name": m.Name(),
		}).Info("[StdoutOutput] metric")
	}
}
