[![Build Status](https://travis-ci.org/zalando/skipper.svg)](https://travis-ci.org/zalando/skipper)
[![GoDoc](https://godoc.org/github.com/zalando/skipper/proxy?status.svg)](https://godoc.org/github.com/zalando/skipper/proxy)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/zalando/skipper)](https://goreportcard.com/report/zalando/skipper)
[![codecov](https://codecov.io/gh/zalando/skipper/branch/master/graph/badge.svg)](https://codecov.io/gh/zalando/skipper)

<p align="center"><img height="360" alt="Skipper" src="https://raw.githubusercontent.com/zalando/skipper/gh-pages/img/skipper.h360.png"></p>

# Skipper

Skipper is an HTTP router built on top of a reverse proxy with the ability to modify requests and
responses with filters. You can use it out of the box or add your own custom filters and predicates.

### What Skipper Does
- identifies routes based on the requests' properties, such as path, method, host and headers
- routes each request to the configured server endpoint
- allows modification of requests and responds with filters that are independently configured for each route
- optionally acts as a final endpoint (shunt)
- updates the routing rules without restarting, while supporting multiple types of data sources — including [etcd](https://github.com/coreos/etcd), [Innkeeper](https://github.com/zalando/innkeeper) and static files

Skipper provides a default executable command with a few built-in filters,
however, its primary use case is to be extended with custom filters,
predicates or data sources. See more in the
[Documentation](https://godoc.org/github.com/zalando/skipper)

#### Inspiration
Skipper's design is largely inspired by [Vulcand](https://github.com/vulcand/vulcand).

### Getting Started
#### Prerequisites/Requirements
In order to build and run Skipper, only the latest version of Go needs to be installed.

Skipper can use Innkeeper or Etcd as data sources for routes. See more
details in the [Documentation](https://godoc.org/github.com/zalando/skipper).

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

#### Kubernetes Ingress

Skipper can be used to run as an Ingress implementation. A [production
example](https://github.com/zalando-incubator/kubernetes-on-aws/blob/dev/cluster/manifests/skipper/daemonset.yaml)
can be found in our [Kubernetes
configuration](https://github.com/zalando-incubator/kubernetes-on-aws).

#### Authentication Proxy

Skipper can be used as an authentication proxy, to check incoming requests with a OAuth provider. See the
documentation at:
[https://godoc.org/github.com/zalando/skipper/filters/auth](https://godoc.org/github.com/zalando/skipper/filters/auth).

#### Working with the code

Getting the code with the test dependencies (`-t` switch):

    go get -t github.com/zalando/skipper/...

Build all packages:

    cd src/github.com/zalando/skipper
    go install ./...

Test all packages:

    etcd/install.sh
    go test ./...

### Documentation
Skipper's [godoc](https://godoc.org/github.com/zalando/skipper) page includes detailed information on these topics:
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

### Packaging support

See https://github.com/zalando/skipper/blob/master/packaging/readme.md
