// Package tracing handles opentelemetry and opentracing support for skipper
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
//
// Also note that the API always return and receive opentelemetry.Tracer, to use opentracing
// within skipper its necessary to embedd it into a TracerWrapper, this type will implement
// opentelemetry.Tracer interface but will instead call opentracing.Tracer under the hood
// making it possible to pass and receive opentracing.Tracer to and from skipper.
package tracing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"plugin"

	ot "github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/tracing/tracers/basic"
	"github.com/zalando/skipper/tracing/tracers/instana"
	"github.com/zalando/skipper/tracing/tracers/jaeger"
	"github.com/zalando/skipper/tracing/tracers/lightstep"
	"github.com/zalando/skipper/tracing/tracers/otel"
	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	originstana "github.com/instana/go-sensor"
	origlightstep "github.com/lightstep/lightstep-tracer-go"
	origbasic "github.com/opentracing/basictracer-go"
	origjaeger "github.com/uber/jaeger-client-go"
)

// InitTracer initializes an opentelemetry tracer. The first option item is the
// tracer implementation name. If the first option is not "otel" InitTracer will
// initialize a opentracing tracer and embedd it into a TracerWrapper that will
// convert calls from opentelemetry API to opentracing API when necessary.
func InitTracer(opts []string) (tracer trace.Tracer, err error) {
	if len(opts) == 0 {
		return nil, errors.New("tracing: the implementation parameter is mandatory")
	}
	var impl string
	impl, opts = opts[0], opts[1:]

	switch impl {
	case "otel":
		ctx := context.TODO()
		return otel.InitTracer(ctx, opts)
	case "noop":
		return &TracerWrapper{Ot: &ot.NoopTracer{}}, nil
	case "basic":
		var err error
		tracer := &TracerWrapper{}
		tracer.Ot, err = basic.InitTracer(opts)
		return tracer, err
	case "instana":
		var err error
		tracer := &TracerWrapper{}
		tracer.Ot, err = instana.InitTracer(opts)
		return tracer, err
	case "jaeger":
		var err error
		tracer := &TracerWrapper{}
		tracer.Ot, err = jaeger.InitTracer(opts)
		return tracer, err
	case "lightstep":
		var err error
		tracer := &TracerWrapper{}
		tracer.Ot, err = lightstep.InitTracer(opts)
		return tracer, err
	default:
		return nil, fmt.Errorf("tracer '%s' not supported", impl)
	}
}

func LoadTracingPlugin(pluginDirs []string, opts []string) (tracer trace.Tracer, err error) {
	for _, dir := range pluginDirs {
		tracer, err = LoadPlugin(dir, opts)
		if err == nil {
			return tracer, nil
		}
	}
	return nil, err
}

// LoadPlugin loads the given opentracing plugin and returns an opentelemetry.Tracer
// be aware that in case your system only uses opentracing.Tracer, it will be necessary
// to use the TracerWrapper also provided in this package. TacerWrapper converts calls
// from opentelemetry.Tracer to opentracing.Tracer.
// DEPRECATED, use LoadTracingPlugin
func LoadPlugin(pluginDir string, opts []string) (trace.Tracer, error) {
	if len(opts) == 0 {
		return nil, errors.New("opentracing: the implementation parameter is mandatory")
	}
	var impl string
	impl, opts = opts[0], opts[1:]

	if impl == "noop" {
		return &TracerWrapper{Ot: &ot.NoopTracer{}}, nil
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
	fn, ok := sym.(func([]string) (trace.Tracer, error))
	if !ok {
		otfn, ok := sym.(func([]string) (ot.Tracer, error))
		if !ok {
			return nil, fmt.Errorf("module %s's InitTracer function has wrong signature", impl)
		}

		t, err := otfn(opts)
		if err != nil {
			return nil, fmt.Errorf("module %s returned: %s", impl, err)
		}
		return &TracerWrapper{Ot: t}, nil
	}
	tracer, err := fn(opts)
	if err != nil {
		return nil, fmt.Errorf("module %s returned: %s", impl, err)
	}
	return tracer, nil
}

// GetSpanID retrieves SpanID from HTTP request, for example to search for this span
// in the UI of your tracing solution and to get more context about it
func GetSpanID(span trace.Span) string {
	if span == nil {
		return ""
	}

	if sw, ok := span.(*SpanWrapper); ok {

		spanContext := sw.Ot.Context()
		if spanContext == nil {
			return ""
		}

		switch spanContextType := spanContext.(type) {
		case origbasic.SpanContext:
			return fmt.Sprintf("%x", spanContextType.SpanID)
		case originstana.SpanContext:
			return fmt.Sprintf("%x", spanContextType.SpanID)
		case origjaeger.SpanContext:
			return spanContextType.TraceID().String()
		case origlightstep.SpanContext:
			return fmt.Sprintf("%x", spanContextType.SpanID)
		}
	}

	return span.SpanContext().SpanID().String()
}


// GetTraceID retrieves TraceID from HTTP request, for example to search for this trace
// in the UI of your tracing solution and to get more context about it
func GetTraceID(span trace.Span) string {
	if span == nil {
		return ""
	}

	if sw, ok := span.(*SpanWrapper); ok {

		spanContext := sw.Ot.Context()
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
		}
	}

	return span.SpanContext().TraceID().String()
}

// SpanFromContext retrieves a span from a context, this span can either be a
// SpanWrapper or a open telemetry SDK span.
func SpanFromContext(ctx context.Context, tracer trace.Tracer) trace.Span {
	_, ok := tracer.(*TracerWrapper)
	if ok {
		s := ot.SpanFromContext(ctx)
		if s == nil {
			return nil
		}
		return &SpanWrapper{Ot: s}
	}

	// There is a possibility that otel.SpanFromContext returns a otel.noopSpan{}
	// This might be an unexpected behaviour to our code because ot.SpanFromContext 
    // in the same situation would return nil.	
    // NoopSpan has empty spanContext, to check if this is a noop span just check if TraceID/SpanID
    // exists.
    s := trace.SpanFromContext(ctx)
    if !s.SpanContext().HasTraceID() {
        return nil 
    }
    return s
}

// ContextWithSpan is the oposite of SpanFromContext, it will store a span into the context.
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	sw, ok := span.(*SpanWrapper)
	if ok {
		return ot.ContextWithSpan(ctx, sw.Ot)
	}

	return trace.ContextWithSpan(ctx, span)
}

// Extract will extract any inter-process stored in the request like previous
// spans or baggage items created by other services and store it in the context
// being returned.
func Extract(tracer trace.Tracer, req *http.Request) context.Context {
	t, ok := tracer.(*TracerWrapper)

	if !ok {
		carrier := propagation.HeaderCarrier(req.Header)
		ctx := otelapi.GetTextMapPropagator().Extract(req.Context(), carrier)
		return ctx
	}

	wireContext, err := t.Ot.Extract(ot.HTTPHeaders, ot.HTTPHeadersCarrier(req.Header))
	if err != nil {
		return req.Context()
	}
	return context.WithValue(req.Context(), wireContextKey, wireContext)
}

// Inject in the oposite of Extract. It will inject any inter-process information into a 
// *http.Request so this information can be propagated to other services.
func Inject(ctx context.Context, req *http.Request, span trace.Span, tracer trace.Tracer) *http.Request {
	t, ok := tracer.(*TracerWrapper)
	if !ok {
		carrier := propagation.HeaderCarrier(req.Header)
		otelapi.GetTextMapPropagator().Inject(ctx, carrier)
		return req.WithContext(ctx)
	}

	sp, ok := span.(*SpanWrapper)
	if !ok {
		return req.WithContext(ctx)
	}
	carrier := ot.HTTPHeadersCarrier(req.Header)
	_ = t.Ot.Inject(sp.Ot.Context(), ot.HTTPHeaders, carrier)

	return req.WithContext(ot.ContextWithSpan(ctx, sp.Ot))
}

// GetBaggageMember gets a BaggageItem from the context. This works both for TracerWrapper and
// open telemetry.
func GetBaggageMember(ctx context.Context, span trace.Span, key string) baggage.Member {
	sw, ok := span.(*SpanWrapper)
	if !ok {
		return baggage.FromContext(ctx).Member(key)
	}

	bagItem := sw.Ot.BaggageItem(key)
	m, err := baggage.NewMemberRaw(key, bagItem)
	if err != nil {
		return m
	}
	return m
}

// SetBaggageMember sets a BaggageItem to the context. This is the oposite of GetBaggageMember.
func SetBaggageMember(ctx context.Context, span trace.Span, key string, value string) error {
	sw, ok := span.(*SpanWrapper)
	if !ok {
        member, err := baggage.NewMember(key, value)
        if err != nil {
            return err
        }
        baggage.FromContext(ctx).SetMember(member)
        return nil
	}

    sw.Ot.SetBaggageItem(key, value)
	return nil
}

