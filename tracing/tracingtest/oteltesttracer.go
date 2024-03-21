/*
Package tracingtest provides an OpenTelemetry implementation for testing purposes.
*/
package tracingtest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	otelembedded "go.opentelemetry.io/otel/trace/embedded"
)

// OtelTracer is an implementation of opentelemetry.OtelTracer for testing. It records
// the defined spans during a series of operations.
type OtelTracer struct {
	otelembedded.Tracer

	mu            sync.Mutex
	recordedSpans []*OtelSpan

	// TraceContent represents the tracing content passed along the wire.
	// The test tracer considers it an opaque value and doesn't modify it.
	TraceContent   string
	traceContentMu sync.Mutex
}

// OtelSpan is an implementation of the opentelemetry.OtelSpan interface for testing.
type OtelSpan struct {
	otelembedded.Span

	// Trace contains the current trace as string.
	Trace string

	// Holds a reference to the parent span as a SpanContext
	Parent *OtelSpan

	// Contains a count of how many children this span has.
	ChildSpanCount int

	spanContext trace.SpanContext

	// StartTime contains the timestamp when the span was started.
	StartTime time.Time

	// FinishTime contains the timestamp when the span was finished.
	FinishTime time.Time

	// Name passed to the span during its initialization.
	OperationName string

	// Attributes contains all attributes added to the span
	Attributes map[string]interface{}

	// Event contains all events added to the span
	Events []sdk.Event

	// Tracer used to create this span
	tracer *OtelTracer
}

type SpanContextContextKey struct{}

var traceContentContextKey SpanContextContextKey

type MockPropagator struct {
	t           *testing.T
	traceHeader string
}

func (mp *MockPropagator) Fields() []string {
	return []string{fmt.Sprintf("%t", traceContentContextKey)}
}

func (mp *MockPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	val := ctx.Value(traceContentContextKey)
	if val == nil {
		return
	}
	traceContent, ok := val.(string)
	if !ok {
		mp.t.Fatalf("Expect traceContent to be of type string, got %t", ctx.Value(traceContentContextKey))
		return
	}
	carrier.Set(mp.traceHeader, traceContent)
}

func (mp *MockPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return context.WithValue(ctx, traceContentContextKey, carrier.Get(mp.traceHeader))
}

func InitPropagator(t *testing.T, traceHeader string) {
	otel.SetTextMapPropagator(&MockPropagator{t, traceHeader})
}

func NewSpan(operation string) *OtelSpan {
	return &OtelSpan{
		OperationName: operation,
		Attributes:    make(map[string]interface{}),
		Events:        []sdk.Event{},
	}
}

// FindAllSpans returns all the spans with the defined operation name.
func (t *OtelTracer) FindAllSpans(operationName string) []*OtelSpan {
	var spans []*OtelSpan
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.recordedSpans {
		if s.OperationName == operationName {
			spans = append(spans, s)
		}
	}

	return spans
}

// FindSpan returns the first span with the defined operation name and true,
// if at least one was collected, otherwise nil and false.
func (t *OtelTracer) FindSpan(operationName string) (*OtelSpan, bool) {
	all := t.FindAllSpans(operationName)
	if len(all) > 0 {
		return all[0], true
	}

	return nil, false
}

// Reset clears the recorded spans and sets the trace content to defined
// value.
func (t *OtelTracer) Reset(traceContent string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TraceContent = traceContent
	t.recordedSpans = nil
}

func (t *OtelTracer) createSpanBase(traceID trace.TraceID) *OtelSpan {
	sc := trace.SpanContext{}
	t.traceContentMu.Lock()
	defer t.traceContentMu.Unlock()
	return &OtelSpan{
		Trace:       t.TraceContent,
		StartTime:   time.Now(),
		tracer:      t,
		spanContext: sc.WithTraceID(traceID),
		Attributes:  make(map[string]interface{}),
	}
}

// Start creates, starts, and returns a new Span with the given `operationName` if
// the provided context already has a span, creates a child span from the
// context span. A opentelemetry compatible span and a new context with
// the newly created span is returned.
func (t *OtelTracer) Start(c context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	var s *OtelSpan
	parent := trace.SpanFromContext(c)
	// trace.noopSpan has empty SpanContext, in other words propeties like traceID are all zeros
	// which means its an invalid traceID that only is used on trace.noopSpan
	if !parent.SpanContext().HasTraceID() {
		traceID := trace.TraceID{}
		traceID[0]++
		s = t.createSpanBase(traceID)
	} else {
		traceID := parent.SpanContext().TraceID()
		traceID[0]++
		s = t.createSpanBase(traceID)
	}

	s.OperationName = name
	if p, ok := parent.(*OtelSpan); ok {
		p.ChildSpanCount++
		s.Parent = p
	}

	t.traceContentMu.Lock()
	defer t.traceContentMu.Unlock()
	if t.TraceContent == "" {
		val := c.Value(traceContentContextKey)
		if val != nil {
			tc, ok := val.(string)
			if ok {
				t.TraceContent = tc
			}
		}
	}

	return trace.ContextWithSpan(context.WithValue(c, traceContentContextKey, t.TraceContent), s), s
}

// RecordedSpans returns all Ended spans that were created by this tracer.
func (t *OtelTracer) RecordedSpans() []*OtelSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.recordedSpans
}

func (t *OtelTracer) recordSpan(span *OtelSpan) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recordedSpans = append(t.recordedSpans, span)
}

// End sets the end timestamp and finalizes Span state.
func (s *OtelSpan) End(opts ...trace.SpanEndOption) {
	s.FinishTime = time.Now()
	if s.tracer != nil {
		s.tracer.recordSpan(s)
	}
}

// SpanContext yields the SpanContext for this Span. Note that the return
// value of Context() is still valid after a call to Span.End().
func (s *OtelSpan) SpanContext() trace.SpanContext {
	return s.spanContext
}

// SetName sets or changes the operation name.
func (s *OtelSpan) SetName(operationName string) {
	s.OperationName = operationName
}

// SetAttributes adds a tag/attribute to the span.
func (s *OtelSpan) SetAttributes(kv ...attribute.KeyValue) {
	for _, attr := range kv {
		s.Attributes[string(attr.Key)] = attr.Value.AsInterface()
	}
}

// AddEvent add an Event to the span
func (s *OtelSpan) AddEvent(k string, opts ...trace.EventOption) {
	ec := trace.NewEventConfig(opts...)
	s.Events = append(s.Events, sdk.Event{
		Name:       k,
		Attributes: ec.Attributes(),
		Time:       time.Now(),
	})
}

// IsRecording returns wether this is a recording Span or not. For this implementation
// this is always true.
func (s *OtelSpan) IsRecording() bool {
	// Is there any moment that this is false for Opentelemetry?
	return true
}

// RecordError record an error into the span
func (s *OtelSpan) RecordError(err error, options ...trace.EventOption) {
	s.AddEvent("error", trace.WithAttributes(attribute.String("message", err.Error())))
}

func (s *OtelSpan) SetStatus(code codes.Code, description string) {
	panic("SetStatus is not implemented")
}

func (s *OtelSpan) TracerProvider() trace.TracerProvider {
	panic("The function `testtracer.Span.TracerProvider()` is not implemented")
}
