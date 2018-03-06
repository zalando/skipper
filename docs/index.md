# Architecture

Skipper is written as library and is also a multi binary project with
2 binaries, named `skipper` and `eskip`. `Skipper` is the HTTP proxy
and `eskip` is a CLI application to verify, print, update or delete
Skipper routes.

Skipper's internal architecture is split into different packages. The
`skipper` package has connections to multiple `dataclient`, that pull
information from different sources, for example static routes from an
eskip file or dynamic routes from Kubernetes ingress objects.

The `proxy` package gets the routes populated by skipper and has
always a current routing table which will be replaced on change.

A route is one entry in the routing table. A route consists of one or
more `predicate`, that are used to find a route for a given HTTP
request. A route can also have one or more `filter`, that can modify
the content of the request or response.

[Opentracing API](http://opentracing.io/) is supported via
[skipper-plugins](https://github.com/skipper-plugins/opentracing). For
example [Jaeger](https://github.com/jaegertracing/jaeger) is supported.

Skipper has a rich set of metrics that are exposed as json, but can be
exported in [Prometheus](https://prometheus.io) format.

![Skipper's architecture ](../img/architecture.svg)

## Route processing

Package `skipper` has a Go `http.Server` and does the `ListenAndServe`
call with the `loggingHandler` wrapped `proxy`.  The `loggingHandler`
is basically a middleware for the `proxy` and both implement the plain
Go [http.Handler interface](https://golang.org/pkg/net/http/#Handler).

For each incoming `http.Request` the `proxy` will create a request
context and enhance it with an OpentracingAPI Span. It will check
proxy global ratelimits first and after that lookup the route in the
routing table. After that skipper will apply all request filters, that
can modify the `http.Request`. It will then check the route local
ratelimits, the circuitbreakers and do the backend call. If the
backend call got a TCP or TLS connection error in a loadbalanced
route, skipper will do a retry to another backend of that loadbalanced
group automatically. Just before the response to the caller, skipper
will process the response filters, that can change the
`http.Response`.

![Skipper's request and response processing ](../img/req-and-resp-processing.svg)
