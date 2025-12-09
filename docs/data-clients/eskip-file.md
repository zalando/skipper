# Eskip File

Eskip file dataclient can be used to serve static defined routes, read
from an eskip file. The [file format eskip](https://pkg.go.dev/github.com/zalando/skipper/eskip)
shows your route definitions in a clear way:

```sh
% cat example.eskip
hello: Path("/hello") -> "https://www.example.org"
```

The [Skipper project](https://github.com/zalando/skipper) has two
binaries, one is `skipper`, the other is `eskip`.
[Eskip](https://pkg.go.dev/github.com/zalando/skipper/cmd/eskip)
can be used to validate the syntax of your routes file before
reloading a production server:

    % eskip check example.eskip

To run Skipper serving routes from an `eskip` file you have to use
`-routes-file <file>` parameter:

    % skipper -routes-file example.eskip


A more complicated example with different routes, matches,
[predicates](https://pkg.go.dev/github.com/zalando/skipper/predicates) and
[filters](https://pkg.go.dev/github.com/zalando/skipper/filters) shows that
you can name your route and use preconditions and create, change, delete
HTTP headers as you like:

```sh
% cat complicated_example.eskip
hostHeaderMatch:
         Host("^skipper.teapot.org$")
         -> setRequestHeader("Authorization", "Basic YWRtaW46YWRtaW5zcGFzc3dvcmQK")
         -> "https://target-to.auth-with.basic-auth.enterprise.com";
baiduPathMatch:
        Path("/baidu")
        -> setRequestHeader("Host", "www.baidu.com")
        -> setPath("/s")
        -> setQuery("wd", "godoc skipper")
        -> "http://www.baidu.com";
googleWildcardMatch:
        *
        -> setPath("/search")
        -> setQuery("q", "godoc skipper")
        -> "https://www.google.com";
yandexWildacardIfCookie:
        * && Cookie("yandex", "true")
        -> setPath("/search/")
        -> setQuery("text", "godoc skipper")
        -> tee("http://127.0.0.1:12345/")
        -> "https://yandex.ru";
```

The former example shows 4 routes: hostHeaderMatch,
baiduPathMatch, googleWildcardMatch and yandexWildcardIfCookie.

* hostHeaderMatch:
    * used if HTTP host header is exactly: "skipper.teapot.org",
    * sets a Basic Authorization header and
    * sends the modified request to https://target-to.auth-with.basic-auth.enterprise.com
* baiduPathMatch:
    * used in case the request patch matches /baidu
    * it will set the Host header to the proxy request
    * it will set the path from /baidu to /s
    * it will set the querystring to "ws=godoc skipper" and
    * sends the modified request to http://baidu.com
* googleWildcardMatch:
    * used as default if no other route matches
    * it will set the path to /search
    * it will set the querystring to "q=godoc skipper" and
    * sends the modified request to https://www.google.com
* yandexWildcardIfCookie:
    * used as default if a Cookie named "yandex" has the value "true"
    * it will set the path to /search/
    * it will set the querystring to "text=godoc skipper"
    * it will send a copy of the modified request to http://127.0.0.1:12345/ (similar to unix `tee`) and drop the response and
    * sends the modified request to https://yandex.ru

More examples you find in [eskip file format](https://pkg.go.dev/github.com/zalando/skipper/eskip)
description, in [filters](https://pkg.go.dev/github.com/zalando/skipper/filters)
and in [predicates](https://pkg.go.dev/github.com/zalando/skipper/predicates).


Eskip file format is also used if you print your current routes in skipper,
for example:

```sh
% curl localhost:9911/routes
*
  -> setResponseHeader("Content-Type", "application/json; charset=utf-8")
  -> inlineContent("{\"foo\": 3}")
  -> <shunt>
```
