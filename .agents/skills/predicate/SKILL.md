---
name: predicate
description: Create or modify code in the predicates package and all its sub-folders
license: MIT
compatibility: opencode, claude
metadata:
  audience: maintainers, contributors
---
## What I do

- Create predicate code that is efficient and readable

## When to use me

Use this when you are writing code in sub-folders of
./predicates/. Some predicates are also located in the ./routing/
folder for historical reasons.

## Quick start

A predicate consists of 2 types `spec` and `predicate` that implement 2 different interfaces:


```go
// PredicateSpec instances are used to create custom predicates
// (of type Predicate) with concrete arguments during the
// construction of the routing tree.
type PredicateSpec interface {

	// Name of the predicate as used in the route definitions.
	Name() string

	// Creates a predicate instance with concrete arguments.
	Create([]any) (Predicate, error)
}
```

```go
// Predicate instances are used as custom user defined route
// matching predicates.
type Predicate interface {

	// Returns true if the request matches the predicate.
	Match(*http.Request) bool
}
```

The `Create([]any) (Predicate, error)` function is run at routing tree
creation time, so only once in a while. Everything we can pre-compute
to run a faster `Match(*http.Request) bool` execution the better. Here
we process input from developers or operators that might do
mistakes. We want to show errors clearly.

The `Predicate` interface function `Match(*http.Request) bool` is
running in the hotpath of the request processing. Here we need to be
very efficient and want to reduce latency, increase throughput,
minimize allocations and we run heavily concurrent. Every request will
be proceesed by a lot of `Predicates` and so each `Match` needs to be
as fast as possible.
