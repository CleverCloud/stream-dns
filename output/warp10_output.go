/**
NOTE: To use this output, you should set the environment variables: WARP10_WRITE_TOKEN
*/
package output

import (
	"os"

	w "github.com/miton18/go-warp10/base"
	ms "kafka-dns/metrics"
)

type Warp10Output struct {
	client *w.Client
}

func (a Warp10Output) Name() string {
	return "warp10"
}

func (a Warp10Output) Connect() error {
	a.client = w.NewClient("https://warp10.gra1.metrics.ovh.net")
	a.client.WriteToken = os.Getenv("WARP10_WRITE_TOKEN")

	return nil
}

func (a Warp10Output) Write(metrics []ms.Metric) {
	//TODO
}
