# Built your own skipper proxy

One of the biggest advantages of skipper compared to other HTTP
proxies is that skipper is a library first design. This means that it
is common to built your custom proxy based on skipper.

A minimal example project is
[skipper-example-proxy](https://github.com/szuecs/skipper-example-proxy).

```go
/*
This command provides an executable version of skipper with the default
set of filters.

For the list of command line options, run:

    skipper -help

For details about the usage and extensibility of skipper, please see the
documentation of the root skipper package.

To see which built-in filters are available, see the skipper/filters
package documentation.
*/
package main

import (
	log "github.com/sirupsen/logrus"
	lfilters "github.com/szuecs/skipper-example-proxy/filters"
	"github.com/zalando/skipper"
	"github.com/zalando/skipper/config"
)

func main() {
	cfg := config.NewConfig()
	if err := cfg.Parse(); err != nil {
		log.Fatalf("Error processing config: %s", err)
	}

	log.SetLevel(cfg.ApplicationLogLevel)

	opt := cfg.ToOptions()
	opt.CustomFilters = append(opt.CustomFilters, lfilters.NewMyFilter())

	log.Fatal(skipper.Run(opt))
}
```

## Code
Write the code and use the custom filter implemented in https://github.com/szuecs/skipper-example-proxy/blob/main/filters/custom.go
```
[:~]% mkdir -p /tmp/go/skipper
[:~]% cd /tmp/go/skipper
[:/tmp/go/skipper]% go mod init myproject
go: creating new go.mod: module myproject
[:/tmp/go/skipper]% cat >main.go
package main

import (
	log "github.com/sirupsen/logrus"
	lfilters "github.com/szuecs/skipper-example-proxy/filters"
	"github.com/zalando/skipper"
	"github.com/zalando/skipper/config"
)

func main() {
	cfg := config.NewConfig()
	if err := cfg.Parse(); err != nil {
		log.Fatalf("Error processing config: %s", err)
	}

	log.SetLevel(cfg.ApplicationLogLevel)

	opt := cfg.ToOptions()
	opt.CustomFilters = append(opt.CustomFilters, lfilters.NewMyFilter())

	log.Fatal(skipper.Run(opt))
}
CTRL-D
[:/tmp/go/skipper]%
```

## Build
Fetch dependencies and build your skipper binary.
```
[:/tmp/go/skipper]% go mod tidy
go: finding module for package github.com/zalando/skipper/config
go: finding module for package github.com/szuecs/skipper-example-proxy/filters
go: finding module for package github.com/sirupsen/logrus
go: finding module for package github.com/zalando/skipper
go: found github.com/sirupsen/logrus in github.com/sirupsen/logrus v1.9.3
go: found github.com/szuecs/skipper-example-proxy/filters in github.com/szuecs/skipper-example-proxy v0.0.0-20230622190245-63163cbaabc8
go: found github.com/zalando/skipper in github.com/zalando/skipper v0.16.117
go: found github.com/zalando/skipper/config in github.com/zalando/skipper v0.16.117
go: finding module for package github.com/nxadm/tail
go: finding module for package github.com/kr/text
go: finding module for package github.com/rogpeppe/go-internal/fmtsort
go: found github.com/kr/text in github.com/kr/text v0.2.0
go: found github.com/rogpeppe/go-internal/fmtsort in github.com/rogpeppe/go-internal v1.10.0
...

[:/tmp/go/skipper]% go build -o skipper .
[:/tmp/go/skipper]%
```

## Test
We start the proxy

```
# start the proxy
[:/tmp/go/skipper]% ./skipper -inline-routes='* -> myFilter() -> status(250) -> <shunt>'
[APP]INFO[0000] Expose metrics in codahale format
[APP]INFO[0000] enable swarm: false
[APP]INFO[0000] Replacing tee filter specification
[APP]INFO[0000] Replacing teenf filter specification
[APP]INFO[0000] Replacing lua filter specification
[APP]INFO[0000] support listener on :9911
[APP]INFO[0000] Dataclients are updated once, first load complete
[APP]INFO[0000] proxy listener on :9090
[APP]INFO[0000] TLS settings not found, defaulting to HTTP
[APP]INFO[0000] route settings, reset, route: : * -> myFilter() -> status(250) -> <shunt>
[APP]INFO[0000] route settings received
[APP]INFO[0000] route settings applied
127.0.0.1 - - [22/Jun/2023:21:13:46 +0200] "GET /foo HTTP/1.1" 250 0 "-" "curl/7.49.0" 0 127.0.0.1:9090 - -
```

Then we start the client to call the proxy endpoint.
```
# client
% curl -v http://127.0.0.1:9090/foo
*   Trying 127.0.0.1...
* Connected to 127.0.0.1 (127.0.0.1) port 9090 (#0)
> GET /foo HTTP/1.1
> Host: 127.0.0.1:9090
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 250 status code 250   <-- skipper core filter status(250)
< My-Filter: response            <-- your custom filter myFilter()
< Server: Skipper
< Date: Thu, 22 Jun 2023 19:13:46 GMT
< Transfer-Encoding: chunked
<
* Connection #0 to host 127.0.0.1 left intact
```
