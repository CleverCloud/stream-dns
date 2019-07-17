// Output that follow the https://github.com/etsy/statsd spec.
package output

import (
	ms "stream-dns/metrics"

	"github.com/cactus/go-statsd-client/statsd"
	log "github.com/sirupsen/logrus"
)

type StatsdOutput struct {
	Address string
	Prefix  string // prefix is the statsd client prefix. Can be "" if no prefix is desired.
	Client  statsd.Statter
}

func NewStatsdOutput(address string, prefix string) *StatsdOutput {
	return &StatsdOutput{
		Address: address,
		Prefix:  prefix,
		Client:  nil,
	}
}

func (a *StatsdOutput) Name() string {
	return "statsd"
}

func (a *StatsdOutput) Connect() error {
	client, err := statsd.NewClient(a.Address, a.Prefix)

	if err != nil {
		return err
	}

	a.Client = client
	return nil
}

// We ignore the Tags because their are not supported by statsd protocol
func (a *StatsdOutput) Write(metrics []ms.Metric) {
	for _, m := range metrics {
		switch m.Type() {
		case ms.Counter:
			a.Client.Inc(m.Name(), m.Value().(int64), 1.0)
		case ms.Gauge:
			a.Client.Gauge(m.Name(), m.Value().(int64), 1.0)
		default:
			log.Warn("Unsupported metrics type by statsd: ", m.Type())
		}
	}
}
