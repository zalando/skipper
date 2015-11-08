# Skipper

Skipper is an HTTP router acting as a reverse proxy with support for flexible route definitions and altering the
requests and responses with filters.

- identifies routes based on the properties of the requests, like path, method, host and headers
- routes each request to the configured server endpoint
- allows altering the requests and responses with filters independently configured for each route
- optionally acts as a final endpoint (shunt)
- updates the routing rules without restarting, supporting multiple types of data sources, like
  [Innkeeper](https://github.com/zalando/innkeeper), [etcd](https://github.com/coreos/etcd) or static files
- extensible by custom filters

Skipper's design is largely inspired by [Vulcand](https://github.com/mailgun/vulcand).


### Quickstart

Skipper is 'go get' compatible. If needed, create a go workspace first:

    mkdir ws
    cd ws
    export GOPATH=$(pwd)
    export PATH=$PATH:$GOPATH/bin

Get the skipper packages:

    go get github.com/zalando/skipper/...

Create a file with a route:

    echo 'hello: Path("/hello") -> "https://www.example.org"' > example.eskip

Optionally, verify the syntax of the file:

    eskip check example.eskip

Start skipper and make an HTTP request through skipper:

    skipper -routes-file example.eskip &
    curl localhost:9090/hello


### Documentation

Skipper is documented in detail in godoc:
[https://godoc.org/github.com/zalando/skipper](https://godoc.org/github.com/zalando/skipper)


### Compiling

Getting the code (with optionally creating a workspace):

    mkdir ws
    cd ws
    export GOPATH=$(pwd)
    export PATH=$PATH:$GOPATH/bin
    go get github.com/zalando/skipper

Build:

    cd src/github.com/zalando/skipper
    go install ./cmd/skipper

The tests require [etcd](https://github.com/coreos/etcd) in the workspace:

    go get github.com/coreos/etcd

Test:

    go test ./...


### License

Copyright 2015 Zalando SE

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
