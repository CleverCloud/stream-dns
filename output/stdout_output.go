// Prefer to use this Ouput for development only
package output

import (
	"fmt"
	ms "stream-dns/metrics"

	log "github.com/sirupsen/logrus"
)

type StdoutOutput struct{}

func (a StdoutOutput) Name() string {
	return "stdout"
}

func (a StdoutOutput) Connect() error {
	return nil
}

func (a StdoutOutput) Write(metrics []ms.Metric) {
	log.WithField("output", "stdout").Infof("Last metrics since the last flush: %d", len(metrics))

	for _, m := range metrics {
		fmt.Printf("%s\n", m.ToString())
	}

	metrics = nil

	fmt.Print("\n\n")
}
