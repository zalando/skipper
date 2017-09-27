[![Build Status](https://travis-ci.org/zalando/skipper.svg)](https://travis-ci.org/zalando/skipper)
[![GoDoc](https://godoc.org/github.com/zalando/skipper/proxy?status.svg)](https://godoc.org/github.com/zalando/skipper/proxy)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/zalando/skipper)](https://goreportcard.com/report/zalando/skipper)
[![codecov](https://codecov.io/gh/zalando/skipper/branch/master/graph/badge.svg)](https://codecov.io/gh/zalando/skipper)

<p align="center"><img height="360" alt="Skipper" src="https://raw.githubusercontent.com/zalando/skipper/gh-pages/img/skipper.h360.png"></p>


# Skipper

Skipper is an HTTP router and reverse proxy for service composition. It's designed to handle >100k HTTP route
definitions with detailed lookup conditions, and flexible augmentation of the request flow with filters. It can be
used out of the box or extended with custom lookup, filter logic and configuration sources.

## Main features:

- identifies routes based on the requests' properties, such as path, method, host and headers
- allows modification of the requests and responses with filters that are independently configured for each route
- simultaneously streams incoming requests and backend responses
- optionally acts as a final endpoint (shunt), e.g. as a static file server or a mock backend for diagnostics
- updates routing rules without downtime, while supporting multiple types of data sources — including
  [etcd](https://github.com/coreos/etcd), [Innkeeper](https://github.com/zalando/innkeeper), static files and
  custom configuration sources
- can serve as a Kubernetes Ingress implementation in combination with a controller; [see example](https://github.com/zalando-incubator/kube-ingress-aws-controller)
- shipped with eskip: a descriptive configuration language designed for routing rules

Skipper provides a default executable command with a few built-in filters. However, its primary use case is to
be extended with custom filters, predicates or data sources. [Go here for additional documentation](https://godoc.org/github.com/zalando/skipper).

A few examples for extending Skipper:

- Authentication proxy https://github.com/zalando-incubator/skoap
- Image server https://github.com/zalando-incubator/skrop


### Getting Started

#### Prerequisites/Requirements

In order to build and run Skipper, only the latest version of Go needs to be installed. Skipper can use
Innkeeper or Etcd as data sources for routes, or for the simplest cases, a local configuration file. See more
details in the documentation: https://godoc.org/github.com/zalando/skipper.


#### Installation

Skipper is 'go get' compatible. If needed, create a Go workspace first:

    mkdir ws
    cd ws
    export GOPATH=$(pwd)
    export PATH=$PATH:$GOPATH/bin

Get the Skipper packages:

    go get github.com/zalando/skipper/...


#### Running

Create a file with a route:

    echo 'hello: Path("/hello") -> "https://www.example.org"' > example.eskip

Optionally, verify the file's syntax:

    eskip check example.eskip

Start Skipper and make an HTTP request:

    skipper -routes-file example.eskip &
    curl localhost:9090/hello


#### Working with the code

Getting the code with the test dependencies (`-t` switch):

    go get -t github.com/zalando/skipper/...

Build and test all packages:

    cd src/github.com/zalando/skipper
    make deps
    make install
    make check


### Kubernetes Ingress

Skipper can be used to run as an Ingress implementation in combination with a controller, e.g.
https://github.com/zalando-incubator/kube-ingress-aws-controller.
A production example,
https://github.com/zalando-incubator/kubernetes-on-aws/blob/dev/cluster/manifests/skipper/daemonset.yaml,
can be found in our Kubernetes configuration https://github.com/zalando-incubator/kubernetes-on-aws.


### Documentation

Skipper's Godoc page, https://godoc.org/github.com/zalando/skipper, includes detailed information on these
topics:

- The Routing Mechanism
- Matching Requests
- Filters - Augmenting Requests
- Service Backends
- Route Definitions
- Data Sources
- Circuit Breakers
- Extending It with Customized Predicates, Filters, and Builds
- Proxy Packages
- Logging and Metrics
- Performance Considerations
- Rate Limiters


### Packaging support

See https://github.com/zalando/skipper/blob/master/packaging/readme.md

## Community

### Proposals

We do our proposals open in [Skipper's Google drive](https://drive.google.com/drive/folders/0B9LwJMF9koB-ZEk4bEhZal9uOWM).
If you want to make a proposal feel free to create an
[issue](https://github.com/zalando/skipper/issues) and if it is a
bigger change we will invite you to a document, such that we can work together.
