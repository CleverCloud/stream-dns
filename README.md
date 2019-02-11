# Kafka DNS

A DNS server, written in Go.

## Overview

The DNS server collects the records (in a JSON format) from a Kafka node. The kafka acts as a source for him.
The DNS server serve both UDP and TCP connections. If there is a problem with the server, just restart the service and he will recreate his state by re-reading the records in the kafka topic.
The records are registered on the FS by using a An embedded key/value database: [bbolt](https://github.com/boltdb/bolt). The key has (same as kafka) the format: `<domain>.|<qtype>` and the value:
```json
[
	{ "name": "...", "type": "A", "content": "10.234.128.12", "priority": 0 },
	...
]
```

Overview of the DNS server components:

``` mermaid
graph TD
Kafka -.->|dns record/json| D
D(dns serveur) --- A
D --- B(bbolt)
C[client dns] -- query --> D
D(dns serveur) -- answer --> C
A(metrics agent) --> O(outputs)
O -.-> Warp10
```

## Build

The project use Go 1.11 [Modules](https://github.com/golang/go/wiki/Modules) to resolve his dependencies. So make sure you have the env variable GO111MODULE set to `on` or `auto`.
You have two way to build this program:

*Recommanded:* `$ go install kafka-dns` (The binary output will be place in the directory `$GOPATH/bin`)

or

```sh
$ export GO111MODULE=on
$ cd $GOPATH/src/<project path>
$ go build
```

NOTE: Don't forget to setup your [$GOPATH](https://golang.org/doc/code.html#GOPATH) before.

## Run

### Development mode

- Start a single Kafka node (follow this instructions: [kafka.apache.org/quickstart](https://kafka.apache.org/quickstart))
- Run the nodeJS migration script: [clever-cloud/migration-powerdns-pg-to-kafka](https://gitlab.corp.clever-cloud.com/clever-cloud/migration-powerdns-pg-to-kafka)
- (optional) Set the connection URI of the powerdns follower in the PG_CON env variable (optional)
- configure the project by using a `.env` file (more info § configuration)
- run the command `bin/kafka-dns`

### Production mode (WIP)

`systemctl start kafka-dns`

### Test suites

Run the test suites:

```sh
$ cd $GOPATH/src/kafka-dns
$ go test -v ./...
```

NOTE: If you like color stdout output, you can run the tests with: [richgo](https://github.com/kyoh86/richgo)

Measure the code coverage:

```sh
$ cd $GOPATH/src/kafka-dns
$ go test -coverprofile=c.out ./...
```

## Query the server

You can use the `dig` or `host` linux command to query server:

Examples:
```bash
dig @localhost -p 8053 yourdomain.com
dig @localhost -p 8053 axfr zonetransfer.me
```


## Configuration

This services use environment variables for it's configuration.
The following env variables are needed:

| Variable                         | Type           | Description                                                                                     |
|----------------------------------|----------------|-------------------------------------------------------------------------------------------------|
| DNS_ADDRESS                      | string         | Address for the DNS server e.g: ":8053"                                                         |
| DNS_TCP                          | bool           | Accept TCP DNS connection                                                                       |
| DNS_UDP                          | bool           | Accept UDP DNS connection                                                                       |
| DNS_RESOLVER_ADDRESS             | string         | Address use to resolve unsupported zone                                                         |
| DNS_ZONES                        | List of string | List of supported zones e.g: "clvrcld.net. services.clever-cloud.com." (separate by whitespace) |
| DNS_KAFKA_ADDRESS                | string         | Address of one kafka node e.g: "localhost:9092"                                                 |
| DNS_KAFKA_TOPIC                  | string         | Kafka topic of the records                                                                      |
| DNS_AGENT_METRICS_BUFFER_SIZE    | int            | Size of the metrics buffer in bytes                                                             |
| DNS_AGENT_METRICS_FLUSH_INTERVAL | int            | Flushing interval of the metrics                                                                |
| DNS_PATHDB                       | string         | Path of the bbolt database e.g: "/tmp/my.db"                                                    |

## Tools

This project provide some tools to help during the development:

### A DNS client

Client DNS lookup which support multiple questions:

Build: `go install kafka-dns/tools/client`
Usage: `client [[qname], [qtype]]...`
Run: `$GOPATH/bin/client yolo.com A foo.bar AAAA`

NOTE: qtype should be in upper case).

### Kafka record producer

A Kafka producer to register custom record in the Kafka

Build: `go install kafka-dns/tools/producer`
Usage: `producer name type content ttl priority`
Run: `$GOPATH/bin/producer yolo.com A 2.4.4.6 3600 0`
