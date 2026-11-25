package proxy

import (
	stdcontext "context"
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
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	skpotel "github.com/zalando/skipper/predicates/otel"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing/tracingtest"
	ottrace "go.opentelemetry.io/contrib/propagators/ot"
	"go.opentelemetry.io/otel"
	otBridge "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/propagation"
	oteltracesdk "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"

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
	verifyTag(t, span, FlowIDTag, "test-flow-id")
	verifyTag(t, span, HTTPStatusCodeTag, uint16(200))
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
	verifyTag(t, span, FlowIDTag, "test-flow-id")
	verifyTag(t, span, HTTPStatusCodeTag, uint16(205))
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
%s: Path("/shunt") -> setPath("/bye") -> setQuery("void") -> tracingTag("tracing-tag", "%s") -> status(204) -> <shunt>;
%s: Path("/loop1") -> setPath("/shunt") -> tracingTag("tracing-tag", "%s") -> <loopback>;
%s: Path("/loop2") -> setPath("/loop1") -> tracingTag("tracing-tag", "%s") -> <loopback>;
`, shuntRouteID, shuntRouteID, loop1RouteID, loop1RouteID, loop2RouteID, loop2RouteID)

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
		verifyTag(t, span, FlowIDTag, "test-flow-id")
		verifyTag(t, span, "tracing-tag", rid)
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
		w.WriteHeader(http.StatusNoContent)
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

	_, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	span := tracer.FindSpan("proxy")
	if span == nil {
		t.Fatal("proxy span not found")
	}

	backendAddr := s.Listener.Addr().String()

	verifyTag(t, span, SpanKindTag, SpanKindClient)
	verifyTag(t, span, SkipperRouteIDTag, "hello")
	verifyTag(t, span, ComponentTag, "skipper")
	verifyTag(t, span, HTTPUrlTag, "http://"+backendAddr+"/bye") // proxy removes query
	verifyTag(t, span, NetworkPeerAddressTag, backendAddr)
	verifyTag(t, span, HTTPMethodTag, "GET")
	verifyTag(t, span, HostnameTag, "proxy.tracing.test")
	verifyTag(t, span, HTTPPathTag, "/bye")
	verifyTag(t, span, HTTPHostTag, backendAddr)
	verifyTag(t, span, FlowIDTag, "test-flow-id")
	verifyTag(t, span, HTTPStatusCodeTag, uint16(204))
	verifyNoTag(t, span, HTTPRemoteIPTag)
}

func TestTracingProxySpanClientTracingTags(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := &mocktracer.TextMapPropagator{}
		_, err := p.Extract(ot.HTTPHeadersCarrier(r.Header))
		if err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer s.Close()

	doc := fmt.Sprintf(`hello: Path("/hello") -> setPath("/bye") -> setQuery("void") -> "%s"`, s.URL)
	tracer := tracingtest.NewTracer()

	t.Setenv("HOSTNAME", "proxy.tracing.test")

	tp, err := newTestProxyWithParams(doc, Params{
		OpenTracing: &OpenTracingParams{
			Tracer:           tracer,
			ClientTraceByTag: true,
		},
	})
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

	_, err = ps.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}

	span := tracer.FindSpan("proxy")
	if span == nil {
		t.Fatal("proxy span not found")
	}

	backendAddr := s.Listener.Addr().String()

	verifyTag(t, span, SpanKindTag, SpanKindClient)
	verifyTag(t, span, SkipperRouteIDTag, "hello")
	verifyTag(t, span, ComponentTag, "skipper")
	verifyTag(t, span, HTTPUrlTag, "http://"+backendAddr+"/bye") // proxy removes query
	verifyTag(t, span, NetworkPeerAddressTag, backendAddr)
	verifyTag(t, span, HTTPMethodTag, "GET")
	verifyTag(t, span, HostnameTag, "proxy.tracing.test")
	verifyTag(t, span, HTTPPathTag, "/bye")
	verifyTag(t, span, HTTPHostTag, backendAddr)
	verifyTag(t, span, FlowIDTag, "test-flow-id")
	verifyTag(t, span, HTTPStatusCodeTag, uint16(204))
	verifyNoTag(t, span, HTTPRemoteIPTag)

	verifyNoTag(t, span, ClientTraceGot100Continue)
	verifyNoTag(t, span, ClientTraceDNS)
	verifyNoTag(t, span, ClientTraceTLS)

	verifyHasTag(t, span, ClientTraceConnect)
	verifyHasTag(t, span, ClientTraceGetConn)
	verifyHasTag(t, span, ClientTraceWroteHeaders)
	verifyHasTag(t, span, ClientTraceWroteRequest)
	verifyHasTag(t, span, ClientTraceGotFirstByte)
	verifyHasTag(t, span, ClientTraceHTTPRoundTrip)
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
	// One of them cannot be connected to, and the proxy should fall back to the other. We
	// want to verify here that the proxy span is traced properly in the fallback case.
	//lint:ignore SA4000 valid testcase in this case
	if !testFallback() && !testFallback() {
		t.Error("failed to trace the right span duration for fallback")
	}
}

func TestTracingProxySpanWithTeeLoopbackFilter(t *testing.T) {
	// shadow backend server for shadow traffic, if shunt backend is used proxy span will be nil
	sb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello! I'm shadow backend!"))
	}))
	defer sb.Close()

	doc := fmt.Sprintf(`
main: Path("/main") -> teeLoopback("shadow-main") -> inlineContent("<h1>Hello! I'm main :)</h1>") -> <shunt>;
shadow: Path("/main") && Tee("shadow-main") && Weight(2) -> "%s";
`, sb.URL)

	tracer := tracingtest.NewTracer()
	params := Params{
		OpenTracing: &OpenTracingParams{
			Tracer: tracer,
		},
		Flags: FlagsNone,
	}

	t.Setenv("HOSTNAME", "shadow-tag.tracing.test")

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	req, err := http.NewRequest("GET", ps.URL+"/main", nil)
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

	// find the proxy span for the shadow route
	var shadowProxySpan *tracingtest.MockSpan
	for _, s := range tracer.FinishedSpans() {
		if s.OperationName == "proxy" && s.Tag(SkipperRouteIDTag) == "shadow" {
			shadowProxySpan = s
			break
		}
	}
	if shadowProxySpan == nil {
		t.Fatal("proxy span for shadow route not found")
	}
	verifyTag(t, shadowProxySpan, HTTPStatusCodeTag, uint16(200))
	verifyTag(t, shadowProxySpan, "shadow", "true")
}

func TestTracingProxySpanWithLoopbackIfStatusFilter(t *testing.T) {
	// b: backend server that returns 404 to trigger loopbackIfStatus
	// fb: fallback backend server for loopback request
	// We cannot use shunt backends instead because in that case the proxy span will be nil
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Hello! I'm main backend!"))
	}))
	defer b.Close()
	fb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello! I'm fallback backend!"))
	}))
	defer fb.Close()

	doc := fmt.Sprintf(`
main: Path("/main") -> loopbackIfStatus(404, "/fallback") -> "%s";
fallback: Path("/fallback") -> "%s";
`, b.URL, fb.URL)

	tracer := tracingtest.NewTracer()
	params := Params{
		OpenTracing: &OpenTracingParams{
			Tracer: tracer,
		},
		Flags: FlagsNone,
	}

	t.Setenv("HOSTNAME", "shadow-tag.tracing.test")

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	req, err := http.NewRequest("GET", ps.URL+"/main", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Flow-Id", "test-flow-id-fallback")

	rsp, err := ps.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()
	io.Copy(io.Discard, rsp.Body)
	t.Logf("got response %d", rsp.StatusCode)

	pxSpanCount := 0
	for _, s := range tracer.FinishedSpans() {
		if s.OperationName == "proxy" {
			pxSpanCount++
			verifyNoTag(t, s, "shadow")
			if s.Tag(SkipperRouteIDTag) == "fallback" {
				verifyTag(t, s, HTTPStatusCodeTag, uint16(200))
			}
			if s.Tag(SkipperRouteIDTag) == "main" {
				verifyTag(t, s, HTTPStatusCodeTag, uint16(404))
			}
		}
	}
	if pxSpanCount == 0 {
		t.Fatal("No proxy spans found! Should found two proxy spans for main and fallback routes")
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

func TestLogFilterEventsByTag(t *testing.T) {
	tracer := tracingtest.NewTracer()
	tracing := newProxyTracing(&OpenTracingParams{
		Tracer:           tracer,
		LogFilterEvents:  true,
		ClientTraceByTag: true,
	})

	ctx := &context{request: &http.Request{}}

	ft := tracing.startFilterTracing("request_filters", ctx)
	for _, f := range []string{"f1", "f2"} {
		ft.logStart(f)
		ft.logEnd(f)
	}
	ft.finish()

	spans := tracer.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]

	if len(span.Logs()) != 0 {
		t.Errorf("expected no log events when clientTraceByTag is enabled, got: %d", len(span.Logs()))
	}

	tags := span.Tags()
	for _, key := range []string{"f1" + FilterStartTagSuffix, "f1" + FilterEndTagSuffix, "f2" + FilterStartTagSuffix, "f2" + FilterEndTagSuffix} {
		ts, ok := tags[key]
		if !ok {
			t.Errorf("expected tag %q to be set", key)
			continue
		}
		if v, ok := ts.(int64); !ok || v <= 0 {
			t.Errorf("expected tag %q to hold a positive timestamp, got: %v", key, ts)
		}
	}
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

func TestLogErrorByTag(t *testing.T) {
	tracer := tracingtest.NewTracer()
	tracing := newProxyTracing(&OpenTracingParams{
		Tracer:           tracer,
		ClientTraceByTag: true,
	})
	span := tracer.StartSpan("test")

	tracing.logError(span, BackendErrorTag, "backend down")
	span.Finish()

	mockSpan := span.(*tracingtest.MockSpan)

	if got := mockSpan.Tags()[BackendErrorTag]; got != "backend down" {
		t.Errorf("expected error message as tag %q, got: %v", BackendErrorTag, got)
	}
	if len(mockSpan.Logs()) != 0 {
		t.Errorf("expected no log events when clientTraceByTag is enabled, got: %d", len(mockSpan.Logs()))
	}
}

func TestLogErrorByEvent(t *testing.T) {
	tracer := tracingtest.NewTracer()
	tracing := newProxyTracing(&OpenTracingParams{
		Tracer: tracer,
	})
	span := tracer.StartSpan("test")

	tracing.logError(span, BackendErrorTag, "backend down")
	span.Finish()

	mockSpan := span.(*tracingtest.MockSpan)

	if _, ok := mockSpan.Tags()[BackendErrorTag]; ok {
		t.Errorf("expected no error tag when clientTraceByTag is disabled")
	}
	if len(mockSpan.Logs()) != 1 {
		t.Fatalf("expected one error log event, got: %d", len(mockSpan.Logs()))
	}
}

func TestLogErrorWithEmptySpan(t *testing.T) {
	tracer := tracingtest.NewTracer()
	tracing := newProxyTracing(&OpenTracingParams{
		Tracer:           tracer,
		ClientTraceByTag: true,
	})

	// should not panic
	tracing.logError(nil, BackendErrorTag, "backend down")
}

func TestSetTagWithEmptySpan(t *testing.T) {
	tracer := tracingtest.NewTracer()
	tracing := newProxyTracing(&OpenTracingParams{
		Tracer: tracer,
	})

	// should not panic
	tracing.setTag(nil, "test", "val")
}

// TestOTelBaggagePredicateWithIncomingHeader verifies that the OTelBaggage predicate
// can match baggage values that arrive via the W3C baggage HTTP header.
// Baggage values may contain "=" characters (e.g. sub_key=sub_value)
// which baggage.FromContext can also understand even if it's percent-encoded.
func TestOTelBaggagePredicateWithIncomingHeader(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		baggageHeader        string
		expectedResponseCode int
	}{
		{
			name:                 "plain equals sign in value",
			baggageHeader:        "baggage_key=sub_key=sub_value",
			expectedResponseCode: http.StatusOK,
		},
		{
			name:                 "percent-encoded equals sign (%3D) in value",
			baggageHeader:        "baggage_key=sub_key%3Dsub_value",
			expectedResponseCode: http.StatusOK,
		},
		{
			name:                 "empty baggage header",
			baggageHeader:        "",
			expectedResponseCode: http.StatusNotFound,
		},
	} {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Logf("raw baggage header seen at backend: %q", r.Header.Get("baggage"))
		}))
		defer backend.Close()

		doc := fmt.Sprintf(`
matched:  OTelBaggage("baggage_key", "sub_key=sub_value") -> "%s";
`, backend.URL)

		tracer := tracingtest.NewTracer()
		tp, err := newTestProxyWithPredicatesAndParams([]routing.PredicateSpec{skpotel.NewBaggage()}, doc, Params{
			OpenTracing: &OpenTracingParams{
				Tracer: tracer,
			},
		})
		require.NoError(t, err)
		defer tp.close()

		ps := httptest.NewServer(tp.proxy)
		defer ps.Close()

		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", ps.URL+"/", nil)
			require.NoError(t, err)
			req.Header.Set("baggage", tc.baggageHeader)
			t.Logf("sending baggage header: %q", tc.baggageHeader)

			rsp, err := ps.Client().Do(req)
			require.NoError(t, err)
			defer rsp.Body.Close()
			io.Copy(io.Discard, rsp.Body)

			assert.Equal(t, tc.expectedResponseCode, rsp.StatusCode,
				"OTelBaggage predicate did not match: baggage from incoming W3C header was not extracted into context")
		})
	}
}

// otelBaggageResponseCode builds a one-route test proxy for the OTelBaggage
// predicate, sends a request with headers set by setHeaders and returns the
// response status code.
func otelBaggageResponseCode(t *testing.T, params Params, doc func(backendURL string) string, setHeaders func(h http.Header)) int {
	t.Helper()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()

	tp, err := newTestProxyWithPredicatesAndParams([]routing.PredicateSpec{skpotel.NewBaggage()}, doc(backend.URL), params)
	require.NoError(t, err)
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	req, err := http.NewRequest("GET", ps.URL+"/", nil)
	require.NoError(t, err)
	setHeaders(req.Header)

	rsp, err := ps.Client().Do(req)
	require.NoError(t, err)
	defer rsp.Body.Close()
	io.Copy(io.Discard, rsp.Body)

	return rsp.StatusCode
}

func otelBaggageDoc(key, value string) func(string) string {
	return func(backendURL string) string {
		return fmt.Sprintf(`matched: OTelBaggage(%q, %q) -> %q;`, key, value, backendURL)
	}
}

func TestOTelBaggagePredicateWithOTELPropagators(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		otelPropagators      string
		setHeaders           func(h http.Header)
		expectedResponseCode int
	}{
		{
			name:            "W3C baggage header, OTEL_PROPAGATORS unset",
			otelPropagators: "",
			setHeaders: func(h http.Header) {
				h.Set("baggage", "baggage_key=sub_value")
			},
			expectedResponseCode: http.StatusOK,
		},
		{
			name:            "ot-baggage header with valid ot-tracer headers, OTEL_PROPAGATORS=ottrace",
			otelPropagators: "ottrace",
			setHeaders: func(h http.Header) {
				h.Set("ot-tracer-traceid", "0000000000000001")
				h.Set("ot-tracer-spanid", "0000000000000001")
				h.Set("ot-tracer-sampled", "true")
				h.Set("ot-baggage-baggage_key", "sub_value")
			},
			expectedResponseCode: http.StatusOK,
		},
		{
			name:            "ot-baggage header without ot-tracer headers is not extracted",
			otelPropagators: "ottrace",
			setHeaders: func(h http.Header) {
				h.Set("ot-baggage-baggage_key", "sub_value")
			},
			expectedResponseCode: http.StatusNotFound,
		},
		{
			name:            "W3C baggage header still works alongside ottrace",
			otelPropagators: "ottrace,baggage",
			setHeaders: func(h http.Header) {
				h.Set("baggage", "baggage_key=sub_value")
			},
			expectedResponseCode: http.StatusOK,
		},
		{
			name:                 "no baggage headers at all",
			otelPropagators:      "",
			setHeaders:           func(h http.Header) {},
			expectedResponseCode: http.StatusNotFound,
		},
		{
			name:            "OTEL_PROPAGATORS=none disables baggage extraction, including the W3C header",
			otelPropagators: "none",
			setHeaders: func(h http.Header) {
				h.Set("baggage", "baggage_key=sub_value")
			},
			expectedResponseCode: http.StatusNotFound,
		},
		{
			name:            "OTEL_PROPAGATORS without baggage disables the W3C header",
			otelPropagators: "tracecontext",
			setHeaders: func(h http.Header) {
				h.Set("baggage", "baggage_key=sub_value")
			},
			expectedResponseCode: http.StatusNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.otelPropagators != "" {
				t.Setenv("OTEL_PROPAGATORS", tc.otelPropagators)
			}

			params := Params{OpenTracing: &OpenTracingParams{Tracer: tracingtest.NewTracer()}}
			code := otelBaggageResponseCode(t, params, otelBaggageDoc("baggage_key", "sub_value"), tc.setHeaders)
			assert.Equal(t, tc.expectedResponseCode, code)
		})
	}
}

// With a BridgeTracer (OpenTelemetry configured via sotel.Init), OTelBaggage
// reads the globally configured propagator (the "propagators" option or
// OTEL_PROPAGATORS, via sotel.Init) instead of reading OTEL_PROPAGATORS
// itself.
func TestOTelBaggagePredicateWithBridgeTracerPropagator(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		globalPropagator     propagation.TextMapPropagator
		setHeaders           func(h http.Header)
		expectedResponseCode int
	}{
		{
			name:             "global propagator set to ottrace",
			globalPropagator: ottrace.OT{},
			setHeaders: func(h http.Header) {
				h.Set("ot-tracer-traceid", "0000000000000001")
				h.Set("ot-tracer-spanid", "0000000000000001")
				h.Set("ot-tracer-sampled", "true")
				h.Set("ot-baggage-baggage_key", "sub_value")
			},
			expectedResponseCode: http.StatusOK,
		},
		{
			name:             "global propagator set to tracecontext and baggage",
			globalPropagator: propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
			setHeaders: func(h http.Header) {
				h.Set("baggage", "baggage_key=sub_value")
			},
			expectedResponseCode: http.StatusOK,
		},
		{
			name:             "empty global propagator",
			globalPropagator: propagation.NewCompositeTextMapPropagator(),
			setHeaders: func(h http.Header) {
				h.Set("baggage", "baggage_key=sub_value")
			},
			expectedResponseCode: http.StatusNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := otel.GetTextMapPropagator()
			otel.SetTextMapPropagator(tc.globalPropagator)
			t.Cleanup(func() { otel.SetTextMapPropagator(previous) })

			otelTracerProvider := oteltracesdk.NewTracerProvider()
			defer otelTracerProvider.Shutdown(stdcontext.Background())
			bridgeTracer, _ := otBridge.NewTracerPair(otelTracerProvider.Tracer("test"))

			params := Params{OpenTracing: &OpenTracingParams{Tracer: bridgeTracer}}
			code := otelBaggageResponseCode(t, params, otelBaggageDoc("baggage_key", "sub_value"), tc.setHeaders)
			assert.Equal(t, tc.expectedResponseCode, code)
		})
	}
}

func TestOTelBaggagePredicateWithConflictingFormats(t *testing.T) {
	t.Setenv("OTEL_PROPAGATORS", "ottrace,baggage")

	for _, tc := range []struct {
		name       string
		setHeaders func(h http.Header)
		key        string
		value      string
		want       bool
	}{
		{
			name: "same key in both formats: baggage (last propagator) wins",
			setHeaders: func(h http.Header) {
				h.Set("ot-tracer-traceid", "0000000000000001")
				h.Set("ot-tracer-spanid", "0000000000000001")
				h.Set("ot-tracer-sampled", "true")
				h.Set("ot-baggage-shared_key", "from_ot_baggage")
				h.Set("baggage", "shared_key=from_w3c_baggage")
			},
			key: "shared_key", value: "from_w3c_baggage",
			want: true,
		},
		{
			name: "same key in both formats: ot-baggage value is overridden, so it does not match",
			setHeaders: func(h http.Header) {
				h.Set("ot-tracer-traceid", "0000000000000001")
				h.Set("ot-tracer-spanid", "0000000000000001")
				h.Set("ot-tracer-sampled", "true")
				h.Set("ot-baggage-shared_key", "from_ot_baggage")
				h.Set("baggage", "shared_key=from_w3c_baggage")
			},
			key: "shared_key", value: "from_ot_baggage",
			want: false,
		},
		{
			name: "different keys in different formats: baggage (last propagator) still replaces the ot-baggage set entirely",
			setHeaders: func(h http.Header) {
				h.Set("ot-tracer-traceid", "0000000000000001")
				h.Set("ot-tracer-spanid", "0000000000000001")
				h.Set("ot-tracer-sampled", "true")
				h.Set("ot-baggage-ot_key", "ot_value")
				h.Set("baggage", "w3c_key=w3c_value")
			},
			key: "ot_key", value: "ot_value",
			want: false,
		},
		{
			name: "different keys in different formats: the W3C key from the last propagator still matches",
			setHeaders: func(h http.Header) {
				h.Set("ot-tracer-traceid", "0000000000000001")
				h.Set("ot-tracer-spanid", "0000000000000001")
				h.Set("ot-tracer-sampled", "true")
				h.Set("ot-baggage-ot_key", "ot_value")
				h.Set("baggage", "w3c_key=w3c_value")
			},
			key: "w3c_key", value: "w3c_value",
			want: true,
		},
		{
			name: "ot-baggage key survives when the W3C baggage header is absent",
			setHeaders: func(h http.Header) {
				h.Set("ot-tracer-traceid", "0000000000000001")
				h.Set("ot-tracer-spanid", "0000000000000001")
				h.Set("ot-tracer-sampled", "true")
				h.Set("ot-baggage-ot_key", "ot_value")
			},
			key: "ot_key", value: "ot_value",
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := Params{OpenTracing: &OpenTracingParams{Tracer: tracingtest.NewTracer()}}
			code := otelBaggageResponseCode(t, params, otelBaggageDoc(tc.key, tc.value), tc.setHeaders)

			wantCode := http.StatusNotFound
			if tc.want {
				wantCode = http.StatusOK
			}
			assert.Equal(t, wantCode, code)
		})
	}
}

// Baggage extraction must not replace the native OTel ingress span with one
// derived from an incoming traceparent header (OTel's trace.ContextWithSpan
// and trace.ContextWithRemoteSpanContext share the same context key).
func TestOTelBaggageExtractionPreservesIngressSpan(t *testing.T) {
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })

	otelTracerProvider := oteltracesdk.NewTracerProvider()
	defer otelTracerProvider.Shutdown(stdcontext.Background())
	bridgeTracer, _ := otBridge.NewTracerPair(otelTracerProvider.Tracer("test"))

	fr := builtin.MakeRegistry()
	fr.Register(&captureNativeOTelSpanID{})

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()

	doc := fmt.Sprintf(`matched: * -> captureNativeOTelSpanID() -> "%s";`, backend.URL)
	tp, err := newCustomTestProxy(nil, fr, doc, Params{
		OpenTracing: &OpenTracingParams{Tracer: bridgeTracer},
	}, nil)
	require.NoError(t, err)
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	req, err := http.NewRequest("GET", ps.URL+"/", nil)
	require.NoError(t, err)
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	rsp, err := ps.Client().Do(req)
	require.NoError(t, err)
	defer rsp.Body.Close()
	io.Copy(io.Discard, rsp.Body)

	gotSpanID := rsp.Header.Get("X-Native-Otel-Span-Id")
	assert.NotEmpty(t, gotSpanID)
	assert.NotEqual(t, "b7ad6b7169203331", gotSpanID,
		"native OTel context should keep skipper's own ingress span, not the incoming traceparent's span")
}

// captureNativeOTelSpanID reads the native OTel span out of the request
// context and returns it in a response header for assertions.
type captureNativeOTelSpanID struct{}

func (f *captureNativeOTelSpanID) Name() string { return "captureNativeOTelSpanID" }
func (f *captureNativeOTelSpanID) CreateFilter([]any) (filters.Filter, error) {
	return f, nil
}
func (f *captureNativeOTelSpanID) Request(ctx filters.FilterContext) {
	sc := oteltrace.SpanContextFromContext(ctx.Request().Context())
	ctx.StateBag()["native-otel-span-id"] = sc.SpanID().String()
}
func (f *captureNativeOTelSpanID) Response(ctx filters.FilterContext) {
	if id, ok := ctx.StateBag()["native-otel-span-id"].(string); ok {
		ctx.Response().Header.Set("X-Native-Otel-Span-Id", id)
	}
}

func findSpanByRouteID(tracer *tracingtest.MockTracer, routeID string) (*tracingtest.MockSpan, bool) {
	for _, s := range tracer.FinishedSpans() {
		if s.Tag(SkipperRouteIDTag) == routeID {
			return s, true
		}
	}
	return nil, false
}

func verifyTag(t *testing.T, span *tracingtest.MockSpan, name string, expected any) {
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
	} else {
		t.Logf("%s: %v", name, got)
	}
}
