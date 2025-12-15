/*
Package skipper provides an HTTP routing library with flexible
configuration as well as a runtime update of the routing rules.

Skipper works as an HTTP reverse proxy that is responsible for mapping
incoming requests to multiple HTTP backend services, based on routes
that are selected by the request attributes. At the same time, both the
requests and the responses can be augmented by a filter chain that is
specifically defined for each route. Optionally, it can provide circuit
breaker mechanism individually for each backend host.

Skipper can load and update the route definitions from multiple data
sources without being restarted.

It provides a default executable command with a few built-in filters,
however, its primary use case is to be extended with custom filters,
predicates or data sources. For further information read
'Extending Skipper'.

Skipper took the core design and inspiration from Vulcand:
https://github.com/mailgun/vulcand.

# Quickstart

Skipper is 'go get' compatible. If needed, create a 'go workspace' first:

	mkdir ws
	cd ws
	export GOPATH=$(pwd)
	export PATH=$PATH:$GOPATH/bin

Get the Skipper packages:

	go get github.com/zalando/skipper/...

Create a file with a route:

	echo 'hello: Path("/hello") -> "https://www.example.org"' > example.eskip

Optionally, verify the syntax of the file:

	eskip check example.eskip

Start Skipper and make an HTTP request:

	skipper -routes-file example.eskip &
	curl localhost:9090/hello

# Routing Mechanism

The core of Skipper's request processing is implemented by a reverse
proxy in the 'proxy' package. The proxy receives the incoming request,
forwards it to the routing engine in order to receive the most specific
matching route. When a route matches, the request is forwarded to all
filters defined by it. The filters can modify the request or execute any
kind of program logic. Once the request has been processed
by all the filters, it is forwarded to the backend endpoint of the
route. The response from the backend goes once again through all the
filters in reverse order. Finally, it is mapped as the response of the
original incoming request.

Besides the default proxying mechanism, it is possible to define routes
without a real network backend endpoint. One of these cases is called a
'shunt' backend, in which case one of the filters needs to handle the
request providing its own response (e.g. the 'static' filter). Actually,
filters themselves can instruct the request flow to shunt by calling the
Serve(*http.Response) method of the filter context.

Another case of a route without a network backend is the 'loopback'. A
loopback route can be used to match a request, modified by filters,
against the lookup tree with different conditions and then execute a
different route. One example scenario can be to use a single route as
an entry point to execute some calculation to get an A/B testing
decision and then matching the updated request metadata for the actual
destination route. This way the calculation can be executed for only
those requests that don't contain information about a previously
calculated decision.

For further details, see the 'proxy' and 'filters' package
documentation.

# Matching Requests

Finding a request's route happens by matching the request attributes to
the conditions in the route's definitions. Such definitions may have the
following conditions:

- method

- path (optionally with wildcards)

- path regular expressions

- host regular expressions

- headers

- header regular expressions

It is also possible to create custom predicates with any other matching
criteria.

The relation between the conditions in a route definition is 'and',
meaning, that a request must fulfill each condition to match a route.

For further details, see the 'routing' package documentation.

# Filters - Augmenting Requests

Filters are applied in order of definition to the request and in reverse
order to the response. They are used to modify request and response
attributes, such as headers, or execute background tasks, like logging.
Some filters may handle the requests without proxying them to service
backends. Filters, depending on their implementation, may accept/require
parameters, that are set specifically to the route.

For further details, see the 'filters' package documentation.

# Service Backends

Each route has one of the following backends: HTTP endpoint, shunt,
loopback or dynamic.

Backend endpoints can be any HTTP service. They are specified by their
network address, including the protocol scheme, the domain name or the
IP address, and optionally the port number: e.g.
"https://www.example.org:4242". (The path and query are sent from the
original request, or set by filters.)

A shunt route means that Skipper handles the request alone and doesn't
make requests to a backend service. In this case, it is the
responsibility of one of the filters to generate the response.

A loopback route executes the routing mechanism on current state of
the request from the start, including the route lookup. This way it
serves as a form of an internal redirect.

A dynamic route means that the final target will be defined in a filter.
One of the filters in the chain must set the target backend url explicitly.

# Route Definitions

Route definitions consist of the following:

- request matching conditions (predicates)

- filter chain (optional)

- backend

The eskip package implements the in-memory and text representations of
route definitions, including a parser.

(Note to contributors: in order to stay compatible with 'go get', the
generated part of the parser is stored in the repository. When changing
the grammar, 'go generate' needs to be executed explicitly to update the
parser.)

For further details, see the 'eskip' package documentation

# Authentication and Authorization

Skipper has filter implementations of basic auth and OAuth2. It can be
integrated with tokeninfo based OAuth2 providers. For details, see:
https://pkg.go.dev/github.com/zalando/skipper/filters/auth.

# Data Sources

Skipper's route definitions of Skipper are loaded from one or more data
sources. It can receive incremental updates from those data sources at
runtime. It provides three different data clients:

- Kubernetes: Skipper can be used as part of a Kubernetes Ingress Controller
implementation together with https://github.com/zalando-incubator/kube-ingress-aws-controller .
In this scenario, Skipper uses the Kubernetes API's Ingress extensions as
a source for routing. For a complete deployment example, see more details
in: https://github.com/zalando-incubator/kubernetes-on-aws/ .

- Innkeeper: the Innkeeper service implements a storage for large sets
of Skipper routes, with an HTTP+JSON API, OAuth2 authentication and role
management. See the 'innkeeper' package and
https://github.com/zalando/innkeeper.

- etcd: Skipper can load routes and receive updates from etcd clusters
(https://github.com/coreos/etcd). See the 'etcd' package.

- static file: package eskipfile implements a simple data client, which
can load route definitions from a static file in eskip format.
Currently, it loads the routes on startup. It doesn't support runtime
updates.

Skipper can use additional data sources, provided by extensions. Sources
must implement the DataClient interface in the routing package.

# Circuit Breaker

Skipper provides circuit breakers, configured either globally, based on
backend hosts or based on individual routes. It supports two types of
circuit breaker behavior: open on N consecutive failures, or open on N
failures out of M requests. For details, see:
https://pkg.go.dev/github.com/zalando/skipper/circuit.

# Running Skipper

Skipper can be started with the default executable command 'skipper', or
as a library built into an application. The easiest way to start Skipper
as a library is to execute the 'Run' function of the current, root
package.

Each option accepted by the 'Run' function is wired in the
default executable as well, as a command line flag. E.g. EtcdUrls
becomes -etcd-urls as a comma separated list. For command line help,
enter:

	skipper -help

An additional utility, eskip, can be used to verify, print, update and
delete routes from/to files or etcd (Innkeeper on the roadmap). See the
cmd/eskip command package, and/or enter in the command line:

	eskip -help

# Extending Skipper

Skipper doesn't use dynamically loaded plugins, however, it can be used
as a library, and it can be extended with custom predicates, filters
and/or custom data sources.

# Custom Predicates

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

	func (s *randomSpec) Create(args []interface{}) (routing.Predicate, error) {
	    p := &randomPredicate{.5}
	    if len(args) > 0 {
	        if c, ok := args[0].(float64); ok {
	            p.chance = c
	        }
	    }

	    return p, nil
	}

	func (p *randomPredicate) Match(_ *http.Request) bool {
	    return rand.Float64() < p.chance
	}

In the above example, a custom predicate is created, that can be
referenced in eskip definitions with the name 'Random':

	Random(.33) -> "https://test.example.org";
	* -> "https://www.example.org"

# Custom Filters

To create a custom filter we need to implement the Spec interface of the
filters package. 'Spec' is the specification of a filter, and it is used
to create concrete filter instances, while the raw route definitions are
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
route definition it is referenced as:

	r: * -> hello("world") -> "https://www.example.org";

# Custom Build

The easiest way to create a custom Skipper variant is to implement the
required filters (as in the example above) by importing the Skipper
package, and starting it with the 'Run' command.

Example, hello.go:

	package main

	import (
	    "log"

	    "github.com/zalando/skipper"
	    "github.com/zalando/skipper/filters"
	    "github.com/zalando/skipper/routing"
	)

	func main() {
	    log.Fatal(skipper.Run(skipper.Options{
	        Address: ":9090",
	        RoutesFile: "routes.eskip",
	        CustomPredicates: []routing.PredicateSpec{&randomSpec{}},
	        CustomFilters: []filters.Spec{&helloSpec{}}}))
	}

A file containing the routes, routes.eskip:

	random:
	    Random(.05) -> hello("fish?") -> "https://fish.example.org";
	hello:
	    * -> hello("world") -> "https://www.example.org"

Start the custom router:

	go run hello.go

# Proxy Package Used Individually

The 'Run' function in the root Skipper package starts its own listener
but it doesn't provide the best composability. The proxy package,
however, provides a standard http.Handler, so it is possible to use it
in a more complex solution as a building block for routing.

# Logging and Metrics

Skipper provides detailed logging of failures, and access logs in Apache
log format. Skipper also collects detailed performance metrics, and
exposes them on a separate listener endpoint for pulling snapshots.

For details, see the 'logging' and 'metrics' packages documentation.

# Performance Considerations

The router's performance depends on the environment and on the used
filters. Under ideal circumstances, and without filters, the biggest
time factor is the route lookup. Skipper is able to scale to thousands
of routes with logarithmic performance degradation. However, this comes
at the cost of increased memory consumption, due to storing the whole
lookup tree in a single structure.

Benchmarks for the tree lookup can be run by:

	go test github.com/zalando/skipper/routing -bench=Tree

In case more aggressive scale is needed, it is possible to setup Skipper
in a cascade model, with multiple Skipper instances for specific route
segments.
*/
package skipper
