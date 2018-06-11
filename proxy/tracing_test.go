package proxy

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

const traceHeader = "X-Trace-Header"

func TestTracingFromWire(t *testing.T) {
	traceContent := fmt.Sprintf("%x", md5.New().Sum([]byte(time.Now().String())))
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
	tracer := &tracer{}
	params := Params{
		OpenTracer: tracer,
		Flags:      FlagsNone,
	}

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Error(err)
		return
	}
	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if len(tracer.recordedSpans) == 0 {
		t.Fatal("no span recorded...")
	}
	if tracer.recordedSpans[0].trace != traceContent {
		t.Errorf("trace not found, got `%s` instead", tracer.recordedSpans[0].trace)
	}
	if len(tracer.recordedSpans[0].refs) == 0 {
		t.Errorf("no references found, this is a root span")
	}
}

func TestTracingRoot(t *testing.T) {
	traceContent := fmt.Sprintf("%x", md5.New().Sum([]byte(time.Now().String())))
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
	tracer := &tracer{traceContent: traceContent}
	params := Params{
		OpenTracer: tracer,
		Flags:      FlagsNone,
	}

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Error(err)
		return
	}
	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if len(tracer.recordedSpans) == 0 {
		t.Fatal("no span recorded...")
	}
	if tracer.recordedSpans[0].trace != traceContent {
		t.Errorf("trace not found, got `%s` instead", tracer.recordedSpans[0].trace)
	}

	root, ok := tracer.findSpan("ingress")
	if !ok {
		t.Fatal("root span not found")
	}

	if len(root.refs) != 0 {
		t.Error("root span cannot have references")
	}
}

func TestTracingSpanName(t *testing.T) {
	traceContent := fmt.Sprintf("%x", md5.New().Sum([]byte(time.Now().String())))
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

	doc := fmt.Sprintf(`hello: Path("/hello") -> tracingSpanName("test-span") -> "%s"`, s.URL)
	tracer := &tracer{traceContent: traceContent}
	params := Params{
		OpenTracer: tracer,
		Flags:      FlagsNone,
	}

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Fatal(err)
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	_, ok := tracer.findSpan("test-span")
	if !ok {
		t.Error("setting the span name failed")
	}
}

func TestTracingInitialSpanName(t *testing.T) {
	traceContent := fmt.Sprintf("%x", md5.New().Sum([]byte(time.Now().String())))
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
	tracer := &tracer{traceContent: traceContent}
	params := Params{
		OpenTracer:             tracer,
		OpenTracingInitialSpan: "test-initial-span",
		Flags: FlagsNone,
	}

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Fatal(err)
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if _, ok := tracer.findSpan("test-initial-span"); !ok {
		t.Error("setting the span name failed")
	}
}
