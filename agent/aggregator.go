package agent

import (
	"stream-dns/metrics"
	"time"
)

//TODO: manage labels in metrics
type Aggregator interface {
	Run(time.Duration) error
	GetInput() chan interface{}
}

type AggregatorGauge struct {
	input      chan interface{}
	agentInput chan metrics.Metric
	metricName string
	gauge      float64
	reset      bool
}

func NewAggregatorGauge(agentInput chan metrics.Metric, metricName string, reset bool) AggregatorGauge {
	return AggregatorGauge{
		agentInput: agentInput,
		input:      make(chan interface{}),
		metricName: metricName,
		gauge:      0.0,
		reset:      reset,
	}
}

// Thread safe method
func (a AggregatorGauge) Update(val float64) {
	a.input <- val
}

func (a AggregatorGauge) Run(flushInterval time.Duration) error {
	for {
		select {
		case val := <-a.input:
			a.gauge = val.(float64)
		case <-time.After(flushInterval):
			a.agentInput <- metrics.NewMetric(a.metricName, nil, time.Now(), metrics.Gauge, a.gauge)
			if a.reset {
				a.ResetVal()
			}
		}
	}
}

func (a AggregatorGauge) GetInput() chan interface{} {
	return a.input
}

func (a *AggregatorGauge) ResetVal() {
	a.gauge = 0.0
}

type AggregatorCounter struct {
	input      chan interface{}
	agentInput chan metrics.Metric
	metricName string
	counter    int
	reset      bool
}

func NewAggregatorCounter(agentInput chan metrics.Metric, metricName string, reset bool) AggregatorCounter {
	return AggregatorCounter{
		agentInput: agentInput,
		input:      make(chan interface{}),
		metricName: metricName,
		counter:    0,
		reset:      reset,
	}
}

// Thread safe method
func (a AggregatorCounter) Inc(val int) {
	a.input <- val
}

func (a AggregatorCounter) Dec(val int) {
	a.input <- val
}

func (a AggregatorCounter) Run(flushInterval time.Duration) error {
	flushTimerEvent := time.NewTimer(flushInterval)

	for {
		select {
		case val := <-a.input:
			a.counter += val.(int)
		case <-flushTimerEvent.C:
			a.agentInput <- metrics.NewMetric(a.metricName, nil, time.Now(), metrics.Counter, a.counter)
			if a.reset {
				a.ResetVal()
			}
			flushTimerEvent.Reset(flushInterval)
		}
	}
}

func (a *AggregatorCounter) ResetVal() {
	a.counter = 0
}

func (a AggregatorCounter) GetInput() chan interface{} {
	return a.input
}
