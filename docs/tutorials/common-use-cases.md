## Common Use Cases

To understand common use cases, we assume you read the [basics](basics.md).

### Redirect handling

If you want to do a redirect from a route, you can use the
`redirectTo()` filter in combination with the `<shunt>` backend.
If you do not specify a path in your redirect, then the path from the
client will be passed further and not modified by the redirect.

Example:

```sh
% ./bin/skipper -address :8080 -inline-routes 'r: * -> redirectTo(308, "http://127.0.0.1:9999") -> <shunt>'
::1 - - [01/Nov/2018:18:42:02 +0100] "GET / HTTP/1.1" 308 0 "-" "curl/7.49.0" 0 localhost:8080 - -
::1 - - [01/Nov/2018:18:42:08 +0100] "GET /foo HTTP/1.1" 308 0 "-" "curl/7.49.0" 0 localhost:8080 - -

% curl localhost:8080 -v
* Rebuilt URL to: localhost:8080/
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
> GET / HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 308 Permanent Redirect
< Location: http://127.0.0.1:9999/
< Server: Skipper
< Date: Thu, 01 Nov 2018 17:42:18 GMT
< Content-Length: 0
<
* Connection #0 to host localhost left intact

% curl localhost:8080/foo -v
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
> GET /foo HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 308 Permanent Redirect
< Location: http://127.0.0.1:9999/foo
< Server: Skipper
< Date: Thu, 01 Nov 2018 17:42:14 GMT
< Content-Length: 0
<
* Connection #0 to host localhost left intact
```

#### set absolute path

If you set a path, in this example **/**, in your redirect definition, then the path is set to
the chosen value. The Location header is set in the response to `/`,
but the client sent `/foo`.

```sh
% ./bin/skipper -address :8080 -inline-routes 'r: * -> redirectTo(308, "http://127.0.0.1:9999/") -> <shunt>'

% curl localhost:8080/foo -v
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
> GET /foo HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 308 Permanent Redirect
< Location: http://127.0.0.1:9999/
< Server: Skipper
< Date: Thu, 01 Nov 2018 17:47:17 GMT
< Content-Length: 0
<
* Connection #0 to host localhost left intact
```

#### change base path

If you want a redirect definition that adds a base path and the
specified path by the client should be appended to this base path you
can use the `modPath` filter just before the `redirectTo()` to modify
the base path as you like.

Route Example shows, that calls to `/a/base/foo/bar` would be
redirected to `https://another-example.com/my/new/base/foo/bar`:

```sh
redirect: Path("/a/base/")
          -> modPath("/a/base/", "/my/new/base/")
          -> redirectTo(308, "https://another-example.com")
          -> <shunt>'
```

The next example shows how to test a redirect with changed base path
on your computer:

```sh
% ./bin/skipper -address :8080 -inline-routes 'r: * -> modPath("/", "/my/new/base/") -> redirectTo(308, "http://127.0.0.1:9999") -> <shunt>'
::1 - - [01/Nov/2018:18:49:45 +0100] "GET /foo HTTP/1.1" 308 0 "-" "curl/7.49.0" 0 localhost:8080 - -

% curl localhost:8080/foo -v
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
> GET /foo HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 308 Permanent Redirect
< Location: http://127.0.0.1:9999/my/new/base/foo
< Server: Skipper
< Date: Thu, 01 Nov 2018 17:49:45 GMT
< Content-Length: 0
<
* Connection #0 to host localhost left intact

```

#### Manipulating the redirect Location query string

`redirectTo` reuses the request query string when the configured location has no
query string. If the configured location contains a query string, that query
string is used for the `Location` header instead.

To remove the incoming query string from the redirect `Location`, run
`stripQuery` before `redirectTo`:

```
strip_redirect_query:
  * -> stripQuery() -> redirectTo(301, "https://example.com/new-path") -> <shunt>;
```

To replace the incoming query string, configure the redirect location with the
query string to send:

```
replace_redirect_query:
  * -> redirectTo(301, "https://example.com/new-path?key=value") -> <shunt>;
```

To rewrite the generated `Location` header, run `modResponseHeader` before
`redirectTo` in the route. `redirectTo` creates the response on the request
path, and only the filters reached before the request is shunted run on the
response path:

```
rewrite_redirect_query:
  * -> modResponseHeader("Location", "([?].*)?$", "?key=value")
    -> redirectTo(301, "https://example.com/new-path")
    -> <shunt>;
```

Request-side query filters, such as [stripQuery](../reference/filters.md#stripquery),
[setQuery](../reference/filters.md#setquery), and [dropQuery](../reference/filters.md#dropquery),
must be placed before `redirectTo` when they should affect the inherited redirect query string.
