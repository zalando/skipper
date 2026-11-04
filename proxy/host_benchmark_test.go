package proxy

import (
	stdlibcontext "context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/predicates/query"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
	otbridge "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type hostBenchmarkBackend struct{}

func (hostBenchmarkBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": {"text/plain"}},
		Body:          io.NopCloser(strings.NewReader("ok")),
		ContentLength: 2,
		Request:       req,
	}, nil
}

func BenchmarkProxyKubernetesHosts(b *testing.B) {
	for _, n := range []int{300, 3000} {
		for _, backend := range []string{"canned", "http"} {
			b.Run(fmt.Sprintf("%d/%s", n, backend), func(b *testing.B) {
				endpoints := make([]*eskip.LBEndpoint, 5)
				for i := range endpoints {
					endpoints[i] = &eskip.LBEndpoint{Address: fmt.Sprintf("http://127.0.0.1:%d", 10000+i)}
					if backend == "http" {
						s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
							w.Header().Set("Content-Type", "text/plain")
							io.WriteString(w, "ok")
						}))
						b.Cleanup(s.Close)
						endpoints[i].Address = s.URL
					}
				}
				defs := make([]*eskip.Route, n)
				requests := make([]*http.Request, n)
				for i := range defs {
					host := fmt.Sprintf("site%d.example.org", i)
					defs[i] = &eskip.Route{
						Id: fmt.Sprintf("r%d", i),
						Predicates: []*eskip.Predicate{
							{Name: "PathSubtree", Args: []any{"/"}},
							{Name: "QueryParam", Args: []any{"lang", "^en$"}},
							{Name: "Host", Args: []any{"^(" + strings.ReplaceAll(host, ".", "[.]") + "[.]?(:[0-9]+)?)$"}},
						},
						Filters: []*eskip.Filter{
							{Name: "fifo", Args: []any{100, 150, "10s"}},
							{Name: "disableAccessLog", Args: []any{1, 2, 3}},
							{Name: "stateBagToTag", Args: []any{"auth-user", "client_id"}},
						},
						BackendType: eskip.LBBackend, LBAlgorithm: "powerOfRandomNChoices", LBEndpoints: endpoints,
					}
					requests[i] = httptest.NewRequest("GET", "http://"+host+"/resource?lang=en", nil)
				}

				m := metrics.NewPrometheus(metrics.Options{EnableServeHostMetrics: true, EnableBackendHostMetrics: true})
				b.Cleanup(m.Close)
				sr := scheduler.NewRegistry()
				b.Cleanup(sr.Close)
				er := routing.NewEndpointRegistry(routing.RegistryOptions{PassiveHealthCheckEnabled: true, StatsResetPeriod: 10 * time.Second, MinRequests: 10, MaxHealthCheckDropProbability: .5})
				dc := testdataclient.New(defs)
				b.Cleanup(dc.Close)
				rt := routing.New(routing.Options{
					FilterRegistry: builtin.MakeRegistry(), DataClients: []routing.DataClient{dc},
					Predicates:      []routing.PredicateSpec{query.New()},
					PreProcessors:   []routing.PreProcessor{sr.PreProcessor()},
					PostProcessors:  []routing.PostProcessor{loadbalancer.NewAlgorithmProvider(), er, sr},
					SignalFirstLoad: true, SuppressLogs: true,
				})
				b.Cleanup(rt.Close)
				<-rt.FirstLoad()
				tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
				b.Cleanup(func() { tp.Shutdown(stdlibcontext.Background()) })
				tracer := otbridge.NewBridgeTracer()
				tracer.SetOpenTelemetryTracer(tp.Tracer("skipper"))
				tracer.SetTextMapPropagator(propagation.TraceContext{})
				params := Params{
					Routing: rt, Metrics: m, EndpointRegistry: er,
					OpenTracing:              &OpenTracingParams{Tracer: tracer},
					EnablePassiveHealthCheck: true, PassiveHealthCheck: &PassiveHealthCheck{MaxUnhealthyEndpointsRatio: .5},
				}
				if backend == "canned" {
					params.CustomHttpRoundTripperWrap = func(http.RoundTripper) http.RoundTripper { return hostBenchmarkBackend{} }
				}
				p := WithParams(params)
				b.Cleanup(func() { p.Close() })
				// Warm every host's metrics and all persistent backend connections.
				for i, req := range requests {
					route, _ := rt.Route(req)
					if route == nil || route.Id != defs[i].Id {
						b.Fatalf("unexpected route for %s: %v", req.Host, route)
					}
					rsp := httptest.NewRecorder()
					p.ServeHTTP(rsp, req)
					if rsp.Code != http.StatusOK || rsp.Body.String() != "ok" {
						b.Fatalf("unexpected response: %d %s", rsp.Code, rsp.Body)
					}
				}
				b.ReportAllocs()
				var rsp *httptest.ResponseRecorder
				i := 0
				for b.Loop() {
					rsp = httptest.NewRecorder()
					p.ServeHTTP(rsp, requests[i])
					i = (i + 1) % len(requests)
				}
				if rsp.Code != http.StatusOK || rsp.Body.String() != "ok" {
					b.Fatalf("unexpected response: %d %s", rsp.Code, rsp.Body)
				}
			})
		}
	}
}
