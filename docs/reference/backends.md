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
```
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
```
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

Route Example proxying to `"https://www.zalando.de/"` in case Host
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
```
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
```
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

- Route `r0` is a route with loopback backend that will be matched for requests with paths that start with `/api`. The route will modify the http request removing /api in the path of the incoming request. In the second step of the routing the modified request will be matched by route `r1`.
- Route `r1` is a default route with a network backend to call `"https://www.zalando.de/"`

```
r0: PathSubtree("/api")
    -> modPath("^/api", "")
    -> <loopback>;
r1: * -> "https://www.zalando.de/";
```

Proxy configured as defined above with access logs showing 404 for the first call and 200 for the second:
```
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
```
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
```
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

## Load Balancer backend

The loadbalancer backend, `<$algorithm, "backend1", "backend2">`, will
balance the load across all given backends using the algorithm set in
`$algorithm`. If `$algorithm` is not specified it will use the default
algorithm set by Skipper at start.

Current implemented algorithms:

- `roundRobin`: backend is chosen by the round robin algorithm, starting with a random selected backend to spread across all backends from the beginning
- `random`: backend is chosen at random
- `consistentHash`: backend is chosen by a consistent hashing algorithm with the client remote IP as input to the hash function
- __TODO__: https://github.com/zalando/skipper/issues/557

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

Proxy with `roundRobin` loadbalancer and two backends:
```
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
```
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
```
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
```
$ curl -s http://localhost:9090/
A
$ curl -s http://localhost:9090/
B
$ curl -s http://localhost:9090/
A
$ curl -s http://localhost:9090/
B
```
