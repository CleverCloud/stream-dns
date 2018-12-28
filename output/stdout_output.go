// Prefer to use this Ouput for development only
package output

import (
	"log"

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
		tp := m.Type()
		log.Print("%s ", m.Name)

		for key, value := range m.Tags() {
			log.Printf("tag: %s = %s ", key, value)
		}

		if tp == ms.Counter || tp == ms.Gauge {
			for key, value := range m.Fields() {
				log.Printf("field: %s = %d ", key, value)
			}
		} else {
			for key, value := range m.Fields() {
				log.Printf("field: %s = %s ", key, value)
			}
		}

		log.Printf("\n")
	}
}
