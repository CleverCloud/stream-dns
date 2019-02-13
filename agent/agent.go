/*
The Agent metric is an active object who acts as multiplexer for all "input metrics" and
a demultiplexer for all outputs. The metrics are gathers in a buffer and sent in batch mode.
The agent flush an incomplete buffer if he has a timeout.

```
P            Output
  \         /
P -> Agent > -- Output
  /         \
P            Output
```
*/
package agent

import (
	"time"

	"stream-dns/metrics"
	"stream-dns/output"

	log "github.com/sirupsen/logrus"
)

type Config struct {
	BufferSize    int
	FlushInterval time.Duration
}

// Agent runs a set of plugins.
type Agent struct {
	Config Config
	Input  chan metrics.Metric

	outputs []output.Output
}

// NewAgent returns an Agent for the given Config.
func NewAgent(config Config) Agent {
	a := Agent{
		Config: config,
		Input:  make(chan metrics.Metric),
	}

	return a
}

func (a *Agent) Run() error {
	log.Infof("[agent] Config: Flush Interval:%s", a.Config.FlushInterval)

	err := a.connectOutputs()

	if err != nil {
		return err
	}

	var buf []metrics.Metric

	for {
		select {
		case ret := <-a.Input:
			buf = append(buf, ret)

			if len(buf) == a.Config.BufferSize {
				a.flushMetricsToOutput(buf)
				buf = buf[:0] // Clear just the slice and keep his capacity
			}
		case <-time.After(a.Config.FlushInterval * time.Millisecond):
			a.flushMetricsToOutput(buf)
			buf = buf[:0]
		}
	}

	return nil
}

func (a *Agent) AddOutput(output output.Output) {
	a.outputs = append(a.outputs, output)
}

// Connects to all outputs
func (a *Agent) connectOutputs() error {
	for _, output := range a.outputs {
		err := output.Connect() //TODO We should try to reconnect at least a second time

	if err != nil {
			log.Fatal("[agent] Failed to connect to output: ", output.Name(), "error was: ", err)
		}

		log.Info("[agent] Successfully connected to output: ", output.Name())
	}

	return nil
}

// Create a deepcopy of the metrics slice and send it to outputs
func (a *Agent) flushMetricsToOutput(metricsBuffer []metrics.Metric) {
	// Create an immutable slice of this metrics. We need the buff for the next loop
	tmp := make([]metrics.Metric, len(metricsBuffer))
	copy(tmp, metricsBuffer)
	a.sendMetricsToOutput(tmp)
}

// Send the batch of metrics to all registered Output
func (a *Agent) sendMetricsToOutput(metrics []metrics.Metric) {
	for _, output := range a.outputs {
		// We run this in a goroutine to reduce the latency due to IO
		go output.Write(metrics)
	}
}
