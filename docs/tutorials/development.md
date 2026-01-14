## Local Setup

### Build Skipper Binary

Clone repository and compile with [Go](https://golang.org/dl).

```sh
git clone https://github.com/zalando/skipper.git
cd skipper
make skipper
```

binary will be `./bin/skipper`

### Run Skipper as Proxy with 2 backends

As a small example, we show how you can run one proxy skipper and 2
backend skippers.

Start the proxy that listens on port 9999 and serves all requests with a single route, that
proxies to two backends using the round robin algorithm:
```sh
./bin/skipper -inline-routes='r1: * -> <roundRobin, "http://127.0.0.1:9001", "http://127.0.0.1:9002">' --address :9999
```

Start two backends, with similar routes, one responds with "1" and the
other with "2" in the HTTP response body:
```sh
./bin/skipper -inline-routes='r1: * -> inlineContent("1") -> <shunt>' --address :9001 &
./bin/skipper -inline-routes='r1: * -> inlineContent("2") -> <shunt>' --address :9002
```

Test the proxy with curl as a client:
```sh
curl -s http://localhost:9999/foo
1
curl -s http://localhost:9999/foo
2
curl -s http://localhost:9999/foo
1
curl -s http://localhost:9999/foo
2
```

### Debugging Skipper

It can be helpful to run Skipper in a debug session locally that enables one to inspect variables and do other debugging activities in order to analyze filter and token states.

For *Visual Studio Code* users, a simple setup could be to create following *launch configuration* that compiles Skipper, runs it in a *Delve* debug session, and then opens the default web browser creating the request. By setting a breakpoint, you can inspect the state of the filter or application. This setup is especially useful when inspecting *oauth* flows and tokens as it allows stepping through the states.

<details>
<summary>Example `.vscode/launch.json` file</summary>

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Launch Package",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/cmd/skipper/main.go",
            "args": [
                "-application-log-level=debug",
                "-address=:9999",
                "-inline-routes=PathSubtree(\"/\") -> inlineContent(\"Hello World\") -> <shunt>",
               // example OIDC setup, using https://developer.microsoft.com/en-us/microsoft-365/dev-program
               //  "-oidc-secrets-file=${workspaceFolder}/.vscode/launch.json",
               //  "-inline-routes=* -> oauthOidcAnyClaims(\"https://login.microsoftonline.com/<tenant id>/v2.0\",\"<application id>\",\"<client secret>\",\"http://localhost:9999/authcallback\", \"profile\", \"\", \"\", \"x-auth-email:claims.email x-groups:claims.groups\") -> inlineContent(\"restricted access\") -> <shunt>",
            ],
            "serverReadyAction": {
                "pattern": "route settings applied",
                "uriFormat": "http://localhost:9999",
                "action": "openExternally"
            }
        }
    ]
}
```

</details>

## Docs

We have user documentation and developer documentation separated.
In `docs/` you find the user documentation in [mkdocs](https://www.mkdocs.org/) format and
rendered at [https://opensource.zalando.com/skipper](https://opensource.zalando.com/skipper) which is updated automatically with each `docs/` change merged to `master` branch.
Developer documentation for skipper as library users
[godoc format](https://blog.golang.org/godoc-documenting-go-code) is used and rendered at [https://pkg.go.dev/github.com/zalando/skipper](https://pkg.go.dev/github.com/zalando/skipper).

### User documentation

To see rendered documentation locally run `mkdocs serve` and navigate to [http://127.0.0.1:8000](http://127.0.0.1:8000).

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

The simplest filter possible is, if `filters.Spec` and
`filters.Filter` are the same type:

```go
type myFilter struct{}

func NewMyFilter() *myFilter {
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

Find a detailed example at [how to develop a filter](../reference/development.md#how-to-develop-a-filter).

### Filters with cleanup

Sometimes your filter needs to cleanup resources on shutdown. In Go
functions that do this have often the name `Close()`.
There is the `filters.FilterCloser` interface and if you comply with
it, the routing.Route will make sure your filters are closed if
`routing.Routing` was closed.

```go
type myFilter struct{}

func NewMyFilter() *myFilter {
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

func (f *myFilter) Close() error {
     // cleanup your filter
}
```

### Filters with error handling

Sometimes you want to have a filter that wants to get called
`Response()` even if the proxy will not send a response from the
backend, for example you want to count error status codes, like
the [admissionControl](../reference/filters.md#admissioncontrol)
filter.
In this case you need to comply with the following proxy interface:

```go
// errorHandlerFilter is an opt-in for filters to get called
// Response(ctx) in case of errors.
type errorHandlerFilter interface {
	// HandleErrorResponse returns true if a filter wants to get called
	HandleErrorResponse() bool
}
```

Example:
```go
type myFilter struct{}

func NewMyFilter() *myFilter {
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

func (f *myFilter) HandleErrorResponse() bool() {
     return true
}
```


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

```go
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
complete example, find an example [how to develop a filter](../reference/development.md#how-to-develop-a-filter).

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
should be discussed first in a GitHub issue, such that we can think
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
