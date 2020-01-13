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

TODO

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

Use the log package: [logrus](https://github.com/Sirupsen/logrus). You can configure it through an environment variables:

TODO

## Metrics

TODO