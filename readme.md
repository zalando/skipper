[![Build Status](https://travis-ci.org/zalando/skipper.svg)](https://travis-ci.org/zalando/skipper)
[![GoDoc](https://godoc.org/github.com/zalando/skipper/proxy?status.svg)](https://godoc.org/github.com/zalando/skipper/proxy)
[![License](https://img.shields.io/badge/license-APACHE-red.svg?style=flat)](https://raw.githubusercontent.com/zalando/skipper/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/zalando/skipper)](https://goreportcard.com/report/zalando/skipper)
[![Coverage](http://gocover.io/_badge/github.com/zalando/skipper)](http://gocover.io/github.com/zalando/skipper)

<p align="center"><img height="360" alt="Skipper" src="https://raw.githubusercontent.com/zalando/skipper/gh-pages/img/skipper.h360.png"></p>

# Skipper

Skipper is an HTTP router built on top of a reverse proxy with the ability to modifying requests and
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
- Extending It with Customized Predicates, Filters, and Builds
- Proxy Packages
- Logging and Metrics
- Performance Considerations
