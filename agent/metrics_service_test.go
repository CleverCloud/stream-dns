package agent

import (
	"stream-dns/metrics"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const aggregatorName = "test"

func TestGetOrCreateAggregateWhenItDoesntExist(t *testing.T) {
	//got
	metricsService := NewMetricsService(nil, 100*time.Millisecond)

	//do
	aggregator := metricsService.GetOrCreateAggregator(aggregatorName, metrics.Counter)

	//want
	assert.NotNil(t, aggregator)
}

func TestGetOrCreateAggregateWhenItAlreadyExist(t *testing.T) {
	//got
	metricsService := NewMetricsService(nil, 100*time.Millisecond)

	//do
	aggregator := metricsService.GetOrCreateAggregator(aggregatorName, metrics.Counter)
	aggregator2 := metricsService.GetOrCreateAggregator(aggregatorName, metrics.Counter)
	aggregator3 := metricsService.Get(aggregatorName)

	//want
	assert.NotNil(t, aggregator)
	//FIXME: Improve the equal method
	assert.Equal(t, aggregator, aggregator2, aggregator3)
}

func TestGetAggregateWhenItDoesntExist(t *testing.T) {
	//got
	metricsService := NewMetricsService(nil, 100*time.Millisecond)

	//do
	aggregator := metricsService.Get(aggregatorName)

	//want
	assert.Nil(t, aggregator)
}

func TestGetAggregateWhenItAlreadyExist(t *testing.T) {
	//got
	metricsService := NewMetricsService(nil, 100*time.Millisecond)

	//do
	metricsService.GetOrCreateAggregator(aggregatorName, metrics.Counter)
	aggregator := metricsService.Get(aggregatorName)

	//want
	assert.NotNil(t, aggregator)
}
