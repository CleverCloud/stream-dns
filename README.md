# Stream-DNS

[![Build Status](https://travis-ci.org/CleverCloud/stream-dns.svg?branch=master)](https://travis-ci.org/CleverCloud/stream-dns)

Stream-dns is a DNS server written in Go originally written at [Clever Cloud](https://www.clever-cloud.com/).
Stream-dns can listen for DNS requests coming in over UDP/TCP and use kafka as datastore and zone propagation.
It grew from a need at [Clever Cloud](https://www.clever-cloud.com/) to handle resilience and availability in our DNS.
It innovates through his use of an external event source to manage DNS zones, which reduce DNS propagation, remove multiple levels of DNS caches, increase the scalability and more.
Take a look at the [Design Motivation](de) to know more.

## Table of Contents

* [Manual](ma)
* [Design](de)
* [Architecture](ar)
* [Monitoring](mo)

## Features:

Following are some highlights of Stream-DNS:

* Serve zone data and resolve for none zone authority
* act as a primaries server
* Automatically load zone from Kafka.
* Forward queries to some other (recursive) nameserver.
* Provide metrics with the `statsd` format
* Provide query and error logging
* Systemd integration (service)

## Usage

Take a look at the [manual](ma) to understand how to build, configure and run Stream-DNS.
The [documentation](https://github.com/CleverCloud/stream-dns/tree/master/doc/index.md) will provide more precise information to help you to use Stream-DNS.

### Compilation from source

To compile Stream-dns, we assume you have a working Go setup. See various tutorials if you don’t have that already configured. Stream-dns is using Go modules for its dependency management.

Build and run:
```
$ go build -o stream-dns
$ ./stream-dns
```

(Optional step) create a vendor directory for dependencies:

`go mod vendor`

### Run it

Once you have a stream-dns binary. You must take a look at the [configuration](ma) section in the [manual](ma) to see how you can configure a stream-DNS.
So as a test, we start stream-dns to run on port `8053` and send it a query using [dig](https://en.wikipedia.org/wiki/Dig_(command)):
```bash
$ DNS_ADDRESS="127.0.0.1:8053" ./stream-dns
```

And from a different terminal window, a dig should return something similar to this:

``` bash
$ dig @127.0.0.1 -p 8053 a <my domain>
...
;; QUESTION SECTION:
<your question>

;; ANSWER SECTION:
<the answers>
```

### Run it with Docker

Stream-DNS requires Go to compile. However, if you already have docker installed and prefer not to setup a Go environment, you could use our docke image easily:

``` bash
docker build -t $USER/stream-dns .
docker run --rm -p 8053:53 stream-dns
```

## Contact

All of the discussion around the project happens in the [issues](https://github.com/CleverCloud/stream-dns/issues), so please check the list there.

[ma]: ./doc/manual.md
[ar]: ./doc/architecture.md
[de]: ./doc/design_and_motivation.md
[mo]: ./doc/monitoring.md