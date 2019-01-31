package agent

import (
	"testing"
	"time"

	ms "kafka-dns/metrics"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type MockOutput struct {
	c chan int // send the number of metrics received to the main thread
}

func (o MockOutput) Name() string { return "mock" }

func (o MockOutput) Connect() error {
	return nil
}

func (o MockOutput) Write(metrics []ms.Metric) {
	o.c <- len(metrics)
}

func TestAgentFlushTheMetricsWhenBufferIsFilled(t *testing.T) {
	// got
	c := make(chan int)

	agent := NewAgent(Config{3, 100})
	agent.AddOutput(MockOutput{c})

	go agent.Run()

	// do
	agent.Input <- ms.NewMetric("bar", nil, nil, time.Now(), ms.Counter)
	agent.Input <- ms.NewMetric("foo", nil, nil, time.Now(), ms.Counter)
	agent.Input <- ms.NewMetric("rab", nil, nil, time.Now(), ms.Counter)

	// want
	select {
	case len := <-c:
		assert.Equal(t, 3, len)
	case <-time.After(500 * time.Millisecond):
		log.Fatal("[TIMEOUT] the mock output never forwarded the len metrics")
		t.Fail()
	}
}

func TestAgentFlushIncompleteBufferWhenHeGotATimeout(t *testing.T) {
	// got
	c := make(chan int)

	agent := NewAgent(Config{3, 100})
	agent.AddOutput(MockOutput{c})

	go agent.Run()

	// do
	agent.Input <- ms.NewMetric("bar", nil, nil, time.Now(), ms.Counter)
	agent.Input <- ms.NewMetric("foo", nil, nil, time.Now(), ms.Counter)
	// NOTE: We only send 2 metrics here and we defined the buffer size to 3.
	// The agent should timeout and send an incomplete buffer

	select {
	case len := <-c:
		// want
		assert.Equal(t, 2, len)
	case <-time.After(500 * time.Millisecond):
		log.Fatal("[TIMEOUT] the mock output never forwarded the len metrics")
		t.Fail()
	}
}

func TestAgentFlushTheMetricsWhenBufferIsFilled(t *testing.T) {
	// got
	c := make(chan int)

	agent := NewAgent(Config{3, 100})
	agent.AddOutput(MockOutput{c})

	go agent.Run()

	// do
	agent.Input <- ms.NewMetric("bar", nil, nil, time.Now(), ms.Counter)
	agent.Input <- ms.NewMetric("foo", nil, nil, time.Now(), ms.Counter)
	agent.Input <- ms.NewMetric("rab", nil, nil, time.Now(), ms.Counter)

	// NOTE: We send more metrics than the size of the buffer
	agent.Input <- ms.NewMetric("more", nil, nil, time.Now(), ms.Counter)
	agent.Input <- ms.NewMetric("more", nil, nil, time.Now(), ms.Counter)

	// want
	select {
	case len := <-c:
		assert.Equal(t, 3, len)
	case <-time.After(500 * time.Millisecond):
		log.Fatal("[TIMEOUT] the mock output never forwarded the len metrics")
		t.Fail()
	}
}
