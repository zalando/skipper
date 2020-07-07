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

## backendIsProxy

Notifies the proxy that the backend handling this request is also a
proxy. The proxy type is based in the URL scheme which can be either
`http`, `https` or `socks5`.

Keep in mind that Skipper currently cannot handle `CONNECT` requests
by tunneling the traffic to the target destination, however, the
`CONNECT` requests can be forwarded to a different proxy using this
filter.


Example:

```
foo1:
  *
  -> backendIsProxy()
  -> "http://proxy.example.com";

foo2:
  *
  -> backendIsProxy()
  -> <roundRobin, "http://proxy1.example.com", "http://proxy2.example.com">;

foo3:
  *
  -> setDynamicBackendUrl("http://proxy.example.com")
  -> backendIsProxy()
  -> <dynamic>;
```

## modRequestHeader

Replace all matched regex expressions in the given header.

Parameters:

* header name (string)
* the expression to match (regex)
* the replacement (string)

Example:

```
enforce_www: * -> modRequestHeader("Host", "^zalando\.(\w+)$", "www.zalando.$1") -> redirectTo(301);
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

## appendRequestHeader

Same as [setRequestHeader](#setRequestHeader),
but appends the provided value to the already existing ones.

## dropRequestHeader

Removes a header from the request

Parameters:

* header name (string)

Example:

```
foo: * -> dropRequestHeader("User-Agent") -> "https://backend.example.org";
```

## setResponseHeader

Same as [setRequestHeader](#setrequestheader), only for responses

## appendResponseHeader

Same as [appendRequestHeader](#appendrequestheader), only for responses

## dropResponseHeader

Same as [dropRequestHeader](#droprequestheader) but for responses from the backend

## setContextRequestHeader

Set headers for requests using values from the filter context (state bag). If the
provided key (second parameter) cannot be found in the state bag, then it doesn't
set the header.

Parameters:

* header name (string)
* key in the state bag (string)

The the route in the following example checkes whether the request is authorized with the
oauthTokeninfoAllScope() filter. This filter stores the authenticated user with "auth-user"
key in the context, and the setContextRequestHeader() filter in the next step stores it in
the header of the outgoing request with the X-Uid name:

```
foo: * -> oauthTokeninfoAllScope("address_service.all") -> setContextRequestHeader("X-Uid", "auth-user") -> "https://backend.example.org";
```

## appendContextRequestHeader

Same as [setContextRequestHeader](#setContextRequestHeader),
but appends the provided value to the already existing ones.

## setContextResponseHeader

Same as [setContextRequestHeader](#setContextRequestHeader), except for responses.

## appendContextResponseHeader

Same as [appendContextRequestHeader](#appendContextRequestHeader), except for responses.

## modPath

Replace all matched regex expressions in the path.

Parameters:

* the expression to match (regex)
* the replacement (string)

Example:

```
rm_api: Path("/api") -> modPath("/api", "/") -> "https://backend.example.org";
append_bar: Path("/foo") -> modPath("/foo", "/foo/bar") -> "https://backend.example.org";
new_base: PathSubtree("/base") -> modPath("/base", "/new/base) -> "https://backend.example.org";
rm_api_regex: Path("/api") -> modPath("^/api/(.*)/v2$", "/$1") -> "https://backend.example.org";
```

## setPath

Replace the path of the original request to the replacement.

Parameters:

* the replacement (string)

## redirectTo

Creates an HTTP redirect response.

Parameters:

* redirect status code (int)
* location (string) - optional

Example:

```
redirect1: PathRegexp(/^\/foo\/bar/) -> redirectTo(302, "/foo/newBar") -> <shunt>;
redirect2: * -> redirectTo(301) -> <shunt>;
```

- Route redirect1 will do a redirect with status code 302 to https
  with new path `/foo/newBar` for requests, that match the path `/foo/bar`.
- Route redirect2 will do a `https` redirect with status code 301 for all
  incoming requests that match no other route

see also [redirect-handling](../tutorials/common-use-cases.md#redirect-handling)

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

Removes the query parameter from the request URL, and if the first filter
parameter is `"true"`, preserves the query parameter in the form of
`x-query-param-<queryParamName>: <queryParamValue>` headers, so that `?foo=bar`
becomes `x-query-param-foo: bar`

Example:
```
* -> stripQuery() -> "http://backend.example.org";
* -> stripQuery("true") -> "http://backend.example.org";
```

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

## decompress

The filter, when executed on the response path, checks if the response entity is
compressed by a supported algorithm. To decide, it checks the Content-Encoding
header.

When compressing the response, it updates the response header. It deletes the
`Content-Length` value triggering the proxy to always return the response with chunked
transfer encoding, deletes the Content-Encoding and the Vary headers, if set.

The decompression happens in a streaming way, using only a small internal buffer.

Example:

```
* -> decompress() -> "https://www.example.org"
```

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

* content (string)
* content type (string) - optional

Example:

```
* -> inlineContent("<h1>Hello</h1>") -> <shunt>
* -> inlineContent("[1,2,3]", "application/json") -> <shunt>
```

Content type will be automatically detected when not provided.

!!! note
    `inlineContent` filter is special and must be the last in the filter chain.

## inlineContentIfStatus

Returns arbitrary content in the HTTP body, if the response has the specified status code.

Parameters:

* status code (int)
* content (string)
* content type (string) - optional

Example:

```
* -> inlineContentIfStatus(404, "<p class=\"problem\">We don't have what you're looking for.</p>") -> "https://www.example.org"
* -> inlineContentIfStatus(401, "{\"error\": \"unauthorized\"}", "application/json") -> "https://www.example.org"
```

The content type will be automatically detected when not provided.

## flowId

Sets an X-Flow-Id header, if it's not already in the request.
This allows you to have a trace in your logs, that traces from
the incoming request on the edge to all backend services.

Flow IDs must be in a certain format to be reusable in skipper. Valid formats
depend on the generator used in skipper. Default generator creates IDs of
length 16 matching the following regex: `^[0-9a-zA-Z+-]+$`

Parameters:

* no parameter: resets always the X-Flow-Id header to a new value
* `"reuse"`: only create X-Flow-Id header if not already set or if the value is invalid in the request

Example:

```
* -> flowId() -> "https://some-backend.example.org";
* -> flowId("reuse") -> "https://some-backend.example.org";
```

## xforward

Standard proxy headers. Appends the client remote IP to the X-Forwarded-For and sets the X-Forwarded-Host
header.

## xforwardFirst

Same as [xforward](xforward), but instead of appending the last remote IP, it prepends it to comply with the
approach of certain LB implementations.

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

## absorb

The absorb filter reads and discards the payload of the incoming requests.
It logs with INFO level and a unique ID per request:

- the event of receiving the request
- partial and final events for consuming request payload and total consumed byte count
- the finishing event of the request
- any read errors other than EOF

## logHeader

The logHeader filter prints the request line and the header, but not the body, to
stderr. Note that this filter should be used only in diagnostics setup and with care,
since the request headers may contain sensitive data, and they also can explode the
amount of logs. Authorization headers will be truncated in request and
response header logs. You can log request or response headers, which
defaults for backwards compatibility to request headers.

Parameters:

* no arg, similar to: "request"
* "request" or "response" (string varargs)

Example:

```
* -> logHeader() -> "https://www.example.org";
* -> logHeader("request") -> "https://www.example.org";
* -> logHeader("response") -> "https://www.example.org";
* -> logHeader("request", "response") -> "https://www.example.org";
```

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

## teeLoopback

This filter provides a unix-like tee feature for routing, but unlike the [tee](#tee),
this filter feeds the copied request to the start of the routing, including the
route lookup and executing the filters on the matched route.

It is recommended to use this solution instead of the tee filter, because the same
routing facilities are used for the outgoing tee requests as for the normal
requests, and all the filters and backend types are supported.

To ensure that the right route, or one of the right set of routes, is matched
after the loopback, use the filter together with the [Tee](predicates.md#tee)
predicate, however, this is not mandatory if the request is changed via other
filters, such that other predicates ensure matching the right route. To avoid
infinite looping, the number of requests spawn from a single incoming request
is limited similarly as in case of the
[loopback backend](backends.md#loopback-backend).

Parameters:

* tee group (string): a label identifying which routes should match the loopback
  request, marked with the [Tee](predicates.md#tee) predicate

Example, generate shadow traffic from 10% of the production traffic:

```
main: * -> "https://main-backend.example.org;
main-split: Traffic(.1) -> teeLoopback("test-A") -> "https://main-backend.example.org";
shadow: Tee("test-A") && True() -> "https://test-backend.example.org";
```

See also:

* [Tee predicate](predicates.md#tee)
* [Shadow Traffic Tutorial](../tutorials/shadow-traffic.md)

## sed

The filter sed replaces all occurences of a pattern with a replacement string
in the response body.

Example:

```
editorRoute: * -> sed("foo", "bar") -> "https://www.example.org";
```

Example with larger max buffer:

```
editorRoute: * -> sed("foo", "bar", 64000000) -> "https://www.example.org";
```

This filter expects a regexp pattern and a replacement string as arguments.
During the streaming of the response body, every occurence of the pattern will
be replaced with the replacement string. The editing doesn't happen right when
the filter is executed, only later when the streaming normally happens, after
all response filters were called.

The sed() filter accepts two optional arguments, the max editor buffer size in
bytes, and max buffer handling flag. The max buffer size, when set, defines how
much data can be buffered at a given time by the editor. The default value is
2MiB. The max buffer handling flag can take one of two values: "abort" or
"best-effort" (default). Setting "abort" means that the stream will be aborted
when reached the limit. Setting "best-effort", will run the replacement on the
available content, in case of certain patterns, this may result in content that
is different from one that would have been edited in a single piece. See more
details below.

The filter uses the go regular expression implementation:
https://github.com/google/re2/wiki/Syntax . Due to the streaming nature, matches
with zero length are ignored.

### Memory handling and limitations

In order to avoid unbound buffering of unprocessed data, the sed* filters need to
apply some limitations. Some patterns, e.g. `.*` would allow to match the complete
payload, and it could result in trying to buffer it all and potentially causing
running out of available memory. Similarly, in case of certain expressions, when
they don't match, it's impossible to tell if they would match without reading more
data from the source, and so would potentially need to buffer the entire payload.

To prevent too high memory usage, the max buffer size is limited in case of each
variant of the filter, by default to 2MiB, which is the same limit as the one we
apply when reading the request headers by default. When the limit is reached, and
the buffered content matches the pattern, then it is processed by replacing it,
when it doesn't match the pattern, then it is forwarded unchanged. This way, e.g.
`sed(".*", "")` can be used safely to consume and discard the payload.

As a result of this, with large payloads, it is possible that the resulting content
will be different than if we had run the replacement on the entire content at once.
If we have enough preliminary knowledge about the payload, then it may be better to
use the delimited variant of the filters, e.g. for line based editing.

If the max buffer handling is set to "abort", then the stream editing is stopped
and the rest of the payload is dropped.

## sedDelim

Like [sed()](#sed), but it expects an additional argument, before the optional max buffer
size argument, that is used to delimit chunks to be processed at once. The pattern
replacement is executed only within the boundaries of the chunks defined by the
delimiter, and matches across the chunk boundaries are not considered.

Example:

```
editorRoute: * -> sedDelim("foo", "bar", "\n") -> "https://www.example.org";
```

## sedRequest

Like [sed()](#sed), but for the request content.

Example:

```
editorRoute: * -> sedRequest("foo", "bar") -> "https://www.example.org";
```

## sedRequestDelim

Like [sedDelim()](#seddelim), but for the request content.

Example:

```
editorRoute: * -> sedRequestDelim("foo", "bar", "\n") -> "https://www.example.org";
```

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
is being done to the webhook endpoint. It is possible to copy headers
from the webhook response into the continuing request by specifying the
headers to copy as an optional second argument to the filter.

Responses from the webhook with status code less than 300 will be
authorized, the rest will be unauthorized.

Examples:

```
webhook("https://custom-webhook.example.org/auth")
webhook("https://custom-webhook.example.org/auth", "X-Copy-Webhook-Header,X-Copy-Another-Header")
```

The webhook timeout has a default of 2 seconds and can be globally
changed, if skipper is started with `-webhook-timeout=2s` flag.

## oauthTokeninfoAnyScope

If skipper is started with `-oauth2-tokeninfo-url` flag, you can use
this filter.

The filter accepts variable number of string arguments, which are used to
validate the incoming token from the `Authorization: Bearer <token>`
header. There are two rejection scenarios for this filter. If the token
is not successfully validated by the oauth server, then a 401 Unauthorised
response will be returned. However, if the token is successfully validated
but the required scope match isn't satisfied, then a 403 Forbidden response
will be returned. If any of the configured scopes from the filter is found
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
header. There are two rejection scenarios for this filter. If the token
is not successfully validated by the oauth server, then a 401 Unauthorised
response will be returned. However, if the token is successfully validated
but the required scope match isn't satisfied, then a 403 Forbidden response
will be returned. If all of the configured scopes from the filter are found
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
`Authorization: Bearer <token>` header. There are two rejection scenarios
for this filter. If the token is not successfully validated by the oauth
server, then a 401 Unauthorised response will be returned. However,
if the token is successfully validated but the required scope match
isn't satisfied, then a 403 Forbidden response will be returned.
If any of the configured key value pairs from the filter is found
inside the tokeninfo result for the incoming token, it will allow
the request to pass.

Examples:

```
oauthTokeninfoAnyKV("k1", "v1", "k2", "v2")
oauthTokeninfoAnyKV("k1", "v1", "k1", "v2")
```

## oauthTokeninfoAllKV

If skipper is started with `-oauth2-tokeninfo-url` flag, you can use
this filter.

The filter accepts an even number of variable arguments of type
string, which are used to validate the incoming token from the
`Authorization: Bearer <token>` header. There are two rejection
scenarios for this filter. If the token is not successfully validated
by the oauth server, then a 401 Unauthorised response will be
returned. However, if the token is successfully validated but
the required scope match isn't satisfied, then a 403 Forbidden response
will be returned. If all of the configured key value pairs from
the filter are found inside the tokeninfo result for the incoming
token, it will allow the request to pass.

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
oauthTokenintrospectionAnyKV("k1", "v1", "k1", "v2")
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

## secureOauthTokenintrospectionAnyClaims

The filter accepts variable number of string arguments, which are used
to validate the incoming token from the `Authorization: Bearer
<token>` header. The first argument to the filter is the issuer URL,
for example `https://accounts.google.com`, that will be used as
described in [RFC Draft](https://tools.ietf.org/html/draft-ietf-oauth-discovery-06#section-5)
to find the configuration and for example supported claims.

Use this filter if the Token Introspection endpoint requires authorization to validate and decode the incoming token.
The filter will optionally read client-id and client-secret from environment variables: OAUTH_CLIENT_ID, OAUTH_CLIENT_SECRET

If one of the configured and supported claims from the filter are
found inside the tokenintrospection (RFC7662) result for the incoming
token, it will allow the request to pass.

Examples:

```
secureOauthTokenintrospectionAnyClaims("issuerURL", "client-id", "client-secret", "claim1", "claim2")
```

Read client-id and client-secret from environment variables
```
secureOauthTokenintrospectionAnyClaims("issuerURL", "", "", "claim1", "claim2")
```

## secureOauthTokenintrospectionAllClaims

The filter accepts variable number of string arguments, which are used
to validate the incoming token from the `Authorization: Bearer
<token>` header. The first argument to the filter is the issuer URL,
for example `https://accounts.google.com`, that will be used as
described in [RFC Draft](https://tools.ietf.org/html/draft-ietf-oauth-discovery-06#section-5)
to find the configuration and for example supported claims.

Use this filter if the Token Introspection endpoint requires authorization to validate and decode the incoming token.
The filter will optionally read client-id and client-secret from environment variables: OAUTH_CLIENT_ID, OAUTH_CLIENT_SECRET

If all of the configured and supported claims from the filter are
found inside the tokenintrospection (RFC7662) result for the incoming
token, it will allow the request to pass.

Examples:

```
secureOauthTokenintrospectionAllClaims("issuerURL", "client-id", "client-secret", "claim1", "claim2")
```

Read client-id and client-secret from environment variables
```
secureOauthTokenintrospectionAllClaims("issuerURL", "", "", "claim1", "claim2")
```

## secureOauthTokenintrospectionAnyKV

The filter accepts an even number of variable arguments of type
string, which are used to validate the incoming token from the
`Authorization: Bearer <token>` header.  The first argument to the
filter is the issuer URL, for example `https://accounts.google.com`,
that will be used as described in
[RFC Draft](https://tools.ietf.org/html/draft-ietf-oauth-discovery-06#section-5)
to find the configuration and for example supported claims.

Use this filter if the Token Introspection endpoint requires authorization to validate and decode the incoming token.
The filter will optionally read client-id and client-secret from environment variables: OAUTH_CLIENT_ID, OAUTH_CLIENT_SECRET

If one of the configured key value pairs from the filter are found
inside the tokenintrospection (RFC7662) result for the incoming token,
it will allow the request to pass.

Examples:

```
secureOauthTokenintrospectionAnyKV("issuerURL", "client-id", "client-secret", "k1", "v1", "k2", "v2")
```

Read client-id and client-secret from environment variables
```
secureOauthTokenintrospectionAnyKV("issuerURL", "", "", "k1", "v1", "k2", "v2")
```

## secureOauthTokenintrospectionAllKV

The filter accepts an even number of variable arguments of type
string, which are used to validate the incoming token from the
`Authorization: Bearer <token>` header.  The first argument to the
filter is the issuer URL, for example `https://accounts.google.com`,
that will be used as described in
[RFC Draft](https://tools.ietf.org/html/draft-ietf-oauth-discovery-06#section-5)
to find the configuration and for example supported claims.

Use this filter if the Token Introspection endpoint requires authorization to validate and decode the incoming token.
The filter will optionally read client-id and client-secret from environment variables: OAUTH_CLIENT_ID, OAUTH_CLIENT_SECRET

If all of the configured key value pairs from the filter are found
inside the tokenintrospection (RFC7662) result for the incoming token,
it will allow the request to pass.

Examples:

```
secureOauthTokenintrospectionAllKV("issuerURL", "client-id", "client-secret", "k1", "v1", "k2", "v2")
```

Read client-id and client-secret from environment variables
```
secureOauthTokenintrospectionAllKV("issuerURL", "", "", "k1", "v1", "k2", "v2")
```

## forwardToken

The filter takes the (string) header name as its first argument. The result of token info or token introspection is added to
this header when the request is passed to the backend. If there are additional arguments, these
values are treated as a whitelisted set of JSON keys to be included in the
header payload when forwarding to the backend service.

If this filter is used when there is no token introspection or token info data
then it does not have any effect.

Examples:

```
forwardToken("X-Tokeninfo-Forward")
forwardToken("X-Tokeninfo-Forward", "access_token")
```

## oauthOidcUserInfo

```
oauthOidcUserInfo("https://oidc-provider.example.com", "client_id", "client_secret",
    "http://target.example.com/subpath/callback", "email profile", "name email picture",
    "parameter=value", "X-Auth-Authorization:claims.email")
```

The filter needs the following parameters:

* **OpenID Connect Provider URL** For example Google OpenID Connect is available on `https://accounts.google.com`
* **Client ID** This value is obtained from the provider upon registration of the application.
* **Client Secret**  Also obtained from the provider
* **Callback URL** The entire path to the callback from the provider on which the token will be received.
    It can be any value which is a subpath on which the filter is applied.
* **Scopes** The OpenID scopes separated by spaces which need to be specified when requesting the token from the provider.
* **Claims** The claims which should be present in the token returned by the provider.
* **Auth Code Options** (optional) Passes key/value parameters to a provider's authorization endpoint.
* **Upstream Headers** (optional) The upstream endpoint will receive these headers which values are parsed from the OIDC information. The header definition can be one or more header-query pairs, space delimited. The query syntax is [GJSON](https://github.com/tidwall/gjson/blob/master/SYNTAX.md).

## oauthOidcAnyClaims

```
oauthOidcAnyClaims("https://oidc-provider.example.com", "client_id", "client_secret",
    "http://target.example.com/subpath/callback", "email profile", "name email picture",
    "parameter=value", "X-Auth-Authorization:claims.email")
```

The filter needs the following parameters:

* **OpenID Connect Provider URL** For example Google OpenID Connect is available on `https://accounts.google.com`
* **Client ID** This value is obtained from the provider upon registration of the application.
* **Client Secret**  Also obtained from the provider
* **Callback URL** The entire path to the callback from the provider on which the token will be received.
    It can be any value which is a subpath on which the filter is applied.
* **Scopes** The OpenID scopes separated by spaces which need to be specified when requesting the token from the provider.
* **Claims** Several claims can be specified and the request is allowed as long as at least one of them is present.
* **Auth Code Options** (optional) Passes key/value parameters to a provider's authorization endpoint.
* **Upstream Headers** (optional) The upstream endpoint will receive these headers which values are parsed from the OIDC information. The header definition can be one or more header-query pairs, space delimited. The query syntax is [GJSON](https://github.com/tidwall/gjson/blob/master/SYNTAX.md).

## oauthOidcAllClaims

```
oauthOidcAllClaims("https://oidc-provider.example.com", "client_id", "client_secret",
    "http://target.example.com/subpath/callback", "email profile", "name email picture",
    "parameter=value", "X-Auth-Authorization:claims.email")
```

The filter needs the following parameters:

* **OpenID Connect Provider URL** For example Google OpenID Connect is available on `https://accounts.google.com`
* **Client ID** This value is obtained from the provider upon registration of the application.
* **Client Secret**  Also obtained from the provider
* **Callback URL** The entire path to the callback from the provider on which the token will be received.
    It can be any value which is a subpath on which the filter is applied.
* **Scopes** The OpenID scopes separated by spaces which need to be specified when requesting the token from the provider.
* **Claims** Several claims can be specified and the request is allowed only when all claims are present.
* **Auth Code Options** (optional) Passes key/value parameters to a provider's authorization endpoint.
* **Upstream Headers** (optional) The upstream endpoint will receive these headers which values are parsed from the OIDC information. The header definition can be one or more header-query pairs, space delimited. The query syntax is [GJSON](https://github.com/tidwall/gjson/blob/master/SYNTAX.md).

## requestCookie

Append a cookie to the request header.

Parameters:

* cookie name (string)
* cookie value (string)

Example:

```
requestCookie("test-session", "abc")
```

## oidcClaimsQuery

```
oidcClaimsQuery("<path>:[<query>]", ...)
```

The filter is chained after `oauthOidc*` authentication as it parses the ID token that has been saved in the internal `StateBag` for this request. It validates access control of the requested path against the defined query.
It accepts one or more arguments, thats is a path prefix which is granted access to when the query definition evaluates positive.
It supports exact matches of keys, key-value pairs, introspecting of arrays or exact and wildcard matching of nested structures.
The query definition can be one or more queries per path, space delimited. The query syntax is [GJSON](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) with a convenience modifier of `@_` which unfolds to `[@this].#("+arg+")`

Given following example ID token:

```json
{
  "email": "someone@example.org",
  "groups": [
    "CD-xyz",
    "appX-Test-Users"
    "Purchasing-Department",
  ],
  "name": "Some One"
}
```

Access to path `/` would be granted to everyone in `example.org`, however path `/login` only to those being member of `group "appX-Tester"`:

```
oauthOidcAnyClaims(...) -> oidcClaimsQuery("/login:groups.#[==\"appX-Tester\"]", "/:@_:email%\"*@example.org\"")
```

For above ID token following query definitions would also be positive:

```
oidcClaimsQuery("/:email")
oidcClaimsQuery("/another/path:groups.#[%\"CD-*\"]")
oidcClaimsQuery("/:name%\"*One\"", "/path:groups.#[%\"*-Test-Users\"] groups.#[==\"Purchasing-Department\"]")
```

As of now there is no negative/deny rule possible. The first matching path is evaluated against the defined query/queries and if positive, permitted.

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

Can be used as [egress](egress.md) feature.

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

Can be used as [egress](egress.md) feature.

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

Can be used as [egress](egress.md) feature.

## ~~localRatelimit~~

**DEPRECATED** use [clientRatelimit](#clientratelimit) with the same
  settings instead.

## clientRatelimit

Per skipper instance calculated ratelimit, that allows number of
requests by client. The definition of the same client is based on data
of the http header and can be changed with an optional third
parameter. If the third parameter is set skipper will use the
defined HTTP header to put the request in the same client bucket,
else the X-Forwarded-For Header will be used. You need to run skipper
with command line flag `-enable-ratelimits`. Skipper will consume
roughly 15 MB per filter for 100.000 clients.

Parameters:

* number of allowed requests per time period (int)
* time period for requests being counted (time.Duration)
* optional parameter to set the same client by header, in case the provided string contains `,`, it will combine all these headers (string)

```
clientRatelimit(3, "1m")
clientRatelimit(3, "1m", "Authorization")
clientRatelimit(3, "1m", "X-Foo,Authorization,X-Bar")
```

See also the [ratelimit docs](https://godoc.org/github.com/zalando/skipper/ratelimit).

## ratelimit

Per skipper instance calculated ratelimit, that allows forwarding a
number of requests to the backend group. You need to run skipper with
command line flag `-enable-ratelimits`.

Parameters:

* number of allowed requests per time period (int)
* time period for requests being counted (time.Duration)

```
ratelimit(20, "1m")
ratelimit(300, "1h")
```

See also the [ratelimit docs](https://godoc.org/github.com/zalando/skipper/ratelimit).

## clusterClientRatelimit

This ratelimit is calculated across all skipper peers and the same
rate limit group. The first parameter is a string to select the same
ratelimit group across one or more routes.  The rate limit group
allows the given number of requests by client. The definition of the
same client is based on data of the http header and can be changed
with an optional fourth parameter. If the fourth parameter is set
skipper will use the HTTP header defined by this to put the request in
the same client bucket, else the X-Forwarded-For Header will be used.
You need to run skipper with command line flags `-enable-swarm` and
`-enable-ratelimits`. See also our [cluster ratelimit tutorial](../../tutorials/ratelimit/#cluster-ratelimit)

Parameters:

* rate limit group (string)
* number of allowed requests per time period (int)
* time period for requests being counted (time.Duration)
* optional parameter to set the same client by header, in case the provided string contains `,`, it will combine all these headers (string)

```
clusterClientRatelimit("groupA", 10, "1h")
clusterClientRatelimit("groupA", 10, "1h", "Authorization")
clusterClientRatelimit("groupA", 10, "1h", "X-Forwarded-For,Authorization,User-Agent")
```

See also the [ratelimit docs](https://godoc.org/github.com/zalando/skipper/ratelimit).

## clusterRatelimit

This ratelimit is calculated across all skipper peers and the same
rate limit group. The first parameter is a string to select the same
ratelimit group across one or more routes.  The rate limit group
allows the given number of requests to a backend. You need to have run
skipper with command line flags `-enable-swarm` and
`-enable-ratelimits`. See also our [cluster ratelimit tutorial](../../tutorials/ratelimit/#cluster-ratelimit)


Parameters:

* rate limit group (string)
* number of allowed requests per time period (int)
* time period for requests being counted (time.Duration)

```
clusterRatelimit("groupB", 20, "1m")
clusterRatelimit("groupB", 300, "1h")
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
while access log, in general, is enabled. It is also possible to disable access logs only for a subset of response codes
from backend by providing an optional list of response code prefixes.

Parameters:

* response code prefixes (variadic int) - optional

Example:

```
disableAccessLog()
disableAccessLog(1, 301, 40)
```

This disables logs of all requests with status codes `1xxs`, `301` and all `40xs`.

## enableAccessLog

Filter overrides global Skipper `AccessLogDisabled` setting and allows to turn-on the access log for specific route
while access log, in general, is disabled. It is also possible to enable access logs only for a subset of response codes
from backend by providing an optional list of response code prefixes.

Parameters:

* response code prefixes (variadic int) - optional

Example:

```
enableAccessLog()
enableAccessLog(1, 301, 20)
```

This enables logs of all requests with status codes `1xxs`, `301` and all `20xs`.

## auditLog

Filter `auditLog()` logs the request and N bytes of the body into the
log file. N defaults to 1024 and can be overidden with
`-max-audit-body=<int>`. `N=0` omits logging the body.

Example:

```
auditLog()
```

## unverifiedAuditLog

Filter `unverifiedAuditLog()` adds a Header, `X-Unverified-Audit`, to the request, the content of which, will also
be written to the log file. By default, the value of the audit header will be equal to the value of the `sub` key, from
the Authorization token. This can be changed by providing a `string` input to the filter which matches another key from the
token.

*N.B.* It is important to note that, if the content of the `X-Unverified-Audit` header does not match the following regex, then
a default value of `invalid-sub` will be populated in the header instead:
    `^[a-zA-z0-9_/:?=&%@.#-]*$`

Examples:

```
unverifiedAuditLog()
```

```
unverifiedAuditLog("azp")
```

## setDynamicBackendHostFromHeader

Filter sets the backend host for a route, value is taken from the provided header.
Can be used only with `<dynamic>` backend. Meant to be used together with [setDynamicBackendSchemeFromHeader](#setdynamicbackendschemefromheader)
or [setDynamicBackendScheme](#setdynamicbackendscheme). If this filter chained together with [setDynamicBackendUrlFromHeader](#setdynamicbackendurlfromheader)
or [setDynamicBackendUrl](#setdynamicbackendurl) filters, the latter ones would have priority.

Parameters:

* header name (string)

Example:

```
foo: * -> setDynamicBackendHostFromHeader("X-Forwarded-Host") -> <dynamic>;
```

## setDynamicBackendSchemeFromHeader

Filter sets the backend scheme for a route, value is taken from the provided header.
Can be used only with `<dynamic>` backend. Meant to be used together with [setDynamicBackendHostFromHeader](#setdynamicbackendhostfromheader)
or [setDynamicBackendHost](#setdynamicbackendhost). If this filter chained together with
[setDynamicBackendUrlFromHeader](#setdynamicbackendurlfromheader) or [setDynamicBackendUrl](#setdynamicbackendurl), the latter ones would have priority.

Parameters:

* header name (string)

Example:

```
foo: * -> setDynamicBackendSchemeFromHeader("X-Forwarded-Proto") -> <dynamic>;
```

## setDynamicBackendUrlFromHeader

Filter sets the backend url for a route, value is taken from the provided header.
Can be used only with `<dynamic>` backend.

Parameters:

* header name (string)

Example:

```
foo: * -> setDynamicBackendUrlFromHeader("X-Custom-Url") -> <dynamic>;
```

## setDynamicBackendHost

Filter sets the backend host for a route. Can be used only with `<dynamic>` backend.
Meant to be used together with [setDynamicBackendSchemeFromHeader](#setdynamicbackendschemefromheader)
or [setDynamicBackendScheme](#setdynamicbackendscheme). If this filter chained together with [setDynamicBackendUrlFromHeader](#setdynamicbackendurlfromheader)
or [setDynamicBackendUrl](#setdynamicbackendurl), the latter ones would have priority.

Parameters:

* host (string)

Example:

```
foo: * -> setDynamicBackendHost("example.com") -> <dynamic>;
```

## setDynamicBackendScheme

Filter sets the backend scheme for a route. Can be used only with `<dynamic>` backend.
Meant to be used together with [setDynamicBackendHostFromHeader](#setdynamicbackendhostfromheader)
or [setDynamicBackendHost](#setdynamicbackendhost). If this filter chained together with
[setDynamicBackendUrlFromHeader](#setdynamicbackendurlfromheader) or [setDynamicBackendUrl](#setdynamicbackendurl), the latter ones would have priority.

Parameters:

* scheme (string)

Example:

```
foo: * -> setDynamicBackendScheme("https") -> <dynamic>;
```

## setDynamicBackendUrl

Filter sets the backend url for a route. Can be used only with `<dynamic>` backend.

Parameters:

* url (string)

Example:

```
foo: * -> setDynamicBackendUrl("https://example.com") -> <dynamic>;
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
| `api-usage-monitoring-realms-tracking-pattern`         | RegEx of _realms_ to be monitored. Defaults to 'services'.                                                                                                                                                                |

NOTE: Make sure to activate the metrics flavour proper to your environment using the `metrics-flavour`
flag in order to get those metrics.

Example:

```bash
skipper -metrics-flavour prometheus -enable-api-usage-monitoring -api-usage-monitoring-realm-keys="realm" -api-usage-monitoring-client-keys="managed-id" api-usage-monitoring-realms-tracking-pattern="services,users"
```

The structure of the metrics is all of those elements, separated by `.` dots:

| Part                        | Description                                                                                           |
|-----------------------------|-------------------------------------------------------------------------------------------------------|
| `apiUsageMonitoring.custom` | Every filter metrics starts with the name of the filter followed by `custom`. This part is constant.  |
| Application ID              | Identifier of the application, configured in the filter under `app_id`.                               |
| Tag                         | Tag of the application (e.g. staging), configured in the filter under `tag`.                               |
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
apiUsageMonitoring.custom.orders-backend.staging.orders-api.GET.foo/orders/{order-id}.*.*.http_count
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
apiUsageMonitoring.custom.orders-backend.staging.orders-api.*.*.users.mmustermann.http_count
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
of the format mentioned below. In case any of the required parameters is missing, `no-op` filter is created, i.e. no
metrics are captured, but the creation of the route does not fail.

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
    tag:
      type: string
      description: tag of the application
      example: staging
    api_id:
      type: string
      description: ID of the API
      example: orders-api
    path_templates:
      description: Endpoints to be monitored.
      type: array
      minLength: 1
      items:
        type: string
        description: >
          Path template in /articles/{article-id} (OpenAPI 3) or in /articles/:article-id format.
          NOTE: They will be normalized to the :this format for metrics naming.
        example: /orders/{order-id}
    client_tracking_pattern:
        description: >
            The pattern that matches client id in form of a regular expression.

            By default (if undefined), it is set to `.*`.

            An empty string disables the client metrics completely.
        type: string
        examples:
            all_services:
                summary: All services are tracked (for all activated realms).
                value: ".*"
            just_some_services:
                summary: Only services `orders-service` and `shipment-service` are tracked.
                value: "(orders\-service|shipment\-service)"
```

Configuration Example:

```
apiUsageMonitoring(`
    {
        "application_id": "my-app",
        "tag": "staging",
        "api_id": "orders-api",
        "path_templates": [
            "foo/orders",
            "foo/orders/:order-id",
            "foo/orders/:order-id/order_item/{order-item-id}"
        ],
        "client_tracking_pattern": "(shipping\-service|payment\-service)"
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
apiUsageMonitoring.custom.my-app.staging.orders-api.GET.foo/orders/{order-id}.*.*.http_count
```

Note that a missing `tag` in the configuration will be replaced by `{no-tag}` in the metric:

```
apiUsageMonitoring.custom.my-app.{no-tag}.customers-api.GET.foo/customers.*.*.http_count
```

Here is the _Prometheus_ query to obtain it.

```
sum(rate(skipper_custom_total{key="apiUsageMonitoring.custom.my-app.staging.orders-api.GET.foo/orders/{order-id}.*.*.http_count"}[60s])) by (key)
```

Here is an example of a histogram metric.

```
apiUsageMonitoring.custom.my_app.staging.orders-api.POST.foo/orders.latency
```

Here is the _Prometheus_ query to obtain it.

```
histogram_quantile(0.5, sum(rate(skipper_custom_duration_seconds_bucket{key="apiUsageMonitoring.custom.my-app.staging.orders-api.POST.foo/orders.*.*.latency"}[60s])) by (le, key))
```

NOTE: Non configured paths will be tracked with `{unknown}` Application ID, Tag, API ID
and path template.

However, if all `application_id`s of your configuration refer to the same application,
the filter assume that also non configured paths will be directed to this application.
E.g.:

```
apiUsageMonitoring.custom.my-app.{unknown}.{unknown}.GET.{no-match}.*.*.http_count
```

## lifo

This Filter changes skipper to handle the route with a bounded last in
first out queue (LIFO), instead of an unbounded first in first out
queue (FIFO). The default skipper scheduler is based on Go net/http
package, which provides an unbounded FIFO request handling. If you
enable this filter the request scheduling will change to a LIFO.  The
idea of a LIFO queue is based on Dropbox bandaid proxy, which is not
opensource. Dropbox shared their idea in a
[public blogpost](https://blogs.dropbox.com/tech/2018/03/meet-bandaid-the-dropbox-service-proxy/).
All bounded scheduler filters will respond requests with server status error
codes in case of overrun. All scheduler filters return HTTP status code:

- 502, if the specified timeout is reached, because a request could not be scheduled fast enough
- 503, if the queue is full

Parameters:

* MaxConcurrency specifies how many goroutines are allowed to work on this queue(int)
* MaxQueueSize sets the queue size (int)
* Timeout sets the timeout to get request scheduled (time)

Example:

```
lifo(100, 150, "10s")
```

The above configuration will set MaxConcurrency to 100, MaxQueueSize
to 150 and Timeout to 10 seconds.

When multiple lifo filters are set in a route, only one of them will be
applied. It is undefined which one.

## lifoGroup

This filter is similar to the [lifo](#lifo) filter.

Parameters:

* GroupName to group multiple one or many routes to the same queue, which have to have the same settings (string)
* MaxConcurrency specifies how many goroutines are allowed to work on this queue(int)
* MaxQueueSize sets the queue size (int)
* Timeout sets the timeout to get request scheduled (time)

Example:

```
lifoGroup("mygroup", 100, 150, "10s")
```

The above configuration will set MaxConcurrency to 100, MaxQueueSize
to 150 and Timeout to 10 seconds for the lifoGroup "mygroup", that can
be shared between multiple routes.

It is enough to set the concurrency, queue size and timeout parameters for one instance of
the filter in the group, and only the group name for the rest. Setting these values for
multiple instances is fine, too. While only one of them will be used as the source for the
applied settings, if there is accidentally a difference between the settings in the same
group, a warning will be logged.

It is possible to use the lifoGroup filter together with the single lifo filter, e.g. if
a route belongs to a group, but needs to have additional stricter settings then the whole
group.

## rfcPath

This filter forces an alternative interpretation of the RFC 2616 and RFC 3986 standards,
where paths containing reserved characters will have these characters unescaped when the
incoming request also has them unescaped.

Example:

```
Path("/api/*id) -> rfcPath() -> "http://api-backend"
```

In the above case, if the incoming request has something like foo%2Fbar in the id
position, the api-backend service will also receive it in the format foo%2Fbar, while
without the rfcPath() filter the outgoing request path will become /api/foo/bar.

In case we want to use the id while routing the request, we can use the <loopback>
backend. Example:

```
api: Path("/api/:id") -> setPath("/api/${id}/summary") -> "http://api-backend";
patch: Path("/api/*id") -> rfcPath() -> <loopback>;
```

In the above case, if the incoming request path is /api/foo%2Fbar, it will match
the 'patch' route, and then the patched request will match the api route, and
the api-backend service will receive a request with the path /api/foo%2Fbar/summary.

It is also possible to enable this behavior centrally for a Skipper instance with
the -rfc-patch-path flag. See
[URI standards interpretation](../../operation/operation/#uri-standards-interpretation).

## bearerinjector

This filter injects `Bearer` tokens into `Authorization` headers read
from file providing the token as content. This is only for use cases
using skipper as sidecar to inject tokens for the application on the
[**egress**](egress.md) path, if it's used in the **ingress** path you likely
create a security issue for your application.

This filter should be used as an [egress](egress.md) only feature.

Example:

```
egress1: Method("POST") && Host("api.example.com") -> bearerinjector("write-token") -> "https://api.example.com/shoes";
egress2: Method("GET") && Host("api.example.com") -> bearerinjector("read-token") -> "https://api.example.com/shoes";
```

To integrate with the `bearerinjector` filter you need to run skipper
with `-credentials-paths=/tmp/secrets` and specify an update interval
`-credentials-update-interval=10s`. Files in the credentials path can
be a directory, which will be able to find all files within this
directory, but it won't walk subtrees. For the example case, there
have to be filenames `write-token` and `read-token` within the
specified credential paths `/tmp/secrets/`, resulting in
`/tmp/secrets/write-token` and `/tmp/secrets/read-token`.

## tracingBaggageToTag

This filter adds an opentracing tag for a given baggage item in the trace.

Syntax:
```
tracingBaggageToTag("<baggage_item_name>", "<tag_name>")
```

Example: If a trace consists of baggage item named `foo` with a value `bar`. Adding below filter will add a tag named `baz` with value `bar`
```
tracingBaggageToTag("foo", "baz")
```

## tracingTag

This filter adds an opentracing tag.

Syntax:
```
tracingTag("<tag_name>", "<tag_value>")
```

Example: Adding the below filter will add a tag named `foo` with the value `bar`.
```
tracingTag("foo", "bar")
```

## originMarker

This filter is used to measure the time it took to create a route. Other than that, it's a no-op.
You can include the same origin marker when you re-create the route. As long as the `origin` and `id` are the same, the route creation time will not be measured again.
If there are multiple origin markers with the same origin, the earliest timestamp will be used.

Parameters:

* the name of the origin
* the ID of the object that is the logical source for the route
* the creation timestamp (rfc3339)

Example:

```
originMarker("apiUsageMonitoring", "deployment1", "2019-08-30T09:55:51Z")
```
