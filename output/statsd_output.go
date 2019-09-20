// Output that follow the https://github.com/etsy/statsd spec.
package output

import (
	ms "stream-dns/metrics"

	"github.com/labstack/gommon/log"
	statsd "gopkg.in/alexcesaro/statsd.v2"
)

type StatsdOutput struct {
	Address string
	Prefix  string // prefix is the statsd client prefix. Can be "" if no prefix is desired.
	Client  *statsd.Client
	Tags    []string
}

func NewStatsdOutput(address string, prefix string, tags ...string) *StatsdOutput {
	return &StatsdOutput{
		Address: address,
		Prefix:  prefix,
		Client:  nil,
		Tags:    tags,
	}
}

func (a *StatsdOutput) Name() string {
	return "statsd"
}

func (a *StatsdOutput) Connect() error {
	c, err := statsd.New(
		statsd.Address(a.Address),
		statsd.TagsFormat(statsd.InfluxDB),
		statsd.Tags(a.Tags...),
	)

	if err != nil {
		return err
	}

	a.Client = c

	return nil
}

// We ignore the Tags because their are not supported by statsd protocol
func (a *StatsdOutput) Write(metrics []ms.Metric) {
	for _, m := range metrics {
		switch m.Type() {
		case ms.Counter:
			a.Client.Count(m.Name(), int64(m.Value().(int)))
		case ms.Gauge:
			a.Client.Gauge(m.Name(), int64(m.Value().(int)))
		default:
			log.Warn("Unsupported metrics type by statsd: ", m.Type())
		}
	}
}
