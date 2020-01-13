**Last modification**: 13/01/2020 by Coltellacci Alessio <alessio.coltellacci@clever-cloud.com>

# Manual

> This manual is a work-in-progress! Help is appreciated! See the source of this manual if you want to help. Either by sending pull requests or by filing issues with specifics of what you want to see addressed here, any missing content or just confusing bits.
– Stream-DNS Authors

## Table of Contents

* What is Stream-DNS
* Installation
* Run it
* Configuration

## What is Stream-DNS

Stream-DNS is  a fresh new DNS server. It is different from other DNS servers, such as BIND, PowerDNS, CoreDNS, because it remains on the Event Sourcing pattern to manage his zones. For now, it relies on Kafka as event sources. Which has the consequence that all DNS transaction relative to the manage of the zone, like DNS zone transfer, DNS Notify, etc. don't need to be implemented in the DNS server and can be implemented on other external services. 

This event sourcing model allows it to emancipate to the primary/secondary DNS server architecture. All Stream-DNS instances on your infrastructure can be connected to the same event source, for instance, your Kafka clusters and get the content of the zones from this seed. You no longer need to have a primary and secondary server to improve the reliability of your DNS. This will now rely on the  reliability of your event source chosen, for example Kafka, which has a great reliability. So all the instances will be a primary server.

Stream-DNS embeds a separated DNS resolver: dnsr, in order to resolve the query for non-authoritative zone. The authority server part and the resolver are clearly separated because  that can add trouble on debugging DNS , and the second reason is more abstract: the two functions, resolver and authority, are conceptually very different, and many practical problems with DNS come from ignorance of the actors of these two functions, their respective roles, and the practical problems they face.

## Installation

Stream-DNS is written in Go, but unless you want to compile Stream-dns yourself, you probably don't care. The following sections detail how you can get Stream-dns binaries or install from source.

### Binaries

For every Stream-DNS release, we provide pre-compiled binaries for various operating systems.

### Docker

We push every release as a [Docker image](https://hub.docker.com/r/clevercloud/stream-dns) as well. You can find them in the public Docker hub for the CleverCloud organization. We also provide a [docker-compose](https://github.com/CleverCloud/stream-dns/blob/master/docker-compose.yml) file to help you during your development.

### From source

To compile Stream-dns, we assume you have a working Go setup. See various tutorials if you don’t have that already configured. Stream-dns is using Go modules for its dependency management.

Build and run:
```
$ go build -o stream-dns
$ ./stream-dns
```

(Optional step) create a vendor directory:

`go mod vendor`

## Configuration

Stream-dns remains on environment variables to get his configuration. This table shows the list of setting that can be tweaked:

| Name                       | Type           | Description                                                  |
| -------------------------- | -------------- | ------------------------------------------------------------ |
| DNS_INSTANCE_ID            | string         | (optional) Unique Identifier of this stream-dns instance     |
| DNS_ADDRESS                | string         | Address for the DNS server e.g: ":8053"                      |
| DNS_TCP                    | bool           | Accept TCP DNS connection                                    |
| DNS_UDP                    | bool           | Accept UDP DNS connection                                    |
| DNS_ZONES                  | List of string | List of supported zones e.g: "clvrcld.net. services.clever-cloud.com." (separate by whitespace) |
| DNS_KAFKA_ADDRESS          | string         | Address of one kafka node e.g: "localhost:9092"              |
| DNS_KAFKA_TOPIC            | string         | Kafka topic of the records                                   |
| DNS_METRICS_BUFFER_SIZE    | int            | Size of the metrics buffer in bytes                          |
| DNS_METRICS_FLUSH_INTERVAL | int            | Flushing interval of the metrics                             |
| DNS_PATHDB                 | string         | Path of the bbolt database e.g: "/tmp/my.db"                 |
| DNS_STATSD_ADDRESS         | string         | Address use to output the metrics in a statd format e.g: "127.0.0.1:8125" |
| DNS_STATSD_PREFIX          | string         | (optional) Add a prefix on statd field metric                |
| DNS_ADMIN_USERNAME         | bool           | (optional) username for HTTP administrator service           |
| DNS_ADMIN_PASSWORD         | bool           | (optional) password for HTTP administrator service           |
| DNS_ADMIN_ADDRESS          | bool           | (optional) Address for the HTTP administrator                |
| DNS_ADMIN_JWTSECRET        | bool           | (optional) JWT secret for administrator credentials          |
| DNS_LOCAL_RECORDS          | string         | (optional) Set record(s) specific to one instance. Must follow the format: [NAME]. [TTL] IN A [CONTENT] e.g.: www.example.internal. 2700 IN A 127.0.0.1 |

## Run it

Once you have a stream-dns binary. You must take a look at the configuration section to see how you can configure a stream-dbs. So to test, we start stream-dns to run on port `8053` and send it a query using dig:

```
$ DNS_ADDRESS="127.0.0.1:8053" ./stream-dns
```

And from a different terminal window, a dig should return something similar to this:

```
$ dig @127.0.0.1 -p 8053 a <my domain>
...
;; QUESTION SECTION:
<your question>

;; ANSWER SECTION:
<the answers>
```

## Logging

Use the log package: [logrus](https://github.com/Sirupsen/logrus) a a structured logger for Golang. You can configure it through an environment variables. Logging is controlled via the `LOG_LEVEL` environment variable. The actual level is optional to specify. If omitted, all logging will be enabled. If specified, the value of this environment variable must be one of the strings: `trace, debug, info, warn, error, fatal, panic`. 
NOTE: support `CamelCase` and `uppercase`.

The log format can be controlled via the `LOG_FORMAT` environment variable. Possible value is: `json` or `plain`.

## Metrics

For now, Stream-DNS only support statsd external metric service. You can specify an address to connect so that Stream-DNS will send the metrics by setting the `DNS_STATSD_ADDRESS` environment variable.

NOTE: look at [monitoring](https://github.com/CleverCloud/stream-dns/blob/master/doc/monitoring.md) if you want to know more about monitoring.

## Administration tool

The administrator server is a light embedded server that  allow you to check the health of a Stream-DNS instance. For known, the HTTP administrator server is exposed through an optional `JWT` authentication system. To setup it, you just have to set the environment variables:

* DNS_ADMIN_USERNAME
* DNS_ADMIN_PASSWORD
* DNS_ADMIN_JWTSECRET

You can set the environment value `DNS_ADMIN_ADDRESS` to specify at which address the administrator server will listen incoming request.


See below for examples input forms: 

**Sign in**:

`curl -X -v http://<address>/signin -d '{"username":"<your username>","password":"<your password>"}'`

**Search records following a pattern:**

`curl --cookie token=<JWT token> "http://<address>/search?pattern=<your pattern>`

## Integration with systemd

This repository provide a [systemd UNIT file](https://github.com/CleverCloud/stream-dns/blob/add-doc/data/stream-dns.service) that you can place at: `/etc/systemd/system`, this is the location where they are placed by default. Unit files stored here are able to be started and stopped on-demand during a session. 

To start the stream-DNS service in systemd run the command as shown:

`systemctl start stream-dns`

To verify that the service is running, run:

`systemctl status stream-dns`

To stop the service running service, run:

`systemctl stop stream-dnsstatus`

**To enable stream-dns service on boot up, run:**

`systemctl enable stream-dns`

NOTE: look at the man `systemctl(1)` to learn more on how to manage Systemd services. 