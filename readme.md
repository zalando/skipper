# Skipper

Skipper is an HTTP router acting as a reverse proxy with support for custom route definitions and altering
the requests and responses with filters.

- identifies routes based on the properties of the requests, like path, header and method
- routes each request to the configured server endpoint
- allows altering the requests and responses with filters independently configured for each route
- optionally acts as a final endpoint (shunt)
- the routing definitions are stored in [Innkeeper](https://github.com/zalando/innkeeper) or [etcd](https://github.com/coreos/etcd/)


Skipper's design is largely inspired by Mailgun's Vulcand, and just as Vulcand, it uses Mailgun's Route package
to identify which route a request belongs to.

- [https://github.com/mailgun/vulcand](https://github.com/mailgun/vulcand)
- [https://github.com/mailgun/route](https://github.com/mailgun/route)


### Quickstart

  Create workspace
  
    mkdir ws
    cd ws
    export GOPATH=$(pwd)

  Get packages
  
    go get github.com/coreos/etcd
    go get github.com/zalando/skipper

  Start etcd and create a simple route
  
    bin/etcd &
    curl -X PUT -d 'value=Path("/") -> "https://www.example.org"' http://127.0.0.1:2379/v2/keys/skipper/routes/hello

  Start skipper and make a request to the route
  
    bin/skipper &
    curl -s localhost:9090 | sed q2


### Running Skipper

Skipper requires etcd to read the route definitions from. (It can be started before etcd becomes accessible, but
won't be able to route the incoming requests until first read of the settings.)

To start Skipper:

    skippper -address :9090 -etcd-urls http://127.0.0.1:2379,http://127.0.0.1:4001 -insecure


##### Options

- **-address**: address where Skipper will listen on. Default: `:9090`.
- **-etcd-urls**: list of addresses to an etcd cluster. Default: `http://127.0.0.1:2379,http://127.0.0.1:4001`.
  If the listed addresses don't belong to the same cluster, then the behavior is undefined.
- **-insecure**: if this flag is set, Skipper doesn't verify the TLS certificates of the server endpoints in
  the routes. Default: false (verifies).


### Operation

Skipper's operations can be described based on three kinds of artifacts:

- frontend
- backend
- filters

1. When a request hits Skipper, it is matched based on its properties like method, path and headers to an available
frontend. This way a frontend identifies a route. If none of the configured frontends match the request, Skipper
404s.
2. The filters of the route are executed in the order they are configured. The filters can alter the
request's properties like its method, headers and path, or do any other actions like e.g. logging.
3. Skipper forwards the request to the backend of the route, which is a server endpoint described by its address (schema,
hostname and port), and starts streaming the request payload, if any. When the backend responds, the response is
passed to all filters in the route in reverse order, and they can modify its properties just like in the case of
the request (no method, but status instead).
4. Skipper returns the response to the incoming connection and starts streaming the response payload, if any.

A special case of the backends is the 'shunt', in which case the request is not forwarded to a real server
endpoint, but Skipper itself generates the response. For routes ending with shunts, it is the filters'
responsibility to set the response status code and optional content. The default status code is 404.


### Routing

Routes are stored in etcd under directory `/v2/keys/skipper/routes`. To create a simple route:

    curl -X PUT -d 'value=Path("/") -> "https://example.com"' http://127.0.0.1:2379/v2/keys/skipper/routes/test

This creates a route that will forward requests with the path `/` to https://example.com, and stores it with the
key `test`.

A route definition looks like this:

    <frontend> -> [<filter> -> ...] <backend>


##### Frontend

In the example below, there is a frontend and a backend:

    Path("/") -> "https://example.com:4545"

The frontend is specified by `Path("/")`, and it means that requests with path `/` will be forwarded through
this route.

Further frontend examples:

    Path("/static/<string>")
    PathRegexp(/\.html(?.*)?$/)
    Method("POST")

For the full documentation of frontend definitions, please, refer to the Mailgun Route project:

[http://godoc.org/github.com/mailgun/route](http://godoc.org/github.com/mailgun/route)


##### Backend

Backends represent the server endpoint where the requests for a given route are forwarded to. Backends are
defined by the HTTP server address, including the schema, the hostname and optionally the port. (To set the
request path, one can use filters.)

    "https://example.com:4545"


##### Shunt

A special case of backends is the shunt:

    <shunt>

Shunts don't forward requests to server endpoints and are usually used together with filters. A route with a
shunt may look like this:

    Path("/images") -> static("/var/www") -> <shunt>


##### Filters

Filters represent manipulations over the requests and/or responses. In the routing syntax, they look like
function calls with arguments:

    myFilter("arg1", "arg2", 3.14, /^rx[^/]*$/)

The number and the type of the arguments depend on the filter implementation. The possible types are string,
number and regexp.

A route with multiple filters may look like this:

    Path("/api/<string>") -> modPath(/^\/api/, "") -> responseHeader("Server", "Example API") -> "https://api.example.com"


### Filtering

If not using filters, the request defined by the frontend is forwarded to the backend as is, with the same
HTTP method, path and headers. The response is forwareded back to the client as it is received from the
backend, with the same status code and headers. With filters, it is possible to change any of these properties
of both the request and the response, including the payload.

Skipper was designed to make it simple to create custom filters (see Creating custom filters), and it includes
only a small set of simple built-in filters.


Built-in filters:

##### Healtcheck

This filters always sets the response status to 200.

    Path("/healthcheck") -> healthcheck() -> <shunt>


##### Humans.txt

Provides a debuttal default humans.txt.

    Path("/humans.txt") -> humanstxt() -> <shunt>


##### Path rewrite

This filter can be used to modify the request path. It executes a replace-all call on the path, with two
arguments: a regexp expression to match the whole or parts of the path, and a replacement string. It can be used
to set fixed path for a request, too:

    PathRegexp(/[?&]doc(&.*)?/) -> modPath(/.*/, "/doc") -> "https://api.example.com/doc"


##### Request header

Sets a fixed request header.

    Method("POST") -> requestHeader("Host", "update.example.com") -> "https://proxy.example.com"


##### Response header

Sets a fixed response header.

    Path("/<string>") -> responseHeader("Server", "Skipper") -> "https://example.com"


### Creating custom filters

TBD


### Compiling

Getting the code:

    mkdir ws
    cd ws
    export GOPATH=$(pwd)
    go get github.com/zalando/skipper

The tests require etcd in the workspace:

    go get github.com/coreos/etcd

Build:

    cd src/github.com/zalando/skipper
    go install

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
