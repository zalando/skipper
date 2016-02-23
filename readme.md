[![Build Status](https://travis-ci.org/zalando/skipper.svg)](https://travis-ci.org/zalando/skipper)
[![GoDoc](https://godoc.org/github.com/zalando/skipper/proxy?status.svg)](https://godoc.org/github.com/zalando/skipper/proxy)

# Skipper

Skipper is an HTTP router that acts as a reverse proxy (with support for flexible route definitions), alters
requests and responses with filters. You can use it out of the box and add your own custom filters and predicates.

###What Skipper Does
- identifies routes based on the requests' properties, such as path, method, host and headers
- routes each request to the configured server endpoint
- allows alteration of requests and responds with filters that are independently configured for each route
- optionally acts as a final endpoint (shunt)
- updates the routing rules without restarting, while supporting multiple types of data sources — including [etcd](https://github.com/coreos/etcd), [Innkeeper](https://github.com/zalando/innkeeper) and static files

####Inspiration
Skipper's design is largely inspired by [Vulcand](https://github.com/vulcand/vulcand). 

### Getting Started
####Prerequisites/Requirements
- [What technologies/special versions?]

####Running
Skipper is 'go get' compatible. If needed, create a Go workspace first:

    mkdir ws
    cd ws
    export GOPATH=$(pwd)
    export PATH=$PATH:$GOPATH/bin

Get the Skipper packages:

    go get github.com/zalando/skipper/...

Create a file with a route:

    echo 'hello: Path("/hello") -> "https://www.example.org"' > example.eskip

Optionally, verify the file's syntax:

    eskip check example.eskip

Start Skipper and make an HTTP request through it:

    skipper -routes-file example.eskip &
    curl localhost:9090/hello

#### Compiling

Getting the code (optionally, you can create a workspace):

    mkdir ws
    cd ws
    export GOPATH=$(pwd)
    export PATH=$PATH:$GOPATH/bin
    go get -t github.com/zalando/skipper

#### Building
[needs more context copy]

    cd src/github.com/zalando/skipper
    go install ./cmd/skipper

#### Testing

    go test ./...

### Additional Documentation
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

### Contributing/TODO
We welcome contributions to this project. [Need to add TODO's, any guidelines/formatting preferences.]
