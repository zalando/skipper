package proxy_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/tracing/tracingtest"
)

type hedgeAttemptRecorder struct {
	mu           sync.Mutex
	destinations []string
}

type hedgeRecordingRoundTripper struct {
	inner    http.RoundTripper
	recorder *hedgeAttemptRecorder
}

type scriptedHedgeRoundTripper func(*http.Request) (*http.Response, error)

type hedgeObservation struct {
	ready bool
	delay time.Duration
}

type hedgeObservationSpec struct {
	observations chan<- hedgeObservation
}

type hedgeObservationFilter struct {
	observations chan<- hedgeObservation
}

type hedgeStateSeedSpec struct {
	sampleCount   int
	eligibleCount int
	sampleLatency time.Duration
}

type hedgeStateSeedFilter struct {
	sampleCount   int
	eligibleCount int
	sampleLatency time.Duration
}

type trackedLBMetrics struct {
	detected        time.Time
	lastSeen        time.Time
	inflight        atomic.Int64
	requests        atomic.Int64
	failures        atomic.Int64
	dropProbability float64
	fixedDrop       bool
}

type trackedLBMetricsPostProcessor struct {
	mu      sync.Mutex
	entries map[string]*trackedLBMetrics
}

type hedgeEndpointPolicyPostProcessor struct {
	fadeInDuration time.Duration
	fadeInExponent float64
	detectedAges   map[string]time.Duration
	dropProb       map[string]float64
	zones          map[string]string
}

type observedResponseBody struct {
	data       []byte
	requestCtx context.Context
	release    <-chan struct{}
	wait       bool
	started    chan struct{}
	closed     chan struct{}
	canceled   chan struct{}
	startOnce  sync.Once
	cancelOnce sync.Once
	closeCalls atomic.Int32
	readCalls  atomic.Int32
	offset     int
}

func (f scriptedHedgeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func (*hedgeObservationSpec) Name() string { return "hedgeObservation" }

func (s *hedgeObservationSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	return &hedgeObservationFilter{observations: s.observations}, nil
}

func (*hedgeObservationFilter) Request(filters.FilterContext) {}

func (f *hedgeObservationFilter) Response(ctx filters.FilterContext) {
	state, ok := ctx.StateBag()[filters.HedgeStateKey].(filters.HedgeState)
	if !ok {
		f.observations <- hedgeObservation{}
		return
	}
	delay, ready := state.HedgeDelay()
	f.observations <- hedgeObservation{ready: ready, delay: delay}
}

func (*hedgeStateSeedSpec) Name() string { return "hedgeStateSeed" }

func (s *hedgeStateSeedSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	return &hedgeStateSeedFilter{sampleCount: s.sampleCount, eligibleCount: s.eligibleCount, sampleLatency: s.sampleLatency}, nil
}

func (f *hedgeStateSeedFilter) Request(ctx filters.FilterContext) {
	if ctx.Request().Header.Get("X-Hedge-Seed") == "" {
		return
	}
	state, ok := ctx.StateBag()[filters.HedgeStateKey].(filters.HedgeState)
	if !ok {
		return
	}
	for sample := 0; sample < f.sampleCount; sample++ {
		state.ObserveResponseHeaderLatency(f.sampleLatency)
	}
	for request := 0; request < f.eligibleCount; request++ {
		state.RecordEligibleRequest()
	}
}

func (*hedgeStateSeedFilter) Response(filters.FilterContext) {}

func newObservedResponseBody(data string, requestCtx context.Context, release <-chan struct{}, wait bool) *observedResponseBody {
	return &observedResponseBody{
		data:       []byte(data),
		requestCtx: requestCtx,
		release:    release,
		wait:       wait,
		started:    make(chan struct{}),
		closed:     make(chan struct{}),
		canceled:   make(chan struct{}),
	}
}

func (b *observedResponseBody) Read(p []byte) (int, error) {
	b.readCalls.Add(1)
	b.startOnce.Do(func() { close(b.started) })
	if b.wait {
		select {
		case <-b.release:
		case <-b.requestCtx.Done():
			b.cancelOnce.Do(func() { close(b.canceled) })
			return 0, b.requestCtx.Err()
		}
	}
	if b.offset == len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.offset:])
	b.offset += n
	return n, nil
}

func (b *observedResponseBody) Close() error {
	if b.closeCalls.Add(1) == 1 {
		close(b.closed)
	}
	return nil
}

func (m *trackedLBMetrics) DetectedTime() time.Time     { return m.detected }
func (m *trackedLBMetrics) SetDetected(value time.Time) { m.detected = value }
func (m *trackedLBMetrics) LastSeen() time.Time         { return m.lastSeen }
func (m *trackedLBMetrics) SetLastSeen(value time.Time) { m.lastSeen = value }
func (m *trackedLBMetrics) InflightRequests() int64     { return m.inflight.Load() }
func (m *trackedLBMetrics) IncInflightRequest()         { m.inflight.Add(1) }
func (m *trackedLBMetrics) DecInflightRequest()         { m.inflight.Add(-1) }
func (m *trackedLBMetrics) IncRequests(options routing.IncRequestsOptions) {
	m.requests.Add(1)
	if options.FailedRoundTrip {
		m.failures.Add(1)
	}
}
func (m *trackedLBMetrics) HealthCheckDropProbability() float64 {
	if m.fixedDrop {
		return m.dropProbability
	}
	if requests := m.requests.Load(); requests != 0 {
		return float64(m.failures.Load()) / float64(requests)
	}
	return 0
}
func (*trackedLBMetrics) Weight() float64 { return 1 }

func (p *trackedLBMetricsPostProcessor) Do(routes []*routing.Route) []*routing.Route {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, route := range routes {
		for i := range route.LBEndpoints {
			endpoint := &route.LBEndpoints[i]
			metrics := p.entries[endpoint.Host]
			if metrics == nil {
				metrics = &trackedLBMetrics{detected: time.Now(), lastSeen: time.Now()}
				p.entries[endpoint.Host] = metrics
			}
			endpoint.Metrics = metrics
		}
	}
	return routes
}

func (p *trackedLBMetricsPostProcessor) forHost(host string) *trackedLBMetrics {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.entries[host]
}

func (p *hedgeEndpointPolicyPostProcessor) Do(routes []*routing.Route) []*routing.Route {
	for _, route := range routes {
		if route.BackendType != eskip.LBBackend {
			continue
		}
		route.LBFadeInDuration = p.fadeInDuration
		route.LBFadeInExponent = p.fadeInExponent
		for i := range route.LBEndpoints {
			endpoint := &route.LBEndpoints[i]
			if age, ok := p.detectedAges[endpoint.Host]; ok {
				endpoint.Metrics.SetDetected(time.Now().Add(-age))
			}
			if drop, ok := p.dropProb[endpoint.Host]; ok {
				if tracked, ok := endpoint.Metrics.(*trackedLBMetrics); ok {
					tracked.dropProbability = drop
					tracked.fixedDrop = true
				}
			}
			if zone, ok := p.zones[endpoint.Host]; ok {
				endpoint.Zone = zone
			}
		}
	}
	return routes
}

func (r *hedgeAttemptRecorder) wrap(inner http.RoundTripper) http.RoundTripper {
	return &hedgeRecordingRoundTripper{inner: inner, recorder: r}
}

func (r *hedgeRecordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.recorder.mu.Lock()
	r.recorder.destinations = append(r.recorder.destinations, req.URL.Scheme+"://"+req.URL.Host)
	r.recorder.mu.Unlock()
	return r.inner.RoundTrip(req)
}

func (r *hedgeAttemptRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.destinations...)
}

func closedHedgeBackend(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return "http://" + address
}

func newHedgeTestProxy(t *testing.T, recorder *hedgeAttemptRecorder, endpoints []string, enabled bool) *proxytest.TestProxy {
	t.Helper()
	quotedEndpoints := make([]string, len(endpoints))
	for i, endpoint := range endpoints {
		quotedEndpoints[i] = strconv.Quote(endpoint)
	}
	filter := ""
	if enabled {
		filter = "hedge() -> "
	}
	route := eskip.MustParse(fmt.Sprintf("r: * -> %s<roundRobin, %s>;", filter, strings.Join(quotedEndpoints, ", ")))
	tp := proxytest.WithParams(builtin.MakeRegistry(), proxy.Params{CustomHttpRoundTripperWrap: recorder.wrap}, route[0])
	t.Cleanup(func() { _ = tp.Close() })
	return tp
}

func requestHedgeRoute(t *testing.T, client *proxytest.TestClient, target, method, body string) {
	t.Helper()
	var requestBody io.Reader
	if body != "" {
		requestBody = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, target, requestBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("response status = %d, want %d", response.StatusCode, http.StatusBadGateway)
	}
}

func requestHedgeUpgrade(t *testing.T, client *proxytest.TestClient, target string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("response status = %d, want %d", response.StatusCode, http.StatusBadGateway)
	}
}

func newSeededHedgeTestProxy(t *testing.T, sampleCount, eligibleCount int, sampleLatency time.Duration, backendTimeout string, roundTrip func(*http.Request) (*http.Response, error)) *proxytest.TestProxy {
	t.Helper()
	registry := builtin.MakeRegistry()
	registry.Register(&hedgeStateSeedSpec{sampleCount: sampleCount, eligibleCount: eligibleCount, sampleLatency: sampleLatency})
	prefix := ""
	if backendTimeout != "" {
		prefix = fmt.Sprintf("backendTimeout(%q) -> ", backendTimeout)
	}
	route := eskip.MustParse(fmt.Sprintf("r: * -> %shedge() -> hedgeStateSeed() -> <roundRobin, %q, %q>;", prefix, closedHedgeBackend(t), closedHedgeBackend(t)))
	tp := proxytest.WithParams(registry, proxy.Params{
		CustomHttpRoundTripperWrap: func(http.RoundTripper) http.RoundTripper {
			return scriptedHedgeRoundTripper(roundTrip)
		},
	}, route[0])
	t.Cleanup(func() { _ = tp.Close() })
	return tp
}

func waitForEndpointRequests(t *testing.T, endpoint *trackedLBMetrics, count int64) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for endpoint.requests.Load() < count {
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatalf("endpoint recorded %d requests, want at least %d", endpoint.requests.Load(), count)
		}
	}
}

func newHedgeEndpointAccountingProxy(t *testing.T, roundTrip func(*http.Request, http.RoundTripper) (*http.Response, error)) (*httptest.Server, *trackedLBMetricsPostProcessor, []string) {
	return newHedgeEndpointAccountingProxyWithAlgorithm(t, "roundRobin", roundTrip)
}

func newHedgeEndpointAccountingProxyWithAlgorithm(t *testing.T, algorithm string, roundTrip func(*http.Request, http.RoundTripper) (*http.Response, error)) (*httptest.Server, *trackedLBMetricsPostProcessor, []string) {
	t.Helper()
	endpoints := []string{closedHedgeBackend(t), closedHedgeBackend(t)}
	server, metrics := newHedgeEndpointAccountingProxyWithPolicy(t, endpoints, algorithm, proxy.Params{}, nil, roundTrip)
	return server, metrics, endpoints
}

func newHedgeEndpointAccountingProxyWithPolicy(t *testing.T, endpoints []string, algorithm string, proxyParams proxy.Params, policy routing.PostProcessor, roundTrip func(*http.Request, http.RoundTripper) (*http.Response, error)) (*httptest.Server, *trackedLBMetricsPostProcessor) {
	t.Helper()
	routes := eskip.MustParse(fmt.Sprintf("r: Path(\"/hedge\") -> hedge() -> hedgeStateSeed() -> <%s, %q, %q>;", algorithm, endpoints[0], endpoints[1]))
	registry := builtin.MakeRegistry()
	registry.Register(&hedgeStateSeedSpec{sampleCount: 100, eligibleCount: 20, sampleLatency: time.Millisecond})
	dataClient := testdataclient.New(routes)
	endpointMetrics := &trackedLBMetricsPostProcessor{entries: make(map[string]*trackedLBMetrics)}
	postProcessors := []routing.PostProcessor{loadbalancer.NewAlgorithmProvider(), endpointMetrics}
	if policy != nil {
		postProcessors = append(postProcessors, policy)
	}
	routingInstance := routing.New(routing.Options{
		FilterRegistry:  registry,
		DataClients:     []routing.DataClient{dataClient},
		SignalFirstLoad: true,
		PostProcessors:  postProcessors,
	})
	select {
	case <-routingInstance.FirstLoad():
	case <-time.After(2 * time.Second):
		t.Fatal("routing table did not load")
	}
	proxyParams.Routing = routingInstance
	proxyParams.CloseIdleConnsPeriod = -time.Second
	proxyParams.CustomHttpRoundTripperWrap = func(inner http.RoundTripper) http.RoundTripper {
		return scriptedHedgeRoundTripper(func(req *http.Request) (*http.Response, error) {
			return roundTrip(req, inner)
		})
	}
	proxyInstance := proxy.WithParams(proxyParams)
	server := httptest.NewServer(proxyInstance)
	t.Cleanup(dataClient.Close)
	t.Cleanup(routingInstance.Close)
	t.Cleanup(func() { _ = proxyInstance.Close() })
	t.Cleanup(server.Close)
	return server, endpointMetrics
}

func newTracedSeededHedgeTestProxy(t *testing.T, tracer *tracingtest.MockTracer, roundTrip func(*http.Request) (*http.Response, error)) *proxytest.TestProxy {
	t.Helper()
	registry := builtin.MakeRegistry()
	registry.Register(&hedgeStateSeedSpec{sampleCount: 100, eligibleCount: 20, sampleLatency: time.Millisecond})
	route := eskip.MustParse(fmt.Sprintf("r: * -> hedge() -> hedgeStateSeed() -> <roundRobin, %q, %q>;", closedHedgeBackend(t), closedHedgeBackend(t)))
	tp := proxytest.WithParams(registry, proxy.Params{
		OpenTracing: &proxy.OpenTracingParams{Tracer: tracer},
		CustomHttpRoundTripperWrap: func(http.RoundTripper) http.RoundTripper {
			return scriptedHedgeRoundTripper(roundTrip)
		},
	}, route[0])
	t.Cleanup(func() { _ = tp.Close() })
	return tp
}

func assertProxySpanCount(t *testing.T, tracer *tracingtest.MockTracer, want int) {
	t.Helper()
	count := 0
	for _, span := range tracer.FinishedSpans() {
		if span.OperationName == "proxy" {
			count++
		}
	}
	if count != want {
		t.Fatalf("finished %d proxy spans, want %d", count, want)
	}
}

func TestHedgeStreamsWinnerAndClosesLateLoserOnce(t *testing.T) {
	tracer := tracingtest.NewTracer()
	var attempts atomic.Int32
	primaryCanceled := make(chan struct{})
	lateLoserReady := make(chan *observedResponseBody, 1)
	winnerReady := make(chan *observedResponseBody, 1)
	releaseLoser := make(chan struct{})
	releaseWinner := make(chan struct{})
	var releaseLoserOnce, releaseWinnerOnce sync.Once
	defer releaseLoserOnce.Do(func() { close(releaseLoser) })
	defer releaseWinnerOnce.Do(func() { close(releaseWinner) })

	testProxy := newTracedSeededHedgeTestProxy(t, tracer, func(req *http.Request) (*http.Response, error) {
		switch attempts.Add(1) {
		case 1:
			<-req.Context().Done()
			close(primaryCanceled)
			<-releaseLoser
			body := newObservedResponseBody("late loser", req.Context(), nil, false)
			lateLoserReady <- body
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
		default:
			body := newObservedResponseBody("streamed winner", req.Context(), releaseWinner, true)
			winnerReady <- body
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
		}
	})

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, testProxy.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Hedge-Seed", "seed")
	responses := make(chan *http.Response, 1)
	go func() {
		response, _ := testProxy.Client().Do(request)
		responses <- response
	}()

	var response *http.Response
	select {
	case response = <-responses:
	case <-time.After(2 * time.Second):
		releaseLoserOnce.Do(func() { close(releaseLoser) })
		releaseWinnerOnce.Do(func() { close(releaseWinner) })
		t.Fatal("winner response waited for the pending loser")
	}
	if response == nil {
		t.Fatal("proxy request failed")
	}
	var winner *observedResponseBody
	select {
	case winner = <-winnerReady:
	case <-time.After(2 * time.Second):
		t.Fatal("winning attempt body was not returned")
	}
	select {
	case <-primaryCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("losing attempt was not canceled")
	}
	select {
	case <-winner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("winning body did not begin streaming")
	}
	if got := winner.closeCalls.Load(); got != 0 {
		t.Fatalf("winning body closed before streaming completed: %d", got)
	}

	releaseLoserOnce.Do(func() { close(releaseLoser) })
	var loser *observedResponseBody
	select {
	case loser = <-lateLoserReady:
	case <-time.After(2 * time.Second):
		t.Fatal("late loser response was not returned")
	}
	select {
	case <-loser.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("late losing response body was not closed")
	}
	if got := loser.closeCalls.Load(); got != 1 {
		t.Fatalf("late losing body closed %d times, want 1", got)
	}
	if got := loser.readCalls.Load(); got != 0 {
		t.Fatalf("late losing body was read %d times, want 0", got)
	}

	releaseWinnerOnce.Do(func() { close(releaseWinner) })
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "streamed winner" {
		t.Fatalf("winning body = %q, want %q", body, "streamed winner")
	}
	select {
	case <-winner.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("winning body was not closed after streaming")
	}
	if got := winner.closeCalls.Load(); got != 1 {
		t.Fatalf("winning body closed %d times, want 1", got)
	}
	assertProxySpanCount(t, tracer, 2)
}

func TestHedgeClosesSimultaneousLoserResponseOnce(t *testing.T) {
	tracer := tracingtest.NewTracer()
	var attempts atomic.Int32
	ready := make(chan *observedResponseBody, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })

	testProxy := newTracedSeededHedgeTestProxy(t, tracer, func(req *http.Request) (*http.Response, error) {
		attempt := attempts.Add(1)
		body := newObservedResponseBody(fmt.Sprintf("attempt-%d", attempt), req.Context(), nil, false)
		ready <- body
		<-release
		status := http.StatusOK
		if attempt == 2 {
			status = http.StatusAccepted
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: body, Request: req}, nil
	})

	request, err := http.NewRequest(http.MethodGet, testProxy.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Hedge-Seed", "seed")
	responses := make(chan *http.Response, 1)
	go func() {
		response, _ := testProxy.Client().Do(request)
		responses <- response
	}()
	bodies := make([]*observedResponseBody, 2)
	for i := range bodies {
		select {
		case bodies[i] = <-ready:
		case <-time.After(2 * time.Second):
			t.Fatal("both simultaneous attempts did not start")
		}
	}
	releaseOnce.Do(func() { close(release) })
	var response *http.Response
	select {
	case response = <-responses:
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not return a simultaneous response winner")
	}
	if response == nil {
		t.Fatal("proxy request failed")
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(bodies[0].data) && string(body) != string(bodies[1].data) {
		t.Fatalf("response body = %q, want one of the attempt bodies", body)
	}
	for _, attemptBody := range bodies {
		select {
		case <-attemptBody.closed:
		case <-time.After(2 * time.Second):
			t.Fatal("attempt response body was not closed")
		}
		if got := attemptBody.closeCalls.Load(); got != 1 {
			t.Fatalf("attempt response body closed %d times, want 1", got)
		}
	}
	if (bodies[0].readCalls.Load() == 0) == (bodies[1].readCalls.Load() == 0) {
		t.Fatal("expected exactly one response body to be streamed")
	}
	assertProxySpanCount(t, tracer, 2)
}

func TestHedgeClientCancellationStopsActiveAttempts(t *testing.T) {
	tracer := tracingtest.NewTracer()
	var attempts atomic.Int32
	secondaryStarted := make(chan struct{})
	canceled := make(chan int32, 2)
	testProxy := newTracedSeededHedgeTestProxy(t, tracer, func(req *http.Request) (*http.Response, error) {
		attempt := attempts.Add(1)
		if attempt == 2 {
			close(secondaryStarted)
		}
		<-req.Context().Done()
		canceled <- attempt
		return nil, req.Context().Err()
	})

	requestContext, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, testProxy.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Hedge-Seed", "seed")
	results := make(chan error, 1)
	go func() {
		response, err := testProxy.Client().Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		results <- err
	}()
	select {
	case <-secondaryStarted:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("second attempt did not start")
	}
	cancel()
	for range 2 {
		select {
		case <-canceled:
		case <-time.After(2 * time.Second):
			t.Fatal("active attempt did not observe caller cancellation")
		}
	}
	select {
	case err := <-results:
		if err == nil {
			t.Fatal("client request succeeded after caller cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client request did not stop after cancellation")
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("caller cancellation started %d attempts, want 2", got)
	}
	assertProxySpanCount(t, tracer, 2)
}

func TestHedgeEarlyClientClosureCancelsWinningStream(t *testing.T) {
	tracer := tracingtest.NewTracer()
	var attempts atomic.Int32
	primaryCanceled := make(chan struct{})
	winnerReady := make(chan *observedResponseBody, 1)
	testProxy := newTracedSeededHedgeTestProxy(t, tracer, func(req *http.Request) (*http.Response, error) {
		switch attempts.Add(1) {
		case 1:
			<-req.Context().Done()
			close(primaryCanceled)
			return nil, req.Context().Err()
		default:
			body := newObservedResponseBody("stream cancelled", req.Context(), nil, true)
			winnerReady <- body
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
		}
	})

	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, testProxy.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Hedge-Seed", "seed")
	response, err := testProxy.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var winner *observedResponseBody
	select {
	case winner = <-winnerReady:
	case <-time.After(2 * time.Second):
		t.Fatal("winning attempt body was not returned")
	}
	select {
	case <-winner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("winning response body did not start streaming")
	}
	_ = response.Body.Close()
	select {
	case <-winner.canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("early client closure did not cancel the winning backend stream")
	}
	cancel()
	select {
	case <-winner.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("winning response body was not closed after client cancellation")
	}
	select {
	case <-primaryCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("losing attempt was not canceled")
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("early client closure made %d attempts, want 2", got)
	}
	if got := winner.closeCalls.Load(); got != 1 {
		t.Fatalf("winning body closed %d times, want 1", got)
	}
	assertProxySpanCount(t, tracer, 2)
}

func TestHedgeUpgradeRemainsBidirectional(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Connection", "Upgrade")
		w.Header().Set("Upgrade", "websocket")
		w.WriteHeader(http.StatusSwitchingProtocols)
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("backend writer does not support connection upgrades")
			return
		}
		conn, buffered, err := hijacker.Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		line, err := buffered.ReadString('\n')
		if err != nil {
			t.Error(err)
			return
		}
		if line != "ping\n" {
			t.Errorf("backend received %q, want ping", line)
			return
		}
		_, _ = buffered.WriteString("pong\n")
		_ = buffered.Flush()
	}))
	defer backend.Close()

	route := eskip.MustParse(fmt.Sprintf("r: * -> hedge() -> <roundRobin, %q>;", backend.URL))
	testProxy := proxytest.WithParams(builtin.MakeRegistry(), proxy.Params{ExperimentalUpgrade: true}, route[0])
	t.Cleanup(func() { _ = testProxy.Close() })
	proxyURL, err := url.Parse(testProxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", proxyURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	requestURL := &url.URL{Scheme: "http", Host: proxyURL.Host, Path: "/ws"}
	request := &http.Request{
		Method: http.MethodGet,
		URL:    requestURL,
		Header: http.Header{"Connection": {"Upgrade"}, "Upgrade": {"websocket"}},
	}
	if err := request.Write(conn); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade response status = %d, want %d", response.StatusCode, http.StatusSwitchingProtocols)
	}
	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	pong, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if pong != "pong\n" {
		t.Fatalf("upgrade response = %q, want pong", pong)
	}
}

func TestHedgeIntentionalLoserCancellationDoesNotMarkHealthFailure(t *testing.T) {
	var attempts atomic.Int32
	primaryHost := make(chan string, 1)
	winnerHost := make(chan string, 1)
	primaryCanceled := make(chan struct{})
	server, endpointMetrics, _ := newHedgeEndpointAccountingProxy(t, func(req *http.Request, _ http.RoundTripper) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			primaryHost <- req.URL.Host
			<-req.Context().Done()
			close(primaryCanceled)
			return nil, req.Context().Err()
		}
		winnerHost <- req.URL.Host
		return hedgeTestResponse(req, http.StatusOK, "winner"), nil
	})

	req, err := http.NewRequest(http.MethodGet, server.URL+"/hedge", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Hedge-Seed", "seed")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	<-primaryCanceled
	failedHost, winner := <-primaryHost, <-winnerHost
	if failedHost == winner {
		t.Fatalf("hedged attempts used the same endpoint %q", failedHost)
	}

	failedMetrics, winnerMetrics := endpointMetrics.forHost(failedHost), endpointMetrics.forHost(winner)
	waitForEndpointRequests(t, failedMetrics, 1)
	waitForEndpointRequests(t, winnerMetrics, 1)
	if got := failedMetrics.requests.Load(); got != 1 {
		t.Fatalf("losing endpoint recorded %d attempts, want 1", got)
	}
	if got := winnerMetrics.requests.Load(); got != 1 {
		t.Fatalf("winning endpoint recorded %d attempts, want 1", got)
	}
	if got := failedMetrics.failures.Load(); got != 0 {
		t.Fatalf("intentional loser cancellation recorded %d backend health failures, want 0", got)
	}
	if got := failedMetrics.HealthCheckDropProbability(); got != 0 {
		t.Fatalf("intentional loser cancellation raised endpoint drop probability to %v, want 0", got)
	}
	if got := winnerMetrics.failures.Load(); got != 0 {
		t.Fatalf("winning HTTP response recorded %d backend health failures, want 0", got)
	}
	if got := failedMetrics.InflightRequests(); got != 0 {
		t.Fatalf("losing endpoint has %d inflight requests, want 0", got)
	}
	if got := winnerMetrics.InflightRequests(); got != 0 {
		t.Fatalf("winning endpoint has %d inflight requests, want 0", got)
	}
}

func TestGenuineDialFailureRemainsBackendHealthFailure(t *testing.T) {
	var attempts atomic.Int32
	failedHost := make(chan string, 1)
	winnerHost := make(chan string, 1)
	server, endpointMetrics, _ := newHedgeEndpointAccountingProxy(t, func(req *http.Request, inner http.RoundTripper) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			failedHost <- req.URL.Host
			return inner.RoundTrip(req)
		}
		winnerHost <- req.URL.Host
		return hedgeTestResponse(req, http.StatusOK, "winner"), nil
	})

	req, err := http.NewRequest(http.MethodGet, server.URL+"/hedge", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Hedge-Seed", "seed")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	failed, winner := <-failedHost, <-winnerHost
	if failed == winner {
		t.Fatalf("retry used the same endpoint %q", failed)
	}

	failedMetrics, winnerMetrics := endpointMetrics.forHost(failed), endpointMetrics.forHost(winner)
	waitForEndpointRequests(t, failedMetrics, 1)
	waitForEndpointRequests(t, winnerMetrics, 1)
	if got := failedMetrics.requests.Load(); got != 1 {
		t.Fatalf("failed endpoint recorded %d attempts, want 1", got)
	}
	if got := winnerMetrics.requests.Load(); got != 1 {
		t.Fatalf("winning endpoint recorded %d attempts, want 1", got)
	}
	if got := failedMetrics.failures.Load(); got != 1 {
		t.Fatalf("genuine dial failure recorded %d backend health failures, want 1", got)
	}
	if got := failedMetrics.HealthCheckDropProbability(); got != 1 {
		t.Fatalf("genuine dial failure produced endpoint drop probability %v, want 1", got)
	}
	if got := winnerMetrics.failures.Load(); got != 0 {
		t.Fatalf("winning HTTP response recorded %d backend health failures, want 0", got)
	}
	if got := failedMetrics.InflightRequests(); got != 0 {
		t.Fatalf("failed endpoint has %d inflight requests, want 0", got)
	}
	if got := winnerMetrics.InflightRequests(); got != 0 {
		t.Fatalf("winning endpoint has %d inflight requests, want 0", got)
	}
}

func TestHedgeEndpointEligibilityFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name          string
		policyKind    string
		passiveHealth bool
		wantAttempts  int32
		wantStatus    int
	}{
		{name: "fade-in fallback keeps alternative endpoint", policyKind: "fade-in-fallback", wantAttempts: 2, wantStatus: http.StatusOK},
		{name: "fade-in does not bypass a filtered alternative", policyKind: "fade-in-filtered", wantAttempts: 1, wantStatus: http.StatusBadGateway},
		{name: "passive-health fallback keeps alternative endpoint", policyKind: "passive-health-fallback", passiveHealth: true, wantAttempts: 2, wantStatus: http.StatusOK},
		{name: "passive-health does not bypass a filtered alternative", policyKind: "passive-health-filtered", passiveHealth: true, wantAttempts: 1, wantStatus: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoints := []string{closedHedgeBackend(t), closedHedgeBackend(t)}
			hosts := []string{mustEndpointHost(t, endpoints[0]), mustEndpointHost(t, endpoints[1])}
			policy := &hedgeEndpointPolicyPostProcessor{}
			switch tc.policyKind {
			case "fade-in-fallback":
				policy.fadeInDuration, policy.fadeInExponent = time.Hour, 1000
				policy.detectedAges = map[string]time.Duration{hosts[0]: time.Millisecond, hosts[1]: time.Millisecond}
			case "fade-in-filtered":
				policy.fadeInDuration, policy.fadeInExponent = time.Hour, 1000
				policy.detectedAges = map[string]time.Duration{hosts[0]: 2 * time.Hour, hosts[1]: time.Millisecond}
			case "passive-health-fallback":
				policy.dropProb = map[string]float64{hosts[0]: 1, hosts[1]: 1}
			case "passive-health-filtered":
				policy.dropProb = map[string]float64{hosts[1]: 1}
			}
			var attempts atomic.Int32
			primaryHost := make(chan string, 1)
			winnerHost := make(chan string, 1)
			releasePrimary := make(chan struct{})
			var releasePrimaryOnce sync.Once
			releasePrimaryRequest := func() { releasePrimaryOnce.Do(func() { close(releasePrimary) }) }
			server, endpointMetrics := newHedgeEndpointAccountingProxyWithPolicy(t, endpoints, "roundRobin", proxy.Params{
				EnablePassiveHealthCheck: tc.passiveHealth,
				PassiveHealthCheck:       &proxy.PassiveHealthCheck{MaxUnhealthyEndpointsRatio: 1},
			}, policy, func(req *http.Request, inner http.RoundTripper) (*http.Response, error) {
				if attempts.Add(1) == 1 {
					primaryHost <- req.URL.Host
					if tc.policyKind == "passive-health-filtered" {
						select {
						case <-req.Context().Done():
							return nil, req.Context().Err()
						case <-releasePrimary:
							return hedgeTestResponse(req, http.StatusOK, "primary"), nil
						}
					}
					if tc.wantAttempts == 1 {
						return inner.RoundTrip(req)
					}
					<-req.Context().Done()
					return nil, req.Context().Err()
				}
				winnerHost <- req.URL.Host
				return hedgeTestResponse(req, http.StatusOK, "winner"), nil
			})
			for host, wantDrop := range policy.dropProb {
				if gotDrop := endpointMetrics.forHost(host).HealthCheckDropProbability(); gotDrop != wantDrop {
					t.Fatalf("endpoint %q drop probability = %v, want %v", host, gotDrop, wantDrop)
				}
			}

			req, err := http.NewRequest(http.MethodGet, server.URL+"/hedge", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-Hedge-Seed", "seed")
			type responseResult struct {
				response *http.Response
				err      error
			}
			responses := make(chan responseResult, 1)
			go func() {
				response, err := server.Client().Do(req)
				responses <- responseResult{response: response, err: err}
			}()
			var primary string
			if tc.policyKind == "passive-health-filtered" {
				primary = <-primaryHost
				timer := time.NewTimer(20 * time.Millisecond)
				<-timer.C
				timer.Stop()
				releasePrimaryRequest()
			}
			var response *http.Response
			select {
			case result := <-responses:
				if result.err != nil {
					t.Fatal(result.err)
				}
				response = result.response
			case <-time.After(2 * time.Second):
				releasePrimaryRequest()
				t.Fatal("hedge request did not complete")
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if primary == "" {
				primary = <-primaryHost
			}
			primaryEligible := false
			for _, host := range hosts {
				primaryEligible = primaryEligible || host == primary
			}
			if !primaryEligible {
				t.Fatalf("primary selected unexpected endpoint %q", primary)
			}
			if got := attempts.Load(); got != tc.wantAttempts {
				t.Fatalf("made %d attempts, want %d; endpoint drops=%v/%v", got, tc.wantAttempts, endpointMetrics.forHost(hosts[0]).HealthCheckDropProbability(), endpointMetrics.forHost(hosts[1]).HealthCheckDropProbability())
			}
			if tc.wantAttempts == 1 {
				if response.StatusCode != tc.wantStatus {
					t.Fatalf("response status = %d, want %d", response.StatusCode, tc.wantStatus)
				}
				return
			}
			winner := <-winnerHost
			if winner == primary {
				t.Fatalf("fallback reintroduced primary endpoint %q", primary)
			}
			if response.StatusCode != tc.wantStatus {
				t.Fatalf("response status = %d, want %d", response.StatusCode, tc.wantStatus)
			}
		})
	}
}

func mustEndpointHost(t *testing.T, address string) string {
	t.Helper()
	u, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func TestHedgeWinnerMetadataAndTracingModes(t *testing.T) {
	for _, clientTraceByTag := range []bool{false, true} {
		t.Run(fmt.Sprintf("clientTraceByTag-%v", clientTraceByTag), func(t *testing.T) {
			endpoints := []string{closedHedgeBackend(t), closedHedgeBackend(t)}
			hosts := []string{mustEndpointHost(t, endpoints[0]), mustEndpointHost(t, endpoints[1])}
			zones := map[string]string{hosts[0]: "zone-a", hosts[1]: "zone-b"}
			policy := &hedgeEndpointPolicyPostProcessor{zones: zones}
			var attempts atomic.Int32
			primaryHost := make(chan string, 1)
			winnerHost := make(chan string, 1)
			tracer := tracingtest.NewTracer()
			mockMetrics := &metricstest.MockMetrics{}
			server, endpointMetrics := newHedgeEndpointAccountingProxyWithPolicy(t, endpoints, "roundRobin", proxy.Params{
				Metrics: mockMetrics,
				OpenTracing: &proxy.OpenTracingParams{
					Tracer:           tracer,
					ClientTraceByTag: clientTraceByTag,
				},
			}, policy, func(req *http.Request, _ http.RoundTripper) (*http.Response, error) {
				if attempts.Add(1) == 1 {
					primaryHost <- req.URL.Host
					<-req.Context().Done()
					return nil, req.Context().Err()
				}
				winnerHost <- req.URL.Host
				return hedgeTestResponse(req, http.StatusServiceUnavailable, "winner"), nil
			})
			defer mockMetrics.Close()

			req, err := http.NewRequest(http.MethodGet, server.URL+"/hedge", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-Hedge-Seed", "seed")
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("winning HTTP status = %d, want %d", response.StatusCode, http.StatusServiceUnavailable)
			}
			failedHost, winningHost := <-primaryHost, <-winnerHost
			if failedHost == winningHost {
				t.Fatalf("attempts used the same endpoint %q", failedHost)
			}
			failedMetrics, winningMetrics := endpointMetrics.forHost(failedHost), endpointMetrics.forHost(winningHost)
			waitForEndpointRequests(t, failedMetrics, 1)
			waitForEndpointRequests(t, winningMetrics, 1)
			if failedMetrics.failures.Load() != 0 || winningMetrics.failures.Load() != 0 {
				t.Fatalf("HTTP response or intentional cancellation counted as health failure: primary=%d winner=%d", failedMetrics.failures.Load(), winningMetrics.failures.Load())
			}
			if failedMetrics.InflightRequests() != 0 || winningMetrics.InflightRequests() != 0 {
				t.Fatalf("attempt inflight accounting did not return to zero")
			}
			mockMetrics.WithCounters(func(counters map[string]int64) {
				if got := counters["incoming.HTTP/1.1"]; got != 1 {
					t.Errorf("incoming request count = %d, want 1", got)
				}
			})
			winnerZone, loserZone := zones[winningHost], zones[failedHost]
			mockMetrics.WithMeasures(func(measures map[string][]time.Duration) {
				if len(measures["backendzone."+winnerZone]) != 1 {
					t.Errorf("winning backend zone %q measurements = %d, want 1", winnerZone, len(measures["backendzone."+winnerZone]))
				}
				if len(measures["backendzone."+loserZone]) != 0 {
					t.Errorf("losing backend zone %q measurements = %d, want 0", loserZone, len(measures["backendzone."+loserZone]))
				}
			})
			assertProxySpanCount(t, tracer, 2)
		})
	}
}

func TestHedgeAdditionalAttemptUsesEachBuiltInLBAlgorithm(t *testing.T) {
	for _, algorithm := range []string{"roundRobin", "random", "consistentHash", "powerOfRandomNChoices", "weightedRoundRobin", "leastRequests"} {
		t.Run(algorithm, func(t *testing.T) {
			var attempts atomic.Int32
			primaryHost := make(chan string, 1)
			winnerHost := make(chan string, 1)
			primaryCanceled := make(chan struct{})
			endpoints := []string{closedHedgeBackend(t), closedHedgeBackend(t), closedHedgeBackend(t)}
			server, endpointMetrics := newHedgeEndpointAccountingProxyWithPolicy(t, endpoints, algorithm, proxy.Params{}, nil, func(req *http.Request, _ http.RoundTripper) (*http.Response, error) {
				if attempts.Add(1) == 1 {
					primaryHost <- req.URL.Host
					<-req.Context().Done()
					close(primaryCanceled)
					return nil, req.Context().Err()
				}
				winnerHost <- req.URL.Host
				return hedgeTestResponse(req, http.StatusOK, "winner"), nil
			})

			req, err := http.NewRequest(http.MethodGet, server.URL+"/hedge", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-Hedge-Seed", "seed")
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			<-primaryCanceled
			primary, winner := <-primaryHost, <-winnerHost
			if primary == winner {
				t.Fatalf("%s selected the primary endpoint again", algorithm)
			}
			eligibleWinner := false
			for _, endpoint := range endpoints {
				eligibleWinner = eligibleWinner || winner == mustEndpointHost(t, endpoint)
			}
			if !eligibleWinner {
				t.Fatalf("%s selected ineligible endpoint %q", algorithm, winner)
			}
			primaryMetrics, winnerMetrics := endpointMetrics.forHost(primary), endpointMetrics.forHost(winner)
			waitForEndpointRequests(t, primaryMetrics, 1)
			waitForEndpointRequests(t, winnerMetrics, 1)
			if got := primaryMetrics.InflightRequests(); got != 0 {
				t.Fatalf("primary endpoint has %d inflight attempts", got)
			}
			if got := winnerMetrics.InflightRequests(); got != 0 {
				t.Fatalf("winning endpoint has %d inflight attempts", got)
			}
			if got := attempts.Load(); got != 2 {
				t.Fatalf("%s made %d attempts, want 2", algorithm, got)
			}
		})
	}
}

func TestHedgeFirstHTTPResponseWins(t *testing.T) {
	var attempts atomic.Int32
	testProxy := newSeededHedgeTestProxy(t, 100, 20, 10*time.Second, "", func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return hedgeTestResponse(req, http.StatusServiceUnavailable, "primary failure"), nil
	})
	req, err := http.NewRequest(http.MethodGet, testProxy.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Hedge-Seed", "seed")
	response, err := testProxy.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusServiceUnavailable || string(body) != "primary failure" {
		t.Fatalf("response = %d %q, want 503 %q", response.StatusCode, body, "primary failure")
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("fast first HTTP response made %d attempts, want 1", got)
	}
}

func TestHedgeTransportErrorDoesNotDefeatPendingResponse(t *testing.T) {
	var attempts atomic.Int32
	secondFailed := make(chan struct{})
	releasePrimary := make(chan struct{})
	testProxy := newSeededHedgeTestProxy(t, 100, 20, time.Millisecond, "", func(req *http.Request) (*http.Response, error) {
		switch attempts.Add(1) {
		case 1:
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-releasePrimary:
				return hedgeTestResponse(req, http.StatusAccepted, "primary"), nil
			}
		case 2:
			close(secondFailed)
			return nil, fmt.Errorf("secondary transport error")
		default:
			return hedgeTestResponse(req, http.StatusInternalServerError, "unexpected attempt"), nil
		}
	})
	client := testProxy.Client()
	req, err := http.NewRequest(http.MethodGet, testProxy.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Hedge-Seed", "seed")
	responses := make(chan *http.Response, 1)
	go func() {
		response, _ := client.Do(req)
		responses <- response
	}()

	select {
	case <-secondFailed:
	case <-time.After(2 * time.Second):
		close(releasePrimary)
		if response := <-responses; response != nil {
			_ = response.Body.Close()
		}
		t.Fatal("secondary attempt did not start")
	}
	close(releasePrimary)
	var response *http.Response
	select {
	case response = <-responses:
	case <-time.After(2 * time.Second):
		t.Fatal("primary response did not win after the secondary transport error")
	}
	if response == nil {
		t.Fatal("proxy request failed")
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusAccepted || string(body) != "primary" {
		t.Fatalf("response = %d %q, want 202 %q", response.StatusCode, body, "primary")
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("made %d backend attempts, want at most 2", got)
	}
}

func TestHedgeDoesNotStartAttemptAfterBackendDeadline(t *testing.T) {
	var attempts atomic.Int32
	testProxy := newSeededHedgeTestProxy(t, 100, 20, 100*time.Millisecond, "25ms", func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	req, err := http.NewRequest(http.MethodGet, testProxy.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Hedge-Seed", "seed")
	response, err := testProxy.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("response status = %d, want %d", response.StatusCode, http.StatusGatewayTimeout)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("deadline case made %d attempts, want 1", got)
	}
}

func TestHedgeRecordsWinningResponseHeaders(t *testing.T) {
	observations := make(chan hedgeObservation, 100)
	var attempts atomic.Int32
	registry := builtin.MakeRegistry()
	registry.Register(&hedgeObservationSpec{observations: observations})
	route := eskip.MustParse(fmt.Sprintf("r: * -> hedge() -> hedgeObservation() -> <roundRobin, %q, %q>;", closedHedgeBackend(t), closedHedgeBackend(t)))
	testProxy := proxytest.WithParams(registry, proxy.Params{
		CustomHttpRoundTripperWrap: func(http.RoundTripper) http.RoundTripper {
			return scriptedHedgeRoundTripper(func(req *http.Request) (*http.Response, error) {
				attempts.Add(1)
				return hedgeTestResponse(req, http.StatusOK, "sampled"), nil
			})
		},
	}, route[0])
	t.Cleanup(func() { _ = testProxy.Close() })
	client := testProxy.Client()

	for request := 1; request <= 100; request++ {
		req, err := http.NewRequest(http.MethodGet, testProxy.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		observation := <-observations
		wantReady := request == 100
		if observation.ready != wantReady {
			t.Fatalf("sample %d: hedge delay ready = %v, want %v", request, observation.ready, wantReady)
		}
		if wantReady && observation.delay <= 0 {
			t.Fatalf("observed p95 = %v, want positive latency", observation.delay)
		}
	}
	if got := attempts.Load(); got != 100 {
		t.Fatalf("warmup generated %d backend attempts, want 100", got)
	}
}

func TestHedgeStartsSecondAttemptAfterObservedP95(t *testing.T) {
	var mu sync.Mutex
	raceAttempt := 0
	var destinations [2]string
	secondStarted := make(chan struct{})
	primaryCanceled := make(chan struct{})
	releasePrimary := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releasePrimary) }) }

	proxy := newScriptedHedgeTestProxy(t, func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("X-Hedge-Test") != "race" {
			return hedgeTestResponse(req, http.StatusOK, "warm"), nil
		}
		mu.Lock()
		raceAttempt++
		attempt := raceAttempt
		if attempt <= len(destinations) {
			destinations[attempt-1] = req.URL.Host
		}
		mu.Unlock()
		if attempt == 1 {
			select {
			case <-req.Context().Done():
				close(primaryCanceled)
				return nil, req.Context().Err()
			case <-releasePrimary:
				return hedgeTestResponse(req, http.StatusOK, "primary"), nil
			}
		}
		close(secondStarted)
		return hedgeTestResponse(req, http.StatusCreated, "hedged"), nil
	})
	defer release()
	client := proxy.Client()
	for request := 0; request < 100; request++ {
		req, err := http.NewRequest(http.MethodGet, proxy.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("warmup response status = %d, want %d", response.StatusCode, http.StatusOK)
		}
	}

	result := make(chan *http.Response, 1)
	request, err := http.NewRequest(http.MethodGet, proxy.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Hedge-Test", "race")
	go func() {
		response, _ := client.Do(request)
		result <- response
	}()

	select {
	case <-secondStarted:
	case <-time.After(3 * time.Second):
		release()
		if response := <-result; response != nil {
			_ = response.Body.Close()
		}
		t.Fatal("delayed attempt did not start after the observed p95")
	}

	var response *http.Response
	select {
	case response = <-result:
	case <-time.After(3 * time.Second):
		release()
		t.Fatal("proxy did not return the delayed attempt response")
	}
	if response == nil {
		t.Fatal("proxy request failed")
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated || string(body) != "hedged" {
		t.Fatalf("winning response = %d %q, want 201 %q", response.StatusCode, body, "hedged")
	}
	mu.Lock()
	sameDestination := destinations[0] == destinations[1]
	mu.Unlock()
	if sameDestination {
		t.Fatal("delayed attempt used the primary endpoint")
	}
	select {
	case <-primaryCanceled:
	case <-time.After(3 * time.Second):
		t.Fatal("losing primary attempt was not canceled")
	}
}

func hedgeTestResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

func newScriptedHedgeTestProxy(t *testing.T, roundTrip func(*http.Request) (*http.Response, error)) *proxytest.TestProxy {
	t.Helper()
	route := eskip.MustParse(fmt.Sprintf("r: * -> hedge() -> <roundRobin, %q, %q>;", closedHedgeBackend(t), closedHedgeBackend(t)))
	tp := proxytest.WithParams(builtin.MakeRegistry(), proxy.Params{
		CustomHttpRoundTripperWrap: func(http.RoundTripper) http.RoundTripper {
			return scriptedHedgeRoundTripper(roundTrip)
		},
	}, route[0])
	t.Cleanup(func() { _ = tp.Close() })
	return tp
}

func TestHedgeRetryBudgetAndAlternativeEndpoint(t *testing.T) {
	recorder := &hedgeAttemptRecorder{}
	proxy := newHedgeTestProxy(t, recorder, []string{closedHedgeBackend(t), closedHedgeBackend(t)}, true)
	client := proxy.Client()

	for request := 1; request <= 40; request++ {
		before := len(recorder.snapshot())
		requestHedgeRoute(t, client, proxy.URL, http.MethodGet, "")
		after := recorder.snapshot()
		attempts := after[before:]
		wantAttempts := 1
		if request%20 == 0 {
			wantAttempts = 2
		}
		if len(attempts) != wantAttempts {
			t.Fatalf("request %d made %d backend attempts, want %d", request, len(attempts), wantAttempts)
		}
		if wantAttempts == 2 && attempts[0] == attempts[1] {
			t.Fatalf("request %d retried the same backend endpoint %q", request, attempts[0])
		}
	}
}

func TestHedgeUnsupportedRequestsKeepExistingRetryBehavior(t *testing.T) {
	recorder := &hedgeAttemptRecorder{}
	proxy := newHedgeTestProxy(t, recorder, []string{closedHedgeBackend(t), closedHedgeBackend(t)}, true)
	client := proxy.Client()

	before := len(recorder.snapshot())
	requestHedgeRoute(t, client, proxy.URL, http.MethodGet, "body")
	if got := len(recorder.snapshot()) - before; got != 1 {
		t.Fatalf("body-bearing GET made %d attempts, want 1", got)
	}

	before = len(recorder.snapshot())
	requestHedgeRoute(t, client, proxy.URL, http.MethodPost, "")
	if got := len(recorder.snapshot()) - before; got != 2 {
		t.Fatalf("bodyless POST made %d attempts, want existing retry behavior of 2", got)
	}

	before = len(recorder.snapshot())
	requestHedgeUpgrade(t, client, proxy.URL)
	if got := len(recorder.snapshot()) - before; got != 2 {
		t.Fatalf("protocol upgrade made %d attempts, want existing retry behavior of 2", got)
	}
}

func TestHedgeRetryBudgetIsPerRoute(t *testing.T) {
	recorder := &hedgeAttemptRecorder{}
	endpoints := []string{closedHedgeBackend(t), closedHedgeBackend(t)}
	quotedEndpoints := []string{strconv.Quote(endpoints[0]), strconv.Quote(endpoints[1])}
	backend := "<roundRobin, " + strings.Join(quotedEndpoints, ", ") + ">"
	routes := eskip.MustParse(fmt.Sprintf("one: Path(\"/one\") -> hedge() -> %s; two: Path(\"/two\") -> hedge() -> %s;", backend, backend))
	testProxy := proxytest.WithParams(builtin.MakeRegistry(), proxy.Params{CustomHttpRoundTripperWrap: recorder.wrap}, routes...)
	t.Cleanup(func() { _ = testProxy.Close() })
	client := testProxy.Client()

	for request := 1; request <= 20; request++ {
		for _, path := range []string{"/one", "/two"} {
			before := len(recorder.snapshot())
			requestHedgeRoute(t, client, testProxy.URL+path, http.MethodGet, "")
			attempts := len(recorder.snapshot()) - before
			want := 1
			if request == 20 {
				want = 2
			}
			if attempts != want {
				t.Fatalf("route %s request %d made %d attempts, want %d", path, request, attempts, want)
			}
		}
	}
}

func TestHedgeBudgetIncludesHeadRequests(t *testing.T) {
	recorder := &hedgeAttemptRecorder{}
	testProxy := newHedgeTestProxy(t, recorder, []string{closedHedgeBackend(t), closedHedgeBackend(t)}, true)
	client := testProxy.Client()

	for request := 1; request <= 20; request++ {
		before := len(recorder.snapshot())
		requestHedgeRoute(t, client, testProxy.URL, http.MethodHead, "")
		want := 1
		if request == 20 {
			want = 2
		}
		if got := len(recorder.snapshot()) - before; got != want {
			t.Fatalf("HEAD request %d made %d attempts, want %d", request, got, want)
		}
	}
}

func TestHedgeRetryBudgetIsConcurrentSafe(t *testing.T) {
	recorder := &hedgeAttemptRecorder{}
	testProxy := newHedgeTestProxy(t, recorder, []string{closedHedgeBackend(t), closedHedgeBackend(t)}, true)
	client := testProxy.Client()
	const requests = 100
	var wait sync.WaitGroup
	errs := make(chan error, requests)
	for request := 0; request < requests; request++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			req, err := http.NewRequest(http.MethodGet, testProxy.URL, nil)
			if err != nil {
				errs <- err
				return
			}
			response, err := client.Do(req)
			if err != nil {
				errs <- err
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusBadGateway {
				errs <- fmt.Errorf("response status = %d, want %d", response.StatusCode, http.StatusBadGateway)
			}
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	got := len(recorder.snapshot())
	if got <= requests || got > requests+requests/20 {
		t.Fatalf("made %d backend attempts for %d eligible requests, want between %d and %d", got, requests, requests+1, requests+requests/20)
	}
}

func TestHedgeRetryBudgetWaitsForAlternativeEndpoint(t *testing.T) {
	recorder := &hedgeAttemptRecorder{}
	proxy := newHedgeTestProxy(t, recorder, []string{closedHedgeBackend(t)}, true)
	client := proxy.Client()

	for request := 0; request < 20; request++ {
		requestHedgeRoute(t, client, proxy.URL, http.MethodGet, "")
	}
	if got := len(recorder.snapshot()); got != 20 {
		t.Fatalf("made %d backend attempts without an alternative endpoint, want 20", got)
	}
}
