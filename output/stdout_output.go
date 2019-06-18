// Prefer to use this Ouput for development only
package output

import (
	"fmt"
	ms "stream-dns/metrics"
)

type StdoutOutput struct{}

func (a StdoutOutput) Name() string {
	return "stdout"
}

func (a StdoutOutput) Connect() error {
	return nil
}

func (a StdoutOutput) Write(metrics []ms.Metric) {
	fmt.Print("Last metrics since the last flush:\n")

	for _, m := range metrics {
		fmt.Printf("%s\n", m.ToString())
	}

	fmt.Print("\n\n")
}
