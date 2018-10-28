## Architecture

The core business of skipper is routing based on HTTP. It performs and
scales well, for example it handles more than 300000 routes in
production with 60000 requests per second.

Skipper is written as a library and is also a multi binary project with
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
the content of the request or response.  A route always points to a
backend, or it's a `<shunt>`, meaning that skipper serves the requests
for the route, or a `<loopback>`, meaning that the request will be
matched against the routing table again after filters have modified
it.

[Opentracing API](http://opentracing.io/) is supported via
[skipper-plugins](https://github.com/skipper-plugins/opentracing). For
example [Jaeger](https://github.com/jaegertracing/jaeger) is supported.

Skipper has a rich set of metrics that are exposed as json, but can be
exported in [Prometheus](https://prometheus.io) format.

![Skipper's architecture ](/skipper/img/architecture.svg)

## Concepts

### Route definition

A route consists of an ID, predicates, filters and a backend and is
most often written in [eskip syntax](https://godoc.org/github.com/zalando/skipper/eskip).

Syntax:

```
ID:
        Predicate1() && .. && PredicateN()
        -> filter1()
        ...
        -> filterN()
        -> BACKEND
```

An example routing configuration:

```
baidu:
        Path("/baidu")
        -> setRequestHeader("Host", "www.baidu.com")
        -> setPath("/s")
        -> setQuery("wd", "godoc skipper")
        -> "http://www.baidu.com";
google:
        *
        -> setPath("/search")
        -> setQuery("q", "godoc skipper")
        -> "https://www.google.com";
yandex:
        * && Cookie("yandex", "true")
        -> setPath("/search/")
        -> setQuery("text", "godoc skipper")
        -> tee("http://127.0.0.1:12345/")
        -> "https://yandex.ru";
```

### Predicate

A Predicate adds a matching rule to a route.
For example the Cookie predicate, `Cookie("yandex", "true")`, matched
if there is a cookie in the request with name "yandex" and the value
is "true", else the route processing will go one and try to find
another matching route for the given request.

Special Predicates:

- `*` catch all is always true
- `Path()` reduces the number of routes
- `PathSubtree()` reduces the number of routes

### Filter

A filter changes a HTTP request or response or both.

Some special filters are:

- `inlineContent` sets the HTTP response body, should be used with status() filter and  <shunt> backend
- `static` serves static files and should be used with <shunt> backend
- `status` sets HTTP status code to a given value, should be used with <shunt> backend
- `tee()` clones request to given target


### Backend

The last entry of a route is the backend definition, that will be
called with the result request after filter processing. Normally this
is an URL string.

Special backends:

- `<loopback>` restart route processing with the possibly changed request
- `<shunt>` stops processing, used for fast returns

### Dataclient

A dataclient is used to pull route information from a data source. The
data will be used to create routes according to the dataclient. As a
special case, for example kubernetes dataclient automatically adds
HTTP->HTTPS redirects if skipper is started with `-kubernetes-https-redirect`.

Dataclients:

- [eskip-file](/skipper/data-clients/eskip-file)
- [route string](/skipper/data-clients/route-string)
- [kubernetes](/skipper/data-clients/kubernetes)
- [etcd](/skipper/data-clients/etcd)

## Route processing

Package `skipper` has a Go `http.Server` and does the `ListenAndServe`
call with the `loggingHandler` wrapped `proxy`.  The `loggingHandler`
is basically a middleware for the `proxy` providing access logs and
both implement the plain Go [http.Handler interface](https://golang.org/pkg/net/http/#Handler).

For each incoming `http.Request` the `proxy` will create a request
context and enhance it with an [Opentracing API](http://opentracing.io/) Span.
It will check proxy global ratelimits first and after that lookup the
route in the routing table. After that skipper will apply all request
filters, that can modify the `http.Request`. It will then check the
route local ratelimits, the circuitbreakers and do the backend
call. If the backend call got a TCP or TLS connection error in a
loadbalanced route, skipper will do a retry to another backend of that
loadbalanced group automatically. Just before the response to the
caller, skipper will process the response filters, that can change the
`http.Response`.

In two special cases, skipper doesn't forward the request to the
backend. When the route is shunted (`<shunt>`), skipper serves the
request alone, by using only the filters. When the route is a
`<loopback>`, the request is passed to the routing table for finding
another route, based on the changes that the filters made to the
request.

![Skipper's request and response processing ](/skipper/img/req-and-resp-processing.svg)

### Routing mechanism

The routing executes the following steps in the typical case:

1. Select the best fitting route by matching the request against the
   predicates. When no route found, respond with 404 (unless the default
   status code is configured to a different value).

2. Execute the filters defined in the route in normal order on the
   request. The filters may or may not alter the request.

3. Forward the request to the backend defined by the route and receive
   a response.

4. Execute the filters defined in the route in reverse order on the
   response. The filters may or may not alter the response.

5. Respond to the incoming request with the resulting response.

### Route matching

Skipper can handle a relatively large number of routes with acceptable
performance, while being able to use any attribute of the incoming HTTP
requests to distinguish between them. In order to be able to do so, the
path matching predicates (`Path()` and `PathSubtree()` but not `PathRegexp()`)
have a special role during route matching, which is a tradeoff by
design, and needs to be kept in mind to understand in some cases why a
certain route was matched for a request instead of another.

The route matching logic can be summed up as follows:

1. Lookup in the path tree based on the `Path()` and the `PathSubtree()`
   predicates, using the path component of the incoming request's URI. Then
   the remaining predicates of the found route(s) are evaluated.

    * the path lookup is a radix tree with O(log(n)) time complexity

    * in case of intersecting paths, the more specific path is matched in
     the tree

    * `PathRegexp()` is not used in the tree, but it is evaluated only after
     `Path()` or `PathSubtree()`, just like e.g. `Method()` or `Host()`.

2. _If_ step #1 matches multiple routes, which means there are multiple
   routes in the same position of the path tree, and all other predicates
   match the request, too, then the route with the most defined predicates
   is matched.

    * this is an O(n) lookup, but only on the same leaf

    * the root of the tree is considered a single leaf, so if not using the
      `Path()` or `PathSubtree()` predicates, the entire lookup will become O(n)
      over all the routes.

3. _If_ #2 results in multiple matching routes, then one route will be
   selected. It is unspecified which one.
