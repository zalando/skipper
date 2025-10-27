# Skipper plugins

Skipper may be extended with functionality not present in the core.
These additions can be built as go plugin, so they do not have to
be present in the main skipper repository.

Note the warning from Go's plugin.go:

    // The plugin support is currently incomplete, only supports Linux,
    // and has known bugs. Please report any issues.

Note the known problem of using plugins together with vendoring, best
described here:

https://github.com/golang/go/issues/20481

## Plugin directories

Plugins are loaded from sub directories of the plugin directories. By default
the plugin directory is set to `./plugins` (i.e. relative to skipper's working
directory). An additional directory may be given with the `-plugindir=/path/to/dir`
option to skipper.

Any file with the suffix `.so` found below the plugin directories (also in sub
directories) is attempted to load without any arguments. When a plugin needs an
argument, this must be explicitly loaded and the arguments passed, e.g. with
`-filter-plugin geoip,db=/path/to/db`.

## Building a plugin

Each plugin should be built with Go version >= 1.11, enabled Go
modules support similar to the following build command line:

```sh
go build -buildmode=plugin -o example.so example.go
```

There are some pitfalls:

* packages which are shared between skipper and the plugin **must not** be in
  a `vendor/` directory, otherwise the plugin will fail to load or in some
  cases give wrong results (e.g. an opentracing span cannot be found in the
  context even if it is present). This also means:
  Do not vendor skipper in a plugin repo...
* plugins must be rebuilt when skipper is rebuilt
* do not attempt to rebuild a module and copy it over a loaded plugin, that
  will crash skipper immediately...

## Use a plugin

In this example we use a geoip database, that you need to find and download.
We expect that you did a `git clone git@github.com:zalando/skipper.git` and
entered the directory.

Build skipper:

```sh
% make skipper
```

Install filter plugins:

```sh
% mkdir plugins
% git clone git@github.com:skipper-plugins/filters.git plugins/filters
% ls plugins/filters
geoip/  glide.lock  glide.yaml  ldapauth/  Makefile  noop/  plugin_test.go
% cd plugins/filters/geoip
% go build -buildmode=plugin -o geoip.so geoip.go
% cd -
~/go/src/github.com/zalando/skipper
```

Start a pseudo backend that shows all headers in plain:

```sh
% nc -l 9000

```

Run the proxy with geoip database:

```sh
% ./bin/skipper -filter-plugin geoip,db=$HOME/Downloads/GeoLite2-City_20181127/GeoLite2-City.mmdb -inline-routes '* -> geoip() -> "http://127.0.0.1:9000"'
[APP]INFO[0000] found plugin geoip at plugins/filters/geoip/geoip.so
[APP]INFO[0000] loaded plugin geoip (geoip) from plugins/filters/geoip/geoip.so
[APP]INFO[0000] attempting to load plugin from plugins/filters/geoip/geoip.so
[APP]INFO[0000] plugin geoip already loaded with InitFilter
[APP]INFO[0000] Expose metrics in codahale format
[APP]INFO[0000] support listener on :9911
[APP]INFO[0000] proxy listener on :9090
[APP]INFO[0000] route settings, reset, route: : * -> geoip() -> "http://127.0.0.1:9000"
[APP]INFO[0000] certPathTLS or keyPathTLS not found, defaulting to HTTP
[APP]INFO[0000] route settings received
[APP]INFO[0000] route settings applied
```

Or passing a yaml file via `config-file` flag:

```yaml
inline-routes: '* -> geoip() -> "http://127.0.0.1:9000"'
filter-plugin:
  geoip:
    - db=$HOME/Downloads/GeoLite2-City_20181127/GeoLite2-City.mmdb
```

Use a client to lookup geoip:

```sh
% curl -H"X-Forwarded-For: 107.12.53.5" localhost:9090/
^C
```


pseudo backend should show X-Geoip-Country header:

```sh
# nc -l 9000
GET / HTTP/1.1
Host: 127.0.0.1:9000
User-Agent: curl/7.49.0
Accept: */*
X-Forwarded-For: 107.12.53.5
X-Geoip-Country: US
Accept-Encoding: gzip
^C
```

skipper should show additional log lines, because of the CTRL-C:

```sh
[APP]ERRO[0082] error while proxying, route  with backend http://127.0.0.1:9000, status code 500: dialing failed false: EOF
107.12.53.5 - - [28/Nov/2018:14:39:40 +0100] "GET / HTTP/1.1" 500 22 "-" "curl/7.49.0" 2753 localhost:9090 - -
```

## Filter plugins

All plugins must have a function named `InitFilter` with the following signature

    func([]string) (filters.Spec, error)

The parameters passed are all arguments for the plugin, i.e. everything after the first
word from skipper's `-filter-plugin` parameter. E.g. when the `-filter-plugin`
parameter is

    myfilter,datafile=/path/to/file,foo=bar

the `myfilter` plugin will receive

    []string{"datafile=/path/to/file", "foo=bar"}

as arguments.

The filter plugin implementation is responsible to parse the received arguments.

Filter plugins can be found in the [filter repo](https://github.com/skipper-plugins/filters)

### Example filter plugin

An example `noop` plugin looks like

```go
package main

import (
	"github.com/zalando/skipper/filters"
)

type noopSpec struct{}

func InitFilter(opts []string) (filters.Spec, error) {
	return noopSpec{}, nil
}

func (s noopSpec) Name() string {
	return "noop"
}
func (s noopSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	return noopFilter{}, nil
}

type noopFilter struct{}

func (f noopFilter) Request(filters.FilterContext)  {}
func (f noopFilter) Response(filters.FilterContext) {}
```

## Predicate plugins

All plugins must have a function named `InitPredicate` with the following signature

    func([]string) (routing.PredicateSpec, error)

The parameters passed are all arguments for the plugin, i.e. everything after the first
word from skipper's `-predicate-plugin` parameter. E.g. when the `-predicate-plugin`
parameter is

    mypred,datafile=/path/to/file,foo=bar

the `mypred` plugin will receive

    []string{"datafile=/path/to/file", "foo=bar"}

as arguments.

The predicate plugin implementation is responsible to parse the received arguments.

Predicate plugins can be found in the [predicate repo](https://github.com/skipper-plugins/predicates)

### Example predicate plugin

An example `MatchAll` plugin looks like

```go
package main

import (
	"github.com/zalando/skipper/routing"
	"net/http"
)

type noopSpec struct{}

func InitPredicate(opts []string) (routing.PredicateSpec, error) {
	return noopSpec{}, nil
}

func (s noopSpec) Name() string {
	return "MatchAll"
}
func (s noopSpec) Create(config []interface{}) (routing.Predicate, error) {
	return noopPredicate{}, nil
}

type noopPredicate struct{}

func (p noopPredicate) Match(*http.Request) bool {
    return true
}
```

## DataClient plugins

Similar to the above predicate and filter plugins. The command line option for data
client plugins is `-dataclient-plugin`. The module must have a `InitDataClient`
function with the signature

    func([]string) (routing.DataClient, error)

A `noop` data client looks like

```go
package main

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

func InitDataClient([]string) (routing.DataClient, error) {
	var dc DataClient = ""
	return dc, nil
}

type DataClient string

func (dc DataClient) LoadAll() ([]*eskip.Route, error) {
	return eskip.Parse(string(dc))
}

func (dc DataClient) LoadUpdate() ([]*eskip.Route, []string, error) {
	return nil, nil, nil
}
```

## MultiType plugins

Sometimes it is necessary to combine multiple plugin types into one module. This can
be done with this kind of plugin. Note that these modules are not auto loaded, these
need an explicit `-multi-plugin name,arg1,arg2` command line switch for skipper.

The module must have a `InitPlugin` function with the signature

    func([]string) ([]filters.Spec, []routing.PredicateSpec, []routing.DataClient, error)

Any of the returned types may be nil, so you can have e.g. a combined filter / data client
plugin or share a filter and a predicate, e.g. like

```go
package main

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	ot "github.com/opentracing/opentracing-go"
	maxminddb "github.com/oschwald/maxminddb-golang"

	"github.com/zalando/skipper/filters"
	snet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

type geoipSpec struct {
	db   *maxminddb.Reader
	name string
}

func InitPlugin(opts []string) ([]filters.Spec, []routing.PredicateSpec, []routing.DataClient, error) {
	var db string
	for _, o := range opts {
		switch {
		case strings.HasPrefix(o, "db="):
			db = o[3:]
		}
	}
	if db == "" {
		return nil, nil, nil, fmt.Errorf("missing db= parameter for geoip plugin")
	}
	reader, err := maxminddb.Open(db)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to open db %s: %s", db, err)
	}

	return []filters.Spec{&geoipSpec{db: reader, name: "geoip"}},
		[]routing.PredicateSpec{&geoipSpec{db: reader, name: "GeoIP"}},
		nil,
		nil
}

func (s *geoipSpec) Name() string {
	return s.name
}

func (s *geoipSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	var fromLast bool
	header := "X-GeoIP-Country"
	var err error
	for _, c := range config {
		if s, ok := c.(string); ok {
			switch {
			case strings.HasPrefix(s, "from_last="):
				fromLast, err = strconv.ParseBool(s[10:])
				if err != nil {
					return nil, filters.ErrInvalidFilterParameters
				}
			case strings.HasPrefix(s, "header="):
				header = s[7:]
			}
		}
	}
	return &geoip{db: s.db, fromLast: fromLast, header: header}, nil
}

func (s *geoipSpec) Create(config []interface{}) (routing.Predicate, error) {
	var fromLast bool
	var err error
	countries := make(map[string]struct{})
	for _, c := range config {
		if s, ok := c.(string); ok {
			switch {
			case strings.HasPrefix(s, "from_last="):
				fromLast, err = strconv.ParseBool(s[10:])
				if err != nil {
					return nil, predicates.ErrInvalidPredicateParameters
				}
			default:
				countries[strings.ToUpper(s)] = struct{}{}
			}
		}
	}
	return &geoip{db: s.db, fromLast: fromLast, countries: countries}, nil
}

type geoip struct {
	db        *maxminddb.Reader
	fromLast  bool
	header    string
	countries map[string]struct{}
}

type countryRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

func (g *geoip) lookup(r *http.Request) string {
	var src net.IP
	if g.fromLast {
		src = snet.RemoteHostFromLast(r)
	} else {
		src = snet.RemoteHost(r)
	}

	record := countryRecord{}
	err := g.db.Lookup(src, &record)
	if err != nil {
		fmt.Printf("geoip(): failed to lookup %s: %s", src, err)
	}
	if record.Country.ISOCode == "" {
		return "UNKNOWN"
	}
	return record.Country.ISOCode
}

func (g *geoip) Request(c filters.FilterContext) {
	c.Request().Header.Set(g.header, g.lookup(c.Request()))
}

func (g *geoip) Response(c filters.FilterContext) {}

func (g *geoip) Match(r *http.Request) bool {
	span := ot.SpanFromContext(r.Context())
	if span != nil {
		span.LogKV("GeoIP", "start")
	}

	code := g.lookup(r)
	_, ok := g.countries[code]

	if span != nil {
		span.LogKV("GeoIP", code)
	}
	return ok
}
```

## OpenTracing plugins

The tracers, except for `noop`, are built as Go Plugins. A tracing plugin can
be loaded with `-opentracing NAME` as parameter to skipper.

Implementations of OpenTracing API can be found in the
https://github.com/skipper-plugins/opentracing repository.

All plugins must have a function named `InitTracer` with the following signature

    func([]string) (opentracing.Tracer, error)

The parameters passed are all arguments for the plugin, i.e. everything after the first
word from skipper's -opentracing parameter. E.g. when the -opentracing parameter is
`mytracer foo=bar token=xxx somename=bla:3` the "mytracer" plugin will receive

    []string{"foo=bar", "token=xxx", "somename=bla:3"}

as arguments.

The tracer plugin implementation is responsible to parse the received arguments.

An example plugin looks like
```go
package main

import (
     basic "github.com/opentracing/basictracer-go"
     opentracing "github.com/opentracing/opentracing-go"
)

func InitTracer(opts []string) (opentracing.Tracer, error) {
     return basic.NewTracerWithOptions(basic.Options{
         Recorder:       basic.NewInMemoryRecorder(),
         ShouldSample:   func(traceID uint64) bool { return traceID%64 == 0 },
         MaxLogsPerSpan: 25,
     }), nil
}
```
