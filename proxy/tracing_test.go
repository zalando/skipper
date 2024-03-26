package proxy

import (
	stdlibctx "context"
	"crypto/md5"
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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	tracingtest.InitPropagator(t, traceHeader)
	tracer := &tracingtest.OtelTracer{}
	params := Params{
		OpenTracing: &OpenTracingParams{
			OtelTracer: tracer,
		},
		Flags: FlagsNone,
	}

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Error(err)
		return
	}
	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if len(tracer.RecordedSpans()) == 0 {
		t.Fatal("no span recorded...")
	}
	if tracer.RecordedSpans()[0].Trace != traceContent {
		t.Errorf("trace not found, got `%s` instead", tracer.RecordedSpans()[0].Trace)
	}
	if tracer.RecordedSpans()[0].Parent == nil {
		t.Errorf("no references found, this is a root span")
	}
}

func TestTracingIngressSpanShunt(t *testing.T) {
	routeID := "ingressShuntRoute"
	doc := fmt.Sprintf(`%s: Path("/hello") -> setPath("/bye") -> setQuery("void") -> status(205) -> <shunt>`, routeID)

	tracer := &tracingtest.OtelTracer{}
	params := Params{
		OpenTracing: &OpenTracingParams{
			OtelTracer: tracer,
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

	// client may get response before proxy finishes span
	time.Sleep(10 * time.Millisecond)

	span, ok := tracer.FindSpan("ingress")
	if !ok {
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
	verifyTag(t, span, HTTPStatusCodeTag, "205")
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

	tracer := &tracingtest.OtelTracer{}
	params := Params{
		OpenTracing: &OpenTracingParams{
			OtelTracer: tracer,
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

	// client may get response before proxy finishes span
	time.Sleep(10 * time.Millisecond)

	sp, ok := findSpanByRouteID(tracer, loop2RouteID)
	if !ok {
		t.Fatalf("span for route %q not found", loop2RouteID)
	}
	verifyTag(t, sp, HTTPStatusCodeTag, "204")

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

	tracingtest.InitPropagator(t, traceHeader)
	tracer := &tracingtest.OtelTracer{TraceContent: traceContent}
	params := Params{
		OpenTracing: &OpenTracingParams{
			OtelTracer: tracer,
		},
		Flags: FlagsNone,
	}

	tp, err := newTestProxyWithParams(doc, params)
	if err != nil {
		t.Fatal(err)
	}

	defer tp.close()

	tp.proxy.ServeHTTP(w, r)

	if _, ok := tracer.FindSpan("test-span"); !ok {
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
	tracer := &tracingtest.OtelTracer{TraceContent: traceContent}
	tracingtest.InitPropagator(t, traceHeader)
	params := Params{
		OpenTracing: &OpenTracingParams{
			OtelTracer:  tracer,
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

	if _, ok := tracer.FindSpan("test-initial-span"); !ok {
		t.Error("setting the span name failed")
	}
}

func TestTracingProxyWithOtSpan(t *testing.T) {
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
	tracer := mocktracer.New()

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

	// client may get response before proxy finishes span
	time.Sleep(10 * time.Millisecond)

	span, ok := otFindSpan(tracer, "proxy")
	if !ok {
		t.Fatal("proxy span not found")
	}

	backendAddr := s.Listener.Addr().String()

	otVerifyTag(t, span, SpanKindTag, SpanKindClient)
	otVerifyTag(t, span, SkipperRouteIDTag, "hello")
	otVerifyTag(t, span, ComponentTag, "skipper")
	otVerifyTag(t, span, HTTPUrlTag, "http://"+backendAddr+"/bye") // proxy removes query
	otVerifyTag(t, span, HTTPMethodTag, "GET")
	otVerifyTag(t, span, HostnameTag, "proxy.tracing.test")
	otVerifyTag(t, span, HTTPPathTag, "/bye")
	otVerifyTag(t, span, HTTPHostTag, backendAddr)
	otVerifyTag(t, span, FlowIDTag, "test-flow-id")
	otVerifyTag(t, span, HTTPStatusCodeTag, "204")
	otVerifyNoTag(t, span, HTTPRemoteIPTag)
}

func TestTracingProxyWithOtelSpan(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		)
		ctx := p.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		span := trace.SpanFromContext(ctx)
		if sc := span.SpanContext(); !sc.IsValid() {
			t.Error("expected tracingtest.OtelSpan, got otel.noopSpan/no span in the ctx")
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer s.Close()

	doc := fmt.Sprintf(`hello: Path("/hello") -> setPath("/bye") -> setQuery("void") -> "%s"`, s.URL)

	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("ExampleService"),
		),
	)
	if err != nil {
		t.Fatalf("failed to initialize resource: %v", err)
	}

	ctx := stdlibctx.Background()
	exp := tracetest.NewInMemoryExporter()

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(20*time.Millisecond)),
		sdktrace.WithResource(r),
	)
	defer provider.Shutdown(ctx)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
	tracer := provider.Tracer("testing-skipper-proxy")

	t.Setenv("HOSTNAME", "proxy.tracing.test")
	tp, err := newTestProxyWithParams(doc, Params{OpenTracing: &OpenTracingParams{OtelTracer: tracer}})
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

	// client may get response before spans are exported
	time.Sleep(100 * time.Millisecond)

	var proxySpan sdktrace.ReadOnlySpan
	spans := exp.GetSpans()
	assert.NotEmpty(t, spans)
	for _, s := range spans {
		if s.Name == "proxy" {
			proxySpan = s.Snapshot()
			break
		}
	}

	if proxySpan == nil {
		t.Fatal("proxy span should not be nil")
	}

	backendAddr := s.Listener.Addr().String()

	assert.Contains(t, proxySpan.Attributes(), attribute.String(SpanKindTag, SpanKindClient))
	assert.Contains(t, proxySpan.Attributes(), attribute.String(SkipperRouteIDTag, "hello"))
	assert.Contains(t, proxySpan.Attributes(), attribute.String(ComponentTag, "skipper"))
	assert.Contains(t, proxySpan.Attributes(), attribute.String(HTTPUrlTag, "http://"+backendAddr+"/bye"))
	assert.Contains(t, proxySpan.Attributes(), attribute.String(HTTPMethodTag, "GET"))
	assert.Contains(t, proxySpan.Attributes(), attribute.String(HostnameTag, "proxy.tracing.test"))
	assert.Contains(t, proxySpan.Attributes(), attribute.String(HTTPPathTag, "/bye"))
	assert.Contains(t, proxySpan.Attributes(), attribute.String(HTTPHostTag, backendAddr))
	assert.Contains(t, proxySpan.Attributes(), attribute.String(FlowIDTag, "test-flow-id"))
	assert.Contains(t, proxySpan.Attributes(), attribute.String(HTTPStatusCodeTag, "204"))
	assert.Contains(t, proxySpan.Attributes(), attribute.String(SpanKindTag, SpanKindClient))

	for _, attr := range proxySpan.Attributes() {
		assert.NotEqual(t, string(attr.Key), HTTPRemoteIPTag)
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

	const docFmt = `r: * -> <roundRobin, "%s", "%s">;`
	doc := fmt.Sprintf(docFmt, s0.URL, s1.URL)
	tracer := &tracingtest.OtelTracer{}
	tp, err := newTestProxyWithParams(doc, Params{OpenTracing: &OpenTracingParams{OtelTracer: tracer}})
	if err != nil {
		t.Fatal(err)
	}
	defer tp.close()

	testFallback := func() bool {
		tracer.Reset("")
		req, err := http.NewRequest("GET", "https://www.example.org", nil)
		if err != nil {
			t.Fatal(err)
		}

		tp.proxy.ServeHTTP(httptest.NewRecorder(), req)

		proxySpans := tracer.FindAllSpans("proxy")
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
	if t1.Tracer == nil || t1.initialOperationName == "" {
		t.Errorf("did not set default options")
	}

	t2 := newProxyTracing(&OpenTracingParams{})
	if t2.Tracer == nil || t2.initialOperationName == "" {
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
			tracer := &tracingtest.OtelTracer{}
			tc.params.OtelTracer = tracer
			tracing := newProxyTracing(tc.params)

			ctx := &context{request: &http.Request{}}

			ft := tracing.startFilterTracing(tc.operation, ctx)

			for _, f := range tc.filters {
				ft.logStart(f)
				ft.logEnd(f)
			}
			ft.finish()

			if tc.params.DisableFilterSpans {
				assert.Nil(t, ctx.parentSpan)
				assert.Len(t, tracer.RecordedSpans(), 0)
				return
			}

			require.Len(t, tracer.RecordedSpans(), 1)

			span := tracer.RecordedSpans()[0]
			parentSpan := ctx.parentSpan.(*tracingtest.OtelSpan)

			assert.Equal(t, span, parentSpan)
			assert.Equal(t, tc.operation, span.OperationName)
			assert.Equal(t, tc.expectLogs, spanLogs(span))
		})
	}
}

func spanLogs(span *tracingtest.OtelSpan) string {
	var logs []string
	for _, e := range span.Events {
		for _, f := range e.Attributes {
			logs = append(logs, fmt.Sprintf("%s: %s", f.Key, f.Value.AsString()))
		}
	}
	return strings.Join(logs, ", ")
}

func TestEnabledLogStreamEvents(t *testing.T) {
	ctx := stdlibctx.TODO()
	tracer := &tracingtest.OtelTracer{}
	tracing := newProxyTracing(&OpenTracingParams{
		OtelTracer:      tracer,
		LogStreamEvents: true,
	})
	_, span := tracing.Start(ctx, "test")
	defer span.End()

	tracing.logStreamEvent(span, "test-filter", StartEvent)
	tracing.logStreamEvent(span, "test-filter", EndEvent)

	mockspan, ok := span.(*tracingtest.OtelSpan)
	if !ok {
		t.Errorf("span is not of type *tracingtest.OtelSpan")
	}

	if len(mockspan.Events) != 2 {
		t.Errorf("filter lifecycle events were not logged although it was enabled")
	}
}

func TestDisabledLogStreamEvents(t *testing.T) {
	ctx := stdlibctx.TODO()
	tracer := &tracingtest.OtelTracer{}
	tracing := newProxyTracing(&OpenTracingParams{
		OtelTracer:      tracer,
		LogStreamEvents: false,
	})
	_, span := tracing.Start(ctx, "test")
	defer span.End()

	tracing.logStreamEvent(span, "test-filter", StartEvent)
	tracing.logStreamEvent(span, "test-filter", EndEvent)

	mockspan, ok := span.(*tracingtest.OtelSpan)
	if !ok {
		t.Errorf("span is not of type *tracingtest.OtelSpan")
	}

	if len(mockspan.Events) != 0 {
		t.Errorf("filter lifecycle events were logged although it was disabled")
	}
}

func TestSetEnabledTags(t *testing.T) {
	ctx := stdlibctx.TODO()
	tracer := &tracingtest.OtelTracer{}
	tracing := newProxyTracing(&OpenTracingParams{
		OtelTracer:  tracer,
		ExcludeTags: []string{},
	})
	_, span := tracing.Start(ctx, "test")
	defer span.End()

	tracing.setTag(span, HTTPStatusCodeTag, "200")
	tracing.setTag(span, ComponentTag, "skipper")

	mockspan, ok := span.(*tracingtest.OtelSpan)
	if !ok {
		t.Errorf("span is not of type *tracingtest.OtelSpan")
	}

	_, ok1 := mockspan.Attributes[HTTPStatusCodeTag]
	_, ok2 := mockspan.Attributes[ComponentTag]

	if !ok1 || !ok2 {
		t.Errorf("could not set tags although they were not configured to be excluded")
	}
}

func TestSetDisabledTags(t *testing.T) {
	ctx := stdlibctx.TODO()
	tracer := &tracingtest.OtelTracer{}
	tracing := newProxyTracing(&OpenTracingParams{
		OtelTracer: tracer,
		ExcludeTags: []string{
			SkipperRouteIDTag,
		},
	})
	_, span := tracing.Start(ctx, "test")
	defer span.End()

	tracing.setTag(span, HTTPStatusCodeTag, "200")
	tracing.setTag(span, ComponentTag, "skipper")
	tracing.setTag(span, SkipperRouteIDTag, "long_route_id")

	mockspan, ok := span.(*tracingtest.OtelSpan)
	if !ok {
		t.Errorf("span is not of type *tracingtest.OtelSpan")
	}

	_, ok1 := mockspan.Attributes[HTTPStatusCodeTag]
	_, ok2 := mockspan.Attributes[ComponentTag]
	_, ok3 := mockspan.Attributes[SkipperRouteIDTag]

	if !ok1 || !ok2 {
		t.Errorf("could not set tags although they were not configured to be excluded")
	}

	if ok3 {
		t.Errorf("a tag was set although it was configured to be excluded")
	}
}

func TestLogEventWithEmptySpan(t *testing.T) {
	tracer := &tracingtest.OtelTracer{}
	tracing := newProxyTracing(&OpenTracingParams{
		OtelTracer: tracer,
	})

	// should not panic
	tracing.logEvent(nil, "test", StartEvent)
	tracing.logEvent(nil, "test", EndEvent)
}

func TestSetTagWithEmptySpan(t *testing.T) {
	tracer := &tracingtest.OtelTracer{}
	tracing := newProxyTracing(&OpenTracingParams{
		OtelTracer: tracer,
	})

	// should not panic
	tracing.setTag(nil, "test", "val")
}

func otFindSpan(tracer *mocktracer.MockTracer, name string) (*mocktracer.MockSpan, bool) {
	for _, s := range tracer.FinishedSpans() {
		if s.OperationName == name {
			return s, true
		}
	}
	return nil, false
}

func otVerifyTag(t *testing.T, span *mocktracer.MockSpan, name string, expected interface{}) {
	t.Helper()
	if got := span.Tag(name); got != expected {
		t.Errorf("unexpected '%s' tag value: '%v' != '%v'", name, got, expected)
	}
}

func otVerifyNoTag(t *testing.T, span *mocktracer.MockSpan, name string) {
	t.Helper()
	if got, ok := span.Tags()[name]; ok {
		t.Errorf("unexpected '%s' tag: '%v'", name, got)
	}
}

func findSpanByRouteID(tracer *tracingtest.OtelTracer, routeID string) (*tracingtest.OtelSpan, bool) {
	for _, s := range tracer.RecordedSpans() {
		if s.Attributes[SkipperRouteIDTag] == routeID {
			return s, true
		}
	}
	return nil, false
}

func verifyTag(t *testing.T, span *tracingtest.OtelSpan, name string, expected interface{}) {
	t.Helper()
	if got, ok := span.Attributes[name]; !ok || got != expected {
		t.Errorf("unexpected '%s' tag value: '%v' != '%v'", name, got, expected)
	}
}

func verifyHasTag(t *testing.T, span *tracingtest.OtelSpan, name string) {
	t.Helper()
	if got, ok := span.Attributes[name]; !ok || got == "" {
		t.Errorf("expected '%s' tag", name)
	}
}
