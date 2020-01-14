**Last modification**: 13/01/2020 by Coltellacci Alessio <alessio.coltellacci@clever-cloud.com>

# Monitoring Stream-DNS

 The best way to ensure proper Stream-DNS performance and operation is by monitoring its key metrics in three broad areas:

* **DNS engine metrics** such as client connections and requests.
* **Consumer metrics** such as number of record got from your event source (ex: Kafka).
* **Resolver metrics** such as  number of queries to non-authoritative zone.

Correlating this metrics can gives you a more comprehensive view of your infrastructure and helps you quickly identify potential hotspots. Because you have to remember that DNS is key component in a network communication, as is the first thing to do to know where a node is located.

## DNS engine metrics

| Name | Description | Metric Type |
| ---- | ----------- | ----------- |
| TODO | TODO        | TODO        |

## Consumer metrics

| Name | Description | Metric Type |
| ---- | ----------- | ----------- |
| TODO | TODO        | TODO        |

## Resolver metrics

| Name | Description | Metric Type |
| ---- | ----------- | ----------- |
| TODO | TODO        | TODO        |

## Grafana

This repository provide a dashboard template. You can use it for a quick start or make your own.To import a dashboard open dashboard search and then hit the import button:

![import button](https://grafana.com/static/img/docs/v50/import_step1.png)

From here you can upload a dashboard `JSON` file

. Paste dashboard `JSON` text directly into the text area.

![](https://grafana.com/static/img/docs/v50/import_step2.png)

In step 2 of the import process Grafana will let you change the name of the dashboard, pick what data source you want the dashboard to use and specify any metric prefixes (if the dashboard use any).

## Output

Output write metrics to a location. Outputs commonly write to databases, network services, and messaging systems.

### Configuring outputs

To configure outputs you have to set environment variables related to them.
If you want to setup a `statsd` output, you have to set the environment variables: `DNS_STATSD_ADDRESS` and `DNS_STATSD_PREFIX`.

NOTE: for know, `statsd` is the only output available. 

### Adding output

This section is for developers who want to create a new output sink. Their interface has similar constructs.

#### Guidelines

* An output must conform to the [Output](https://github.com/CleverCloud/stream-dns/blob/master/output/output.go) interface.
* Add your output in the setup of the metric agent [here](https://github.com/CleverCloud/stream-dns/blob/0436727031a95ed4195495ff2cf66afbe2dc92ca/main.go#L176) to register themselves. See below for a quick example.

#### Output example

```go
package output

import (
	ms "stream-dns/metrics"
)

type MyOutput struct{}

func (o MyOutput) Name() string {
	return "an example of an output"
}

func (o MyOutput) Connect() error {
    // Make a connection to the URL here
	return nil
}

func (o MyOutput) Write(metrics []ms.Metric) {
	for _, m := range metrics {
        // write `metric` to the output sink here
	}
    
    metrics = nil
}
```