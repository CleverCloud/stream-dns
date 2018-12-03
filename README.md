# Kafka DNS

A DNS server, written in Go.

## Build

`go install gitlab.clever-cloud.com/kafka-dns`

## Run

### Dev mode

- Start a single Kafka node (follow this instructions: [kafka.apache.org/quickstart](https://kafka.apache.org/quickstart))
- Run the nodeJS migration script: [clever-cloud/migration-powerdns-pg-to-kafka](https://gitlab.corp.clever-cloud.com/clever-cloud/migration-powerdns-pg-to-kafka)
- (optional) Set the connection URI of the powerdns follower in the PG_CON env variable (optional)
- (optional) Fill the config file
- Run the binary with the command: `PG_CON=<connection URI> kafka-dns`

### Prod mode

TODO

### Configuration

This service can use JSON, TOML, YAML, HCL, and Java properties config files.
Override the configuration values by passing through environment variables.
The configuration file can be set in your current directory or in `/etc/kafka-dns`.

### Test the service

```bash
dig @localhost -p 8053 zenaton-rabbitmq-c1-n3.services.clever-cloud.com
dig @localhost -p 8053 yds.cleverapps.io
```

## Test

Run the command `go test kafka-dns`

## Resources

https://github.com/abh/geodns/search?q=RR&unscoped_q=RR

https://github.com/coredns/coredns/blob/b7480d5d1216aa87d80d240a31de750079eba904/plugin/test/helpers.go

https://github.com/miekg/exdns/blob/master/reflect/reflect.go

https://github.com/miekg/dns/blob/master/msg.go

https://github.com/boltdb/bolt#using-keyvalue-pairs

https://github.com/segmentio/kafka-go