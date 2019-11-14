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

	"github.com/getsentry/raven-go"
	log "github.com/sirupsen/logrus"
)

type Config struct {
	BufferSize    int
	FlushInterval time.Duration
}

// Agent runs a set of plugins.
type Agent struct {
	Config  Config
	Input   chan metrics.Metric
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
	err := a.connectOutputs()

	if err != nil {
		raven.CaptureError(err, map[string]string{"unit": "agent-metrics"})
		log.Fatal(err)
		return err
	}

	var buf []metrics.Metric
	flushTimerEvent := time.NewTimer(a.Config.FlushInterval)

	for {
		select {
		case ret := <-a.Input:
			buf = append(buf, ret)
			nbMetricsInBuf := len(buf)
			log.Debugf("buffer fullness: %d / %d metrics", nbMetricsInBuf, a.Config.BufferSize)

			if nbMetricsInBuf >= a.Config.BufferSize {
				a.flushMetricsToOutput(buf)
				buf = buf[:0] // Clear just the slice and keep his capacity
			}
		case <-flushTimerEvent.C:
			if len(buf) > 0 {
				a.flushMetricsToOutput(buf)
				buf = buf[:0]
			}
			flushTimerEvent.Reset(a.Config.FlushInterval)
		}
	}
}

func (a *Agent) AddOutput(output output.Output) {
	a.outputs = append(a.outputs, output)
}

// Connects to all outputs
func (a *Agent) connectOutputs() error {
	for _, output := range a.outputs {
		err := output.Connect() //TODO We should try to reconnect at least a second time

		if err != nil {
			raven.CaptureError(err, map[string]string{"unit": "agent-metrics"})
			log.Fatal("[agent] Failed to connect to output: ", output.Name(), "error was: ", err)
		}

		log.WithField("output", output.Name()).Info("metric agent successfully connected to the output")
	}

	return nil
}

// Create a deepcopy of the metrics slice and send it to outputs
func (a *Agent) flushMetricsToOutput(metricsBuffer []metrics.Metric) {
	log.WithField("length", len(metricsBuffer)).Infof("wrote a batch of metrics")
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
