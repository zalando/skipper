package tracingtest

import (
	"net/http"
	"time"

	tracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

type Tracer struct {
	TraceContent  string
	RecordedSpans []*span
}

type span struct {
	Trace      string
	Refs       []tracing.SpanReference
	FinishTime time.Time

	operationName string
	tags          map[string]interface{}
	tracer        *Tracer
	StartTime     time.Time
}

func (t *Tracer) FindAllSpans(operationName string) []*span {
	var spans []*span
	for _, s := range t.RecordedSpans {
		if s.operationName == operationName {
			spans = append(spans, s)
		}
	}

	return spans
}

func (t *Tracer) FindSpan(operationName string) (*span, bool) {
	all := t.FindAllSpans(operationName)
	if len(all) > 0 {
		return all[0], true
	}

	return nil, false
}

func (t *Tracer) Reset(traceContent string) {
	t.TraceContent = traceContent
	t.RecordedSpans = nil
}

func (t *Tracer) createSpanBase() *span {
	return &span{
		Trace:     t.TraceContent,
		StartTime: time.Now(),
		tracer:    t,
		tags:      make(map[string]interface{}),
	}
}

func (t *Tracer) StartSpan(operationName string, opts ...tracing.StartSpanOption) tracing.Span {
	sso := tracing.StartSpanOptions{}
	for _, o := range opts {
		o.Apply(&sso)
	}
	s := t.createSpanBase()
	s.operationName = operationName
	s.Refs = sso.References
	return s
}

func (t *Tracer) Inject(sm tracing.SpanContext, format interface{}, carrier interface{}) error {
	http.Header(carrier.(tracing.HTTPHeadersCarrier)).Set("X-Trace-Header", t.TraceContent)
	return nil
}

func (t *Tracer) Extract(format interface{}, carrier interface{}) (tracing.SpanContext, error) {
	val := http.Header(carrier.(tracing.HTTPHeadersCarrier)).Get("X-Trace-Header")
	if val != "" {
		t.TraceContent = val
		s := t.createSpanBase()
		s.Refs = []tracing.SpanReference{
			{
				Type:              tracing.ChildOfRef,
				ReferencedContext: &span{Trace: val},
			},
		}
		return s, nil
	}
	return nil, tracing.ErrSpanContextNotFound
}

// SpanContext interface
func (s *span) ForeachBaggageItem(func(k, v string) bool) {}

// Span interface
func (s *span) Finish() {
	s.FinishWithOptions(tracing.FinishOptions{})
}

func (s *span) FinishWithOptions(opts tracing.FinishOptions) {
	s.FinishTime = time.Now()
	s.tracer.RecordedSpans = append(s.tracer.RecordedSpans, s)
}

func (s *span) Context() tracing.SpanContext {
	return s
}

func (s *span) SetOperationName(operationName string) tracing.Span {
	s.operationName = operationName
	return s
}

func (s *span) SetTag(key string, value interface{}) tracing.Span {
	s.tags[key] = value
	return s
}

func (*span) LogFields(...log.Field) {}

func (*span) LogKV(...interface{}) {}

func (s *span) SetBaggageItem(restrictedKey, value string) tracing.Span {
	return s
}

func (*span) BaggageItem(string) string {
	return ""
}

func (s *span) Tracer() tracing.Tracer {
	return s.tracer
}

func (*span) LogEvent(string) {}

func (*span) LogEventWithPayload(string, interface{}) {}

func (*span) Log(tracing.LogData) {}
