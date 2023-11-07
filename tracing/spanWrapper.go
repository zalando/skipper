package tracing

import (
	"context"
	"net/http"

	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	otel "go.opentelemetry.io/otel/trace"
	otelembedded "go.opentelemetry.io/otel/trace/embedded"
)

type TracerWrapper struct {
	otelembedded.Tracer
	Ot ot.Tracer
}

type StartWithExtractOpt struct {
	// Since the Iface has private methods it needs to be embedded into the struct to implement it outside the package.
	otel.SpanStartOption
	req *http.Request
}

type SpanWrapper struct {
	otelembedded.Span
	Ot ot.Span
}

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

// Noop function to implement otel.SpanStartOption and signal when a call to TracerWrapper.extract is necessary.
// applySpanStart(SpanConfig) SpanConfig
func (o StartWithExtractOpt) applySpanStart(c otel.SpanConfig) otel.SpanConfig {
	return c
}

// StartWithExtractOpt returns a startWithExtractOpt that implements otel.SpanStartOption and indicates to
// TracerWrapper.Start that it should Extract the context from the request req passed as argument to the
// function.
func StartWithExtractOption(req *http.Request) StartWithExtractOpt {
	return StartWithExtractOpt{req: req}
}

// TODO(lucastt): Check wether its necessary to convert other otel start options to ot start options. From our code this
// doesn't seems necessary.
func (t *TracerWrapper) Start(ctx context.Context, spanName string, opts ...otel.SpanStartOption) (context.Context, otel.Span) {
	span := &SpanWrapper{}

	// If Extract option is set, try to extract context from the request.
	for _, opt := range opts {
		if extractOpt, ok := opt.(StartWithExtractOpt); ok {
			wireContext, err := t.Ot.Extract(ot.HTTPHeaders, ot.HTTPHeadersCarrier(extractOpt.req.Header))
			if err != nil {
				span.Ot = t.Ot.StartSpan(spanName)
			} else {
				// Why does it assume its always RPC and not HTTP?
				// This is a bug, it should also support HTTP.
				span.Ot = t.Ot.StartSpan(spanName, ext.RPCServerOption(wireContext))
			}
			return ot.ContextWithSpan(ctx, span.Ot), span
		}
	}

	// If extract option is not set, check if there is a parent span in the context.
	parentSpan := ot.SpanFromContext(ctx)
	if parentSpan != nil {
		span.Ot = t.Ot.StartSpan(spanName, ot.ChildOf(parentSpan.Context()))
		return ot.ContextWithSpan(ctx, span.Ot), span
	}

	// If there is not extract option and no parentSpan in the context, just start a root span.
	span.Ot = t.Ot.StartSpan(spanName)
	return ot.ContextWithSpan(ctx, span.Ot), span
}

// func (sw *SpanWrapper) span()                                              {}
func (sw *SpanWrapper) AddEvent(k string, opts ...otel.EventOption) {
	//var alternatingKV []interface{}
	alternatingKV := make([]interface{}, 0)
	ec := otel.NewEventConfig(opts...)
	for _, attr := range ec.Attributes() {
		alternatingKV = append(alternatingKV, string(attr.Key), attr.Value.AsInterface())
	}

	sw.Ot.LogKV(alternatingKV...)
}

func (sw *SpanWrapper) IsRecording() bool {
	// Is there any moment that this is false for Opentelemetry?
	return true
}

func (sw *SpanWrapper) RecordError(err error, options ...otel.EventOption) {
	// I don't see why we don't pass options instead of creating attributes here...
	sw.AddEvent("error", otel.WithAttributes(attribute.String("message", err.Error())))
}

func (sw *SpanWrapper) SpanContext() otel.SpanContext {
	// This is implementation specific and through Ot.Span and Ot.SpanContext interface
	// we don't have access to enough information to build a complete otel.SpanContext.
	// Also each Ot.SpanContext implementation use different types for SpanID, TraceID etc
	// making it impossible to consistently get a value from these implementations and
	// convert it to Otel types.
	return otel.SpanContext{}
}

func (sw *SpanWrapper) SetStatus(code codes.Code, description string) {
	sw.SetAttributes(attribute.Int(HTTPStatusCodeTag, int(code)))
}

func (sw *SpanWrapper) SetName(name string) {
	// Not implemented.
	// Its not possible to change operation name after Span start in opentracing.Span
	// TODO(lucast): Panic here and check if someone is calling it.
}

func (sw *SpanWrapper) SetAttributes(kv ...attribute.KeyValue) {
	for _, attr := range kv {
		sw.Ot.SetTag(string(attr.Key), attr.Value.AsInterface())
	}
}

func (sw *SpanWrapper) TracerProvider() otel.TracerProvider {
	// Not implemented.
	// TODO(lucast): Panic here and check if someone is calling it.
	return nil
}

func (sw *SpanWrapper) End(options ...otel.SpanEndOption) {
	sw.Ot.Finish()
}

func ContextWithSpan(ctx context.Context, span otel.Span) context.Context {
	sw, ok := span.(*SpanWrapper)
	if ok {
		return ot.ContextWithSpan(ctx, sw.Ot)
	}

	return otel.ContextWithSpan(ctx, span)
}

func SpanFromContext(ctx context.Context, tracer otel.Tracer) otel.Span {
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
	return otel.SpanFromContext(ctx)
}

func Inject(ctx context.Context, req *http.Request, span otel.Span, tracer otel.Tracer) *http.Request {
	t, ok := tracer.(*TracerWrapper)
	if !ok {
		// HTTP header injection should be done automatically by textMapPropagator
		// a textMapPropagator should be defined globally as in the package
		// skipper/tracing/tracers/otel/otel.go
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

func GetBaggageMember(ctx context.Context, span otel.Span, key string) baggage.Member {
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
