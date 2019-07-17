package agent

import (
	"stream-dns/metrics"
	"time"
)

type MetricsService struct {
	InputAgent    chan metrics.Metric
	aggregators   map[string]Aggregator
	flushInterval time.Duration
}

func NewMetricsService(inputAgent chan metrics.Metric, flushInterval time.Duration) MetricsService {
	return MetricsService{
		InputAgent:    inputAgent,
		aggregators:   map[string]Aggregator{},
		flushInterval: flushInterval,
	}
}

func (m MetricsService) GetOrCreateAggregator(metricName string, valueType metrics.ValueType, reset bool) Aggregator {
	if m.aggregators[metricName] == nil {
		switch valueType {
		case metrics.Counter:
			m.aggregators[metricName] = NewAggregatorCounter(m.InputAgent, metricName, reset)
		case metrics.Gauge:
			m.aggregators[metricName] = NewAggregatorGauge(m.InputAgent, metricName, reset)
		}

		go m.aggregators[metricName].Run(m.flushInterval)
	}

	return m.aggregators[metricName]
}

// Use this method only after a GetOrCreateAggregator call in the same block to avoid a nil pointer exceptions.
func (m MetricsService) Get(metricName string) Aggregator {
	return m.aggregators[metricName]
}
