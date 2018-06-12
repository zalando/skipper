package proxy

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
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

	if _, ok := tracer.findSpan("test-span"); !ok {
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

func TestTracingProxySpan(t *testing.T) {
	const (
		contentSize         = 1 << 16
		prereadSize         = 1 << 12
		responseStreamDelay = 30 * time.Millisecond
	)

	var content bytes.Buffer
	if _, err := io.CopyN(&content, rand.New(rand.NewSource(0)), contentSize); err != nil {
		t.Fatal(err)
	}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := io.CopyN(w, &content, prereadSize); err != nil {
			t.Fatal(err)
		}

		time.Sleep(responseStreamDelay)
		if _, err := io.Copy(w, &content); err != nil {
			t.Fatal(err)
		}
	}))
	defer s.Close()

	doc := fmt.Sprintf(`* -> "%s"`, s.URL)
	tracer := &tracer{}
	tp, err := newTestProxyWithParams(doc, Params{OpenTracer: tracer})
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	req, err := http.NewRequest("GET", "https://www.example.org", nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	tp.proxy.ServeHTTP(w, req)

	proxySpan, ok := tracer.findSpan("proxy")
	if !ok {
		t.Fatal("proxy span not found")
	}

	if proxySpan.finish.Sub(proxySpan.start) < responseStreamDelay {
		t.Error("proxy span did not wait for response stream to finish")
	}
}

func TestTracingProxySpanWithRetry(t *testing.T) {
	const (
		contentSize         = 1 << 16
		prereadSize         = 1 << 12
		responseStreamDelay = 30 * time.Millisecond
	)

	s0 := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	s0.Close()

	content := rand.New(rand.NewSource(0))
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)

		if _, err := io.CopyN(w, content, prereadSize); err != nil {
			t.Fatal(err)
		}

		time.Sleep(responseStreamDelay)
		if _, err := io.CopyN(w, content, contentSize-prereadSize); err != nil {
			t.Fatal(err)
		}
	}))
	defer s1.Close()

	const docFmt = `
		member0: LBMember("test", 0) -> "%s";
		member1: LBMember("test", 1) -> "%s";
		group: LBGroup("test") -> lbDecide("test", 2) -> <loopback>;
	`
	doc := fmt.Sprintf(docFmt, s0.URL, s1.URL)
	tracer := &tracer{}
	tp, err := newTestProxyWithParams(doc, Params{OpenTracer: tracer})
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	testFallback := func() bool {
		tracer.reset("")
		req, err := http.NewRequest("GET", "https://www.example.org", nil)
		if err != nil {
			t.Fatal(err)
		}

		tp.proxy.ServeHTTP(httptest.NewRecorder(), req)

		proxySpans := tracer.findAllSpans("proxy")
		if len(proxySpans) != 2 {
			t.Log("invalid count of proxy spans", len(proxySpans))
			return false
		}

		for _, s := range proxySpans {
			if s.finish.Sub(s.start) >= responseStreamDelay {
				return true
			}
		}

		t.Log("proxy span with the right duration not found")
		return false
	}

	// Two lb group members are used in round-robin, starting at a non-deterministic index.
	// One of them cannot be connected to, and the proxy should fallback to the other. We
	// want to verify here that the proxy span is traced properly in the fallback case.
	if !testFallback() && !testFallback() {
		t.Error("failed to trace the right span duration for fallback")
	}
}
