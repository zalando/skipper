---
name: skipper-architecture
description: "Skipper architecture reference: routing table lifecycle, request processing pipeline, and package layout"
license: MIT
compatibility: opencode, claude
metadata:
  audience: maintainers, contributors
---

## What I do

- focus on skipper architecture

## When to use me

when designing a new package, understanding data flow between
components, or working on the proxy/routing core — not for routine
filter/predicate edits.

## Quick start

### Architecture

- Omit the use of `internal` or generic package directories like `pkg`.
- No code comments, that are not godoc style for exported functions, types, variables or constants. Write package doc in doc.go.
- ./cmd sub-directory contains the main packages
- ./io and ./net are additions to Go stdlib that should be reused if
  possible and enhanced if needed.
- Skipper creates very often a new routing tree, think of every 3s, from different routing sources.
- Skipper is an http proxy. The core function is in the ./proxy
  package. net.Listener functionality is outside of the proxy
  package. The request processing of the proxy starts with the stdlib
  `net/http.Handler` interface `ServeHTTP(ResponseWriter, *Request)`,
  `func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request)`.

#### Routing table creation

In routing package there is a DataClient interface.

```go
// DataClient instances provide data sources for
// route definitions.
type DataClient interface {
	LoadAll() ([]*eskip.Route, error)
	LoadUpdate() ([]*eskip.Route, []string, error)
}
```

A DataClient fetches information to create routes. Skipper runs in a
loop all dataclients LoadAll and LoadUpdate operations to fetch all
routing information every Options.PollTimeout duration to rebuild the
routing tree.

On every iteration []*eskip.Route will run through all []routing.PreProcessor.

```go
// PreProcessor is an interface for custom pre-processors applying changes
// to the routes before they were created from eskip.Route representation.
type PreProcessor interface {
	Do([]*eskip.Route) []*eskip.Route
}
```

Then `[]*eskip.Route` are processed to create all predicate and filter
instances, which ends in `[]*routing.Route`. After that all
`[]*routing.Route` are processed by all `[]routing.PostProcessors`.

```go
// PostProcessor is an interface for custom post-processors applying changes
// to the routes after they were created from their data representation and
// before they were passed to the proxy.
type PostProcessor interface {
	Do([]*Route) []*Route
}
```

The last step is to swap the routing tree `routing.Routing.routeTable` which is an `atomic.Value`.

```go
type Routing struct {
	routeTable        atomic.Value // of struct routeTable
..
```

The routeTable struct:

```go
type routeTable struct {
	id                 int
	m                  *matcher
	once               sync.Once
	routes             []*Route // only used for closing
	validRoutes        []*eskip.Route
	invalidRoutes      []*eskip.Route
	invalidRouteErrors map[string]string // route ID -> error message
	clients            map[DataClient]struct{}
	created            time.Time
}
```

#### Request Processing

This is hot path code. Everything needs to be efficient!

Starting from ./proxy package `func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request)`.

We use heavy observability tools to support operations: logging,
metrics, OpenTracing/OTel spans and profiling.  We conditionally write
access logs which can be modified per route and for each status code.

Basic request and response processing:

1. ServeHTTP: creates a skipper proxy `*context`, calls `p.do(*context, span)` and serves the response `p.serveResponse(*context)` (`p` is the `*Proxy` instance)
2. do: main functionality of the proxy:
   1. route lookup
   2. request filter processing in a loop call all `filter.Request(FilterContext)`
   3. map to backend
   4. in case of load balancer backend apply load balancer algorithm to select endpoint
   5. send the `*http.Request` to the endpoint
   6. receive `*http.Response`
   7. response filter processing in a loop call all `filter.Response(FilterContext)`
3. serve the response by writing http.Header first and stream the response body.
