package agent

import (
	ms "stream-dns/metrics"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestAggregationOfCounterByAnAggregatorCounter(t *testing.T) {
	//got

	metricName := "test"
	c := make(chan ms.Metric)

	aggregateCounter := NewAggregatorCounter(c, metricName, true)

	go aggregateCounter.Run(100 * time.Millisecond)

	//do
	aggregateCounter.Inc(4)
	aggregateCounter.Inc(2)
	aggregateCounter.Inc(1)

	//want
	select {
	case countMetric := <-c:
		assert.Equal(t, 7, countMetric.Value().(int))
	case <-time.After(500 * time.Millisecond):
		log.Fatal("[TIMEOUT] the agent mock input never got a metric from the count aggregator")
		t.Fail()
	}
}

func TestAggregationOfGaugeByAnAggregatorGauge(t *testing.T) {
	//got
	metricName := "test"
	c := make(chan ms.Metric)
	lastGaugeValue := 1234.0

	aggregateGauge := NewAggregatorGauge(c, metricName, true)

	go aggregateGauge.Run(100 * time.Millisecond)

	//do
	aggregateGauge.Update(4.0)
	aggregateGauge.Update(2.0)
	aggregateGauge.Update(lastGaugeValue)

	//want
	select {
	case gaugeMetric := <-c:
		assert.Equal(t, lastGaugeValue, gaugeMetric.Value().(float64))
	case <-time.After(500 * time.Millisecond):
		log.Fatal("[TIMEOUT] the agent mock input never got a metric from the gauge aggregator")
		t.Fail()
	}
}
