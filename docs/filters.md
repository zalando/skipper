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
## setQuery
## dropQuery
## inlineContent

## flowId

## randomContent
## latency
## chunks
## bandwidth
## backendLatency
## backendBandwidth
## backendChunks

## tee
## teenf

## basicAuth

## requestCookie
## responseCookie
## jsCookie

## consecutiveBreaker
## rateBreaker
## disableBreaker

## localRatelimit
## ratelimit
## disableRatelimit

## lua

See [scripts page](scripts.md)

## corsOrigin

## lbDecide

