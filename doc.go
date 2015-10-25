// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
Package skipper provides an HTTP routing library with flexible
configuration and runtime update of the routing rules.

Skipper acts as an HTTP reverse proxy that maps incoming requests to
multiple HTTP backend services, based on routes selected by the
attributes of the incoming requests. In between, both the requests and
responses can be augmented by a filter chain defined individually for
each route.

Skipper can load and update the route definitions from multiple data
sources without being restarted.

Skipper provides a default executable command with a few built-in
filters but its primary use case is to extend it with custom filters and
compiling one's own variant. See section 'Extending Skipper'.

Skipper took the core design and inspiration from Vulcand:
https://github.com/mailgun/vulcand.


Quickstart

Skipper is 'go get' compatible. If needed, create a go workspace first:

    mkdir ws
    cd ws
    export GOPATH=$(pwd)
    export PATH=$PATH:$GOPATH/bin

Get the skipper packages:

    go get github.com/zalando/skipper

Create a file with a route:

    echo 'hello: Path("/hello") -> "https://www.example.org"' > example.eskip

Start skipper and make an HTTP request through skipper:

    skipper -routes-file example.eskip &
    curl localhost:9090/hello


Routing Mechanism

The core of skipper's request processing is implemented by a rewerse
proxy in the 'proxy' package. The proxy takes the incoming request,
passes it to the routing engine to receive the best matching route. If a
route is found, the request is passed to all filters defined by it. The
filters can manipulate the request or execute any other logic. Once the
request is processed by all the filters, it is forwarded to the backend
endpoint of the route. The response from the backend is passed again to
all the filters, but in reverse order, and then finally mapped as the
response to the original incoming request.

Besides the default proxying mechanism, it is possible to define routes
without a real network backend endpoint, 'shunt' backend, in which case
one of the filters needs to handle the request (e.g. the 'static'
filter).

For details, see the documentation of the proxy subdirectory.


Matching Requests

Finding the route for a request happens by matching the request
attributes against the conditions in the route definitions. Route
definitions may have the following conditions: method, path (optionally
with wildcards), path regular expressions, host regular expressions,
headers and header regular expressions.

The relation between the conditions in a route definition is 'and',
meaning that a request must fulfil each condition to match a route.

For details, see the documentation of the routing subdirectory.


Filters - Augmenting Requests

Filters are executed in order of definition on the request and in
reverse order on the response, and are used to modify request and
response attributes, like setting headers, or execute background tasks,
e.g. like logging.  Some filters may handle the requests without
proxying them to service backends. Filters, depending on their
implementation, may accept/require parameters, that are set specific to
the route.

For details, see the documentation of the filters subdirectory.


Service Backends

Each route has a backend, one of two kinds: network service or shunt.

Network services can serve any web page or network API. They are
specified by their network address, including the protocol scheme, the
domain name or the ip address, and optionally the port number: e.g.
"https://www.example.org:4242". (The path and query are passed from the
original request or set by filters.)

A shunt route means that skipper handles the request alone and doesn't
make requests to a backend service. In this case, it is the
responsibility of one of the filters to generate the response.


Route Definitions

Route definitions consist of request matching conditions, optional
filters and a route backend. The eskip package implements the in-memory
and text representations of route definitions with a parser.

(Note for contributors: in order to stay compatible with 'go get', the
generated part of the parser is stored in the repository. When changing
the grammar, 'go generate' needs to be executed explicitly to update the
parser.)

For details, see the documentation of the eskip subdirectory.


Data Sources

Skipper loads the route definitions from one or more sources, and
receives incremental updates while running. It provides three different
data clients:

- Innkeeper: the Innkeeper service implements a storage for large sets
of skipper routes, with an HTTP+JSON API, OAuth2 authentication and role
management. See the innkeeper subdirectory and
https://github.com/zalando/innkeeper.

- etcd: skipper can load routes and receive updates from etcd clusters
(https://github.com/coreos/etcd). See the etcd subdirectory.

- static file: package eskipfile implements a simple data client, which
can load route definitions from a static file in eskip format.
Currently, it doesn't support updates.

Skipper accepts additional data sources, when extended. Sources must
implement the DataClient interface in the routing package.


Running Skipper

Skipper can be started with the default executable command 'skipper', or
as a library built into a program.  Starting skipper as a library
happens by calling the Run function. Each option accepted by the Run
function is also wired in the default executable as a command line flag.
E.g. EtcdUrls becomes -etcd-urls as a comma separated list.


Extending Skipper

Skipper doesn't use dynamically loaded plugins, but it can be used as a
library and extended with custom filters and/or custom data sources.


Custom Filters

To create a custom filter, the Spec interface of the filters package
needs to be implemented. Spec is the specification of a filter, and it
is used to create concrete filter instances for each route that
references it, while the route definitions are processed.

Example, hellofilter.go:

    package main

    import (
        "fmt"
        "github.com/zalando/skipper/filters"
    )

    type helloSpec struct {}

    type helloFilter struct {
        who string
    }

    func (s *helloSpec) Name() string { return "hello" }

    func (s *helloSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
        if len(config) == 0 {
            return nil, filters.ErrInvalidFilterParameters
        }

        if who, ok := config[0].(string); ok {
            return &helloFilter{who}, nil
        } else {
            return nil, filters.ErrInvalidFilterParameters
        }
    }

    func (f *helloFilter) Request(ctx filters.FilterContext) {}

    func (f *helloFilter) Response(ctx filters.FilterContext) {
        ctx.Response().Header.Set("X-Hello", fmt.Sprintf("Hello, %s!", f.who))
    }

The above example creates a filter specification, whose filter instances
will set the X-Hello header for every response in the routes they are
included. The name of the filter is 'hello', and can be referenced in
route definitions as:

    Any() -> hello("world") -> "https://www.example.org"


Custom Build

The simplest way to creating a custom skipper variant, is to implement
the required filters as in the above example, importing the skipper
package, and starting it with the Run function.

Example, hello.go:

    package main

    import (
        "github.com/zalando/skipper"
        "github.com/zalando/skipper/filters"
        "log"
    )

    func main() {
        log.Fatal(skipper.Run(skipper.Options{
            Address: ":9090",
            RoutesFile: "routes.eskip",
            CustomFilters: []filters.Spec{&helloSpec{}}}))
    }

Routes file, routes.eskip:

    Any() -> hello("world") -> "https://www.example.org"

Start the custom router:

    go run hello.go


Proxy Package Used Individually

The Run function the root skipper package starts its own listener and
doesn't provide the best composability. The proxy package, however,
provides a standard http.Handler, so it is possible to use it in a more
complex solution as a building block for routing.


Performance Considerations

While the real life performance of the router depends on the environment
and the used filters, in ideal circumstances and without filters, the
largest time factor is the route lookup. Skipper can scale to thousands
of routes, with logarithmic performance degradation. However, this comes
at the cost of memory consumption, due to storing the whole lookup tree
in a single structure.

Benchmarks for the tree lookup can be run by:

    go test github.com/zalando/skipper/routing -bench=Tree

In case of more agressive scaling, depending on the available memory,
cascading multiple skipper instances based on segments of routes can be
the preferrable approach.
*/
package skipper
