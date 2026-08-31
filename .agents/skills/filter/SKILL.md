---
name: filter
description: Create or modify code in the filters package and all its sub-folders
license: MIT
compatibility: opencode, claude
metadata:
  audience: maintainers, contributors
---
## What I do

- Create filter code that is efficient and readable

## When to use me

Use this when you are writing code in sub-folders of ./filters/

## Quick start

A filter consists of 2 types `spec` and `filter` that implement 2 different interfaces:


```go
// Spec objects are specifications for filters. When initializing the routes,
// the Filter instances are created using the Spec objects found in the
// registry.
type Spec interface {
	// Name gives the name of the Spec. It is used to identify filters in a route definition.
	Name() string

	// CreateFilter creates a Filter instance. Called with the parameters in the route
	// definition while initializing a route.
	CreateFilter(config []any) (Filter, error)
}
```

```go
// Filter is created by the Spec components, optionally using filter
// specific settings. When implementing filters, it needs to be taken
// into consideration, that filter instances are route specific and not
// request specific, so any state stored with a filter is shared between
// all requests for the same route and can cause concurrency issues.
type Filter interface {
	// The Request method is called while processing the incoming request.
	Request(FilterContext)

	// The Response method is called while processing the response to be
	// returned.
	Response(FilterContext)
}```

The `CreateFilter(config []any) (Filter, error)` function is run at
routing tree creation time, so only once in a while. Everything we can
pre-compute to run a faster Filter execution the better. Here we
process input from developers or operators that might do mistakes. We
want to show errors clearly.

The `Filter` interface functions are running in the hotpath of the
request processing. Here we need to be very efficient and want to
reduce latency, increase throughput, minimize allocations and we run
heavily concurrent. Here we also process untrusted, maybe malicious
input.
