// Output that follow the https://github.com/etsy/statsd spec.
// 
package output

import (
	"github.com/cactus/go-statsd-client/statsd"
	log "github.com/sirupsen/logrus"
	ms "stream-dns/metrics"
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
		Client: nil,
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
			if len(m.Fields()) > 0 {
				// Increment the counter for each fields
				// We use the format: name.field
				for key, val := range m.Fields() {
					if v, ok := val.(int64); ok {
						a.Client.Inc(m.Name()+"."+key, v, 1.0)
					} else {
						log.Error("the metric: ", m.Name(), "has a field: ", key, " which doesn't have the type int64")
					}
				}
			} else {
				//log.Print(a.client)
				a.Client.Inc(m.Name(), 1, 1.0)
			}
		case ms.Gauge:
			for key, val := range m.Fields() {
				if v, ok := val.(int64); ok {
					a.Client.Gauge(m.Name()+"."+key, v, 1.0)
				} else {
					log.Error("the metric: ", m.Name(), "has a field: ", key, " which doesn't have the type int64")
				}
			}
		case ms.Message:
			log.Warn("Statsd doesn't support the message metric type")
		default:
			log.Warn("Unsupported metrics type by statsd: ", m.Type())
		}
	}
}
