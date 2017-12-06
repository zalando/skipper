/*
Package metrics implements collection of common performance metrics.

The metrics can be exposed in multiple formats. At this moment are:

* Coda Hale format.
* Prometheus format.

For CodaHale format it uses the Go implementation of the Go Coda Hale metrics library:

https://github.com/dropwizard/metrics
https://github.com/rcrowley/go-metrics

For Prometheus format is will use Prometheus Go  official client library:

https://github.com/prometheus/client_golang

The collected metrics include detailed information about Skipper's relevant processes while serving requests -
looking up routes, filters (aggregate and individual), backend communication and forwarding the response to
the client.

Options

To enable metrics, it needs to be initialized with a Listener address. In this case, Skipper will start an additional
http listener, where the current metrics values can be downloaded.

You can define a custom Prefix to every reported metrics key. This allows you to avoid conflicts between Skipper's
metrics and other systems if you aggregate them later in some monitoring system. The default prefix is "skipper."

You can also enable some Go garbage collector and runtime metrics using EnableDebugGcMetrics and EnableRuntimeMetrics,
respectively.

REST API

This listener accepts GET requests on the /metrics endpoint like any other REST api. A request to "/metrics" should
return a JSON response including all the collected metrics if CodaHale format is used, or in Plain text if Prometheus
format is use . Please note that a lot of metrics are created lazily whenever a request triggers them. This means that
the API response will depend on the current routes and the filters used. In the case there are no metrics due to inactivity,
the API will return 404 if CodaHale is used or 200 if Prometheus is used.

If you use CodaHale format you can also query for specific metrics, individually or by prefix matching. You can either use the metrics key name
and you should get back only the values for that particular key or a prefix in which case you should get all the
metrics that share the same prefix. If you request an unknown key or prefix the response will be an HTTP 404.

Prometheus doesn't need to support this, the Prometheus server will grab everything.
*/
package metrics
