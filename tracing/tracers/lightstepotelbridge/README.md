# lightstep-otel-bridge tracer

As with [other tracers](https://pkg.go.dev/github.com/zalando/skipper/tracing), the lightstep-otel-bridge tracer is configured by setting
`-opentracing="lightstep-otel-bridge OPTIONS"`. Valid options are:

* `component-name` - set component name instead of `skipper`
* `access-token` - Access token for the lightstep satellites (REQUIRED)
* `protocol` - sets `UseGRPC` option to true if set to `"grpc"`, defaults to `"grpc"`, but can be set to `"http"`
* `tag` - key-value pairs (`key=value`) separated by commas (`,`) to set as tags
  in every span
* `environment` - set the environment tag, defaults to `dev`
* `service-name` - set the service name tag, defaults to `skipper`
* `service-version` - set the service version tag, defaults to `unknown`
* `batch-size` - maximum number of spans to send in a batch
* `batch-timeout` - maximum time ms to wait before sending spans to the satellites
* `processor-queue-size` - maximum number of spans to queue before sending to the satellites
* `export-timeout` - maximum time to wait in ms for a batch to be sent
* `collector` - hostname (+port) - (e.g. `lightstep-satellites.example.org:4443`)  to send the
  spans to, i.e. your lightstep satellites
* `insecure-connection` (boolean) - force plaintext communication with satellites
* `propagators` - set propagators to use (i.e. format of http headers used for tracing). This can be used
  to pick up traces from / to applications which only understand the B3 format (e.g. grafana where the
  jaeger instrumentation can be switched to use the B3 format). This can be combined, e.g. `ottrace,b3`
  should be used to pick up both formats (attempted in that order).
    * `ottrace` - use the standard lightstep headers (default)
    * `b3` - use the B3 propagation format
    * `baggage` - use the baggage propagation format
    * `tracecontext` - use the tracecontext propagation format
