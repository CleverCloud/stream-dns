**Last modification**: 14/01/2020 by Coltellacci Alessio <alessio.coltellacci@clever-cloud.com>

# Stream-DNS

> Note: This project is a *work in progress*
> But it will be awesome when it will be ready

## What is Stream-DNS?

Stream-DNS is  a fresh new DNS server. It is different from other DNS servers, such as BIND, PowerDNS, CoreDNS, because it remains on the Event Sourcing pattern to manage his zones. For now, it relies on Kafka as event sources. Which has the consequence that all DNS transaction relative to the manage of the zone, like DNS zone transfer, DNS Notify, etc. don't need to be implemented in the DNS server and can be implemented on other external services. 

This event sourcing model allows it to emancipate to the primary/secondary DNS server architecture. All Stream-DNS instances on your infrastructure can be connected to the same event source, for instance, your Kafka clusters and get the content of the zones from this seed. You no longer need to have a primary and secondary server to improve the reliability of your DNS. This will now rely on the  reliability of your event source chosen, for example Kafka, which has a great reliability. So all the instances will be a primary server.

Stream-DNS embeds a separated DNS resolver: dnsr, in order to resolve the query for non-authoritative zone. The authority server part and the resolver are clearly separated because  that can add trouble on debugging DNS , and the second reason is more abstract: the two functions, resolver and authority, are conceptually very different, and many practical problems with DNS come from ignorance of the actors of these two functions, their respective roles, and the practical problems they face.

## Introduction

* [How to use it][ma]

* [Design Motivation][dw]

## Overview

* [Architecture Overview][ar]

* [monitoring][mo]

## Presentations & Slides

* [(slides) Sysadmin Days 2019](https://docs.google.com/presentation/d/1hd8HvhzUwAKIg_9NyV_Af8dktmaNr3Wfb72APbMkVIg/edit?usp=sharing)


[ma]: ./manual.md
[ar]: ./architecture.md
[dw]: ./design_and_motivation.md
[mo]: ./monitoring.md