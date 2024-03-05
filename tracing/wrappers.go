package tracing

import (
	"context"
	"net/http"

	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	otelembedded "go.opentelemetry.io/otel/trace/embedded"
)

type TracerWrapper struct {
	otelembedded.Tracer
	Ot ot.Tracer
}

type otSpanContextKey struct{}

var wireContextKey otSpanContextKey

type SpanWrapper struct {
	otelembedded.Span
	Ot ot.Span
}

// TODO(lucastt): its possible that otel has semcov pkg and I missed, I need to double check this.
// These constants are based on semantic convention and are compatible with
// the tags in: github.com/opentracing/opentracing-go/ext/tags.go
const (
	ComponentTag      = "component"
	HTTPUrlTag        = "http.url"
	HTTPMethodTag     = "http.method"
	SpanKindTag       = "span.kind"
	HTTPStatusCodeTag = "http.status_code"
	ErrorTag          = "error"
)

// TODO(lucastt): Check wether its necessary to convert other otel start options to ot start options. From our code this
// doesn't seems necessary.
func (t *TracerWrapper) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	span := &SpanWrapper{}

	wireContext, ok := ctx.Value(wireContextKey).(ot.SpanContext)
	if ok && wireContext != nil { // TODO(lucastt): check if this check is right...
		span.Ot = t.Ot.StartSpan(spanName, ext.RPCServerOption(wireContext))
		return ot.ContextWithSpan(ctx, span.Ot), span
	}

	// If wireContext in the context, check if there is a parent span in the context.
	parentSpan := ot.SpanFromContext(ctx)
	if parentSpan != nil {
		span.Ot = t.Ot.StartSpan(spanName, ot.ChildOf(parentSpan.Context()))
		return ot.ContextWithSpan(ctx, span.Ot), span
	}

	// If wireContext and no parentSpan in the context, just start a root span.
	span.Ot = t.Ot.StartSpan(spanName)
	return ot.ContextWithSpan(ctx, span.Ot), span
}

func (sw *SpanWrapper) AddEvent(k string, opts ...trace.EventOption) {
	//var alternatingKV []interface{}
	alternatingKV := make([]interface{}, 0)
	ec := trace.NewEventConfig(opts...)
	for _, attr := range ec.Attributes() {
		alternatingKV = append(alternatingKV, string(attr.Key), attr.Value.AsInterface())
	}

	sw.Ot.LogKV(alternatingKV...)
}

func (sw *SpanWrapper) IsRecording() bool {
	// Is there any moment that this is false for Opentelemetry?
	return true
}

func (sw *SpanWrapper) RecordError(err error, options ...trace.EventOption) {
	// I don't see why we don't pass options instead of creating attributes here...
	sw.AddEvent("error", trace.WithAttributes(attribute.String("message", err.Error())))
	// TODO(lucastt): do I need to set error tag true?
}

func (sw *SpanWrapper) SpanContext() trace.SpanContext {
	// This is implementation specific and through Ot.Span and Ot.SpanContext interface
	// we don't have access to enough information to build a complete otel.SpanContext.
	// Also each Ot.SpanContext implementation use different types for SpanID, TraceID etc
	// making it impossible to consistently get a value from these implementations and
	// convert it to Otel types.
	return trace.SpanContext{}
}

// TODO(lucastt): This might be different from what I understand, check SDK code
// to see how they implement this.
func (sw *SpanWrapper) SetStatus(code codes.Code, description string) {
	sw.SetAttributes(attribute.Int(HTTPStatusCodeTag, int(code)))
}

func (sw *SpanWrapper) SetName(name string) {
	sw.Ot.SetOperationName(name)
}

func (sw *SpanWrapper) SetAttributes(kv ...attribute.KeyValue) {
	for _, attr := range kv {
		sw.Ot.SetTag(string(attr.Key), attr.Value.AsInterface())
	}
}

func (sw *SpanWrapper) TracerProvider() trace.TracerProvider {
	// Not implemented.
	// TODO(lucast): Panic here and check if someone is calling it.
	return nil
}

func (sw *SpanWrapper) End(options ...trace.SpanEndOption) {
	sw.Ot.Finish()
}

func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	sw, ok := span.(*SpanWrapper)
	if ok {
		return ot.ContextWithSpan(ctx, sw.Ot)
	}

	return trace.ContextWithSpan(ctx, span)
}

func SpanFromContext(ctx context.Context, tracer trace.Tracer) trace.Span {
	_, ok := tracer.(*TracerWrapper)
	if ok {
		s := ot.SpanFromContext(ctx)
		if s == nil {
			return nil
		}
		return &SpanWrapper{Ot: s}
	}

	// TODO(lucastt): there is a possibility that otel.SpanFromContext returns a otel.noopSpan{}
	// This might be an unexpected behaviour to our code because ot.SpanFromContext in there
	// same situation would return nil. This behavior is hard to fake in this function because
	// noopSpan is an internal type.
	// NoopSpan has empty spanContext, to check if this is a noop span just check if TraceID/SpanID
	// in SpanContext is valid or not, like in tracing/tracingtest/testtracer.go:Tracer.Start(...)
	return trace.SpanFromContext(ctx)
}

func Extract(tracer trace.Tracer, req *http.Request) context.Context {
	t, ok := tracer.(*TracerWrapper)

	if !ok {
		carrier := propagation.HeaderCarrier(req.Header)
		ctx := otel.GetTextMapPropagator().Extract(req.Context(), carrier)
		return ctx
	}

	wireContext, err := t.Ot.Extract(ot.HTTPHeaders, ot.HTTPHeadersCarrier(req.Header))
	if err != nil {
		return req.Context()
	}
	return context.WithValue(req.Context(), wireContextKey, wireContext)
}

func Inject(ctx context.Context, req *http.Request, span trace.Span, tracer trace.Tracer) *http.Request {
	t, ok := tracer.(*TracerWrapper)
	if !ok {
		carrier := propagation.HeaderCarrier(req.Header)
		otel.GetTextMapPropagator().Inject(ctx, carrier)
		return req.WithContext(ctx)
	}

	sp, ok := span.(*SpanWrapper)
	if !ok {
		// nothing to do
		return req.WithContext(ctx)
	}
	carrier := ot.HTTPHeadersCarrier(req.Header)
	_ = t.Ot.Inject(sp.Ot.Context(), ot.HTTPHeaders, carrier)

	return req.WithContext(ot.ContextWithSpan(ctx, sp.Ot))
}

// TODO(lucastt): a SetBaggageMember method might be necessary to avoid leaking tracing logic into
// other packages.
func GetBaggageMember(ctx context.Context, span trace.Span, key string) baggage.Member {
	sw, ok := span.(*SpanWrapper)
	if !ok {
		// Need to check for nils here, otherwise I'll get some seg faults
		return baggage.FromContext(ctx).Member(key)
	}

	bagItem := sw.Ot.BaggageItem(key)
	m, err := baggage.NewMemberRaw(key, bagItem)
	if err != nil {
		return m
	}
	return m
}
