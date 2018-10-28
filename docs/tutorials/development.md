## Docs

We have user documentation and developer documentation separated.
In `docs/` you find the user documentation in [mkdocs](TODO) format and
rendered at [https://opensource.zalando.com/skipper](https://opensource.zalando.com/skipper).
Developer documentation for skipper as library users
[godoc format](TODO) is used and rendered at [https://godoc.org/github.com/zalando/skipper](https://godoc.org/github.com/zalando/skipper).

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

A filter consists of at least two types a `spec` and a `filter`.
Spec consists of everything that is needed and known before a user
will instantiate a filter.

A spec will be created in the bootstrap procedure of a skipper
process. A spec has to satisfy the `Spec` interface `Name() string` and
`CreateFilter([]interface{}) (filters.Filter, error)`.

The actual filter implementation has to satisfy the `Filter`
interface `Request(filters.FilterContext)` and `Response(filters.FilterContext)`.
If you need to clean up for example a goroutine you can do it in
`Close()`, which will be called on filter shutdown.

Find a detailed example at [how to develop a Filter](/skipper/reference/development#how-to-develop-a-filter).

## Predicates

__TODO__

## Dataclients

__TODO__

## Opentracing

__TODO__

## Core

__TODO__
