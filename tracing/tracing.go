// Package tracing handles opentracing support for skipper
package tracing

import (
	"errors"
	"strings"

	instana "github.com/instana/golang-sensor"
	lightstep "github.com/lightstep/lightstep-tracer-go"
	ot "github.com/opentracing/opentracing-go"
)

var (
	// ErrUnsupportedTracer is returned when an unsupported opentracing
	// implementation was requested as tracer
	ErrUnsupportedTracer error = errors.New("invalid argument, not a supported tracer")
	// ErrMissingArguments is returned when an empty list is passed to Init()
	ErrMissingArguments error = errors.New("no arguments passed")
)

// Init sets up opentracing
func Init(opts []string) error {
	if len(opts) == 0 {
		return ErrMissingArguments
	}
	var impl string
	impl, opts = opts[0], opts[1:]

	var tracer ot.Tracer
	switch impl {
	case "noop":
		tracer = &ot.NoopTracer{}

	case "instana":
		tracer = instana.NewTracerWithOptions(&instana.Options{
			Service:  "skipper", // FIXME
			LogLevel: instana.Error,
		})

	case "lightstep":
		var token string
		for _, o := range opts {
			if strings.HasPrefix(o, "token=") {
				token = o[6:]
			}
		}
		tracer = lightstep.NewTracer(lightstep.Options{
			AccessToken: token,
			Tags: map[string]interface{}{
				lightstep.ComponentNameKey: "skipper", // FIXME
			},
		})

	default:
		return ErrUnsupportedTracer
	}
	ot.SetGlobalTracer(tracer)
	return nil
}
