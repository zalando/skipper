package proxy

import (
	"net/http"

	ot "github.com/opentracing/opentracing-go"
	log "github.com/opentracing/opentracing-go/log"
)

type tracer struct {
	traceContent  string
	recordedSpans []*span
}

type span struct {
	trace         string
	operationName string
	tags          map[string]interface{}
	tracer        *tracer
	refs          []ot.SpanReference
}

func (t *tracer) findAllSpans(operationName string) []*span {
	var spans []*span
	for _, s := range t.recordedSpans {
		if s.operationName == operationName {
			spans = append(spans, s)
		}
	}

	return spans
}

func (t *tracer) findSpan(operationName string) (*span, bool) {
	all := t.findAllSpans(operationName)
	if len(all) > 0 {
		return all[0], true
	}

	return nil, false
}

func (t *tracer) createSpanBase() *span {
	return &span{
		tracer: t,
		trace:  t.traceContent,
		tags:   make(map[string]interface{}),
	}
}

func (t *tracer) StartSpan(operationName string, opts ...ot.StartSpanOption) ot.Span {
	sso := ot.StartSpanOptions{}
	for _, o := range opts {
		o.Apply(&sso)
	}
	s := t.createSpanBase()
	s.operationName = operationName
	s.refs = sso.References
	return s
}

func (t *tracer) Inject(sm ot.SpanContext, format interface{}, carrier interface{}) error {
	http.Header(carrier.(ot.HTTPHeadersCarrier)).Set("X-Trace-Header", t.traceContent)
	return nil
}

func (t *tracer) Extract(format interface{}, carrier interface{}) (ot.SpanContext, error) {
	val := http.Header(carrier.(ot.HTTPHeadersCarrier)).Get("X-Trace-Header")
	if val != "" {
		t.traceContent = val
		s := t.createSpanBase()
		s.refs = []ot.SpanReference{
			{
				Type:              ot.ChildOfRef,
				ReferencedContext: &span{trace: val},
			},
		}
		return s, nil
	}
	return nil, ot.ErrSpanContextNotFound
}

// SpanContext interface
func (s *span) ForeachBaggageItem(func(k, v string) bool) {}

// Span interface
func (s *span) Finish() {
	s.FinishWithOptions(ot.FinishOptions{})
}

func (s *span) FinishWithOptions(opts ot.FinishOptions) {
	s.tracer.recordedSpans = append(s.tracer.recordedSpans, s)
}

func (s *span) Context() ot.SpanContext {
	return s
}

func (s *span) SetOperationName(operationName string) ot.Span {
	s.operationName = operationName
	return s
}

func (s *span) SetTag(key string, value interface{}) ot.Span {
	s.tags[key] = value
	return s
}

func (*span) LogFields(...log.Field) {}

func (*span) LogKV(...interface{}) {}

func (s *span) SetBaggageItem(restrictedKey, value string) ot.Span {
	return s
}

func (*span) BaggageItem(string) string {
	return ""
}

func (s *span) Tracer() ot.Tracer {
	return s.tracer
}

func (*span) LogEvent(string) {}

func (*span) LogEventWithPayload(string, interface{}) {}

func (*span) Log(ot.LogData) {}
