# Skipper

Routing service with flexible routing rules stored in etcd and support for custom filters.

## Summary

Skipper is a routing service acting as a reverse proxy and selecting the right route for each request based on
the request parameters like method, path and headers. The routing rules can be flexibly configured and stored in
etcd. The etcd storage allows runtime updates. Skipper supports filters based on custom middleware that can be implemented with a well defined interface.

It is heavily inspired by Vulcand and it is using some solutions found in it:
[https://github.com/mailgun/vulcand](https://github.com/mailgun/vulcand)

## Compiling

One time init:

```
git clone ssh://git@stash.zalando.net:7999/shop-rebuild/skipper.git
cd skipper
export GOPATH=$(pwd)
go get

# for tests only:
go get github.com/coreos/etcd
```

In case, git and stash don't play nice on `go get` for eskip:

```
cd src
git clone ssh://git@stash.zalando.net:7999/shop-rebuild/eskip.git
```

Test (optional):

```
go test skipper/...
```

One time pre build:

```
go generate eskip
```

Build:

```
go install skipper
```

## Operation

Skipper has three kinds of artifacts:

- Backends
- Frontends
- Filter specifications

Routing happens between an existing frontend and a backend. If any of those is missing, skipper 404s. Filters are optional.

The routing definitions are stored in etcd, and loaded on startup and whenever they are updated in etcd (using
etcd's watch functionality).

(For the format the definition of each artifact type, see the 'Sample configuration' section below.)

### Backends

Backends are simple server endpoints, defined by their host URI:

E.g:

```
https://server.example.com:4545
```

During routing, the final request made to a backend will have the path and query part of the original incoming request, and also the headers and payload of the original request, but it will be directed to the configured backend.

### Frontends

Frontends are specified by rules matching requests, a backend and optional filters. A sample rule may look like this:

```
PathRegexp(`/some-dir/.*\\.html`)
```

Matching happens using Mailgun's Route library:
[https://github.com/mailgun/route](https://github.com/mailgun/route). For more details on request matching, (currently) please refer to the Mailgun Route documentation.

When a request is matched, all filters are executed on the request object in the order they are defined in the rule, then the request is made to backend, then all filters are executed on the response object in the reverse order, and then the response is forwarded to the client.

### Filters

Filters can modify the request and response headers, the request path, doing regexp matching, configuration parameters, and more and custom and just whatever. :) For more details on filters, see filters.md.

## Sample configuration

*Note: the current configuration format is temporary.*

(Assuming etcd listens on http://127.0.0.1:2379.)

Backend:

```
curl -X PUT -d 'value="https://server.example.com:4545"' http://127.0.0.1:2379/v2/keys/skipper/backends/hello
```

Frontend:

```
curl -X PUT \
    -d 'value={"route": "PathRegexp(`/hello-.*\\.html`)", "backend-id": "hello", "filters": ["hello-header"]}' \
    http://127.0.0.1:2379/v2/keys/skipper/frontends/hello
```

Filter specification:

```
curl -X PUT \
    -d 'value={"middleware-name": "request-header", "config": {"key": "X-Greetings", "value": "Hello!"}}' \
    http://127.0.0.1:2379/v2/keys/skipper/frontends/hello-header
```

## Running Skipper

If it was built as above, then the executable binary can be found in the 'bin' directory:

```
bin/skipper -insecure -etcd-urls "http://127.0.0.1:2379,http://127.0.0.1:4001"
```

Options:

- insecure: optional, ignore TLS certificate verification
- etcd-urls: optional, the urls where the etcd configuration is listening (the example shows the default values)

## Custom filters:

Custom filters can be easily created in go, implementing simple interfaces. Note, that despite somewhere the
'middleware' terminology may by used, these middleware are not dynamically loaded plugins, but they need to be compiled together with the skipper binary. For more details on writing and configuring filters, please see filters.md.

## Contribute

See devnotes.md information about contributing to Skipper.

## License

Copyright 2015 Zalando SE

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
