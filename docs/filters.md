# Skipper Filters

The parameters can be strings, regex or float64 / int

* `string` is a string surrounded by double quotes (`"`)
* `regex` is a regular expression, surrounded by `/`, e.g. `/^www\.example\.org(:\d+)?$/`
* `int` / `float64` are usual (decimal) numbers like `401` or `1.23456`

## setRequestHeader

Set headers for requests.

Parameters:
* header name (string)
* header value (string)

Example:

```
foo: * -> setRequestHeader("X-Passed-Skipper", "true") -> "https://backend.example.org";
```

## setResponseHeader

Same as [setRequestHeader](#setrequestheader), only for responses

## appendRequestHeader

Same as [setRequestHeader](#setrequestheader), does not remove a possibly existing value, but adds a new header value

## appendResponseHeader

Same as [appendRequestHeader](#appendrequestheader), only for responses

## dropRequestHeader

Removes a header from the request

Parameters:
* header name (string)

Example:

```
foo: * -> dropRequestHeader("User-Agent") -> "https://backend.example.org";
```

## dropResponseHeader

Same as [dropRequestHeader](#droprequestheader) but for responses from the backend

## healthcheck

## modPath

Replace all matched regex expressions in the path.

Parameters:
* the expression to match (regex)
* the replacement (string)

## setPath


## redirectTo

Creates an HTTP redirect response.

Parameters:
* redirect status code (int)
* location (string)

Example:

```
redir: PathRegex(/^\/foo\/bar/) -> redirectTo(302, "/foo/newBar") -> <shunt>;
```

## redirectToLower
## static

Serves static content from the filesystem.

Parameters:
* Request path to strip (string)
* Target base path in the filesystem (string)

Example:

This serves files from `/srv/www/dehydrated` when requested via `/.well-known/acme-challenge/`,
e.g. the request `GET /.well-known/acme-challenge/foo` will serve the file `/srv/www/dehydrated/foo`.
```
acme: Host(/./) && Method("GET") && Path("/.well-known/acme-challenge/*")
    -> static("/.well-known/acme-challenge/", "/srv/www/dehydrated") -> <shunt>;
```

Notes:
* redirects to the directory when a file `index.html` exists and it is requested, i.e. `GET /foo/index.html` redirects to `/foo/` which serves then the `/foo/index.html`
* serves the content of the `index.html` when a directory is requested
* does a simple directory listing of files / directories when no `index.html` is present

## stripQuery
## preserveHost

Sets the incoming `Host: ` header also on the outgoing backend connection

Parameters: none

Example:
```
route1: * -> preserveHost() -> "http://backend.example.org";
```

## status

Sets the response status code to the given value, with no regards to the backend response.

Parameters:
* status code (int)

Example:

```
route1: Host(/^all401\.example\.org$/) -> status(401) -> <shunt>;
```

## compress

The filter, when executed on the response path, checks if the response entity can
be compressed. To decide, it checks the Content-Encoding, the Cache-Control and
the Content-Type headers. It doesn't compress the content if the Content-Encoding
is set to other than identity, or the Cache-Control applies the no-transform
pragma, or the Content-Type is set to an unsupported value.

The default supported content types are: `text/plain`, `text/html`, `application/json`,
`application/javascript`, `application/x-javascript`, `text/javascript`, `text/css`,
`image/svg+xml`, `application/octet-stream`.

The default set of MIME types can be reset or extended by passing in the desired
types as filter arguments. When extending the defaults, the first argument needs to
be `"..."`. E.g. to compress tiff in addition to the defaults:

```
* -> compress("...", "image/tiff") -> "https://www.example.org"
```

To reset the supported types, e.g. to compress only HTML, the "..." argument needs
to be omitted:

```
* -> compress("text/html") -> "https://www.example.org"
```

It is possible to control the compression level, by setting it as the first filter
argument, in front of the MIME types. The default compression level is best-speed.
The possible values are integers between 0 and 9 (inclusive), where 0 means
no-compression, 1 means best-speed and 9 means best-compression. Example:

```
* -> compress(9, "image/tiff") -> "https://www.example.org"
```

The filter also checks the incoming request, if it accepts the supported encodings,
explicitly stated in the Accept-Encoding header. The filter currently supports `gzip`
and `deflate`. It does not assume that the client accepts any encoding if the
Accept-Encoding header is not set. It ignores * in the Accept-Encoding header.

When compressing the response, it updates the response header. It deletes the
`Content-Length` value triggering the proxy to always return the response with chunked
transfer encoding, sets the Content-Encoding to the selected encoding and sets the
`Vary: Accept-Encoding` header, if missing.

The compression happens in a streaming way, using only a small internal buffer.

## setQuery

## dropQuery
## inlineContent

## flowId

## randomContent

Generate response with random text of specified length.

Parameters:
* length of data (int)

Example:

```
* -> randomContent(42) -> <shunt>;
```

## latency

Enable adding artificial latency

## chunks

Enables adding chunking responses with custom chunk size

## bandwidth

Enable limiting bandwidth.

## backendLatency
## backendBandwidth
## backendChunks

## tee

Provides a unix-like `tee` feature for routing.

Using this filter, the request will be sent to a "shadow" backend in addition
to the main backend of the route.

Example:

```
* -> tee("https://audit-logging.example.org") -> "https://foo.example.org";
```

This will send an identical request for foo.example.org to
audit-logging.example.org. Another use case could be using it for benchmarking
a new backend with some real traffic

The above route will forward the request to `https://foo.example.org` as it
normally would do, but in addition to that, it will send an identical request to
`https://audit-logging.example.org`. The request sent to
`https://audit-logging.example.org` will receive the same method and headers,
and a copy of the body stream. The `tee` response is ignored for this shadow backend.

It is possible to change the path of the tee request, in a similar way to the
[modPath](#modpath) filter:

```
Path("/api/v1") -> tee("https://api.example.org", "^/v1", "/v2" ) -> "http://api.example.org";
```

In the above example, one can test how a new version of an API would behave on
incoming requests.

## teenf

## basicAuth

Enable Basic Authentication

The filter accepts two parameters, the first mandatory one is the path to the
htpasswd file usually used with Apache or nginx. The second one is the optional
realm name that will be displayed in the browser. MD5, SHA1 and BCrypt are supported
for Basic authentication password storage, see also
[the http-auth module page](https://github.com/abbot/go-http-auth).

Examples:

```
basicAuth("/path/to/htpasswd")
basicAuth("/path/to/htpasswd", "My Website")
```

## requestCookie

Append a cookie to the request header.

Parameters:
* cookie name (string)
* cookie value (string)

Example:

```
requestCookie("test-session", "abc")
```

## responseCookie

Appends cookies to responses in the "Set-Cookie" header. The response cookie
accepts an optional argument to control the max-age property of the cookie,
of type `int`, in seconds. The response cookie accepts an optional fourth
argument, "change-only", to control if the cookie should be set on every
response, or only if the request does not contain a cookie with the provided
name and value.

Example:

```
responseCookie("test-session", "abc")
responseCookie("test-session", "abc", 31536000),
responseCookie("test-session", "abc", 31536000, "change-only")
```

## jsCookie

The JS cookie behaves exactly as the response cookie, but it does not set the
`HttpOnly` directive, so these cookies will be accessible from JS code running
in web browsers.

Example:

```
jsCookie("test-session-info", "abc-debug", 31536000, "change-only")
```

## consecutiveBreaker

This breaker opens when the proxy could not connect to a backend or received
a >=500 status code at least N times in a row. When open, the proxy returns
503 - Service Unavailable response during the breaker timeout. After this timeout,
the breaker goes into half-open state, in which it expects that M number of
requests succeed. The requests in the half-open state are accepted concurrently.
If any of the requests during the half-open state fails, the breaker goes back to
open state. If all succeed, it goes to closed state again.

Parameters:
* number of consecutive failures to open (int)
* timeout (time string, parseable by [time.Duration](https://godoc.org/time#ParseDuration)) - optional
* half-open requests (int) - optional
* idle-ttl (time string, parseable by [time.Duration](https://godoc.org/time#ParseDuration)) - optional

See also the [circuit breaker docs](https://godoc.org/github.com/zalando/skipper/circuit).

## rateBreaker

The "rate breaker" works similar to the [consecutiveBreaker](#consecutivebreaker), but
instead of considering N consecutive failures for going open, it maintains a sliding
window of the last M events, both successes and failures, and opens only when the number
of failures reaches N within the window. This way the sliding window is not time based
and allows the same breaker characteristics for low and high rate traffic.

Parameters:
* number of consecutive failures to open (int)
* sliding window (time string, parseable by [time.Duration](https://godoc.org/time#ParseDuration))
* half-open requests (int) - optional
* idle-ttl (time string, parseable by [time.Duration](https://godoc.org/time#ParseDuration)) - optional

See also the [circuit breaker docs](https://godoc.org/github.com/zalando/skipper/circuit).

## disableBreaker

Change (or set) the breaker configurations for an individual route and disable for another, in eskip:

```
updates: Method("POST") && Host("foo.example.org")
  -> consecutiveBreaker(9)
  -> "https://foo.backend.net";

backendHealthcheck: Path("/healthcheck")
  -> disableBreaker()
  -> "https://foo.backend.net";
```

See also the [circuit breaker docs](https://godoc.org/github.com/zalando/skipper/circuit).

## localRatelimit
## ratelimit
## disableRatelimit

## lua

See [the scripts page](scripts.md)

## corsOrigin

## lbDecide

