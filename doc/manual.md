# Manual

> This manual is a work-in-progress! Help is appreciated! See the source of this manual if you want to help. Either by sending pull requests or by filing issues with specifics of what you want to see addressed here, any missing content or just confusing bits.
– Stream-DNS Authors

## Table of Contents

* What is Stream-DNS
* Installation
* Run it
* Configuration

## Installation

Stream-dns is written in Go, but unless you want to compile Stream-dns yourself, you probably don’t care. The following sections detail how you can get Stream-dns binaries or install from source.

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