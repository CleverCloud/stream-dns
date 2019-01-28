package output

import "kafka-dns/metrics"

type Output interface {
	Name() string
	
	Connect() error

	Write(metrics []metrics.Metric)
}
