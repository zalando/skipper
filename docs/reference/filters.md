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

### oauthOIDCUserInfo

```
oauthOIDCUserInfo("https://oidc-provider.example.com", "client_id", "client_secret",
    "http://target.example.com/subpath/callback", "email profile", "name email picture")
```

The filter needs the following parameters:

* **OpenID Connect Provider URL** For example Google OpenID Connect is available on `https://accounts.google.com`
* **Client ID** This value is obtained from the provider upon registration of the application.
* **Client Secret**  Also obtained from the provider
* **Callback URL** The entire path to the callback from the provider on which the token will be received.
    It can be any value which is a subpath on which the filter is applied.
* **Scopes** The OpenID scopes separated by spaces which need to be specified when requesting the token from the provider.
* **Claims** The claims which should be present in the token returned by the provider.

### oauthOIDCAnyClaims

```
oauthOIDCAnyClaims("https://oidc-provider.example.com", "client_id", "client_secret",
    "http://target.example.com/subpath/callback", "email profile", "name email picture")
```
The filter needs the following parameters:

* **OpenID Connect Provider URL** For example Google OpenID Connect is available on `https://accounts.google.com`
* **Client ID** This value is obtained from the provider upon registration of the application.
* **Client Secret**  Also obtained from the provider
* **Callback URL** The entire path to the callback from the provider on which the token will be received.
    It can be any value which is a subpath on which the filter is applied.
* **Scopes** The OpenID scopes separated by spaces which need to be specified when requesting the token from the provider.
* **Claims** Several claims can be specified and the request is allowed as long as at least one of them is present.

### oauthOIDCAllClaims

```
oauthOIDCAllClaims("https://oidc-provider.example.com", "client_id", "client_secret",
    "http://target.example.com/subpath/callback", "email profile", "name email picture")
```
The filter needs the following parameters:

* **OpenID Connect Provider URL** For example Google OpenID Connect is available on `https://accounts.google.com`
* **Client ID** This value is obtained from the provider upon registration of the application.
* **Client Secret**  Also obtained from the provider
* **Callback URL** The entire path to the callback from the provider on which the token will be received.
    It can be any value which is a subpath on which the filter is applied.
* **Scopes** The OpenID scopes separated by spaces which need to be specified when requesting the token from the provider.
* **Claims** Several claims can be specified and the request is allowed only when all claims are present.

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

## ~~localRatelimit~~

**DEPRECATED** use [clientRatelimit](#clientRatelimit) with the same
  settings instead.

## clientRatelimit

Per skipper instance calculated ratelimit, that allows number of
requests by client. The definition of the same client is based on data
of the http header and can be changed with an optional third
parameter. If the third parameter is set skipper will use the
Authorization header to put the request in the same client bucket,
else the X-Forwarded-For Header will be used. You need to run skipper
with command line flag `-enable-ratelimits`. Skipper will consume
roughly 15 MB per filter for 100.000 clients.

Parameters:

* number of allowed requests per time period (int)
* time period for requests being counted (time.Duration)
* optional parameter can be set to: "auth" (string)

```
clientRatelimit(3, "1m")
clientRatelimit(3, "1m", "auth")
```

See also the [ratelimit docs](https://godoc.org/github.com/zalando/skipper/ratelimit).

## ratelimit

Per skipper instance calculated ratelimit, that allows number of
requests to a backend. You need to run skipper
with command line flag `-enable-ratelimits`.

Parameters:

* number of allowed requests per time period (int)
* time period for requests being counted (time.Duration)

```
ratelimit(20, "1m")
ratelimit(300, "1h")
```

See also the [ratelimit docs](https://godoc.org/github.com/zalando/skipper/ratelimit).

## clusterClientRatelimit

This ratelimit is calculated across all skipper peers and allows the
given number of requests by client. The definition of the same client
is based on data of the http header and can be changed with an
optional third parameter. If the third parameter is set skipper will
use the Authorization header to put the request in the same client
bucket, else the X-Forwarded-For Header will be used.  You need to run
skipper with command line flags `-enable-swarm` and
`-enable-ratelimits`. Skipper will consume roughly 15 MB per filter
for 100.000 clients and 1000 skipper peers.

Parameters:

* number of allowed requests per time period (int)
* time period for requests being counted (time.Duration)
* optional parameter can be set to: "auth" (string)

```
clusterClientRatelimit(10, "1h")
clusterClientRatelimit(10, "1h", "auth")
```

See also the [ratelimit docs](https://godoc.org/github.com/zalando/skipper/ratelimit).

## clusterRatelimit

This ratelimit is calculated across all skipper peers and allows the
given number of requests to a backend. You need to have run skipper
with command line flags `-enable-swarm` and `-enable-ratelimits`.

Parameters:

* number of allowed requests per time period (int)
* time period for requests being counted (time.Duration)

```
clusterRatelimit(20, "1m")
clusterRatelimit(300, "1h")
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

## auditLog

Filter `auditLog()` logs the request and N bytes of the body into the
log file. N defaults to 1024 and can be overidden with
`-max-audit-body=<int>`. `N=0` omits logging the body.

Example:

```
auditLog()
```

## apiUsageMonitoring

The `apiUsageMonitoring` filter adds API related metrics to the Skipper monitoring. It is by default not activated. Activate
it by providing the `-enable-api-usage-monitoring` flag at Skipper startup. In its deactivated state, it is still
registered as a valid filter (allowing route configurations to specify it), but will perform no operation. That allows,
per instance, production environments to use it and testing environments not to while keeping the same route configuration
for all environments.

For the client based metrics, additional flags need to be specified.

| Flag                                                   | Description                                                                                                                                                                                                              |
|--------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `api-usage-monitoring-realm-keys`                      | Name of the property in the JWT JSON body that contains the name of the _realm_.                                                                                                                                         |
| `api-usage-monitoring-client-keys`                     | Name of the property in the JWT JSON body that contains the name of the _client_.                                                                                                                                        |
| `api-usage-monitoring-default-client-tracking-pattern` | Optional. Default is empty. Allows to deploy Skipper with a default client tracking pattern. When `apiUsageMonitoring` configuration does not specify one, this default is used instead of disabling the client metrics. |

NOTE: Make sure to activate the metrics flavour proper to your environment using the `metrics-flavour`
flag in order to get those metrics.

Example:

```bash
skipper -metrics-flavour prometheus -enable-api-usage-monitoring -api-usage-monitoring-realm-keys="realm" -api-usage-monitoring-client-keys="managed-id"
```

The structure of the metrics is all of those elements, separated by `.` dots:

| Part                        | Description                                                                                           |
|-----------------------------|-------------------------------------------------------------------------------------------------------|
| `apiUsageMonitoring.custom` | Every filter metrics starts with the name of the filter followed by `custom`. This part is constant.  |
| Application ID              | Identifier of the application, configured in the filter under `app_id`.                               |
| API ID                      | Identifier of the API, configured in the filter under `api_id`.                                       |
| Method                      | The request's method (verb), capitalized (ex: `GET`, `POST`, `PUT`, `DELETE`).                        |
| Path                        | The request's path, in the form of the path template configured in the filter under `path_templates`. |
| Realm                       | The realm in which the client is authenticated.                                                       |
| Client                      | Identifier under which the client is authenticated.                                                   |
| Metric Name                 | Name (or key) of the metric being tracked.                                                            |

### Available Metrics

#### Endpoint Related Metrics

Those metrics are not identifying the realm and client. They always have `*` in their place.

Example:

```
                                                                             + Realm
                                                                             |
apiUsageMonitoring.custom.orders-backend.orders-api.GET.foo/orders/:order-id.*.*.http_count
                                                                               | |
                                                                               | + Metric Name
                                                                               + Client
```

The available metrics are:

| Type      | Metric Name     | Description                                                                                                                    |
|-----------|-----------------|--------------------------------------------------------------------------------------------------------------------------------|
| Counter   | `http_count`    | number of HTTP exchanges                                                                                                       |
| Counter   | `http1xx_count` | number of HTTP exchanges resulting in information (HTTP status in the 100s)                                                    |
| Counter   | `http2xx_count` | number of HTTP exchanges resulting in success (HTTP status in the 200s)                                                        |
| Counter   | `http3xx_count` | number of HTTP exchanges resulting in a redirect (HTTP status in the 300s)                                                     |
| Counter   | `http4xx_count` | number of HTTP exchanges resulting in a client error (HTTP status in the 400s)                                                 |
| Counter   | `http5xx_count` | number of HTTP exchanges resulting in a server error (HTTP status in the 500s)                                                 |
| Histogram | `latency`       | time between the first observable moment (a call to the filter's `Request`) until the last (a call to the filter's `Response`) |

#### Client Related Metrics

Those metrics are not identifying endpoint (path) and HTTP verb. They always have `*` as their place.

Example:

```
                                                    + HTTP Verb
                                                    | + Path Template     + Metric Name
                                                    | |                   |
apiUsageMonitoring.custom.orders-backend.orders-api.*.*.users.mmustermann.http_count
                                                        |     |
                                                        |     + Client
                                                        + Realm
```

The available metrics are:

| Type    | Metric Name     | Description                                                                                                                                                |
|---------|-----------------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Counter | `http_count`    | number of HTTP exchanges                                                                                                                                   |
| Counter | `http1xx_count` | number of HTTP exchanges resulting in information (HTTP status in the 100s)                                                                                |
| Counter | `http2xx_count` | number of HTTP exchanges resulting in success (HTTP status in the 200s)                                                                                    |
| Counter | `http3xx_count` | number of HTTP exchanges resulting in a redirect (HTTP status in the 300s)                                                                                 |
| Counter | `http4xx_count` | number of HTTP exchanges resulting in a client error (HTTP status in the 400s)                                                                             |
| Counter | `http5xx_count` | number of HTTP exchanges resulting in a server error (HTTP status in the 500s)                                                                             |
| Counter | `latency_sum`   | sum of seconds (in decimal form) between the first observable moment (a call to the filter's `Request`) until the last (a call to the filter's `Response`) |

### Filter Configuration

Endpoints can be monitored using the `apiUsageMonitoring` filter in the route. It accepts JSON objects (as strings)
of the following format.

```yaml
api-usage-monitoring-configuration:
  type: object
  required:
    - application_id
    - api_id
    - path_templates
  properties:
    application_id:
      type: string
      description: ID of the application
      example: order-service
    api_id:
      type: string
      description: ID of the API
      example: orders-api
    path_templates:
      description: Endpoints to be monitored.
      type: array
      items:
        type: string
        description: >
          Path template in /articles/{article-id} (OpenAPI 3) or in /articles/:article-id format.
          NOTE: They will be normalized to the :this format for metrics naming.
        example: /orders/{order-id}
    client_tracking_pattern:
        description: >
            The pattern that the combination `realm.client` must match in order for the client
            based metrics to be tracked, in form of a regular expression.
            
            By default (if undefined), it is set to `services\\..*`.

            An empty string disables the client metrics completely.

            IMPORTANT: Avoid patterns that would match too many different values like `.*` or `users\\..*`. Too
            many different metric keys would badly affect the performances of the metric systems (e.g.: Prometheus).
        type: string
        examples:
            all_services:
                summary: All services are tracked (clients of the realm `services`).
                value: "services\\..*"
            just_some_services:
                summary: Only services `orders` and `shipment` are tracked.
                value: "services\\.(orders|shipment)"
```

Configuration Example:

```
apiUsageMonitoring(`
    {
        "application_id": "my-app",
        "api_id": "orders-api",
        "path_templates": [
            "foo/orders",
            "foo/orders/:order-id",
            "foo/orders/:order-id/order_item/{order-item-id}"
        ],
        "client_tracking_pattern": "users\\.(joe|sabine)"
    }`,`{
        "application_id": "my-app",
        "api_id": "customers-api",
        "path_templates": [
            "/foo/customers/",
            "/foo/customers/{customer-id}/"
        ]
    }
`)
```

Based on the previous configuration, here is an example of a counter metric.

```
apiUsageMonitoring.custom.my-app.orders-api.GET.foo/orders/:order-id.*.*.http_count
```

Here is the _Prometheus_ query to obtain it.

```
sum(rate(skipper_custom_total{key="apiUsageMonitoring.custom.my-app.orders-api.GET.foo/orders/:order-id.*.*.http_count"}[60s])) by (key)
```

Here is an example of a histogram metric.

```
apiUsageMonitoring.custom.my_app.orders-api.POST.foo/orders.latency
```

Here is the _Prometheus_ query to obtain it.

```
histogram_quantile(0.5, sum(rate(skipper_custom_duration_seconds_bucket{key="apiUsageMonitoring.custom.my-app.orders-api.POST.foo/orders.*.*.latency"}[60s])) by (le, key))
```

NOTE: Non configured paths will be tracked with `<unknown>` application ID, API ID
and path template.

```
apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.*.*.http_count
```

However, if all `application_id`s of your configuration refer to the same application,
the filter assume that also non configured paths will be directed to this application.
E.g.:

```
apiUsageMonitoring.custom.my-app.<unknown>.GET.<unknown>.*.*.http_count
```
