package output

import (
	w "github.com/miton18/go-warp10/base"
	ms "kafka-dns/metrics"
	//log "github.com/sirupsen/logrus"
)

type Warp10Output struct{
	client *w.Client
}

func (a Warp10Output) Name() string {
	return "warp10"
}

func (a Warp10Output) Connect() error {
	a.client = w.NewClient("https://warp10.gra1.metrics.ovh.net")
	a.client.WriteToken = "WRITE_TOKEN"
	
	return nil
}

func (a Warp10Output) Write(metrics []ms.Metric) {
	for _, m := range metrics {
		tp := m.Type()

		if tp == ms.Counter {

		}
	}
}
