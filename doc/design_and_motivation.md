**Last modification**: 14/01/2020 by Coltellacci Alessio <alessio.coltellacci@clever-cloud.com>

# Design Motivation

This document represents the goals we should have in mind while developing Stream-DNS. It will inform feature decisions, bug fixes and areas of focus.

## Origin

Stream-DNS grew from a need at [Clever Cloud](https://www.clever-cloud.com/) to handle resilience and availability in our DNS. Classical solutions like [bind](https://www.isc.org/bind/), [unbound](https://www.nlnetlabs.nl/projects/unbound/about/) (a DNS resolver) or [powerdns](https://www.powerdns.com/) were designed with now, a old architecture. This causes a few issues:

- Multiple level of DNS caches due to a composition of unbound and bind or powerdns in infrastructure. That may result on networking error which can be hard to debug.
- Slow DNS propagation for servers like Powerdns based on  PostgresSQL as a database for the record.
- The lack of scalability due to architecture design which transforms DNS servers into a hot spot.
- Lack of metrics for interoperability.

## Main goals

### DNS caches are problematic

Thanks to the architecture of stream-DNS which allows having easily multiples and synchronize Stream-DNS instance, multiple DNS caches are not necessary anymore.   

### DNS propagation

The use of event sourcing architecture (see [architecture](ma)) with Kafka or Pulsar as event sources and Stream-DNS instance as consumers to collect zone information has the effect on improving the DNS propagation.

### Primary and secondary server

A primary server hosts the controlling zone file, which contains all the authoritative information for a domain. Secondary servers contain read-only copies of the zone file, and they get their info from a primary server in a communication known as a zone transfer. With Stream-DNS, this concept is kind of exclude. All Stream-DNS instances are connected to the event source which contain all the authoritative information and the concept of zone file are not needed anymore. 

### Availability and Durability Guarantees

The combination of `Apache Kafka` or `Apache Pulsar` and `bbolt`  (see [architecture](ma)) make a Stream-DNS fast to start. A fresh instance will just have to consume the event source to get the last state of the controlling zones. Clustering features for fault tolerance and high availability provide by  `Apache Kafka` and `Apache Pulsar` are also a benefit for Stream-DNS availability.

### Operability

Stream-DNS collects many statistics about itself and it is able to connect on external storage metric & analytic service to forward his metrics to them. This will allow the operator to monitor to keep tabs on your stream-DNS setup. Using the monitoring of Stream-DNS provide good visibility into the health and performance of your DNS & network infrastructure.

[ma]: ./manual.md













