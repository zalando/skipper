// Package tracing handles opentracing support for skipper
//
// Implementations of Opentracing API can be found in the https://github.com/skipper-plugins.
// It follows how to implement a new tracer plugin for this interface.
//
// The tracers, except for "noop", are built as Go Plugins. Note the warning from Go's
// plugin.go:
//
//	// The plugin support is currently incomplete, only supports Linux,
//	// and has known bugs. Please report any issues.
//
// All plugins must have a function named "InitTracer" with the following signature
//
//	func([]string) (opentracing.Tracer, error)
//
// The parameters passed are all arguments for the plugin, i.e. everything after the first
// word from skipper's -opentracing parameter. E.g. when the -opentracing parameter is
// "mytracer foo=bar token=xxx somename=bla:3" the "mytracer" plugin will receive
//
//	[]string{"foo=bar", "token=xxx", "somename=bla:3"}
//
// as arguments.
//
// The tracer plugin implementation is responsible to parse the received arguments.
//
// An example plugin looks like
//
//	package main
//
//	import (
//	     basic "github.com/opentracing/basictracer-go"
//	     opentracing "github.com/opentracing/opentracing-go"
//	)
//
//	func InitTracer(opts []string) (opentracing.Tracer, error) {
//	     return basic.NewTracerWithOptions(basic.Options{
//	         Recorder:       basic.NewInMemoryRecorder(),
//	         ShouldSample:   func(traceID uint64) bool { return traceID%64 == 0 },
//	         MaxLogsPerSpan: 25,
//	     }), nil
//	}
//
// This should be built with
//
//	go build -buildmode=plugin -o basic.so ./basic/basic.go
//
// and copied to the given as -plugindir (by default, "./plugins").
//
// Then it can be loaded with -opentracing basic as parameter to skipper.
package tracing

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"plugin"

	ot "github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/tracing/tracers/basic"
	"github.com/zalando/skipper/tracing/tracers/instana"
	"github.com/zalando/skipper/tracing/tracers/jaeger"
	"github.com/zalando/skipper/tracing/tracers/lightstep"

	originstana "github.com/instana/go-sensor"
	origlightstep "github.com/lightstep/lightstep-tracer-go"
	origbasic "github.com/opentracing/basictracer-go"
	origjaeger "github.com/uber/jaeger-client-go"
	"go.opentelemetry.io/otel/trace"
)

// InitTracer initializes an opentracing tracer. The first option item is the
// tracer implementation name.
func InitTracer(opts []string) (tracer ot.Tracer, err error) {
	if len(opts) == 0 {
		return nil, errors.New("opentracing: the implementation parameter is mandatory")
	}
	var impl string
	impl, opts = opts[0], opts[1:]

	switch impl {
	case "noop":
		return &ot.NoopTracer{}, nil
	case "basic":
		return basic.InitTracer(opts)
	case "instana":
		return instana.InitTracer(opts)
	case "jaeger":
		return jaeger.InitTracer(opts)
	case "lightstep":
		return lightstep.InitTracer(opts)
	default:
		return nil, fmt.Errorf("tracer '%s' not supported", impl)
	}
}

func LoadTracingPlugin(pluginDirs []string, opts []string) (tracer ot.Tracer, err error) {
	for _, dir := range pluginDirs {
		tracer, err = LoadPlugin(dir, opts)
		if err == nil {
			return tracer, nil
		}
	}
	return nil, err
}

// LoadPlugin loads the given opentracing plugin and returns an opentracing.Tracer
// DEPRECATED, use LoadTracingPlugin
func LoadPlugin(pluginDir string, opts []string) (ot.Tracer, error) {
	if len(opts) == 0 {
		return nil, errors.New("opentracing: the implementation parameter is mandatory")
	}
	var impl string
	impl, opts = opts[0], opts[1:]

	if impl == "noop" {
		return &ot.NoopTracer{}, nil
	}

	pluginFile := filepath.Join(pluginDir, impl+".so") // FIXME this is Linux and other ELF...
	mod, err := plugin.Open(pluginFile)
	if err != nil {
		return nil, fmt.Errorf("open module %s: %s", pluginFile, err)
	}
	sym, err := mod.Lookup("InitTracer")
	if err != nil {
		return nil, fmt.Errorf("lookup module symbol failed for %s: %s", impl, err)
	}
	fn, ok := sym.(func([]string) (ot.Tracer, error))
	if !ok {
		return nil, fmt.Errorf("module %s's InitTracer function has wrong signature", impl)
	}
	tracer, err := fn(opts)
	if err != nil {
		return nil, fmt.Errorf("module %s returned: %s", impl, err)
	}
	return tracer, nil
}

// CreateSpan creates a started span from an optional given parent from context
func CreateSpan(name string, ctx context.Context, openTracer ot.Tracer) ot.Span {
	parentSpan := ot.SpanFromContext(ctx)
	if parentSpan == nil {
		return openTracer.StartSpan(name)
	}
	return openTracer.StartSpan(name, ot.ChildOf(parentSpan.Context()))
}

// LogKV will add a log to the span from the given context
func LogKV(k, v string, ctx context.Context) {
	if span := ot.SpanFromContext(ctx); span != nil {
		span.LogKV(k, v)
	}
}

type otelSpanContext interface {
	TraceID() trace.TraceID
}

// GetTraceID retrieves TraceID from HTTP request, for example to search for this trace
// in the UI of your tracing solution and to get more context about it
func GetTraceID(span ot.Span) string {
	if span == nil {
		return ""
	}

	spanContext := span.Context()
	if spanContext == nil {
		return ""
	}

	switch spanContextType := spanContext.(type) {
	case origbasic.SpanContext:
		return fmt.Sprintf("%x", spanContextType.TraceID)
	case originstana.SpanContext:
		return fmt.Sprintf("%x", spanContextType.TraceID)
	case origjaeger.SpanContext:
		return spanContextType.TraceID().String()
	case origlightstep.SpanContext:
		return fmt.Sprintf("%x", spanContextType.TraceID)
	case otelSpanContext:
		return spanContextType.TraceID().String()
	}

	return ""
}
