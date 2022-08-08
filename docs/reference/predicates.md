# Skipper Predicates

Predicates are used to decide which route will handle an incoming request. Routes can contain multiple
predicates. A request will match a route only if all the predicates of the route match. See the description of
the route matching mechanism here: [Route matching](../tutorials/basics.md#route-matching).

Example route with a Host, Method and Path match predicates and a backend:

```
all: Host(/^my-host-header\.example\.org$/) && Method("GET") && Path("/hello") -> "http://127.0.0.1:1234/";
```

## Predicate arguments

The predicate arguments can be strings, regular expressions or numbers (float64, int). In the eskip syntax
representation:

* strings are surrounded by double quotes (`"`). When necessary, characters can be escaped by `\`, e.g. `\\` or `\"`.
* regular expressions are a [re2 regular expression](https://github.com/google/re2/wiki/Syntax), surrounded by
* `/`, e.g. `/^www\.example\.org(:\d+)?$/`. When a predicate expects a regular expression as an argument, the string representation with double quotes can be used, as well.
* numbers are regular (decimal) numbers like `401` or `1.23456`. The eskip syntax doesn't define a limitation on the size of the numbers, but the underlying implementation currently relies on the float64 values of the Go runtime.

Other higher level argument types must be represented as one of the above types. E.g. it is a convention to
represent time duration values as strings, parseable by [time.Duration](https://godoc.org/time#ParseDuration)).

## The path tree

There is an important difference between the evaluation of the [Path](#path) or [PathSubtree](#pathsubtree) predicates, and the
evaluation of all the other predicates ([PathRegexp](#pathregexp) belonging to the second group). Find an explanation in the
[Route matching](../tutorials/basics.md#route-matching) section explanation section.

### Path

The path predicate is used to match the path in HTTP request line. It accepts a single argument, that can be a
fixed path like "/some/path", or it can contain wildcards. There can be only zero or one path predicate in a
route.

**Wildcards:**

Wildcards can be put in place of one or more path segments in the path, e.g. "/some/:dir/:name", or the path can
end with a free wildcard like `"/some/path/*param"`, where the free wildcard can match against a sub-path with
multiple segments. Note, that this solution implicitly supports the glob standard, e.g. `"/some/path/**"` will
work as expected. The wildcards must follow a `/`.

The arguments are available to the filters while processing the matched requests, but
currently only a few built-in filters utilize them, and they can be used rather only from custom filter
extensions.

**Known bug:**

There is a known bug with how predicates of the form `Path("/foo/*")` are currently handled. Note the wildcard
defined with `*` doesn't have a name here. Wildcards must have a name, but Skipper currently does not reject
these routes, resulting in undefined behavior.

**Trailing slash:**

By default, `Path("/foo")` and `Path("/foo/")` are not equivalent. Ignoring the trailing slash can be toggled
with the `-ignore-trailing-slash` command line flag.

**Examples:**

```
Path("/foo/bar")     //   /foo/bar
Path("/foo/bar/")    //   /foo/bar/, unless started with -ignore-trailing-slash
Path("/foo/:id")     //   /foo/_anything
Path("/foo/:id/baz") //   /foo/_anything/baz
Path("/foo/*rest")   //   /foo/bar/baz
Path("/foo/**")      //   /foo/bar/baz
```

### PathSubtree

The path subtree predicate behaves similar to the path predicate, but it matches the exact path in the
definition and any sub path below it. The subpath is automatically provided among the path parameters with the
name `*`. If a free wildcard is appended to the definition, e.g. `PathSubtree("/some/path/*rest")`, the free
wildcard name is used instead of `*`. The simple wildcards behave similar to the Path predicate. The main
difference between `PathSubtree("/foo")` and `Path("/foo/**")` is that the PathSubtree predicate always ignores
the trailing slashes.

**Examples:**

```
PathSubtree("/foo/bar")
PathSubtree("/")
PathSubtree("/foo/*rest")
```

### PathRegexp

Regular expressions to match the path. It uses Go's standard library
regexp package to match, which is based on
[re2 regular expression syntax](https://github.com/google/re2/wiki/Syntax).

Parameters:

* PathRegexp (regex)

A route can contain more than one PathRegexp predicates. It can be also used in combination with the Path
predicate.

```
Path("/colors/:name/rgb-value") && PathRegexp("^/colors/(red|green|blue|cyan|magenta|pink|yellow)/")
-> returnRGB()
-> <shunt>
```

Further examples:

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

## HostAny

Evaluates to true if request host exactly equals to any of the configured hostnames.

Parameters:

* hostnames (string)

Examples:

```
HostAny("www.example.org", "www.example.com")
HostAny("localhost:9090")
```

## Forwarded header predicates

Uses standardized Forwarded header ([RFC 7239](https://tools.ietf.org/html/rfc7239))

More info about the header: [MDN](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Forwarded)

If multiple proxies chain values in the header, as a comma separated list, the predicates below will only match
the last value in the chain for each part of the header.

Example: Forwarded: host=example.com;proto=https, host=example.org

- `ForwardedHost(/^example\.com$/)` - does not match
- `ForwardedHost(/^example\.org$/)` - matches
- `ForwardedHost(/^example\.org$/) && ForwardedProto("https")` - matches
- `ForwardedHost(/^example\.com$/) && ForwardedProto("https")` - does not match


### ForwardedHost

Regular expressions that the forwarded host header in the request must match.

Parameters:

* Host (regex)

Examples:

```
ForwardedHost(/^my-host-header\.example\.org$/)
ForwardedHost(/header\.example\.org$/)
```

### ForwardedProtocol

Protocol the forwarded header in the request must match.

Parameters:

* Protocol (string)

Only "http" and "https" values are allowed

Examples:

```
ForwardedProtocol("http")
ForwardedProtocol("https")
```

## Weight

By default, the weight (priority) of a route is determined by the number of defined predicates.

If you want to give a route more priority, you can give it more weight.

Parameters:

* Weight (int)

Example where `route2` has more priority because it has more predicates:

```
route1: Path("/test") -> "http://www.zalando.de";
route2: Path("/test") && True() -> "http://www.zalando.de";
```

Example where `route1` has more priority because it has more weight:

```
route1: Path("/test") && Weight(100) -> "http://www.zalando.de";
route2: Path("/test") && True() && True() -> "http://www.zalando.de";
```

## True

Does always match. Before `Weight` predicate existed this was used to give a route more weight.

Example where `route2` has more weight.

```
route1: Path("/test") -> "http://www.zalando.de";
route2: Path("/test") && True() -> "http://www.github.com";
```

## False

Does not match. Can be used to disable certain routes.

Example where `route2` is disabled.

```
route1: Path("/test") -> "http://www.zalando.de";
route2: Path("/test") && False() -> "http://www.github.com";
```

## Shutdown

Evaluates to true if Skipper is shutting down. Can be used to create customized healthcheck.

```
health_up: Path("/health") -> inlineContent("OK") -> <shunt>;
health_down: Path("/health") && Shutdown() -> status(503) -> inlineContent("shutdown") -> <shunt>;
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

## Methods

The HTTP method that the request must match. HTTP methods are one of
GET, HEAD, PATCH, POST, PUT, DELETE, OPTIONS, CONNECT, TRACE.

Parameters:

* Method (...string) methods names

Examples:

```
Methods("GET")
Methods("OPTIONS", "POST")
Methods("OPTIONS", "POST", "patch")
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

### JWTPayloadAnyKVRegexp, JWTPayloadAllKVRegexp

Behaves exactly the same as `JWTPayloadAnyKV`, `JWTPayloadAllKV`,
but the expected values are regular expressions that will be matched
against the JWT value.

Examples:

```
JWTPayloadAllKVRegexp("iss", "^https://")
JWTPayloadAnyKVRegexp("iss", "^https://")
```

## Interval

An interval implements custom predicates to match routes only during some period of time.

There are three predicates: Between, Before and After. All
predicates can be created using the date represented as:
* a string in RFC3339 format (see https://golang.org/pkg/time/#pkg-constants)
* a string in RFC3339 format without numeric timezone offset and a location name corresponding to a file in the IANA Time Zone database
* an `int64` or `float64` number corresponding to the given Unix time in seconds since January 1, 1970 UTC. `float64` number will be converted into `int64` number

### After

Matches if the request is after the specified time

Parameters:

* After (string) RFC3339 datetime string
* After (string, string) RFC3339 datetime string without timezone offset, location name
* After (int) unixtime in seconds

Examples:

```
After("2016-01-01T12:00:00+02:00")
After("2021-02-18T00:00:00", "Europe/Berlin")
After(1451642400)
```

### Before

Matches if the request is before the specified time

Parameters:

* Before (string) RFC3339 datetime string
* Before (string, string) RFC3339 datetime string without timezone offset, location name
* Before (int) unixtime in seconds

Examples:

```
Before("2016-01-01T12:00:00+02:00")
Before("2021-02-18T00:00:00", "Europe/Berlin")
Before(1451642400)
```

### Between

Matches if the request is between the specified timeframe

Parameters:

* Between (string, string) RFC3339 datetime string, from - till
* Between (string, string, string) RFC3339 datetime string without timezone offset, from - till and a location name
* Between (int, int) unixtime in seconds, from - till

Examples:

```
Between("2016-01-01T12:00:00+02:00", "2016-02-01T12:00:00+02:00")
Between("2021-02-18T00:00:00", "2021-02-18T01:00:00", "Europe/Berlin")
Between(1451642400, 1454320800)
```

## Cron

Matches routes when the given cron-like expression matches the system time.

Parameters:

* [Cron](https://en.wikipedia.org/wiki/Cron#CRON_expression)\-like expression. See [the package documentation](https://godoc.org/github.com/sarslanhan/cronmask#New) for supported & unsupported features. Expressions are expected to be in the same time zone as the system that generates the `time.Time` instances.


Examples:

```
// match everything
Cron("* * * * *")
// match only when the hour is between 5-7 (inclusive)
Cron("* 5-7, * * *")
// match only when the hour is between 5-7, equal to 8, or betweeen 12-15
Cron("* 5-7,8,12-15 * * *")
// match only when it is weekdays
Cron("* * * * 1-5")
// match only when it is weekdays & working hours
Cron("* 7-18 * * 1-5")
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

The same as [Source](#source), but use the last part of the
X-Forwarded-For header to match the network. This seems to be only
used in the popular loadbalancers from AWS, ELB and ALB, because they
put the client-IP as last part of the X-Forwarded-For headers.

Parameters:

* SourceFromLast (string, ..) varargs with IPs or CIDR

Examples:

```
SourceFromLast("1.2.3.4", "2.2.2.0/24")
```

## ClientIP

ClientIP implements a custom predicate to match routes based on
the client IP of a request.

Parameters:

* ClientIP (string, ..) varargs with IPs or CIDR

Examples:

```
// only match requests from 1.2.3.4
ClientIP("1.2.3.4")

// only match requests from 1.2.3.0 - 1.2.3.255
ClientIP("1.2.3.0/24")

// only match requests from 1.2.3.4 and the 2.2.2.0/24 network
ClientIP("1.2.3.4", "2.2.2.0/24")
```

## AnySource

Similar to [Source](#source) and [SourceFromLast](#sourcefromlast) but can
correctly work with different types of load balancers in front of Skipper.
The common case is running with AWS Network Load Balancers (NLB) and AWS Application
Load Balancers (ALB) in front of skipper. Here `SourceFromlast` will work for traffic
coming through the ALB, but not for traffic coming through the NLB as the
client could set a custom `X-Forwarded-For` header. For NLB `ClientIP` would
work, but that would again not work for ALB because the source IP of traffic
coming through ALB is the private IP of the ALB nodes.
`AnySource` solves this and allows the users to not care about the details of
the setup but simply define the list of sources they want to allow and nothing
else.

Parameters:

* AnySource (string, ..) varargs with IPs or CIDR

Examples:

```
// only match requests from 1.2.3.4
AnySource("1.2.3.4")

// only match requests from 1.2.3.0 - 1.2.3.255
AnySource("1.2.3.0/24")

// only match requests from 1.2.3.4 and the 2.2.2.0/24 network
AnySource("1.2.3.4", "2.2.2.0/24")
```

## Tee

The Tee predicate matches a route when a request is spawn from the
[teeLoopback](filters.md#teeloopback) filter as a tee request, using
the same provided label.

Parameters:

* tee label (string): the predicate will match only those requests that
  were spawn from a teeLoopback filter using the same label.

See also:

* [teeLoopback filter](filters.md#teeloopback)
* [Shadow Traffic Tutorial](../tutorials/shadow-traffic.md)

## Traffic

Traffic implements a predicate to control the matching probability for
a given route by setting its weight.

The probability for matching a route is defined by the mandatory first
parameter, that must be a decimal number between 0.0 and 1.0 (both
inclusive).

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
    * ->
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
