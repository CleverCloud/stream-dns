package output

import "stream-dns/metrics"

type Output interface {
	Name() string
	
	Connect() error

	Write(metrics []metrics.Metric)
}
