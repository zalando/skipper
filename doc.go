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
configuration as well as a runtime update of the routing rules.

Skipper works as an HTTP reverse proxy that is responsible for mapping
incoming requests to multiple HTTP backend services, based on routes
that are selected by the request attributes. At the same time both the
requests and the responses can be augmented by a filter chain that is
specifically defined for each route.

Skipper can load and update the route definitions from multiple data
sources without restarting it.

It provides a default executable command with a few built-in filters,
however, its primary use case is extending it with custom filters and
compiling one's own variant. For futher information read
'Extending Skipper'.

Skipper took the core design and inspiration from Vulcand:
https://github.com/mailgun/vulcand.


Quickstart

Skipper is 'go get' compatible. If needed, create a 'go workspace' first:

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

Start skipper and make an HTTP request through it:

    skipper -routes-file example.eskip &
    curl localhost:9090/hello


Routing Mechanism

The core of skipper's request processing is implemented by a reverse
proxy in the 'proxy' package. The proxy receives the incoming request,
forwards it to the routing engine in order to receive the most closely
matching route. When a route has been found, the request is forwarded
to all filters defined by it. The filters can modify the request or
execute any kind of program logic. Once the request has been processed
by all the filters, it is forwarded to the backend endpoint of the
route. The response from the backend goes once again through all the
filters, but this time the order is reversed, then it finally gets
mapped as the response of the original incoming request.

Besides the default proxying mechanism, it is possible to define routes
without a real network backend endpoint. This is called a 'shunt'
backend, in which case one of the filters needs to handle the request
(e.g. the 'static' filter). Actually, filters themselves can instruct
the request flow to shunt.

For further details, see the 'proxy' and 'filters' package
documentation.


Matching Requests

Finding a request's route happens by matching the request attributes to
the conditions in the route's definitions. Such definitions may have the
following conditions: method, path (optionally with wildcards), path
regular expressions, host regular expressions, headers and header
regular expressions. There is also a way to use custom predicates as
conditions to be matched.

The relation between the conditions in a route definition is 'and',
meaning, that a request must fulfill each condition to match a route.

For further details, see the 'routing' package documentation.


Filters - Augmenting Requests

Filters are applied in order of definition to the request and in reverse
order to the response. They are used to modify request and response
attributes, such as headers, or execute background tasks, like logging.
Some filters may handle the requests without proxying them to service
backends. Filters, depending on their implementation, may accept/require
parameters, that are set specific to the route.

For details, see the 'filters' package documentation.


Service Backends

Each route has one of the following backends: network service or shunt.

Backend network services can serve any web page or network API. They are
specified by their network address, including the protocol scheme, the
domain name or the IP address, and optionally the port number: e.g.
"https://www.example.org:4242". (The path and query are sent from the
original request, or set by filters.)

A shunt route means that skipper handles the request alone and doesn't
make requests to a backend service. In this case, it is the
responsibility of one of the filters to generate the response.


Route Definitions

Route definitions consist of the following: request matching conditions;
optional filters; a route backend. The eskip package implements the
in-memory and text representations of route definitions, including a
parser.

(Note to contributors: in order to stay compatible with 'go get', the
generated part of the parser is stored in the repository. When changing
the grammar, 'go generate' needs to be executed explicitly to update the
parser.)

For details, see the 'eskip' package documentation


Data Sources

The route definitions of Skipper are loaded from one or more sources and
while running, it receives incremental updates from them. It provides
three different data clients:

- Innkeeper: the Innkeeper service implements a storage for large sets
of skipper routes, with an HTTP+JSON API, OAuth2 authentication and role
management. See the 'innkeeper' package and
https://github.com/zalando/innkeeper.

- etcd: skipper can load routes and receive updates from etcd clusters
(https://github.com/coreos/etcd). See the 'etcd' package.

- static file: package eskipfile implements a simple data client, which
can load route definitions from a static file in eskip format.
Currently, it supports only loading the routes on startup but no updates.

Skipper can use additional data sources, provided by extensions. Sources
must implement the DataClient interface in the routing package.


Running Skipper

Skipper can be started with the default executable command 'skipper', or
as a library built into a program. The easiest way to start skipper as
a library is by executing the 'Run' function of the current, root
package.

Each option accepted by the 'Run' function is wired in the
default executable as well, as a command line flag. E.g. EtcdUrls
becomes -etcd-urls as a comma separated list. For command line help,
enter:

    skipper -help

An additional utility, eskip, is used to verify, print, update and
delete routes from/to files or etcd (Innkeeper on the roadmap). See the
cmd/eskip command package, and/or enter in the command line:

    eskip -help


Extending Skipper

Skipper doesn't use dynamically loaded plugins, however, it can be used
as a library, and it can be extended with custom predicates, filters
and/or custom data sources.


Custom Predicates

To create a custom predicate, one needs to implement the PredicateSpec
interface in the routing package. Instances of the PredicateSpec are
used internally by the routing package to create the actual Predicate
objects as referenced in eskip routes, with concrete arguments.

Example, randompredicate.go:

    package main

    import (
        "github.com/zalando/skipper/routing"
        "math/rand"
        "net/http"
    )

    type randomSpec struct {}

    type randomPredicate struct {
        chance float64
    }

    func (s *randomSpec) Name() string { return "Random" }

    func (s *randomSpec) Create(args []interface{}) routing.Predicate {
        p := &randomPredicate{.5}
        if len(args) > 0 {
            if c, ok := args[0].(float64); ok {
                p.chance = c
            }
        }

        return p
    }

    func (p *randomPredicate) Match(_ *http.Request) bool {
        return rand.Float64() < p.chance
    }

In the above example, a custom predicate is created, that can be
referenced in eskip definitions with the name 'Random':

    Random(.33) -> "https://test.example.org";
    * -> "https://www.example.org"


Custom Filters

To create a custom filter we need to implement the Spec interface of the
filters package. 'Spec' is the specification of a filter, and it is used
to create concrete filter instances while the raw route definitions are
processed.

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

The above example creates a filter specification, and in the routes where
they are included, the filter instances will set the 'X-Hello' header
for each and every response. The name of the filter is 'hello', and in a
route definition it is addressed as:

    * -> hello("world") -> "https://www.example.org"


Custom Build

The easiest way to create a custom skipper variant is to implement the
required filters (as in the example above) by importing the skipper
package, and starting it with the 'Run' command.

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
            CustomPredicates: []routing.PredicateSpec{&randomSpec{}},
            CustomFilters: []filters.Spec{&helloSpec{}}}))
    }

A file containing the routes, routes.eskip:

    Random(.05) -> hello("fish?") -> "https://fish.example.org";
    * -> hello("world") -> "https://www.example.org"

Start the custom router:

    go run hello.go


Proxy Package Used Individually

The 'Run' function in the root skipper package starts its own listener
but it doesn't provide the best composability. The proxy package,
however, provides a standard http.Handler, so it is possible to use it
in a more complex solution as a building block for routing.


Logging and Metrics

Skipper provides detailed logging of failures, and access logs in Apache
log format. When set up, Skipper also collects detailed performance
metrics, and exposes them on a separate listener endpoint for pulling
snapshots.

For details, see the 'logging' and 'metrics' packages documentation.


Performance Considerations

The router's real life performance depends on the environment and on the
used filters. Under ideal circumstances and without filters the biggest
time factor is the route lookup. Skipper is able to scale to thousands
of routes with logarithmic performance degradation. However, this comes
at the cost of increased memory consumption, due to storing the whole
lookup tree in a single structure.

Benchmarks for the tree lookup can be run by:

    go test github.com/zalando/skipper/routing -bench=Tree

In case of more agressive scaling is needed, depending on the available
memory, a preferrable approach can be the cascading of multiple Skipper
instances based on segments of routes.
*/
package skipper
