# Operations

This is the work in progress operations guide for showing information,
which are relevant for production use.

Skipper is proven to scale with number of routes beyond 300.000 routes
per instance. Skipper is running with peaks to 65.000 http requests
per second using multiple instances.

# Connection Options

Skipper's connection options are allowing you to set Go's [http.Server](https://golang.org/pkg/net/http/#Server)
Options on the client side and [http.Transport](https://golang.org/pkg/net/http/#Transport) on the backend side.

"It is recommended to read
[this blog post about net http timeouts](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/)
in order to better understand the impact of these settings.

## Backend

Backend is the side skipper opens a client connection to.

Closing idle connections is required for DNS failover, because Go's
[http.Transport](https://golang.org/pkg/net/http/#Transport) caches
DNS lookups and needs to create new connections for doing so. Skipper
will start a goroutine and use the specified
[time.Duration](https://golang.org/pkg/time/#Duration) to call
CloseIdleConnections() on that
[http.Transport](https://golang.org/pkg/net/http/#Transport).

    -close-idle-conns-period string
        period of closing all idle connections in seconds or as a
        duration string. Not closing when less than 0 (default "20")

This will set MaxIdleConnsPerHost on the
[http.Transport](https://golang.org/pkg/net/http/#Transport) to limit
the number of idle connections per backend such that we do not run out
of sockets.

    -idle-conns-num int
        maximum idle connections per backend host (default 64)

This will set MaxIdleConns on the
[http.Transport](https://golang.org/pkg/net/http/#Transport) to limit
the number for all backends such that we do not run out of sockets.

    -max-idle-connection-backend int
        sets the maximum idle connections for all backend connections

This will set TLSHandshakeTimeout on the
[http.Transport](https://golang.org/pkg/net/http/#Transport) to have
timeouts based on TLS connections.

    -tls-timeout-backend duration
        sets the TLS handshake timeout for backend connections (default 1m0s)

This will set Timeout on
[net.Dialer](https://golang.org/pkg/net/#Dialer) that is the
implementation of DialContext, which is the TCP connection pool used in the
[http.Transport](https://golang.org/pkg/net/http/#Transport).

    -timeout-backend duration
        sets the TCP client connection timeout for backend connections (default 1m0s)

This will set KeepAlive on
[net.Dialer](https://golang.org/pkg/net/#Dialer) that is the
implementation of DialContext, which is the TCP connection pool used in the
[http.Transport](https://golang.org/pkg/net/http/#Transport).

    -keepalive-backend duration
        sets the keepalive for backend connections (default 30s)

This will set DualStack (IPv4 and IPv6) on
[net.Dialer](https://golang.org/pkg/net/#Dialer) that is the
implementation of DialContext, which is the TCP connection pool used in the
[http.Transport](https://golang.org/pkg/net/http/#Transport).

    -enable-dualstack-backend
        enables DualStack for backend connections (default true)


## Client

Client is the side skipper gets incoming calls from.
Here we can set timeouts in different parts of the http connection.

This will set ReadTimeout in
[http.Server](https://golang.org/pkg/net/http/#Server) handling
incoming calls from your clients.

    -read-timeout-server duration
        set ReadTimeout for http server connections (default 5m0s)

This will set ReadHeaderTimeout in
[http.Server](https://golang.org/pkg/net/http/#Server) handling
incoming calls from your clients.

    -read-header-timeout-server duration
        set ReadHeaderTimeout for http server connections (default 1m0s)

This will set WriteTimeout in
[http.Server](https://golang.org/pkg/net/http/#Server) handling
incoming calls from your clients.

    -write-timeout-server duration
        set WriteTimeout for http server connections (default 1m0s)

This will set IdleTimeout in
[http.Server](https://golang.org/pkg/net/http/#Server) handling
incoming calls from your clients. If you have another loadbalancer
layer in front of your Skipper http routers, for example [AWS Application Load
Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/application-load-balancers.html#connection-idle-timeout),
you should make sure, that Skipper's `idle-timeout-server` setting is
bigger than the idle timeout from the loadbalancer in front. Wrong
combinations of idle timeouts can lead to a few unexpected HTTP 502.

    -idle-timeout-server duration
        maximum idle connections per backend host (default 1m0s)

This will set MaxHeaderBytes in
[http.Server](https://golang.org/pkg/net/http/#Server) to limit the
size of the http header from your clients.

    -max-header-bytes int
        set MaxHeaderBytes for http server connections (default 1048576)

## TCP LIFO

Skipper implements now controlling the maximum incoming TCP client
connections. This is an experimental feature.

The purpose of the mechanism is to prevent Skipper requesting more memory
than available in case of too many concurrent connections, especially in
an autoscaling deployment setup, in those case when the scaling is not
fast enough to follow sudden connection spikes.

This solution relies on a listener implementation combined with a LIFO
queue. It allows only a limited number of connections being handled
concurrently, defined by the max concurrency configuration. When the max
concurrency limit is reached, the new incoming client connections are
stored in a queue. When an active (accepted) connection is closed, the
most recent pending connection from the queue will be accepted. When the
queue is full, the oldest pending connection is closed and dropped, and the
new one is inserted into the queue.

The feature can be enabled with the `-enable-tcp-queue` flag. The maximum
concurrency can bet set with the `-max-tcp-listener-concurrency` flag, or,
if this flag is not set, then Skipper tries to infer the maximum accepted
concurrency from the system by reading the
/sys/fs/cgroup/memory/memory.limit_in_bytes file. In this case, it uses the
average expected per request memory requirement, which can be set with the
`-expected-bytes-per-request` flag.

Note that the automatically inferred limit may not work as expected in an
environment other than cgroups v1.

## OAuth2 Tokeninfo

OAuth2 filters integrate with external services and have their own
connection handling. Outgoing calls to these services have a
default timeout of 2s, which can be changed by the flag
`-oauth2-tokeninfo-timeout=<OAuthTokeninfoTimeout>`.

## OAuth2 Tokenintrospection RFC7662

OAuth2 filters integrate with external services and have their own
connection handling. Outgoing calls to these services have a
default timeout of 2s, which can be changed by the flag
`-oauth2-tokenintrospect-timeout=<OAuthTokenintrospectionTimeout>`.

# Monitoring

Monitoring is one of the most important things you need to run in
production and skipper has a [godoc page](https://godoc.org/github.com/zalando/skipper)
for the [metrics package](https://godoc.org/github.com/zalando/skipper/metrics),
describing options and most keys you will find in the metrics handler
endpoint. The default is listening on `:9911/metrics`. You can modify
the listen port with the `-support-listener` flag. Metrics can exposed
using formats Codahale (json) or Prometheus and be configured by
`-metrics-flavour=`, which defaults to `codahale`. To expose both
formats you can use a comma separated list: `-metrics-flavour=codahale,prometheus`.


## Prometheus

In case you want to get metrics in [Prometheus](https://prometheus.io/) format exposed, use this
option to enable it:

    -metrics-flavour=prometheus

It will return [Prometheus](https://prometheus.io/) metrics on the
common metrics endpoint :9911/metrics.

To monitor skipper we recommend the following queries:

- P99 backend latency: `histogram_quantile(0.99, sum(rate(skipper_serve_host_duration_seconds_bucket{}[1m])) by (le))`
- HTTP 2xx rate: `histogram_quantile(0.99, sum(rate(skipper_serve_host_duration_seconds_bucket{code =~ "2.*"}[1m])) by (le) )`
- HTTP 4xx rate: `histogram_quantile(0.99, sum(rate(skipper_serve_host_duration_seconds_bucket{code =~ "4.*"}[1m])) by (le) )`
- HTTP 5xx rate: `histogram_quantile(0.99, sum(rate(skipper_serve_host_duration_seconds_bucket{code =~ "52.*"}[1m])) by (le) )`
- Max goroutines (depends on label selector): `max(go_goroutines{application="skipper-ingress"})`
- Max threads (depends on label selector): `max(go_threads{application="skipper-ingress"})`
- max heap memory in use in MB (depends on label selector): `max(go_memstats_heap_inuse_bytes{application="skipper-ingress"}) / 1024 / 1000`
- Max number of heap objects (depends on label selector): `max(go_memstats_heap_objects{application="skipper-ingress"})`
- Max of P75 Go GC runtime in ms (depends on label selector): `max(go_gc_duration_seconds{application="skipper-ingress",quantile="0.75"}) * 1000 * 1000`
- P99 request filter duration (depends on label selector): `histogram_quantile(0.99, sum(rate(skipper_filter_request_duration_seconds_bucket{application="skipper-ingress"}[1m])) by (le) )`
- P99 response filter duration (depends on label selector): `histogram_quantile(0.99, sum(rate(skipper_filter_response_duration_seconds_bucket{application="skipper-ingress"}[1m])) by (le) )`
- If you use Kubernetes limits or Linux cgroup CFS quotas (depends on label selector): `sum(rate(container_cpu_cfs_throttled_periods_total{container_name="skipper-ingress"}[1m]))`

## Connection metrics

This option will enable known loadbalancer connections metrics, like
counters for active and new connections. This feature sets a metrics
callback on [http.Server](https://golang.org/pkg/net/http/#Server) and
uses a counter to collect
[http.ConnState](https://golang.org/pkg/net/http/#ConnState).

    -enable-connection-metrics
        enables connection metrics for http server connections

It will expose them in /metrics, for example json structure looks like this example:

    {
      "counters": {
        "skipper.lb-conn-active": {
          "count": 6
        },
        "skipper.lb-conn-closed": {
          "count": 6
        },
        "skipper.lb-conn-idle": {
          "count": 6
        },
        "skipper.lb-conn-new": {
          "count": 6
        }
      },
      /* stripped a lot of metrics here */
    }

## LIFO metrics

When enabled in the routes, LIFO queues can control the maximum concurrency level
proxied to the backends and mitigate the impact of traffic spikes. The current
level of concurrency and the size of the queue can be monitored with gauges per
each route using one of the lifo filters. To enable monitoring for the lifo
filters, use the command line option:

    -enable-route-lifo-metrics

When queried, it will return metrics like:

    {
      "gauges": {
        "skipper.lifo.routeXYZ.active": {
          "value": 245
        },
        "skipper.lifo.routeXYZ.queued": {
          "value": 27
        }
      }
    }

## Application metrics

Application metrics for your proxied applications you can enable with the option:

    -serve-host-metrics
        enables reporting total serve time metrics for each host

This will make sure you will get stats for each "Host" header as "timers":

    "timers": {
      "skipper.servehost.app1_example_com.GET.200": {
        "15m.rate": 0.06830666203045982,
        "1m.rate": 2.162612637718806e-06,
        "5m.rate": 0.008312609284452856,
        "75%": 236603815,
        "95%": 236603815,
        "99%": 236603815,
        "99.9%": 236603815,
        "count": 3,
        "max": 236603815,
        "mean": 116515451.66666667,
        "mean.rate": 0.0030589345776699827,
        "median": 91273391,
        "min": 21669149,
        "stddev": 89543653.71950394
      },
      "skipper.servehost.app1_example_com.GET.304": {
        "15m.rate": 0.3503336738177459,
        "1m.rate": 0.07923086447313292,
        "5m.rate": 0.27019839341602214,
        "75%": 99351895.25,
        "95%": 105381847,
        "99%": 105381847,
        "99.9%": 105381847,
        "count": 4,
        "max": 105381847,
        "mean": 47621612,
        "mean.rate": 0.03087161486272533,
        "median": 41676170.5,
        "min": 1752260,
        "stddev": 46489302.203724876
      },
      "skipper.servehost.app1_example_com.GET.401": {
        "15m.rate": 0.16838468990057648,
        "1m.rate": 0.01572861413072501,
        "5m.rate": 0.1194724817779537,
        "75%": 91094832,
        "95%": 91094832,
        "99%": 91094832,
        "99.9%": 91094832,
        "count": 2,
        "max": 91094832,
        "mean": 58090623,
        "mean.rate": 0.012304914018033056,
        "median": 58090623,
        "min": 25086414,
        "stddev": 33004209
      }
    },


To change the sampling type of how metrics are handled from
[uniform](https://godoc.org/github.com/rcrowley/go-metrics#UniformSample)
to [exponential decay](https://godoc.org/github.com/rcrowley/go-metrics#ExpDecaySample),
you can use the following option, which is better for not so huge
utilized applications (less than 100 requests per second):

    -metrics-exp-decay-sample
        use exponentially decaying sample in metrics


## Go metrics

Metrics from the
[go runtime memstats](https://golang.org/pkg/runtime/#MemStats)
are exposed from skipper to the metrics endpoint, default listener
:9911, on path /metrics :

    "gauges": {
      "skipper.runtime.MemStats.Alloc": {
        "value": 3083680
      },
      "skipper.runtime.MemStats.BuckHashSys": {
        "value": 1452675
      },
      "skipper.runtime.MemStats.DebugGC": {
        "value": 0
      },
      "skipper.runtime.MemStats.EnableGC": {
        "value": 1
      },
      "skipper.runtime.MemStats.Frees": {
        "value": 121
      },
      "skipper.runtime.MemStats.HeapAlloc": {
        "value": 3083680
      },
      "skipper.runtime.MemStats.HeapIdle": {
        "value": 778240
      },
      "skipper.runtime.MemStats.HeapInuse": {
        "value": 4988928
      },
      "skipper.runtime.MemStats.HeapObjects": {
        "value": 24005
      },
      "skipper.runtime.MemStats.HeapReleased": {
        "value": 0
      },
      "skipper.runtime.MemStats.HeapSys": {
        "value": 5767168
      },
      "skipper.runtime.MemStats.LastGC": {
        "value": 1516098381155094500
      },
      "skipper.runtime.MemStats.Lookups": {
        "value": 2
      },
      "skipper.runtime.MemStats.MCacheInuse": {
        "value": 6944
      },
      "skipper.runtime.MemStats.MCacheSys": {
        "value": 16384
      },
      "skipper.runtime.MemStats.MSpanInuse": {
        "value": 77368
      },
      "skipper.runtime.MemStats.MSpanSys": {
        "value": 81920
      },
      "skipper.runtime.MemStats.Mallocs": {
        "value": 1459
      },
      "skipper.runtime.MemStats.NextGC": {
        "value": 4194304
      },
      "skipper.runtime.MemStats.NumGC": {
        "value": 0
      },
      "skipper.runtime.MemStats.PauseTotalNs": {
        "value": 683352
      },
      "skipper.runtime.MemStats.StackInuse": {
        "value": 524288
      },
      "skipper.runtime.MemStats.StackSys": {
        "value": 524288
      },
      "skipper.runtime.MemStats.Sys": {
        "value": 9246968
      },
      "skipper.runtime.MemStats.TotalAlloc": {
        "value": 35127624
      },
      "skipper.runtime.NumCgoCall": {
        "value": 0
      },
      "skipper.runtime.NumGoroutine": {
        "value": 11
      },
      "skipper.runtime.NumThread": {
        "value": 9
      }
    },
    "histograms": {
      "skipper.runtime.MemStats.PauseNs": {
        "75%": 82509.25,
        "95%": 132609,
        "99%": 132609,
        "99.9%": 132609,
        "count": 12,
        "max": 132609,
        "mean": 56946,
        "median": 39302.5,
        "min": 28749,
        "stddev": 31567.015005117817
      }
   }

# Dataclient

Dataclients poll some kind of data source for routes. To change the
timeout for calls that polls a dataclient, which could be the
Kubernetes API, use the following option:

    -source-poll-timeout int
        polling timeout of the routing data sources, in milliseconds (default 3000)


# Routing table information

Skipper allows you to get some runtime insights. You can get the
current routing table from skipper with in the
[eskip file format](https://godoc.org/github.com/zalando/skipper/eskip):

```
curl localhost:9911/routes
*
-> "http://localhost:12345/"
```

You also can get the number of routes `X-Count` and the UNIX timestamp
of the last route table update `X-Timestamp`, using a HEAD request:

```
curl -I localhost:9911/routes
HTTP/1.1 200 OK
Content-Type: text/plain
X-Count: 1
X-Timestamp: 1517777628
Date: Sun, 04 Feb 2018 20:54:31 GMT
```

The number of routes given is limited (1024 routes by default).
In order to control this limits, there are two parameters: `limit` and
`offset`. The `limit` defines the number of routes to get and
`offset` where to start the list. Thanks to this, it's possible
to get the results paginated or getting all of them at the same time.

```
curl localhost:9911/routes?offset=200&limit=100
```

# Memory consumption

While Skipper is generally not memory bound, some features may require
some attention and planning regarding the memory consumption.

Potentially high memory consumers:

- Metrics
- Filters
- Slow Backends and chatty clients

Make sure you monitor backend latency, request and error rates.
Additionally use Go metrics for the number of goroutines and threads, GC pause
times should be less than 1ms in general, route lookup time, request
and response filter times and heap memory.

## Metrics

Memory consumption of metrics are dependent on enabled command line
flags. Make sure to monitor Go metrics.

If you use `-metrics-flavour=codahale,prometheus` you enable both
storage backends.

If you use the Prometheus histogram buckets `-histogram-metric-buckets`.

If you enable route based `-route-backend-metrics`
`-route-response-metrics` `-serve-route-metrics`, error codes
`-route-response-metrics` and host `-serve-host-metrics` based metrics
it can count up. Please check the support listener endpoint (default
9911) to understand the usage:

```
% curl localhost:9911/metrics
```

## Filters

Ratelimit filter `clusterClientRatelimit` implementation using the
swim based protocol, consumes roughly 15MB per filter for 100.000
individual clients and 10 maximum hits. Make sure you monitor Go
metrics. Ratelimit filter `clusterClientRatelimit` implementation
using the Redis ring based solution, adds 2 additional roundtrips to
redis per hit. Make sure you monitor redis closely, because skipper
will fallback to allow traffic if redis can not be reached.

## Slow Backends

Skipper has to keep track of all active connections and http
Requests. Slow Backends can pile up in number of connections, that
will consume each a little memory per request. If you have high
traffic per instance and a backend times out it can start to increase
your memory consumption. Make sure you monitor backend latency,
request and error rates.

# Default Filters

Default filters will be applied to all routes created or updated.

## Global Default Filters

Global default filters can be specified via two different command line
flags `-default-filters-prepend` and
`-default-filters-append`. Filters passed to these command line flags
will be applied to all routes. The difference `prepend` and `append` is
where in the filter chain these default filters are applied.

For example a user specified the route: `r: * -> setPath("/foo")`
If you run skipper with `-default-filters-prepend=enableAccessLog(4,5) -> lifo(100,100,"10s")`,
the actual route will look like this: `r: * -> enableAccessLog(4,5) -> lifo(100,100,"10s") -> setPath("/foo")`.
If you run skipper with `-default-filters-append=enableAccessLog(4,5) -> lifo(100,100,"10s")`,
the actual route will look like this: `r: *  -> setPath("/foo") -> enableAccessLog(4,5) -> lifo(100,100,"10s")`.

## Kubernetes Default Filters

Kubernetes dataclient supports default filters. You can enable this feature by
specifying `default-filters-dir`. The defined directory must contain per-service
filter configurations, with file name following the pattern `${service}.${namespace}`.
The content of the files is the actual filter configurations. These filters are then
prepended to the filters already defined in Ingresses.

The default filters are supposed to be used only if the filters of the same kind
are not configured on the Ingress resource. Otherwise, it can and will lead to
potentially contradicting filter configurations and race conditions, i.e.
you should specify a specific filter either on the Ingress resource or as
a default filter.

# Scheduler

HTTP request schedulers change the queuing behavior of in-flight
requests. A queue has two generic properties: a limit of requests and
a concurrency level.  The limit of request can be unlimited (unbounded
queue), or limited (bounded queue).  The concurrency level is either
limited or unlimited.

The default scheduler is an unbounded first in first out (FIFO) queue,
that is provided by [Go's](https://golang.org/) standard library.

Skipper provides 2 last in first out (LIFO) filters to change the
scheduling behavior.

On failure conditions, Skipper will return HTTP status code:

- 503 if the queue is full, which is expected on the route with a failing backend
- 502 if queue access times out, because the queue access was not fast enough
- 500 on unknown errors, please create [an issue](https://github.com/zalando/skipper/issues/new/choose)

## The problem

Why should you use boundaries to limit concurrency level and limit the
queue?

The short answer is resiliency. If you have one route, that is timing
out, the request queue of skipper will pile up and consume much more
memory, than before. This can lead to out of memory kill, which will
affect all other routes. In [this
comment](https://github.bus.zalan.do/teapot/issues/issues/1792#issuecomment-1315569)
you can see the memory usage increased in [Go's](https://golang.org/)
standard library `bufio` package.

Why LIFO queue instead of FIFO queue?

In normal cases the queue should not contain many requests. Skipper is
able to process many requests concurrently without letting the queue
piling up. In overrun situations you might want to process at least
some fraction of requests instead of timing out all requests. LIFO
would not time out all requests within the queue, if the backend is
capable of responding some requests fast enough.

## A solution

Skipper has two filters [`lifo()`](../../reference/filters/#lifo) and
[`lifoGroup()`](../../reference/filters/#lifogroup), that can limit
the number of requests for a route.  A [documented load
test](https://github.com/zalando/skipper/pull/1030#issuecomment-485714338)
shows the behavior with an enabled `lifo(100,100,"10s")` filter for
all routes, that was added by default. You can do this, if you pass
the following flag to skipper:
`-default-filters-prepend=lifo(100,100,"10s")`.

Both LIFO filters will, use a last in first out queue to handle most
requests fast. If skipper is in an overrun mode, it will serve some
requests fast and some will timeout. The idea is based on Dropbox
bandaid proxy, which is not opensource. [Dropbox](https://dropbox.com/)
shared their idea in a [public
blogpost](https://blogs.dropbox.com/tech/2018/03/meet-bandaid-the-dropbox-service-proxy/).

Skipper's scheduler implementation makes sure, that one route will not
interfere with other routes, if these routes are not in the same
scheduler group. [`LifoGroup`](../../reference/filters/#lifogroup) has
a user chosen scheduler group and
[`lifo()`](../../reference/filters/#lifo) will get a per route unique
scheduler group.

# URI standards interpretation

Considering the following request path: /foo%2Fbar, Skipper can handle
it in two different ways. The current default way is that when the
request is parsed purely relying on the Go stdlib url package, this
path becomes /foo/bar. According to RFC 2616 and RFC 3986, this may
be considered wrong, and this path should be parsed as /foo%2Fbar.
This is possible to achieve centrally, when Skipper is started with
the -rfc-patch-path flag. It is also possible to allow the default
behavior and only force the alternative interpretation on a per-route
basis with the rfcPath() filter. See
[`rfcPath()`](../../reference/filters/#rfcPath).

If the second interpretation gets considered the right way, and the
other one a bug, then the default value for this flag may become to
be on.

