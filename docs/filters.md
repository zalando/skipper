# Skipper Filters

## setRequestHeader
## setResponseHeader
## appendRequestHeader
## appendResponseHeader
## dropRequestHeader
## dropResponseHeader
## healthcheck
## modPath
## setPath
## redirectTo
## redirectToLower
## static

Serves static content from the filesystem. 

Parameters:
* Request path to strip
* Target base path in the filesystem.

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

