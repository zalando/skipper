# Skipper Predicates

The parameters can be strings, regex or float64 / int

* `string` is a string surrounded by double quotes (`"`)
* `regex` is a [re2 regular expression](https://github.com/google/re2/wiki/Syntax), surrounded by `/`, e.g. `/^www\.example\.org(:\d+)?$/`
* `int` / `float64` are usual (decimal) numbers like `401` or `1.23456`
* `time` is a string in double quotes, parseable by [time.Duration](https://godoc.org/time#ParseDuration))

Predicates are a generic tool and can change the route matching behavior.
Predicates can be chained using the double ampersand operator `&&`.

Example route with a Host, Method and Path match predicates and a backend:

```
all: Host(/^my-host-header\.example\.org$/) && Method("GET") && Path("/hello") -> "http://127.0.0.1:1234/";
```

## Path

The route definitions may contain a single path condition, optionally
with wildcards, used for looking up routes in the lookup tree.

Parameters:

* Path (string) can contain a wildcard `*` or a named `:wildcard`

Examples:

```
Path("/foo/bar")
Path("/foo/:bar")
Path("/foo*")
Path("/foo/*")
Path("/foo/**")
```

## PathSubtree

Similar to Path, but used to match full subtrees including the path of
the definition.

PathSubtree("/foo") predicate is equivalent to having routes with
Path("/foo"), Path("/foo/") and Path("/foo/**") predicates.

Parameters:

* PathSubtree (string)

Examples:

```
PathSubtree("/foo/bar")
PathSubtree("/")
PathSubtree("/foo*")
```

## PathRegexp

Regular expressions to match the path. It uses Go's standard library
regexp package to match, which is based on
[re2 regular expression syntax](https://github.com/google/re2/wiki/Syntax).

Parameters:

* PathRegexp (regex)

Examples:

```
PathRegexp("^/foo/bar")
PathRegexp("/foo/bar$")
PathRegexp("/foo/bar/")
PathRegexp("^/foo/(bar|qux)")
```

## Host

Regular expressions that the host header in the request must match.

Parameters:

* Host (regex)

Examples:

```
Host(/^my-host-header\.example\.org$/)
Host(/header\.example\.org$/)
```

## Method

The HTTP method that the request must match. HTTP methods are one of
GET, HEAD, PATCH, POST, PUT, DELETE, OPTIONS, CONNECT.

Parameters:

* Method (string)

Examples:

```
Method("GET")
Method("OPTIONS")
```

## Header

A header key and exact value that must be present in the request. Note
that Header("Key", "Value") is equivalent to HeaderRegexp("Key", "^Value$").

Parameters:

* Header (string, string)

Examples:

```
Header("X-Forwarded-For", "192.168.0.2")
Header("Accept", "application/json")
```

## HeaderRegexp

A header key and a regular expression, where the key must be present
in the request and one of the associated values must match the
expression.

Parameters:

* HeaderRegexp (string, regex)

Examples:

```
HeaderRegexp("X-Forwarded-For", "^192\.168\.0\.[0-2]?[0-9]?[0-9] ")
HeaderRegexp("Accept", "application/(json|xml)")
```

## Cookie

Matches if the specified cookie is set in the request.

Parameters:

* Cookie (string, regex) name and value match

Examples:

```
Cookie("alpha", /^enabled$/)
```

## Auth

Authorization header based match.

### JWTPayloadAnyKV

Match the route if at least one of the base64 decoded JWT content
matches the key value configuration.

Parameters:

* Key-Value pairs (...string), odd index is the key of the JWT
  content and even index is the value of the JWT content

Examples:

```
JWTPayloadAnyKV("iss", "https://accounts.google.com")
JWTPayloadAnyKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
```

### JWTPayloadAllKV

Match the route if all of the base64 decoded JWT content
matches the key value configuration.

Parameters:

* Key-Value pairs (...string), odd index is the key of the JWT
  content and even index is the value of the JWT content

Examples:

```
JWTPayloadAllKV("iss", "https://accounts.google.com")
JWTPayloadAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
```

## Interval

An interval implements custom predicates to match routes only during some period of time.

There are three predicates: Between, Before and After. All
predicates can be created using the date represented as a string in
RFC3339 format (see https://golang.org/pkg/time/#pkg-constants), int64
or float64 number. float64 number will be converted into int64 number.

### After

Matches if the request is after the specified time

Parameters:

* After (string) date string
* After (int) unixtime

Examples:

```
After("2016-01-01T12:00:00+02:00")
After(1451642400)
```

### Before

Matches if the request is before the specified time

Parameters:

* Before (string) date string
* Before (int) unixtime

Examples:

```
Before("2016-01-01T12:00:00+02:00")
Before(1451642400)
```
### Between

Matches if the request is between the specified timeframe

Parameters:

* Between (string, string) date string, from - till
* Between (int, int) unixtime, from - till

Examples:

```
Between("2016-01-01T12:00:00+02:00", "2016-02-01T12:00:00+02:00")
Between(1451642400, 1454320800)
```

## QueryParam

Match request based on the Query Params in URL

Parameters:

* QueryParam (string) name
* QueryParam (string, regex) name and value match

Examples:

```
// matches http://example.org?bb=a&query=withvalue
QueryParam("query")

// Even a query param without a value
// matches http://example.org?bb=a&query=
QueryParam("query")

// matches with regexp
// matches http://example.org?bb=a&query=example
QueryParam("query", "^example$")

// matches with regexp and multiple values of query param
// matches http://example.org?bb=a&query=testing&query=example
QueryParam("query", "^example$")
```

## Source

Source implements a custom predicate to match routes based on
the source IP or X-Forwarded-For header of a request.

Parameters:

* Source (string, ..) varargs with IPs or CIDR

Examples:

```
// only match requests from 1.2.3.4
Source("1.2.3.4")

// only match requests from 1.2.3.0 - 1.2.3.255
Source("1.2.3.0/24")

// only match requests from 1.2.3.4 and the 2.2.2.0/24 network
Source("1.2.3.4", "2.2.2.0/24")
```

### SourceFromLast

The same as [Source](#Source), but use the last part of the
X-Forwarded-For header to match the network. This seems to be only
used in the popular loadbalancers from AWS, ELB and ALB, because they
put the client-IP as last part of the X-Forwarded-For headers.

Parameters:

* SourceFromLast (string, ..) varargs with IPs or CIDR

Examples:

```
SourceFromLast("1.2.3.4", "2.2.2.0/24")
```

## Traffic

Traffic implements a predicate to control the matching probability for
a given route by setting its weight.

The probability for matching a route is defined by the mandatory first
parameter, that must be a decimal number between 0.0 and 1.0 (both
exclusive).

The optional second argument is used to specify the cookie name for
the traffic group, in case you want to use stickiness. Stickiness
allows all subsequent requests from the same client to match the same
route. Stickiness of traffic is supported by the optional third
parameter, indicating whether the request being matched belongs to the
traffic group of the current route. If yes, the predicate matches
ignoring the chance argument.


Parameters:

* Traffic (decimal) valid values [0.0, 1.0]
* Traffic (decimal, string, string) session stickyness

Examples:

non-sticky:

```
// hit by 10% percent chance
v2:
    Traffic(.1) ->
    "https://api-test-green";

// hit by remaining chance
v1:
    "https://api-test-blue";
```

stickyness:

```
// hit by 5% percent chance
cartTest:
    Traffic(.05, "cart-test", "test") && Path("/cart") ->
    responseCookie("cart-test", "test") ->
    "https://cart-test";

// hit by remaining chance
cart:
    Path("/cart") ->
    responseCookie("cart-test", "default") ->
    "https://cart";

// hit by 15% percent chance
catalogTestA:
    Traffic(.15, "catalog-test", "A") ->
    responseCookie("catalog-test", "A") ->
    "https://catalog-test-a";

// hit by 30% percent chance
catalogTestB:
    Traffic(.3, "catalog-test", "B") ->
    responseCookie("catalog-test", "B") ->
    "https://catalog-test-b";

// hit by remaining chance
catalog:
    * ->
    responseCookie("catalog-test", "default") ->
    "https://catalog";
```
