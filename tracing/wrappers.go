package tracing

import (
	"context"
	"runtime"

	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	otelembedded "go.opentelemetry.io/otel/trace/embedded"
)

type otSpanContextKey struct{}

var wireContextKey otSpanContextKey

// TracerWrapper is a wrapper around OpenTracing tracer that implements
// Open OpenTelemetry tracer interface, translating all calls to OpenTelemetry
// API to OpenTracing API effectivelly enabling Skipper to operate with
// both OpenTelemetry and OpenTracing keeping retrocompatibility.
type TracerWrapper struct {
	otelembedded.Tracer
	Ot ot.Tracer
}

// SpanWrapper is a wrapper around OpenTracing span that implements
// Open OpenTelemetry Span interface, translating all calls to OpenTelemetry
// API to OpenTracing API effectivelly enabling Skipper to operate with
// both OpenTelemetry and OpenTracing keeping retrocompatibility.
type SpanWrapper struct {
	otelembedded.Span
	Ot ot.Span
}

// These constants are based on semantic conventions.
// This package only defines what is not available in:
// "go.opentelemetry.io/otel/semconv/v1.24.0"
// but was previously supported through Open tracing in:
// github.com/opentracing/opentracing-go/ext/tags.go
const (
	ComponentTag = "component"
	SpanKindTag  = "span.kind"
	ErrorTag     = "error"
)

// Start Creates a SpanWrapper adds it to Go stdlib context and finaly returns both the span and the context.
// currently TracerWrapper.Start does not convert any Open telemetry start option to Open tracing start
// options, the parameter 'opts' is mostly ignored.
func (t *TracerWrapper) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	span := &SpanWrapper{}

	// If the value in the wirecontext is not of type ot.SpanContext it will just be ignored.
	wireContext, ok := ctx.Value(wireContextKey).(ot.SpanContext)
	if ok && wireContext != nil {
		span.Ot = t.Ot.StartSpan(spanName, ext.RPCServerOption(wireContext))
		return ot.ContextWithSpan(ctx, span.Ot), span
	}

	// If there is no wireContext in the context, check if there is a parent span in the context.
	parentSpan := ot.SpanFromContext(ctx)
	if parentSpan != nil {
		span.Ot = t.Ot.StartSpan(spanName, ot.ChildOf(parentSpan.Context()))
		return ot.ContextWithSpan(ctx, span.Ot), span
	}

	// If no wireContext and no parentSpan in the context, just start a root span.
	span.Ot = t.Ot.StartSpan(spanName)
	return ot.ContextWithSpan(ctx, span.Ot), span
}

// AddEvent emulates OpenTelemetry span.AddEvent function by logging key:value data
// in a OpenTracing span within a SpanWrapper. This is done by logging both the key:value
// passed as option and the event name as "event":"$eventName" effectivelly storing two
// key:value pairs. For example:
//
//	spanwrapper.AddEvent("some event name", trace.WithAttributes("key", "value"))
//
// is equivalent to:
//
//	otSpan.LogKV(
//	    "event", "some event name",
//	    "key", "value",
//	)
func (sw *SpanWrapper) AddEvent(eventName string, opts ...trace.EventOption) {
	alternatingKV := make([]interface{}, 0)
	alternatingKV = append(alternatingKV, "event", eventName)
	ec := trace.NewEventConfig(opts...)
	for _, attr := range ec.Attributes() {
		alternatingKV = append(alternatingKV, string(attr.Key), attr.Value.AsInterface())
	}

	sw.Ot.LogKV(alternatingKV...)
}

// IsRecording always return true. The read only behavior is not implemented in the
// wrapper because we rely on ot.SpanFromContext behavior in many parts of the code
// and we can't reliably store this state in the context since this read only concept
// is not present in Open Tracing APIs.
func (sw *SpanWrapper) IsRecording() bool {
	return true
}

// RecordError will record err as a span event for this span. An additional call to
// SetStatus is required if the Status of the Span should be set to Error, this method
// does not change the Span status. If err is nil this method does nothing.
func (sw *SpanWrapper) RecordError(err error, options ...trace.EventOption) {
	if err == nil {
		return
	}

	stackTrace := make([]byte, 2048)
	n := runtime.Stack(stackTrace, false)

	opts := append(options, trace.WithAttributes(
		semconv.ExceptionStacktrace(string(stackTrace[0:n])),
		semconv.ExceptionMessage(err.Error())),
	)

	sw.AddEvent(semconv.ExceptionEventName, opts...)
}

// SpanContext is not implemented. It will not fail when called but will always return
// an empty SpanContext.
// SpanContext is implementation specific and through Ot.Span and Ot.SpanContext interface
// we don't have access to enough information to build a complete otel.SpanContext.
// Also each Ot.SpanContext implementation use different types for SpanID, TraceID etc
// making it impossible to consistently get a value from these implementations and
// convert it to Otel types. To retrieve Span and Trace ID please use the functions
// GetTraceID and GetSpanID defined in this package.
func (sw *SpanWrapper) SpanContext() trace.SpanContext {
	return trace.SpanContext{}
}

// SetStatus is not implemented. There is no equivalent concept in OpenTracing.
func (sw *SpanWrapper) SetStatus(code codes.Code, description string) {
	panic("SpanWrapper.SetStatus() is not implemented")
}

// SetName changes the operation name in the underlying span.
func (sw *SpanWrapper) SetName(name string) {
	sw.Ot.SetOperationName(name)
}

// SetAttributes will set tags in the underlying span.
func (sw *SpanWrapper) SetAttributes(kv ...attribute.KeyValue) {
	for _, attr := range kv {
		sw.Ot.SetTag(string(attr.Key), attr.Value.AsInterface())
	}
}

// TracerProvider is not implemented. A Tracer provider is not necessary
// for OpenTracing API. In case its necessary to support this operation a
// TracerProviderWrapper would have to be created to emulate OpenTelemetry
// behaviour in the wrapper.
func (sw *SpanWrapper) TracerProvider() trace.TracerProvider {
	panic("SpanWrapper.TracerProvider() is not implemented")
}

// End as the name suggests will terminate the span. A terminated span can
// still be referenced in the code, but any state changing functions will
// have undefined behavior.
func (sw *SpanWrapper) End(options ...trace.SpanEndOption) {
	sw.Ot.Finish()
}
