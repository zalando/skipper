/*
Package tracingtest provides an OpenTracing implementation for testing purposes.
*/
package tracingtest

import (
	"maps"
	"net/http"
	"time"

	tracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

// Tracer is an implementation of opentracing.Tracer for testing. It records
// the defined spans during a series of operations.
//
// Deprecated: use [NewTracer] instead.
type Tracer struct {

	// TraceContent represents the tracing content passed along the wire.
	// The test tracer considers it an opaque value and doesn't modify it.
	TraceContent string

	// RecordedSpans contains the collected spans.
	RecordedSpans []*Span
}

// Span is an implementation of the opentracing.Span interface for testing.
type Span struct {

	// Trace contains the current trace as string.
	Trace string

	// Refs contain the collected spans.
	Refs []tracing.SpanReference

	// StartTime contains the timestamp when the span was started.
	StartTime time.Time

	// FinishTime contains the timestamp when the span was finished.
	FinishTime time.Time

	operationName string
	Tags          map[string]any
	baggage       map[string]string
	tracer        *Tracer
}

// Deprecated: use [NewTracer] and [MockTracer.StartSpan] instead.
func NewSpan(operation string) *Span {
	return &Span{
		operationName: operation,
		Tags:          make(map[string]any),
		baggage:       make(map[string]string),
	}
}

// FindAllSpans returns all the spans with the defined operation name.
func (t *Tracer) FindAllSpans(operationName string) []*Span {
	var spans []*Span
	for _, s := range t.RecordedSpans {
		if s.operationName == operationName {
			spans = append(spans, s)
		}
	}

	return spans
}

// FindSpan returns the first span with the defined operation name and true,
// if at least one was collected; otherwise, nil and false.
func (t *Tracer) FindSpan(operationName string) (*Span, bool) {
	all := t.FindAllSpans(operationName)
	if len(all) > 0 {
		return all[0], true
	}

	return nil, false
}

// Reset clears the recorded spans and sets the trace content to defined
// value.
func (t *Tracer) Reset(traceContent string) {
	t.TraceContent = traceContent
	t.RecordedSpans = nil
}

func (t *Tracer) createSpanBase() *Span {
	return &Span{
		Trace:     t.TraceContent,
		StartTime: time.Now(),
		tracer:    t,
		Tags:      make(map[string]any),
		baggage:   make(map[string]string),
	}
}

// Create, start, and return a new Span with the given `operationName` and
// incorporate the given StartSpanOption `opts`.
func (t *Tracer) StartSpan(operationName string, opts ...tracing.StartSpanOption) tracing.Span {
	sso := tracing.StartSpanOptions{}
	for _, o := range opts {
		o.Apply(&sso)
	}
	s := t.createSpanBase()
	s.operationName = operationName
	s.Refs = sso.References
	maps.Copy(s.Tags, sso.Tags)
	return s
}

// Inject() takes the `sm` SpanContext instance and injects it for
// propagation within `carrier`.
//
// It sets the X-Trace-Header to the value of TraceContent.
func (t *Tracer) Inject(sm tracing.SpanContext, format any, carrier any) error {
	http.Header(carrier.(tracing.HTTPHeadersCarrier)).Set("X-Trace-Header", t.TraceContent)
	return nil
}

// Extract() returns a SpanContext instance given `format` and `carrier`.
//
// It copies the X-Trace-Header value to the TraceContent field.
func (t *Tracer) Extract(format any, carrier any) (tracing.SpanContext, error) {
	val := http.Header(carrier.(tracing.HTTPHeadersCarrier)).Get("X-Trace-Header")
	if val != "" {
		t.TraceContent = val
		s := t.createSpanBase()
		s.Refs = []tracing.SpanReference{
			{
				Type:              tracing.ChildOfRef,
				ReferencedContext: &Span{Trace: val},
			},
		}
		return s, nil
	}
	return nil, tracing.ErrSpanContextNotFound
}

// ForeachBaggageItem grants access to all baggage items stored in the
// SpanContext.
func (s *Span) ForeachBaggageItem(func(k, v string) bool) {}

// Sets the end timestamp and finalizes Span state.
func (s *Span) Finish() {
	s.FinishWithOptions(tracing.FinishOptions{})
}

// FinishWithOptions is like Finish() but with explicit control over
// timestamps and log data.
func (s *Span) FinishWithOptions(opts tracing.FinishOptions) {
	s.FinishTime = time.Now()
	s.tracer.RecordedSpans = append(s.tracer.RecordedSpans, s)
}

// Context() yields the SpanContext for this Span. Note that the return
// value of Context() is still valid after a call to Span.Finish(), as is
// a call to Span.Context() after a call to Span.Finish().
func (s *Span) Context() tracing.SpanContext {
	return s
}

// Sets or changes the operation name.
func (s *Span) SetOperationName(operationName string) tracing.Span {
	s.operationName = operationName
	return s
}

// Adds a tag to the span.
func (s *Span) SetTag(key string, value any) tracing.Span {
	s.Tags[key] = value
	return s
}

// LogFields is an efficient and type-checked way to record key:value
// logging data about a Span, though the programming interface is a little
// more verbose than LogKV().
func (*Span) LogFields(...log.Field) {}

// LogKV is a concise, readable way to record key:value logging data about
// a Span, though unfortunately this also makes it less efficient and less
// type-safe than LogFields().
func (*Span) LogKV(...any) {}

// SetBaggageItem sets a key:value pair on this Span and its SpanContext
// that also propagates to descendants of this Span.
func (s *Span) SetBaggageItem(restrictedKey, value string) tracing.Span {
	s.baggage[restrictedKey] = value
	return s
}

// Gets the value for a baggage item given its key. Returns the empty string
// if the value isn't found in this Span.
func (s *Span) BaggageItem(key string) string {
	return s.baggage[key]
}

// Provides access to the Tracer that created this Span.
func (s *Span) Tracer() tracing.Tracer {
	return s.tracer
}

// Deprecated: use LogFields or LogKV
func (*Span) LogEvent(string) {}

// Deprecated: use LogFields or LogKV
func (*Span) LogEventWithPayload(string, any) {}

// Deprecated: use LogFields or LogKV
func (*Span) Log(tracing.LogData) {}
