A backend is the last part of a route and will define the backend to
call for a given request that match the route.

Generic route example:

```
routeID: Predicate1 && Predicate2 -> filter1 -> filter2 -> <backend>;
```


## Network backend

A network backend is an arbitrary HTTP or HTTPS URL, that will be
called by the proxy.

Route example with a network backend `"https://www.zalando.de/"`:
```
r0: Method("GET")
    -> setRequestHeader("X-Passed-Skipper", "true")
    -> "https://www.zalando.de/";
```

Proxy example with request in access log
```sh
./bin/skipper -inline-routes 'r0: Method("GET") -> setRequestHeader("X-Passed-Skipper", "true") -> "https://www.zalando.de/";'
[APP]INFO[0000] Expose metrics in codahale format
[APP]INFO[0000] support listener on :9911
[APP]INFO[0000] proxy listener on :9090
[APP]INFO[0000] TLS settings not found, defaulting to HTTP
[APP]INFO[0000] route settings, reset, route: r0: Method("GET") -> setRequestHeader("X-Passed-Skipper", "true") -> "https://www.zalando.de/"
[APP]INFO[0000] route settings received
[APP]INFO[0000] route settings applied

::1 - - [05/Feb/2019:14:31:05 +0100] "GET / HTTP/1.1" 200 164408 "-" "curl/7.49.0" 457 localhost:9090 - -
```

Client example with request and response headers:
```sh
$ curl -v localhost:9090 >/dev/null
* Rebuilt URL to: localhost:9090/
*   Trying ::1...
* Connected to localhost (::1) port 9090 (#0)
> GET / HTTP/1.1
> Host: localhost:9090
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 200 OK
< Cache-Control: no-cache, no-store, must-revalidate
< Content-Type: text/html
< Date: Tue, 05 Feb 2019 13:31:38 GMT
< Link: <https://mosaic01.ztat.net/base-assets/require-2.1.22.min.js>; rel="preload"; as="script"; nopush; crossorigin
< Pragma: no-cache
< Server: Skipper
< Set-Cookie: ...; Path=/; Domain=zalando.de; Expires=Sun, 04 Aug 2019 13:31:38 GMT; Max-Age=15552000; HttpOnly; Secure
< Vary: Accept-Encoding
< Transfer-Encoding: chunked
<
{ [3205 bytes data]
```

## Shunt backend

A shunt backend, `<shunt>`, will not call a backend, but reply directly from the
proxy itself. This can be used to shortcut, for example have a default
that replies with 404 or use skipper as a backend serving static
content in demos.

Route Example proxying to `"https://www.zalando.de/"` when Host
header is set to `"zalando"` and rest will be served HTTP status code
`404` with the body `"no matching route"`:

```
r0: Host("zalando")
    -> "https://www.zalando.de/";
rest: *
      -> status(404)
      -> inlineContent("no matching route")
      -> <shunt>;
```

Proxy configured as defined above with access log showing 404:
```sh
$ ./bin/skipper -inline-routes 'r0: Host("zalando") -> "https://www.zalando.de/"; rest: * -> status(404) -> inlineContent("no matching route")  -> "http://localhost:9999/";'
[APP]INFO[0000] Expose metrics in codahale format
[APP]INFO[0000] support listener on :9911
[APP]INFO[0000] proxy listener on :9090
[APP]INFO[0000] TLS settings not found, defaulting to HTTP
[APP]INFO[0000] route settings, reset, route: r0: Host(/zalando/) -> "https://www.zalando.de/"
[APP]INFO[0000] route settings, reset, route: rest: * -> status(404) -> inlineContent("no matching route") -> "http://localhost:9999/"
[APP]INFO[0000] route settings received
[APP]INFO[0000] route settings applied
::1 - - [05/Feb/2019:14:39:26 +0100] "GET / HTTP/1.1" 404 17 "-" "curl/7.49.0" 0 localhost:9090 - -

```

Client example with request and response headers:
```sh
$ curl -sv localhost:9090
* Rebuilt URL to: localhost:9090/
*   Trying ::1...
* Connected to localhost (::1) port 9090 (#0)
> GET / HTTP/1.1
> Host: localhost:9090
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 404 Not Found
< Content-Length: 17
< Content-Type: text/plain; charset=utf-8
< Server: Skipper
< Date: Tue, 05 Feb 2019 13:37:27 GMT
<
* Connection #0 to host localhost left intact
no matching route
```

## Loopback backend

The loopback backend, `<loopback>`, will lookup again the routing
table to a better matching route after processing the current route.
Like this you can add some headers or change the request path for some
specific matching requests.

Example:

- Route `r0` is a route with loopback backend that will be matched for requests with paths that start with `/api`. The route will modify the http request removing /api in the path of the incoming request. In the second step of the routing, the modified request will be matched by route `r1`.
- Route `r1` is a default route with a network backend to call `"https://www.zalando.de/"`

```
r0: PathSubtree("/api")
    -> modPath("^/api", "")
    -> <loopback>;
r1: * -> "https://www.zalando.de/";
```

Proxy configured as defined above with access logs showing 404 for the first call and 200 for the second:
```sh
$ ./bin/skipper -inline-routes 'r0: PathSubtree("/api") -> setRequestHeader("X-Passed-Skipper", "true") -> modPath(/^\/api/, "") -> <loopback>;
r1: * -> "https://www.zalando.de/";'
[APP]INFO[0000] Expose metrics in codahale format
[APP]INFO[0000] route settings, reset, route: r0: PathSubtree("/api") -> setRequestHeader("X-Passed-Skipper", "true") -> modPath("^/api", "") -> <loopback>
[APP]INFO[0000] route settings, reset, route: r1: * -> "https://www.zalando.de/"
[APP]INFO[0000] route settings received
[APP]INFO[0000] route settings applied
[APP]INFO[0000] support listener on :9911
[APP]INFO[0000] proxy listener on :9090
[APP]INFO[0000] TLS settings not found, defaulting to HTTP
::1 - - [05/Feb/2019:14:54:14 +0100] "GET /api/foo HTTP/1.1" 404 98348 "-" "curl/7.49.0" 562 localhost:9090 - -
::1 - - [05/Feb/2019:14:54:28 +0100] "GET /api HTTP/1.1" 200 164408 "-" "curl/7.49.0" 120 localhost:9090 - -
```

Client example with request and response headers:
```sh
$ curl -sv localhost:9090/api/foo >/dev/null
*   Trying ::1...
* Connected to localhost (::1) port 9090 (#0)
> GET /api/foo HTTP/1.1
> Host: localhost:9090
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 404 Not Found
< Content-Language: de-DE
< Content-Type: text/html;charset=UTF-8
< Date: Tue, 05 Feb 2019 14:00:33 GMT
< Transfer-Encoding: chunked
<
{ [2669 bytes data]
* Connection #0 to host localhost left intact


$ curl -sv localhost:9090/api >/dev/null
*   Trying ::1...
* Connected to localhost (::1) port 9090 (#0)
> GET /api HTTP/1.1
> Host: localhost:9090
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 200 OK
< Cache-Control: no-cache, no-store, must-revalidate
< Content-Type: text/html
< Date: Tue, 05 Feb 2019 14:00:44 GMT
< Link: <https://mosaic01.ztat.net/base-assets/require-2.1.22.min.js>; rel="preload"; as="script"; nopush; crossorigin
< Transfer-Encoding: chunked
<
{ [3491 bytes data]

```

If the request processing reaches the maximum number of loopbacks (by default max=9), the routing will
result in an error.

## Dynamic backend

The dynamic backend, `<dynamic>`, will get the backend to call by data
provided by filters.  This allows skipper as library users to do proxy
calls to a certain target from their own implementation dynamically
looked up by their filters.

Example shows how to set a target by a provided filter, which would be similar to a network backend:

```
r0: * -> setDynamicBackendUrl("https://www.zalando.de") -> <dynamic>;
```

Proxy configured as defined above with access logs showing 200 for the  call:
```sh
$ ./bin/skipper -inline-routes 'r0: * -> setDynamicBackendUrl("https://www.zalando.de") -> <dynamic>;'
[APP]INFO[0000] Expose metrics in codahale format
[APP]INFO[0000] support listener on :9911
[APP]INFO[0000] proxy listener on :9090
[APP]INFO[0000] TLS settings not found, defaulting to HTTP
[APP]INFO[0000] route settings, reset, route: r0: * -> setDynamicBackendUrl("https://www.zalando.de") -> <dynamic>
[APP]INFO[0000] route settings received
[APP]INFO[0000] route settings applied
::1 - - [05/Feb/2019:15:09:34 +0100] "GET / HTTP/1.1" 200 164408 "-" "curl/7.49.0" 132 localhost:9090 - -
```

Client example with request and response headers:
```
$ curl -sv localhost:9090/ >/dev/null
*   Trying ::1...
* Connected to localhost (::1) port 9090 (#0)
> GET / HTTP/1.1
> Host: localhost:9090
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 200 OK
< Cache-Control: no-cache, no-store, must-revalidate
< Content-Type: text/html
< Date: Tue, 05 Feb 2019 14:09:34 GMT
< Link: <https://mosaic01.ztat.net/base-assets/require-2.1.22.min.js>; rel="preload"; as="script"; nopush; crossorigin
< Pragma: no-cache
< Server: Skipper
< Transfer-Encoding: chunked
<
{ [3491 bytes data]
* Connection #0 to host localhost left intact
```

When no filters modifying the target are set (e.g. `r0: * -> <dynamic>;`), the
target host defaults to either the `Host` header or the host name given in the
URL, and the target scheme defaults to either `https` when TLS is
configured or `http` when TLS is not configured.

## Forward backend

The forward backend, `<forward>`, will set the backend to operators
choice set by `-forward-backend-url`.  This can be useful for data plane
migrations. In one case we want to switch from one Kubernetes cluster
to another Kubernetes cluster, but both cluster data planes can reach
each other. The route with the `<forward>` will get cleaned all
filters so there is no duplicate filter execution. This is useful
because old and new clusters will have the same routing objects, that
apply a set of routes with filters. Filters, for example `modPath`, can
create an unexpected change to requests and responses passing through a
chain of proxies with duplicated routes.

Example:
```
old> skipper -inline-routes='r: * -> modPath("^/", "/foo/") -> <forward>' -address :9090 -forward-backend-url=http://127.0.0.1:9003
new> skipper -inline-routes='r: * -> modPath("^/","/foo/") -> "http://127.0.0.1:12345"' -address :9003

% nc -l 12345
GET /foo/bar?q=a HTTP/1.1
Host: 127.0.0.1:12345
User-Agent: curl/7.49.0
Accept: */*
Accept-Encoding: gzip

curl http://localhost:9090/bar\?q\=a -v
*   Trying ::1...
* Connected to localhost (::1) port 9090 (#0)
> GET /bar?q=a HTTP/1.1
> Host: localhost:9090
> User-Agent: curl/7.49.0
> Accept: */*
```

You can see our netcat (nc) backend observes `/foo/bar` as path and
not `/foo/bar/bar`, if filters would be applied in the route with
`<forward>`.

## Load Balancer backend

The loadbalancer backend, `<$algorithm, "backend1", "backend2">`, will
balance the load across all given backends using the algorithm set in
`$algorithm`. If `$algorithm` is not specified it will use the default
algorithm set by Skipper at start.

Current implemented algorithms:

- `roundRobin`: backend is chosen by the round robin algorithm, starting with a random selected backend to spread across all backends from the beginning
- `random`: backend is chosen at random
- `consistentHash`: backend is chosen by [consistent hashing](https://en.wikipedia.org/wiki/Consistent_hashing) algorithm based on the request key. The request key is derived from `X-Forwarded-For` header or request remote IP address as the fallback. Use [`consistentHashKey`](filters.md#consistenthashkey) filter to set the request key. Use [`consistentHashBalanceFactor`](filters.md#consistenthashbalancefactor) to prevent popular keys from overloading a single backend endpoint.
- `powerOfRandomNChoices`: backend is chosen by powerOfRandomNChoices algorithm with selecting N random endpoints and picking the one with least outstanding requests from them. (http://www.eecs.harvard.edu/~michaelm/postscripts/handbook2001.pdf)
- __TODO__: https://github.com/zalando/skipper/issues/557

All algorithms except `powerOfRandomNChoices` support [fadeIn](filters.md#fadein) filter.

Route example with 2 backends and the `roundRobin` algorithm:
```
r0: * -> <roundRobin, "http://127.0.0.1:9998", "http://127.0.0.1:9997">;
```

Route example with 2 backends and the `random` algorithm:
```
r0: * -> <random, "http://127.0.0.1:9998", "http://127.0.0.1:9997">;
```

Route example with 2 backends and the `consistentHash` algorithm:
```
r0: * -> <consistentHash, "http://127.0.0.1:9998", "http://127.0.0.1:9997">;
```

Route example with 2 backends and the `powerOfRandomNChoices` algorithm:
```
r0: * -> <powerOfRandomNChoices, "http://127.0.0.1:9998", "http://127.0.0.1:9997">;
```

Proxy with `roundRobin` loadbalancer and two backends:
```sh
$ ./bin/skipper -inline-routes 'r0: *  -> <roundRobin, "http://127.0.0.1:9998", "http://127.0.0.1:9997">;'
[APP]INFO[0000] Expose metrics in codahale format
[APP]INFO[0000] route settings, reset, route: r0: * -> <roundRobin, "http://127.0.0.1:9998", "http://127.0.0.1:9997">
[APP]INFO[0000] support listener on :9911
[APP]INFO[0000] route settings received
[APP]INFO[0000] proxy listener on :9090
[APP]INFO[0000] TLS settings not found, defaulting to HTTP
[APP]INFO[0000] route settings applied
::1 - - [05/Feb/2019:15:39:06 +0100] "GET / HTTP/1.1" 200 1 "-" "curl/7.49.0" 3 localhost:9090 - -
::1 - - [05/Feb/2019:15:39:07 +0100] "GET / HTTP/1.1" 200 1 "-" "curl/7.49.0" 0 localhost:9090 - -
::1 - - [05/Feb/2019:15:39:08 +0100] "GET / HTTP/1.1" 200 1 "-" "curl/7.49.0" 0 localhost:9090 - -
::1 - - [05/Feb/2019:15:39:09 +0100] "GET / HTTP/1.1" 200 1 "-" "curl/7.49.0" 0 localhost:9090 - -
```

Backend1 returns "A" in the body:
```sh
$ ./bin/skipper -address=":9998" -inline-routes 'r0: * -> inlineContent("A") -> <shunt>;'
[APP]INFO[0000] Expose metrics in codahale format
[APP]INFO[0000] support listener on :9911
[APP]INFO[0000] proxy listener on :9998
[APP]INFO[0000] TLS settings not found, defaulting to HTTP
[APP]INFO[0000] route settings, reset, route: r0: * -> inlineContent("A") -> <shunt>
[APP]INFO[0000] route settings received
[APP]INFO[0000] route settings applied
127.0.0.1 - - [05/Feb/2019:15:39:06 +0100] "GET / HTTP/1.1" 200 1 "-" "curl/7.49.0" 0 127.0.0.1:9998 - -
127.0.0.1 - - [05/Feb/2019:15:39:08 +0100] "GET / HTTP/1.1" 200 1 "-" "curl/7.49.0" 0 127.0.0.1:9998 - -
```

Backend2 returns "B" in the body:
```sh
$ ./bin/skipper -address=":9997" -inline-routes 'r0: * -> inlineContent("B") -> <shunt>;'
[APP]INFO[0000] Expose metrics in codahale format
[APP]INFO[0000] support listener on :9911
[APP]INFO[0000] proxy listener on :9997
[APP]INFO[0000] route settings, reset, route: r0: * -> inlineContent("B") -> <shunt>
[APP]INFO[0000] TLS settings not found, defaulting to HTTP
[APP]INFO[0000] route settings received
[APP]INFO[0000] route settings applied
127.0.0.1 - - [05/Feb/2019:15:39:07 +0100] "GET / HTTP/1.1" 200 1 "-" "curl/7.49.0" 0 127.0.0.1:9997 - -
127.0.0.1 - - [05/Feb/2019:15:39:09 +0100] "GET / HTTP/1.1" 200 1 "-" "curl/7.49.0" 0 127.0.0.1:9997 - -
```

Client:
```sh
$ curl -s http://localhost:9090/
A
$ curl -s http://localhost:9090/
B
$ curl -s http://localhost:9090/
A
$ curl -s http://localhost:9090/
B
```

## Backend Protocols

Current implemented protocols:

- `http`: (default) http protocol
- `fastcgi`: (*experimental*) directly connect Skipper with a FastCGI backend like PHP FPM.

Route example that uses FastCGI (*experimental*):
```
php: * -> setFastCgiFilename("index.php") -> "fastcgi://127.0.0.1:9000";
php_lb: * -> setFastCgiFilename("index.php") -> <roundRobin, "fastcgi://127.0.0.1:9000", "fastcgi://127.0.0.1:9001">;
```
