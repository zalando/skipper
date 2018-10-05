# Skipper Filters

The parameters can be strings, regex or float64 / int

* `string` is a string surrounded by double quotes (`"`)
* `regex` is a regular expression, surrounded by `/`, e.g. `/^www\.example\.org(:\d+)?$/`
* `int` / `float64` are usual (decimal) numbers like `401` or `1.23456`
* `time` is a string in double quotes, parseable by [time.Duration](https://godoc.org/time#ParseDuration))

Filters are a generic tool and can change HTTP header and body in the request and response path.
Filter can be chained using the arrow operator `->`.

Example route with a match all, 2 filters and a backend:

```
all: * -> filter1 -> filter2 -> "http://127.0.0.1:1234/";
```

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

## modPath

Replace all matched regex expressions in the path.

Parameters:
* the expression to match (regex)
* the replacement (string)

## setPath

Replace the path of the original request to the replacement.

Parameters:
* the replacement (string)

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

Same as [redirectTo](#redirectTo), but replaces all strings to lower case.

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

Sets the incoming `Host: ` header on the outgoing backend connection.

It can be used to override the `proxyPreserveHost` behavior for individual routes.

Parameters: "true" or "false"
* "true" - use the Host header from the incoming request
* "false" - use the host from the backend address

Example:
```
route1: * -> preserveHost("true") -> "http://backend.example.org";
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

Set the query string `?k=v` in the request to the backend to a given value.

Parameters:

* key (string)
* value (string)

Example:

```
setQuery("k", "v")
```

## dropQuery

Delete the query string `?k=v` in the request to the backend for a
given key.

Parameters:

* key (string)

Example:

```
dropQuery("k")
```

## inlineContent

Returns arbitrary content in the HTTP body.

Parameters:

* arbitrary (string)

Example:

```
* -> inlineContent("<h1>Hello</h1>") -> <shunt>
```

## flowId

Sets an X-Flow-Id header, if it's not already in the request.
This allows you to have a trace in your logs, that traces from
the incoming request on the edge to all backend services.

Paramters:

* no parameter: resets always the X-Flow-Id header to a new value
* "reuse": only create X-Flow-Id header if not set in the request

Example:

```
* -> flowId() -> "https://some-backend.example.org";
* -> flowId("reuse") -> "https://some-backend.example.org";
```

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

Parameters:

* latency in milliseconds (int)

Example:

```
* -> latency(120) -> "https://www.example.org";
```

## bandwidth

Enable bandwidth throttling.

Parameters:

* bandwidth in kb/s (int)

Example:

```
* -> bandwidth(30) -> "https://www.example.org";
```

## chunks

Enables adding chunking responses with custom chunk size with
artificial delays in between response chunks. To disable delays, set
the second parameter to "0".

Parameters:

* byte length (int)
* time duration (time.Duration)

Example:

```
* -> chunks(1024, "120ms") -> "https://www.example.org";
* -> chunks(1024, "0") -> "https://www.example.org";
```

## backendLatency

Same as [latency filter](#latency), but on the request path and not on
the response path.

## backendBandwidth

Same as [bandwidth filter](#bandwidth), but on the request path and not on
the response path.

## backendChunks

Same as [chunks filter](#chunks), but on the request path and not on
the response path.

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
a new backend with some real traffic. This we call "shadow traffic".

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

The same as [tee filter](#tee), but does not follow redirects from the backend.

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

## webhook

The `webhook` filter makes it possible to have your own authentication and
authorization endpoint as a filter.

Headers from the incoming request will be copied into the request that
is being done to the webhook endpoint. Responses from the webhook with
status code less than 300 will be authorized, rest unauthorized.

Examples:

```
webhook("https://custom-webhook.example.org/auth")
```

The webhook timeout has a default of 2 seconds and can be globally
changed, if skipper is started with `-webhook-timeout=2s` flag.

## oauthTokeninfoAnyScope

If skipper is started with `-oauth2-tokeninfo-url` flag, you can use
this filter.

The filter accepts variable number of string arguments, which are used to
validate the incoming token from the `Authorization: Bearer <token>`
header. If any of the configured scopes from the filter is found
inside the tokeninfo result for the incoming token, it will allow the
request to pass.

Examples:

```
oauthTokeninfoAnyScope("s1", "s2", "s3")
```

## oauthTokeninfoAllScope

If skipper is started with `-oauth2-tokeninfo-url` flag, you can use
this filter.

The filter accepts variable number of string arguments, which are used to
validate the incoming token from the `Authorization: Bearer <token>`
header. If all of the configured scopes from the filter are found
inside the tokeninfo result for the incoming token, it will allow the
request to pass.

Examples:

```
oauthTokeninfoAllScope("s1", "s2", "s3")
```

## oauthTokeninfoAnyKV

If skipper is started with `-oauth2-tokeninfo-url` flag, you can use
this filter.

The filter accepts an even number of variable arguments of type
string, which are used to validate the incoming token from the
`Authorization: Bearer <token>` header. If any of the configured key
value pairs from the filter is found inside the tokeninfo result for
the incoming token, it will allow the request to pass.

Examples:

```
oauthTokeninfoAnyKV("k1", "v1", "k2", "v2")
```

## oauthTokeninfoAllKV

If skipper is started with `-oauth2-tokeninfo-url` flag, you can use
this filter.

The filter accepts an even number of variable arguments of type
string, which are used to validate the incoming token from the
`Authorization: Bearer <token>` header. If all of the configured key
value pairs from the filter are found inside the tokeninfo result for
the incoming token, it will allow the request to pass.

Examples:

```
oauthTokeninfoAllKV("k1", "v1", "k2", "v2")
```

## oauthTokenintrospectionAnyClaims

The filter accepts variable number of string arguments, which are used
to validate the incoming token from the `Authorization: Bearer
<token>` header. The first argument to the filter is the issuer URL,
for example `https://accounts.google.com`, that will be used as
described in [RFC Draft](https://tools.ietf.org/html/draft-ietf-oauth-discovery-06#section-5)
to find the configuration and for example supported claims.

If one of the configured and supported claims from the filter are
found inside the tokenintrospection (RFC7662) result for the incoming
token, it will allow the request to pass.

Examples:

```
oauthTokenintrospectionAnyClaims("c1", "c2", "c3")
```

## oauthTokenintrospectionAllClaims

The filter accepts variable number of string arguments, which are used
to validate the incoming token from the `Authorization: Bearer
<token>` header. The first argument to the filter is the issuer URL,
for example `https://accounts.google.com`, that will be used as
described in [RFC Draft](https://tools.ietf.org/html/draft-ietf-oauth-discovery-06#section-5)
to find the configuration and for example supported claims.

If all of the configured and supported claims from the filter are
found inside the tokenintrospection (RFC7662) result for the incoming
token, it will allow the request to pass.

Examples:

```
oauthTokenintrospectionAllClaims("c1", "c2", "c3")
```

## oauthTokenintrospectionAnyKV

The filter accepts an even number of variable arguments of type
string, which are used to validate the incoming token from the
`Authorization: Bearer <token>` header.  The first argument to the
filter is the issuer URL, for example `https://accounts.google.com`,
that will be used as described in
[RFC Draft](https://tools.ietf.org/html/draft-ietf-oauth-discovery-06#section-5)
to find the configuration and for example supported claims.

If one of the configured key value pairs from the filter are found
inside the tokenintrospection (RFC7662) result for the incoming token,
it will allow the request to pass.

Examples:

```
oauthTokenintrospectionAnyKV("k1", "v1", "k2", "v2")
```

## oauthTokenintrospectionAllKV

The filter accepts an even number of variable arguments of type
string, which are used to validate the incoming token from the
`Authorization: Bearer <token>` header.  The first argument to the
filter is the issuer URL, for example `https://accounts.google.com`,
that will be used as described in
[RFC Draft](https://tools.ietf.org/html/draft-ietf-oauth-discovery-06#section-5)
to find the configuration and for example supported claims.

If all of the configured key value pairs from the filter are found
inside the tokenintrospection (RFC7662) result for the incoming token,
it will allow the request to pass.

Examples:

```
oauthTokenintrospectionAllKV("k1", "v1", "k2", "v2")
```

## forwardToken

The filter accepts a single string as an argument. The argument is the header 
name where the result of token info or token introspection is added when the
request is passed to the backend.

If this filter is used when there is no token introspection or token info data
then it does not have any effect.

Examples:

```
forwardToken("X-Tokeninfo-Forward")
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

Per skipper instance calculated ratelimit, that allows number of
requests by client. The definition of the same client is based on data
of the http header and can be changed with an optional third
parameter. If the third parameter is set skipper will use the
Authorization header to put the request in the same client bucket,
else  the X-Forwarded-For Header will be used.

Parameters:

* number of allowed requests per time period (int)
* time period for requests being counted (time.Duration)
* optional parameter can be set to: "auth" (string)

```
localRatelimit(3, "1m")
localRatelimit(3, "1m", "auth")
```

See also the [ratelimit docs](https://godoc.org/github.com/zalando/skipper/ratelimit).

## ratelimit

Per skipper instance calculated ratelimit, that allows number of
requests to a backend.

Parameters:

* number of allowed requests per time period (int)
* time period for requests being counted (time.Duration)

```
ratelimit(20, "1m")
ratelimit(300, "1h")
```

See also the [ratelimit docs](https://godoc.org/github.com/zalando/skipper/ratelimit).

## lua

See [the scripts page](scripts.md)

## corsOrigin

The filter accepts an optional variadic list of acceptable origin
parameters. If the input argument list is empty, the header will
always be set to `*` which means any origin is acceptable. Otherwise
the header is only set if the request contains an Origin header and
its value matches one of the elements in the input list. The header is
only set on the response.

Parameters:

*  url (variadic string)

Examples:

```
corsOrigin()
corsOrigin("https://www.example.org")
corsOrigin("https://www.example.org", "http://localhost:9001")
```

## headerToQuery

Filter which assigns the value of a given header from the incoming Request to a given query param

Parameters:

* The name of the header to pick from request
* The name of the query param key to add to request

Examples:

```
headerToQuery("X-Foo-Header", "foo-query-param")
```

The above filter will set `foo-query-param` query param respectively to the `X-Foo-Header` header
and will override the value if the queryparam exists already

## queryToHeader

Filter which assigns the value of a given query param from the
incoming Request to a given Header with optional format string value.

Parameters:

* The name of the query param key to pick from request
* The name of the header to add to request
* The format string used to create the header value, which gets the
  value from the query value as before

Examples:

```
queryToHeader("foo-query-param", "X-Foo-Header")
queryToHeader("access_token", "Authorization", "Bearer %s")
```

The first filter will set `X-Foo-Header` header respectively to the `foo-query-param` query param
and will not override the value if the header exists already.

The second filter will set `Authorization` header to the
`access_token` query param with a prefix value `Bearer ` and will
not override the value if the header exists already.

## ~~accessLogDisabled~~

**Deprecated:** use [disableAccessLog](#disableaccesslog) or [enableAccessLog](#enableaccesslog)

The `accessLogDisabled` filter overrides global Skipper `AccessLogDisabled` setting for a specific route, which allows to either turn-off
the access log for specific route while access log, in general, is enabled or vice versa.

Example:

```
accessLogDisabled("false")
```

## disableAccessLog

Filter overrides global Skipper `AccessLogDisabled` setting and allows to turn-off the access log for specific route
while access log, in general, is enabled.

Example:

```
disableAccessLog()
```

## enableAccessLog

Filter overrides global Skipper `AccessLogDisabled` setting and allows to turn-on the access log for specific route
while access log, in general, is disabled.

Example:

```
enableAccessLog()
```


## apimonitoring

The `apimonitoring` filter adds API related metrics to the monitoring.

WARNING: This is an experimental filter and needs to be enabled explicitly at `skipper` startup.
WARNING: Make sure that the Prometheus Metrics are also enabled.

```bash
skipper -enable-apimonitoring -enable-prometheus-metrics
```

Endpoints can be monitored using the `apimonitoring` function in the route. It accepts a JSON object.
* `application_id`: An application could be offering more than one API. Specify the application's ID here.
* `path_templates`: An endpoint path _template_, given in the OpenAPI format. Serves for grouping parametrized paths
  together. Example: `/foo/1` and `/foo/2` should be monitored as the same endpoint, then provide: `PathPat: /foo/{foo-id}`.
  It accepts both `{foo-id}` and `:foo-id` formats for the variable parts, but are all normalized to `:foo-id`.
* `verbose` (default: `false`): An optional parameter making the filter log more detail about its operation.
  It is bypassed by the `--apimonitoring-verbose` switch when specified.

Example:

```
apimonitoring(`{
  "application_id": "my_app",
  "path_templates": [
    "foo/orders",
    "foo/orders/:order-id",
    "foo/orders/:order-id/order-items/{order-item-id}"
    "/foo/customers/",
    "/foo/customers/{customer-id}/"
  ]
}`)
```

That would monitor metrics like:
* `apimonitoring.custom.my_app.GET.foo/orders/:order-id`
* `apimonitoring.custom.my_app.POST.foo/orders`
* `apimonitoring.custom.my_app.GET.foo/orders/:order-id/order-items/:order-item-id`
* `apimonitoring.custom.my_app.POST.foo/customers`
* `apimonitoring.custom.my_app.DELETE.foo/customers/:customer-id`
