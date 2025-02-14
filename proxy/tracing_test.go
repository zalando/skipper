package proxy

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/zalando/skipper/tracing/tracingtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTracingIngressSpan(t *testing.T) {
	s := startTestServer(nil, 0, func(r *http.Request) {
		p := &mocktracer.TextMapPropagator{}
		_, err := p.Extract(ot.HTTPHeadersCarrier(r.Header))
		if err != nil {
			t.Error(err)
		}
	})
	defer s.Close()

	routeID := "ingressRoute"
	doc := fmt.Sprintf(`%s: Path("/hello") -> setPath("/bye") -> setQuery("void") -> "%s"`, routeID, s.URL)

	tracer := tracingtest.NewTracer()
	params := Params{
		OpenTracing: &OpenTracingParams{
			Tracer: tracer,
		},
		Flags: FlagsNone,
	}

	t.Setenv("HOSTNAME", "ingress.tracing.test")

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	req, err := http.NewRequest("GET", ps.URL+"/hello?world", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Flow-Id", "test-flow-id")

	_, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	span := tracer.FindSpan("ingress")
	if span == nil {
		t.Fatal("ingress span not found")
	}

	verifyTag(t, span, SpanKindTag, SpanKindServer)
	verifyTag(t, span, ComponentTag, "skipper")
	verifyTag(t, span, SkipperRouteIDTag, routeID)
	// to save memory we dropped the URL tag from ingress span
	//verifyTag(t, span, HTTPUrlTag, "/hello?world") // For server requests there is no scheme://host:port, see https://golang.org/pkg/net/http/#Request
	verifyTag(t, span, HTTPMethodTag, "GET")
	verifyTag(t, span, HostnameTag, "ingress.tracing.test")
	verifyTag(t, span, HTTPPathTag, "/hello")
	verifyTag(t, span, HTTPHostTag, ps.Listener.Addr().String())
	verifyTag(t, span, HTTPRequestHeaderCeil, int64(128))
	verifyTag(t, span, FlowIDTag, "test-flow-id")
	verifyTag(t, span, HTTPStatusCodeTag, uint16(200))
	verifyNoTag(t, span, HTTPResponseBodyCeil)
	verifyHasTag(t, span, HTTPRemoteIPTag)
}

func TestTracingIngressSpanShunt(t *testing.T) {
	routeID := "ingressShuntRoute"
	doc := fmt.Sprintf(`%s: Path("/hello") -> setPath("/bye") -> setQuery("void") -> status(205) -> <shunt>`, routeID)

	tracer := tracingtest.NewTracer()
	params := Params{
		OpenTracing: &OpenTracingParams{
			Tracer: tracer,
		},
		Flags: FlagsNone,
	}

	t.Setenv("HOSTNAME", "ingress-shunt.tracing.test")

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	req, err := http.NewRequest("GET", ps.URL+"/hello?world", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Flow-Id", "test-flow-id")

	rsp, err := ps.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()
	io.Copy(io.Discard, rsp.Body)

	span := tracer.FindSpan("ingress")
	if span == nil {
		t.Fatal("ingress span not found")
	}

	verifyTag(t, span, SpanKindTag, SpanKindServer)
	verifyTag(t, span, ComponentTag, "skipper")
	verifyTag(t, span, SkipperRouteIDTag, routeID)
	// to save memory we dropped the URL tag from ingress span
	//verifyTag(t, span, HTTPUrlTag, "/hello?world") // For server requests there is no scheme://host:port, see https://golang.org/pkg/net/http/#Request
	verifyTag(t, span, HTTPMethodTag, "GET")
	verifyTag(t, span, HostnameTag, "ingress-shunt.tracing.test")
	verifyTag(t, span, HTTPPathTag, "/hello")
	verifyTag(t, span, HTTPHostTag, ps.Listener.Addr().String())
	verifyTag(t, span, HTTPRequestHeaderCeil, int64(128))
	verifyTag(t, span, FlowIDTag, "test-flow-id")
	verifyTag(t, span, HTTPStatusCodeTag, uint16(205))
	verifyNoTag(t, span, HTTPResponseBodyCeil)
	verifyHasTag(t, span, HTTPRemoteIPTag)
}

func TestTracingIngressSpanLoopback(t *testing.T) {
	shuntRouteID := "ingressShuntRoute"
	loop1RouteID := "loop1Route"
	loop2RouteID := "loop2Route"
	routeIDs := []string{loop2RouteID, loop1RouteID, shuntRouteID}
	paths := map[string]string{
		loop2RouteID: "/loop2",
		loop1RouteID: "/loop1",
		shuntRouteID: "/shunt",
	}

	doc := fmt.Sprintf(`
%s: Path("/shunt") -> setPath("/bye") -> setQuery("void") -> status(204) -> <shunt>;
%s: Path("/loop1") -> setPath("/shunt") -> <loopback>;
%s: Path("/loop2") -> setPath("/loop1") -> <loopback>;
`, shuntRouteID, loop1RouteID, loop2RouteID)

	tracer := tracingtest.NewTracer()
	params := Params{
		OpenTracing: &OpenTracingParams{
			Tracer: tracer,
		},
		Flags: FlagsNone,
	}

	t.Setenv("HOSTNAME", "ingress-loop.tracing.test")

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	req, err := http.NewRequest("GET", ps.URL+"/loop2", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Flow-Id", "test-flow-id")

	rsp, err := ps.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()
	io.Copy(io.Discard, rsp.Body)
	t.Logf("got response %d", rsp.StatusCode)

	sp, ok := findSpanByRouteID(tracer, loop2RouteID)
	if !ok {
		t.Fatalf("span for route %q not found", loop2RouteID)
	}
	verifyTag(t, sp, HTTPStatusCodeTag, uint16(204))

	for _, rid := range routeIDs {
		span, ok := findSpanByRouteID(tracer, rid)
		if !ok {
			t.Fatalf("span for route %q not found", rid)
		}
		verifyTag(t, span, SpanKindTag, SpanKindServer)
		verifyTag(t, span, ComponentTag, "skipper")
		verifyTag(t, span, SkipperRouteIDTag, rid)
		verifyTag(t, span, HTTPMethodTag, "GET")
		verifyTag(t, span, HostnameTag, "ingress-loop.tracing.test")
		verifyTag(t, span, HTTPPathTag, paths[rid])
		verifyTag(t, span, HTTPHostTag, ps.Listener.Addr().String())
		verifyTag(t, span, HTTPRequestHeaderCeil, int64(128))
		verifyTag(t, span, FlowIDTag, "test-flow-id")
		verifyNoTag(t, span, HTTPResponseBodyCeil)
	}
}

func TestTracingSpanName(t *testing.T) {
	s := startTestServer(nil, 0, func(r *http.Request) {})
	defer s.Close()

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: make(http.Header),
	}
	w := httptest.NewRecorder()

	doc := fmt.Sprintf(`hello: Path("/hello") -> tracingSpanName("test-span") -> "%s"`, s.URL)
	tracer := tracingtest.NewTracer()
	params := Params{
		OpenTracing: &OpenTracingParams{
			Tracer: tracer,
		},
		Flags: FlagsNone,
	}

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Fatal(err)
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if span := tracer.FindSpan("test-span"); span == nil {
		t.Error("setting the span name failed")
	}
}

func TestTracingInitialSpanName(t *testing.T) {
	s := startTestServer(nil, 0, func(r *http.Request) {})
	defer s.Close()

	u, _ := url.ParseRequestURI("https://www.example.org/hello")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Header: make(http.Header),
	}
	w := httptest.NewRecorder()

	doc := fmt.Sprintf(`hello: Path("/hello") -> "%s"`, s.URL)
	tracer := tracingtest.NewTracer()
	params := Params{
		OpenTracing: &OpenTracingParams{
			Tracer:      tracer,
			InitialSpan: "test-initial-span",
		},
		Flags: FlagsNone,
	}

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Fatal(err)
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if span := tracer.FindSpan("test-initial-span"); span == nil {
		t.Error("setting the span name failed")
	}
}

func TestTracingProxySpan(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := &mocktracer.TextMapPropagator{}
		_, err := p.Extract(ot.HTTPHeadersCarrier(r.Header))
		if err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK\n"))
	}))
	defer s.Close()

	doc := fmt.Sprintf(`hello: Path("/hello") -> setPath("/bye") -> setQuery("void") -> "%s"`, s.URL)
	tracer := tracingtest.NewTracer()

	t.Setenv("HOSTNAME", "proxy.tracing.test")

	tp, err := newTestProxyWithParams(doc, Params{OpenTracing: &OpenTracingParams{Tracer: tracer}})
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	req, err := http.NewRequest("GET", ps.URL+"/hello?world", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Flow-Id", "test-flow-id")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	span := tracer.FindSpan("proxy")
	if span == nil {
		t.Fatal("proxy span not found")
	}

	backendAddr := s.Listener.Addr().String()

	verifyTag(t, span, SpanKindTag, SpanKindClient)
	verifyTag(t, span, SkipperRouteIDTag, "hello")
	verifyTag(t, span, ComponentTag, "skipper")
	verifyTag(t, span, HTTPUrlTag, "http://"+backendAddr+"/bye") // proxy removes query
	verifyTag(t, span, HTTPMethodTag, "GET")
	verifyTag(t, span, HostnameTag, "proxy.tracing.test")
	verifyTag(t, span, HTTPPathTag, "/bye")
	verifyTag(t, span, HTTPHostTag, backendAddr)
	verifyTag(t, span, HTTPRequestHeaderCeil, int64(128))
	verifyTag(t, span, FlowIDTag, "test-flow-id")
	verifyTag(t, span, HTTPStatusCodeTag, uint16(200))
	verifyTag(t, span, HTTPResponseBodyCeil, int64(4))
	verifyNoTag(t, span, HTTPRemoteIPTag)
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

	const docFmt = `r: * -> <roundRobin, "%s", "%s">;`
	doc := fmt.Sprintf(docFmt, s0.URL, s1.URL)
	tracer := tracingtest.NewTracer()
	tp, err := newTestProxyWithParams(doc, Params{OpenTracing: &OpenTracingParams{Tracer: tracer}})
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	testFallback := func() bool {
		tracer.Reset()
		req, err := http.NewRequest("GET", "https://www.example.org", nil)
		if err != nil {
			t.Fatal(err)
		}

		tp.proxy.ServeHTTP(httptest.NewRecorder(), req)

		var proxySpans []*tracingtest.MockSpan
		for _, span := range tracer.FinishedSpans() {
			if span.OperationName == "proxy" {
				proxySpans = append(proxySpans, span)
			}
		}

		if len(proxySpans) != 2 {
			t.Log("invalid count of proxy spans", len(proxySpans))
			return false
		}

		for _, s := range proxySpans {
			if s.FinishTime.Sub(s.StartTime) >= responseStreamDelay {
				return true
			}
		}

		t.Log("proxy span with the right duration not found")
		return false
	}

	// Two lb group members are used in round-robin, starting at a non-deterministic index.
	// One of them cannot be connected to, and the proxy should fallback to the other. We
	// want to verify here that the proxy span is traced properly in the fallback case.
	//lint:ignore SA4000 valid testcase in this case
	if !testFallback() && !testFallback() {
		t.Error("failed to trace the right span duration for fallback")
	}
}

func TestProxyTracingDefaultOptions(t *testing.T) {
	t1 := newProxyTracing(nil)
	if t1.tracer == nil || t1.initialOperationName == "" {
		t.Errorf("did not set default options")
	}

	t2 := newProxyTracing(&OpenTracingParams{})
	if t2.tracer == nil || t2.initialOperationName == "" {
		t.Errorf("did not set default options")
	}
}

func TestFilterTracing(t *testing.T) {
	for _, tc := range []struct {
		name       string
		operation  string
		filters    []string
		params     *OpenTracingParams
		expectLogs string
	}{
		{
			name:       "enable log filter events",
			operation:  "request_filters",
			filters:    []string{"f1", "f2"},
			params:     &OpenTracingParams{LogFilterEvents: true},
			expectLogs: "f1: start, f1: end, f2: start, f2: end",
		},
		{
			name:       "disable log filter events",
			operation:  "request_filters",
			filters:    []string{"f1", "f2"},
			params:     &OpenTracingParams{LogFilterEvents: false},
			expectLogs: "",
		},
		{
			name:      "disable filter span (ignores log events)",
			operation: "request_filters",
			filters:   []string{"f1", "f2"},
			params:    &OpenTracingParams{DisableFilterSpans: true, LogFilterEvents: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracer := tracingtest.NewTracer()
			tc.params.Tracer = tracer
			tracing := newProxyTracing(tc.params)

			ctx := &context{request: &http.Request{}}

			ft := tracing.startFilterTracing(tc.operation, ctx)
			for _, f := range tc.filters {
				ft.logStart(f)
				ft.logEnd(f)
			}
			ft.finish()

			spans := tracer.FinishedSpans()

			if tc.params.DisableFilterSpans {
				assert.Nil(t, ctx.parentSpan)
				assert.Len(t, spans, 0)
				return
			}

			require.Len(t, spans, 1)

			span := spans[0]
			assert.Equal(t, span, ctx.parentSpan)
			assert.Equal(t, tc.operation, span.OperationName)
			assert.Equal(t, tc.expectLogs, spanLogs(span))
		})
	}
}

func spanLogs(span *tracingtest.MockSpan) string {
	var logs []string
	for _, e := range span.Logs() {
		for _, f := range e.Fields {
			logs = append(logs, fmt.Sprintf("%s: %s", f.Key, f.ValueString))
		}
	}
	return strings.Join(logs, ", ")
}

func TestEnabledLogStreamEvents(t *testing.T) {
	tracer := tracingtest.NewTracer()
	tracing := newProxyTracing(&OpenTracingParams{
		Tracer:          tracer,
		LogStreamEvents: true,
	})
	span := tracer.StartSpan("test")
	defer span.Finish()

	tracing.logStreamEvent(span, "test-filter", StartEvent)
	tracing.logStreamEvent(span, "test-filter", EndEvent)

	mockSpan := span.(*tracingtest.MockSpan)

	if len(mockSpan.Logs()) != 2 {
		t.Errorf("filter lifecycle events were not logged although it was enabled")
	}
}

func TestDisabledLogStreamEvents(t *testing.T) {
	tracer := tracingtest.NewTracer()
	tracing := newProxyTracing(&OpenTracingParams{
		Tracer:          tracer,
		LogStreamEvents: false,
	})
	span := tracer.StartSpan("test")
	defer span.Finish()

	tracing.logStreamEvent(span, "test-filter", StartEvent)
	tracing.logStreamEvent(span, "test-filter", EndEvent)

	mockSpan := span.(*tracingtest.MockSpan)

	if len(mockSpan.Logs()) != 0 {
		t.Errorf("filter lifecycle events were logged although it was disabled")
	}
}

func TestSetEnabledTags(t *testing.T) {
	tracer := tracingtest.NewTracer()
	tracing := newProxyTracing(&OpenTracingParams{
		Tracer:      tracer,
		ExcludeTags: []string{},
	})
	span := tracer.StartSpan("test")
	defer span.Finish()

	tracing.setTag(span, HTTPStatusCodeTag, 200)
	tracing.setTag(span, ComponentTag, "skipper")

	mockSpan := span.(*tracingtest.MockSpan)

	tags := mockSpan.Tags()

	_, ok := tags[HTTPStatusCodeTag]
	_, ok2 := tags[ComponentTag]

	if !ok || !ok2 {
		t.Errorf("could not set tags although they were not configured to be excluded")
	}
}

func TestSetDisabledTags(t *testing.T) {
	tracer := tracingtest.NewTracer()
	tracing := newProxyTracing(&OpenTracingParams{
		Tracer: tracer,
		ExcludeTags: []string{
			SkipperRouteIDTag,
		},
	})
	span := tracer.StartSpan("test")
	defer span.Finish()

	tracing.setTag(span, HTTPStatusCodeTag, 200)
	tracing.setTag(span, ComponentTag, "skipper")
	tracing.setTag(span, SkipperRouteIDTag, "long_route_id")

	mockSpan := span.(*tracingtest.MockSpan)

	tags := mockSpan.Tags()

	_, ok := tags[HTTPStatusCodeTag]
	_, ok2 := tags[ComponentTag]
	_, ok3 := tags[SkipperRouteIDTag]

	if !ok || !ok2 {
		t.Errorf("could not set tags although they were not configured to be excluded")
	}

	if ok3 {
		t.Errorf("a tag was set although it was configured to be excluded")
	}
}

func TestLogEventWithEmptySpan(t *testing.T) {
	tracer := tracingtest.NewTracer()
	tracing := newProxyTracing(&OpenTracingParams{
		Tracer: tracer,
	})

	// should not panic
	tracing.logEvent(nil, "test", StartEvent)
	tracing.logEvent(nil, "test", EndEvent)
}

func TestSetTagWithEmptySpan(t *testing.T) {
	tracer := tracingtest.NewTracer()
	tracing := newProxyTracing(&OpenTracingParams{
		Tracer: tracer,
	})

	// should not panic
	tracing.setTag(nil, "test", "val")
}

func findSpanByRouteID(tracer *tracingtest.MockTracer, routeID string) (*tracingtest.MockSpan, bool) {
	for _, s := range tracer.FinishedSpans() {
		if s.Tag(SkipperRouteIDTag) == routeID {
			return s, true
		}
	}
	return nil, false
}

func verifyTag(t *testing.T, span *tracingtest.MockSpan, name string, expected interface{}) {
	t.Helper()
	if got := span.Tag(name); got != expected {
		t.Errorf("unexpected '%s' tag value: '%v' != '%v'", name, got, expected)
	}
}

func verifyNoTag(t *testing.T, span *tracingtest.MockSpan, name string) {
	t.Helper()
	if got, ok := span.Tags()[name]; ok {
		t.Errorf("unexpected '%s' tag: '%v'", name, got)
	}
}

func verifyHasTag(t *testing.T, span *tracingtest.MockSpan, name string) {
	t.Helper()
	if got, ok := span.Tags()[name]; !ok || got == "" {
		t.Errorf("expected '%s' tag", name)
	}
}

func TestCeilPow2(t *testing.T) {
	assert.Equal(t, int64(0), ceilPow2(0))
	assert.Equal(t, int64(1), ceilPow2(1))
	assert.Equal(t, int64(2), ceilPow2(2))
	assert.Equal(t, int64(4), ceilPow2(3))
	assert.Equal(t, int64(4), ceilPow2(4))
	assert.Equal(t, int64(8), ceilPow2(5))
	assert.Equal(t, int64(8), ceilPow2(6))
	assert.Equal(t, int64(8), ceilPow2(7))
	assert.Equal(t, int64(8), ceilPow2(8))
	assert.Equal(t, int64(16), ceilPow2(9))

	assert.Equal(t, int64(16384), ceilPow2(10_000))
	assert.Equal(t, int64(16384), ceilPow2(16_000))
	assert.Equal(t, int64(32768), ceilPow2(16_385))
	assert.Equal(t, int64(32768), ceilPow2(20_000))
}
