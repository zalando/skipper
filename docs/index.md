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
