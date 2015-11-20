/*
Package metrics implements collection of common performance metrics.

It uses the Go implementation of the Coda Hale metrics library:

https://github.com/dropwizard/metrics
https://github.com/rcrowley/go-metrics

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
return a JSON response including all the collected metrics. Please note that a lot of metrics are created lazily
whenever a request triggers them. This means that the API response will depend on the current routes and
the filters used. In the case there are no metrics due to inactivity, the API will return 404.

You can also query for specific metrics, individually or by prefix matching. You can either use the metrics key name
and you should get back only the values for that particular key or a prefix in which case you should get all the
metrics that share the same prefix.

If you request an unknown key or prefix the response will be an HTTP 404.

*/
package metrics
