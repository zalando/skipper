# lightstep tracer

As with [other tracers](https://pkg.go.dev/github.com/zalando/skipper/tracing), the lightstep tracer is configured by setting
`-opentracing="lightstep OPTIONS"`. Valid options are:

* `component-name` - set component name instead of `skipper`
* `token` - Access token for the lightstep satellites (REQUIRED)
* `protocol` - sets `UseGRPC` option to true if set to `"grpc"`, defaults to `"grpc"`
* `grpc-max-msg-size` - maximum size for gRPC messages
* `min-period` - minimum time to wait before sending spans to the satellites, string with value parseable by `time.ParseDuration`
* `max-period` - maximum time to wait before sending spans to the satellites, string with value parseable by `time.ParseDuration`
* `max-buffered-spans` - maximum number of buffered spans before sending to the satellites
* `tag` - key-value pairs (`key=value`) separated by commas (`,`) to set as tags
  in every span
* `collector` - hostname (+port) - (e.g. `lightstep-satellites.example.org:4443`)  to send the
   spans to, i.e. your lightstep satellites
* `plaintext` (boolean) - force plaintext communication with satellites
* `cmd-line` - override what is reported as command line
* `log-events` - if this option is present, it will log events from the tracer
* `propagators` - set propagators to use (i.e. format of http headers used for tracing). This can be used
  to pick up traces from / to applications which only understand the B3 format (e.g. grafana where the
  jaeger instrumentation can be switched to use the B3 format). This can be combined, e.g. `lightstep,b3`
  should be used to pick up both formats (attempted in that order).
  * `lightstep` or `ls` - use the standard lightstep headers (default)
  * `b3` - use the B3 propagation format
