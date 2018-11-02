## Docs

We have user documentation and developer documentation separated.
In `docs/` you find the user documentation in [mkdocs](TODO) format and
rendered at [https://opensource.zalando.com/skipper](https://opensource.zalando.com/skipper).
Developer documentation for skipper as library users
[godoc format](https://blog.golang.org/godoc-documenting-go-code) is used and rendered at [https://godoc.org/github.com/zalando/skipper](https://godoc.org/github.com/zalando/skipper).

### User documentation

#### local Preview

To see rendered documentation locally you need to replace `/skipper`
path with `/` to see them correctly. This you can easily do with
skipper in front of `mkdocs serve`. The following skipper inline route
will do this for you, assuming that you build skipper with `make skipper`:

```
./bin/skipper -inline-routes 'r: * -> modPath("/skipper", "") -> "http://127.0.0.1:8000"'
```

Now you should be able to see the documentation at [http://127.0.0.1:9090](http://127.0.0.1:9090).

## Filters

Filters allow to change arbitrary HTTP data in the Request or
Response. If you need to read and write the http.Body, please make
sure you discuss the use case before creating a pull request.

A filter consists of at least two types a `filters.Spec` and a `filters.Filter`.
Spec consists of everything that is needed and known before a user
will instantiate a filter.

A spec will be created in the bootstrap procedure of a skipper
process. A spec has to satisfy the `filters.Spec` interface `Name() string` and
`CreateFilter([]interface{}) (filters.Filter, error)`.

The actual filter implementation has to satisfy the `filter.Filter`
interface `Request(filters.FilterContext)` and `Response(filters.FilterContext)`.
If you need to clean up for example a goroutine you can do it in
`Close()`, which will be called on filter shutdown.

The simplest filter possible is, if `filters.Spec` and
`filters.Filter` are the same type:

```
type myFilter struct{}

func NewMyFilter() filters.Spec {
	return &myFilter{}
}

func (spec *myFilter) Name() string { return "myFilter" }

func (spec *myFilter) CreateFilter(config []interface{}) (filters.Filter, error) {
     return NewMyFilter(), nil
}

func (f *myFilter) Request(ctx filters.FilterContext) {
     // change data in ctx.Request() for example
}

func (f *myFilter) Response(ctx filters.FilterContext) {
     // change data in ctx.Response() for example
}
```

Find a detailed example at [how to develop a filter](/skipper/reference/development#how-to-develop-a-filter).

## Predicates

Predicates allow to match a condition, that can be based on arbitrary
HTTP data in the Request. There are also predicates, that use a chance
`Traffic()` or the current local time, for example `After()`, to match
a request and do not use the HTTP data at all.

A predicate consists of at least two types `routing.Predicate`
and `routing.PredicateSpec`, which are both interfaces.

A spec will be created in the bootstrap procedure of a skipper
process. A spec has to satisfy the `routing.PredicateSpec` interface
`Name() string` and `Create([]interface{}) (routing.Predicate, error)`.

The actual predicate implementation has to satisfy the
`routing.Predicate` interface `Match(*http.Request) bool` and returns
true if the predicate matches the request. If false is returned, the
routing table will be searched for another route that might match the
given request.

The simplest possible predicate implementation is, if `routing.PredicateSpec` and
`routing.Predicate` are the same type:

```
type myPredicate struct{}

func NewMyPredicate() routing.PredicateSpec {
	return &myPredicate{}
}

func (spec *myPredicate) Name() string { return "myPredicate" }

func (spec *myPredicate) Create(config []interface{}) (routing.Predicate, error) {
     return NewMyPredicate(), nil
}

func (f *myPredicate) Match(r *http.Request) bool {
     // match data in *http.Request for example
     return true
}
```

Predicates are quite similar to implement as Filters, so for a more
complete example, find an example [how to develop a filter](/skipper/reference/development#how-to-develop-a-filter).

## Dataclients

Dataclients are the way how to integrate new route
sources. Dataclients pull information from a source and create routes
for skipper's routing table.

You have to implement `routing.DataClient`, which is an interface that defines
function signatures `LoadAll() ([]*eskip.Route, error)` and
`LoadUpdate() ([]*eskip.Route, []string, error)`.

The `LoadUpdate()` method can be implemented either in a way that
returns immediately, or blocks until there is a change. The routing
package will regularly call the `LoadUpdate()` method with a small
delay between the calls.

A complete example is the [routestring implementation](https://github.com/zalando/skipper/blob/master/dataclients/routestring/string.go), which fits in
less than 50 lines of code.

## Opentracing

Your custom Opentracing implementations need to satisfy the `opentracing.Tracer` interface from
https://github.com/opentracing/opentracing-go and need to be loaded as
a plugin, which might change in the future.
Please check the [tracing package](https://github.com/zalando/skipper/blob/master/tracing)
and ask for further guidance in our [community channels](https://github.com/zalando/skipper#community).

## Core

Non trivial changes, proposals and enhancements to the core of skipper
should be discussed first in a Github issue, such that we can think
about how this fits best in the project and how to achieve the most
useful result. Feel also free to reach out to our [community
channels](https://github.com/zalando/skipper#community) and discuss
there your idea.

Every change in core has to have tests included and should be a non
breaking change. We planned since a longer time a breaking change, but
we should coordinate to make it as good as possible for all skipper as
library users. Most often a breaking change can be postponed to the
future and a feature independently added and the old feature might be
deprecated to delete it later. Use of deprecated features should be shown
in logs with a `log.Warning`.
