package proxy

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	ot "github.com/opentracing/opentracing-go"
	log "github.com/opentracing/opentracing-go/log"
)

var recordedSpan *span

const traceHeader string = "X-Trace-Header"

var traceContent string

func TestTracingFromWire(t *testing.T) {
	traceContent = fmt.Sprintf("%x", md5.New().Sum([]byte(time.Now().String())))
	s := startTestServer(nil, 0, func(r *http.Request) {
		th, ok := r.Header[traceHeader]
		if !ok {
			t.Errorf("missing %s request header", traceHeader)
		} else {
			if th[0] != traceContent {
				t.Errorf("wrong X-Trace-Header content: %s", th[0])
			}
		}
	})
	defer s.Close()

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: make(http.Header),
	}
	r.Header.Set(traceHeader, traceContent)
	w := httptest.NewRecorder()

	doc := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s.URL)
	params := Params{
		OpenTracer: &tracer{},
		Flags:      FlagsNone,
	}

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Error(err)
		return
	}
	defer tp.close()
	recordedSpan = nil

	tp.proxy.ServeHTTP(w, r)

	if recordedSpan == nil {
		t.Errorf("no span recorded...")
	}
	if recordedSpan.trace != traceContent {
		t.Errorf("trace not found, got `%s` instead", recordedSpan.trace)
	}
	if len(recordedSpan.refs) == 0 {
		t.Errorf("no references found, this is a root span")
	}
}

func TestTracingRoot(t *testing.T) {
	traceContent = fmt.Sprintf("%x", md5.New().Sum([]byte(time.Now().String())))
	s := startTestServer(nil, 0, func(r *http.Request) {
		th, ok := r.Header[traceHeader]
		if !ok {
			t.Errorf("missing %s request header", traceHeader)
		} else {
			if th[0] != traceContent {
				t.Errorf("wrong X-Trace-Header content: %s", th[0])
			}
		}
	})
	defer s.Close()

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: make(http.Header),
	}
	w := httptest.NewRecorder()

	doc := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s.URL)
	params := Params{
		OpenTracer: &tracer{},
		Flags:      FlagsNone,
	}

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Error(err)
		return
	}
	defer tp.close()
	recordedSpan = nil

	tp.proxy.ServeHTTP(w, r)

	if recordedSpan == nil {
		t.Errorf("no span recorded...")
	}
	if recordedSpan.trace != traceContent {
		t.Errorf("trace not found, got `%s` instead", recordedSpan.trace)
	}
	if len(recordedSpan.refs) > 0 {
		t.Errorf("references found, this is not a root span")
	}
}

type tracer struct {
}

type span struct {
	trace         string
	operationName string
	tags          map[string]interface{}
	tracer        ot.Tracer
	refs          []ot.SpanReference
}

func (t *tracer) StartSpan(operationName string, opts ...ot.StartSpanOption) ot.Span {
	sso := ot.StartSpanOptions{}
	for _, o := range opts {
		o.Apply(&sso)
	}
	return &span{
		operationName: operationName,
		tracer:        t,
		tags:          make(map[string]interface{}),
		trace:         traceContent,
		refs:          sso.References,
	}
}

func (t *tracer) Inject(sm ot.SpanContext, format interface{}, carrier interface{}) error {
	http.Header(carrier.(ot.HTTPHeadersCarrier)).Set("X-Trace-Header", traceContent)
	return nil
}

func (t *tracer) Extract(format interface{}, carrier interface{}) (ot.SpanContext, error) {
	val := http.Header(carrier.(ot.HTTPHeadersCarrier)).Get("X-Trace-Header")
	if val != "" {
		return &span{
			trace:  val,
			tracer: t,
			tags:   make(map[string]interface{}),
			refs: []ot.SpanReference{
				{
					Type:              ot.ChildOfRef,
					ReferencedContext: &span{trace: val},
				},
			},
		}, nil
	}
	return nil, ot.ErrSpanContextNotFound
}

// SpanContext interface
func (s *span) ForeachBaggageItem(handler func(k, v string) bool) {
	return
}

// Span interface
func (s *span) Finish() {
	recordedSpan = s
}

func (s *span) FinishWithOptions(opts ot.FinishOptions) {
	recordedSpan = s
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

func (s *span) LogFields(fields ...log.Field) {
	return
}

func (s *span) LogKV(alternatingKeyValues ...interface{}) {
	return
}

func (s *span) SetBaggageItem(restrictedKey, value string) ot.Span {
	return s
}

func (s *span) BaggageItem(restrictedKey string) string {
	return ""
}

func (s *span) Tracer() ot.Tracer {
	return s.tracer
}

func (s *span) LogEvent(event string) {
	return
}

func (s *span) LogEventWithPayload(event string, payload interface{}) {
	return
}

func (s *span) Log(data ot.LogData) {
	return
}
