package proxy_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	circuitfilters "github.com/zalando/skipper/filters/circuit"
	hedgefilters "github.com/zalando/skipper/filters/hedge"
	"github.com/zalando/skipper/filters/tee"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/predicates/primitive"
	teepred "github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/predicates/traffic"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing/tracingtest"
)

const hedgeDelay = 100 * time.Millisecond

type hedgeBackend struct {
	*httptest.Server
	hits atomic.Int64
}

func newHedgeBackend(name string, slowFor time.Duration, slowStatus int) *hedgeBackend {
	b := &hedgeBackend{}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.hits.Add(1)
		switch r.URL.Path {
		case "/slow":
			select {
			case <-time.After(slowFor):
			case <-r.Context().Done():
				return
			}
			w.WriteHeader(slowStatus)
		case "/err":
			w.WriteHeader(http.StatusInternalServerError)
		}
		io.WriteString(w, name)
	}))
	return b
}

type hedgeSetup struct {
	proxy   *proxytest.TestProxy
	metrics *metricstest.MockMetrics
	base    map[string]int64
}

func newHedgeProxy(t *testing.T, doc string, hp *proxy.HedgeParams) *hedgeSetup {
	t.Helper()

	m := &metricstest.MockMetrics{}
	fr := make(filters.Registry)
	fr.Register(hedgefilters.NewDisableHedge())
	fr.Register(builtin.NewBackendTimeout())
	fr.Register(circuitfilters.NewRateBreaker())
	routes := eskip.MustParse(doc)
	if hp != nil {
		c := *hp
		c.RouteCounters = true
		hp = &c
	}
	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		Metrics:              m,
		Hedge:                hp,
		Flags:                proxy.PreserveHost,
		CircuitBreakers:      circuit.NewRegistry(),
	}, routes...)
	t.Cleanup(func() { p.Close() })

	return &hedgeSetup{proxy: p, metrics: m}
}

func (s *hedgeSetup) get(t *testing.T, path string, headers ...string) (string, int, time.Duration) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, s.proxy.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}

	start := time.Now()
	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()
	b, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b)), rsp.StatusCode, time.Since(start)
}

func (s *hedgeSetup) warmUp(t *testing.T) {
	t.Helper()

	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for range 120 {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			s.get(t, "/")
		})
	}
	wg.Wait()

	s.base = map[string]int64{}
	s.metrics.WithCounters(func(counters map[string]int64) {
		for k, v := range counters {
			if strings.HasPrefix(k, "hedge.") {
				s.base[k] = v
			}
		}
	})
}

func (s *hedgeSetup) counter(route, name string) int64 {
	key := fmt.Sprintf("hedge.%s.%s", route, name)
	v, _ := s.metrics.Counter(key)
	return v - s.base[key]
}

func TestHedgeSlowPrimaryIsHedgedToOtherEndpoint(t *testing.T) {
	slow := newHedgeBackend("slow", time.Second, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, slow.URL, fast.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay})
	s.warmUp(t)

	for range 6 {
		body, code, d := s.get(t, "/slow")
		if body != "fast" || code != http.StatusOK {
			t.Fatalf("got %q %d, want fast 200", body, code)
		}
		if d > 700*time.Millisecond {
			t.Fatalf("response took %v, want the hedge to answer", d)
		}
	}

	if got := s.counter("r", "fired"); got == 0 {
		t.Fatal("no hedge request was sent")
	}
	if fired, won := s.counter("r", "fired"), s.counter("r", "won"); won == 0 || won > fired {
		t.Fatalf("fired=%d won=%d", fired, won)
	}
}

func TestHedgeFastPrimaryIsNotHedged(t *testing.T) {
	a := newHedgeBackend("a", 0, http.StatusOK)
	defer a.Close()
	b := newHedgeBackend("b", 0, http.StatusOK)
	defer b.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, a.URL, b.URL),
		&proxy.HedgeParams{FixedDelay: 2 * time.Second})
	s.warmUp(t)

	for range 10 {
		s.get(t, "/")
	}
	if got := s.counter("r", "fired"); got != 0 {
		t.Fatalf("fired=%d, want 0", got)
	}
}

func TestHedgeColdStartDoesNotHedge(t *testing.T) {
	slow := newHedgeBackend("slow", 300*time.Millisecond, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, slow.URL, fast.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay})

	for range 4 {
		s.get(t, "/slow")
	}
	if got := s.counter("r", "fired"); got != 0 {
		t.Fatalf("fired=%d during cold start, want 0", got)
	}
}

func TestHedgeNotApplied(t *testing.T) {
	for _, tc := range []struct {
		name    string
		route   string
		headers []string
	}{{
		name:  "disableHedge filter",
		route: `r: * -> disableHedge() -> <roundRobin, "%s", "%s">;`,
	}, {
		name:  "consistent hash",
		route: `r: * -> <consistentHash, "%s", "%s">;`,
	}, {
		name:    "request arrived as shadow traffic",
		route:   `r: * -> <roundRobin, "%s", "%s">;`,
		headers: []string{filters.ShadowTrafficHeader, "example.org"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			slow := newHedgeBackend("slow", 200*time.Millisecond, http.StatusOK)
			defer slow.Close()
			fast := newHedgeBackend("fast", 0, http.StatusOK)
			defer fast.Close()

			s := newHedgeProxy(t, fmt.Sprintf(tc.route, slow.URL, fast.URL),
				&proxy.HedgeParams{FixedDelay: hedgeDelay})
			s.warmUp(t)

			for range 6 {
				s.get(t, "/slow", tc.headers...)
			}
			if got := s.counter("r", "fired"); got != 0 {
				t.Fatalf("fired=%d, want 0", got)
			}
		})
	}
}

func TestHedgeNotAppliedToOtherBackendsAndMethods(t *testing.T) {
	slow := newHedgeBackend("slow", 200*time.Millisecond, http.StatusOK)
	defer slow.Close()

	t.Run("network backend", func(t *testing.T) {
		s := newHedgeProxy(t, fmt.Sprintf(`r: * -> "%s";`, slow.URL), &proxy.HedgeParams{FixedDelay: hedgeDelay})
		s.warmUp(t)
		s.get(t, "/slow")
		if got := s.counter("r", "fired") + s.counter("r", "no_alternate"); got != 0 {
			t.Fatalf("hedge counters moved: %d", got)
		}
	})

	t.Run("post", func(t *testing.T) {
		fast := newHedgeBackend("fast", 0, http.StatusOK)
		defer fast.Close()
		s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, slow.URL, fast.URL), &proxy.HedgeParams{FixedDelay: hedgeDelay})
		s.warmUp(t)
		for range 4 {
			rsp, err := http.Post(s.proxy.URL+"/slow", "text/plain", strings.NewReader("x"))
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, rsp.Body)
			rsp.Body.Close()
		}
		if got := s.counter("r", "fired"); got != 0 {
			t.Fatalf("fired=%d for POST, want 0", got)
		}
	})
}

func TestHedgeNoAlternateEndpoint(t *testing.T) {
	slow := newHedgeBackend("slow", 200*time.Millisecond, http.StatusOK)
	defer slow.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s">;`, slow.URL), &proxy.HedgeParams{FixedDelay: hedgeDelay})
	s.warmUp(t)

	body, _, _ := s.get(t, "/slow")
	if body != "slow" {
		t.Fatalf("got %q", body)
	}
	if got := s.counter("r", "no_alternate"); got != 1 {
		t.Fatalf("no_alternate=%d, want 1", got)
	}
	if got := s.counter("r", "fired"); got != 0 {
		t.Fatalf("fired=%d, want 0", got)
	}
}

func TestHedgeServerErrorDoesNotWinWhilePrimaryPending(t *testing.T) {
	slowOK := newHedgeBackend("primary", 300*time.Millisecond, http.StatusOK)
	defer slowOK.Close()
	failing := newHedgeBackend("hedge", 0, http.StatusInternalServerError)
	defer failing.Close()
	failing.Server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, "hedge")
			return
		}
		io.WriteString(w, "hedge")
	})

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, slowOK.URL, failing.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay})
	s.warmUp(t)

	for range 8 {
		body, code, _ := s.get(t, "/slow")
		if code == http.StatusInternalServerError && body == "hedge" {
			continue
		}
		if body != "primary" || code != http.StatusOK {
			t.Fatalf("got %q %d", body, code)
		}
	}
	if got := s.counter("r", "won"); got != 0 {
		t.Fatalf("a 500 hedge response won %d times", got)
	}
}

func TestHedgeThrottledOnErrors(t *testing.T) {
	slow := newHedgeBackend("slow", 200*time.Millisecond, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, slow.URL, fast.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay})
	s.warmUp(t)

	for range 60 {
		s.get(t, "/err")
	}
	for range 4 {
		s.get(t, "/slow")
	}

	if got := s.counter("r", "fired"); got != 0 {
		t.Fatalf("fired=%d while the error ratio is above the threshold", got)
	}
	if got := s.counter("r", "throttled"); got == 0 {
		t.Fatal("throttled counter did not move")
	}
}

func TestHedgeBudgetExhausted(t *testing.T) {
	slow := newHedgeBackend("slow", time.Second, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, slow.URL, fast.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay, BudgetPercent: 1})
	s.warmUp(t)

	var wg sync.WaitGroup
	for range 80 {
		wg.Go(func() { s.get(t, "/slow") })
	}
	wg.Wait()

	fired := s.counter("r", "fired")
	if fired == 0 || fired > 20 {
		t.Fatalf("fired=%d, want a number bounded by the burst", fired)
	}
	if got := s.counter("r", "budget_exhausted"); got == 0 {
		t.Fatal("budget_exhausted counter did not move")
	}
}

func TestHedgeDisabledByDefault(t *testing.T) {
	slow := newHedgeBackend("slow", 100*time.Millisecond, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, slow.URL, fast.URL), nil)
	s.warmUp(t)

	for range 4 {
		s.get(t, "/slow")
	}
	if got := s.counter("r", "fired"); got != 0 {
		t.Fatalf("fired=%d with hedging disabled", got)
	}
}

func TestHedgeTeeLoopbackCopyIsNotHedged(t *testing.T) {
	slow := newHedgeBackend("slow", 300*time.Millisecond, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	m := &metricstest.MockMetrics{}
	fr := make(filters.Registry)
	fr.Register(hedgefilters.NewDisableHedge())
	fr.Register(tee.NewTeeLoopback())
	routes := eskip.MustParse(fmt.Sprintf(`
		main: PathSubtree("/") && Traffic(1)
			-> teeLoopback("shadow")
			-> <roundRobin, "%s", "%s">;
		shadow: PathSubtree("/") && Tee("shadow") && True()
			-> <roundRobin, "%s", "%s">;
	`, slow.URL, fast.URL, slow.URL, fast.URL))

	ro := routing.Options{
		FilterRegistry: fr,
		Predicates:     []routing.PredicateSpec{traffic.New(), teepred.New(), primitive.NewTrue()},
	}
	p := proxytest.WithParamsAndRoutingOptions(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		Metrics:              m,
		Hedge:                &proxy.HedgeParams{FixedDelay: hedgeDelay, RouteCounters: true},
	}, ro, routes...)
	defer p.Close()
	s := &hedgeSetup{proxy: p, metrics: m}

	for range 120 {
		s.get(t, "/")
	}

	for range 4 {
		s.get(t, "/slow")
	}
	time.Sleep(500 * time.Millisecond)

	if got := s.counter("shadow", "fired"); got != 0 {
		t.Fatalf("shadow route fired %d hedges, want 0", got)
	}
}

func TestHedgeSkippedWhenDeadlineIsShorterThanDelay(t *testing.T) {
	slow := newHedgeBackend("slow", 300*time.Millisecond, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> backendTimeout("40ms") -> <roundRobin, "%s", "%s">;`, slow.URL, fast.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay})
	s.warmUp(t)

	for range 4 {
		s.get(t, "/")
	}

	if got := s.counter("r", "skipped_deadline"); got == 0 {
		t.Fatal("skipped_deadline counter did not move")
	}
	if got := s.counter("r", "fired"); got != 0 {
		t.Fatalf("fired=%d although no hedge can finish before the deadline", got)
	}
}

func TestHedgeClientCancelStopsBothAttempts(t *testing.T) {
	var inflight atomic.Int64
	started := make(chan struct{}, 8)
	newBlocking := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/block" {
				return
			}
			inflight.Add(1)
			defer inflight.Add(-1)
			started <- struct{}{}
			<-r.Context().Done()
		}))
	}
	a, b := newBlocking(), newBlocking()
	defer a.Close()
	defer b.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, a.URL, b.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay})
	s.warmUp(t)

	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.proxy.URL+"/block", nil)
		if err != nil {
			t.Fatal(err)
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			if rsp, err := http.DefaultClient.Do(req); err == nil {
				rsp.Body.Close()
			}
		}()

		for range 2 {
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				cancel()
				t.Fatal("the primary and the hedge request did not both reach a backend")
			}
		}
		time.Sleep(20 * time.Millisecond)
		cancel()
		<-done
	}

	deadline := time.Now().Add(2 * time.Second)
	for inflight.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d backend requests still in flight after the client cancelled", inflight.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := s.counter("r", "fired"); got == 0 {
		t.Fatal("no hedge was fired, so the test did not exercise two attempts")
	}
}

func TestHedgeBothAttemptsFail(t *testing.T) {
	newFailing := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/slow" {
				time.Sleep(150 * time.Millisecond)
				w.WriteHeader(http.StatusInternalServerError)
			}
			io.WriteString(w, name)
		}))
	}
	a, b := newFailing("a"), newFailing("b")
	defer a.Close()
	defer b.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, a.URL, b.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay})
	s.warmUp(t)

	for range 4 {
		body, code, _ := s.get(t, "/slow")
		if code != http.StatusInternalServerError {
			t.Fatalf("got %d %q, want the backend 500", code, body)
		}
		if body != "a" && body != "b" {
			t.Fatalf("unexpected body %q", body)
		}
	}
	if got := s.counter("r", "won"); got != 0 {
		t.Fatalf("won=%d although both attempts failed", got)
	}
}

func TestHedgeResponseBodyStreamsAfterWin(t *testing.T) {
	const payload = "0123456789abcdef"
	newStreaming := func(slow bool) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/slow" && slow {
				select {
				case <-time.After(time.Second):
				case <-r.Context().Done():
					return
				}
			}
			w.WriteHeader(http.StatusOK)
			for _, c := range payload {
				io.WriteString(w, string(c))
				w.(http.Flusher).Flush()
				time.Sleep(10 * time.Millisecond)
			}
		}))
	}
	slow, fast := newStreaming(true), newStreaming(false)
	defer slow.Close()
	defer fast.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, slow.URL, fast.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay})
	s.warmUp(t)

	for range 4 {
		body, code, _ := s.get(t, "/slow")
		if code != http.StatusOK || body != payload {
			t.Fatalf("got %d %q, want the full streamed payload", code, body)
		}
	}
	if got := s.counter("r", "won"); got == 0 {
		t.Fatal("the hedge never won, so a hedged body was not streamed")
	}
}

func TestHedgeCircuitBreakerSeesOnlyTheWinner(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x" {
			return
		}
		select {
		case <-time.After(300 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()
	healthy := newHedgeBackend("healthy", 0, http.StatusOK)
	defer healthy.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> rateBreaker(3, 10, "1m") -> <roundRobin, "%s", "%s">;`, failing.URL, healthy.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay})
	s.warmUp(t)

	for i := range 8 {
		body, code, _ := s.get(t, "/x")
		if code != http.StatusOK || body != "healthy" {
			t.Fatalf("request %d: got %d %q fired=%d budget_exhausted=%d", i, code, body, s.counter("r", "fired"), s.counter("r", "budget_exhausted"))
		}
	}
	if got := s.counter("r", "won"); got == 0 {
		t.Fatal("the hedge never won, so the test did not exercise the race")
	}
}

func TestHedgeBackendRequestCount(t *testing.T) {
	slow := newHedgeBackend("slow", time.Second, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, slow.URL, fast.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay})
	s.warmUp(t)

	before := slow.hits.Load() + fast.hits.Load()
	for range 6 {
		s.get(t, "/slow")
	}
	hits := slow.hits.Load() + fast.hits.Load() - before

	fired := s.counter("r", "fired")
	if fired == 0 {
		t.Fatal("no hedge was fired")
	}
	if want := 6 + fired; hits != want {
		t.Fatalf("backends saw %d requests, want %d (6 requests + %d hedges)", hits, want, fired)
	}
}

func TestHedgeCircuitBreakerOpensWithoutHedging(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer failing.Close()
	healthy := newHedgeBackend("healthy", 0, http.StatusOK)
	defer healthy.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> rateBreaker(3, 10, "1m") -> <roundRobin, "%s", "%s">;`, failing.URL, healthy.URL), nil)
	s.warmUp(t)

	opened := false
	for range 8 {
		if _, code, _ := s.get(t, "/x"); code == http.StatusServiceUnavailable {
			opened = true
		}
	}
	if !opened {
		t.Fatal("the breaker never opened, so TestHedgeCircuitBreakerSeesOnlyTheWinner proves nothing")
	}
}

func TestHedgeHeadRequest(t *testing.T) {
	slow := newHedgeBackend("slow", time.Second, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, slow.URL, fast.URL),
		&proxy.HedgeParams{FixedDelay: hedgeDelay})
	s.warmUp(t)

	for range 6 {
		start := time.Now()
		rsp, err := http.Head(s.proxy.URL + "/slow")
		if err != nil {
			t.Fatal(err)
		}
		rsp.Body.Close()
		if rsp.StatusCode != http.StatusOK {
			t.Fatalf("got %d", rsp.StatusCode)
		}
		if d := time.Since(start); d > 700*time.Millisecond {
			t.Fatalf("HEAD took %v, want the hedge to answer", d)
		}
	}
	if got := s.counter("r", "fired"); got == 0 {
		t.Fatal("no hedge was fired for HEAD")
	}
}

func TestHedgeTracing(t *testing.T) {
	slow := newHedgeBackend("slow", time.Second, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	tracer := tracingtest.NewTracer()
	m := &metricstest.MockMetrics{}
	fr := make(filters.Registry)
	routes := eskip.MustParse(fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, slow.URL, fast.URL))
	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		Metrics:              m,
		Hedge:                &proxy.HedgeParams{FixedDelay: hedgeDelay, RouteCounters: true},
		OpenTracing:          &proxy.OpenTracingParams{Tracer: tracer, ExcludeTags: nil},
	}, routes...)
	defer p.Close()
	s := &hedgeSetup{proxy: p, metrics: m}
	s.warmUp(t)

	for range 8 {
		s.get(t, "/slow")
	}

	var hedged, winners int
	for _, sp := range tracer.FinishedSpans() {
		if sp.OperationName != "proxy" {
			continue
		}
		if sp.Tag("hedge") != true {
			continue
		}
		hedged++
		switch sp.Tag("hedge.winner") {
		case "hedge", "primary":
			winners++
		default:
			t.Errorf("unexpected hedge.winner tag %v", sp.Tag("hedge.winner"))
		}
	}
	if hedged == 0 {
		t.Fatal("no proxy span carries the hedge tag")
	}
	if hedged != winners {
		t.Fatalf("%d hedged spans, %d with a winner", hedged, winners)
	}
	if fired, _ := m.Counter("hedge.r.fired"); int64(hedged) > fired {
		t.Fatalf("%d hedged spans but only %d hedges fired", hedged, fired)
	}
}

func TestHedgeDisabledForNonPositiveDelay(t *testing.T) {
	for _, tc := range []struct {
		name  string
		delay time.Duration
	}{
		{"zero", 0},
		{"negative", -time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newHedgeBackend("a", 0, http.StatusOK)
			defer a.Close()
			b := newHedgeBackend("b", 0, http.StatusOK)
			defer b.Close()

			s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, a.URL, b.URL),
				&proxy.HedgeParams{FixedDelay: tc.delay})

			const n = 300
			var wg sync.WaitGroup
			for range n {
				wg.Go(func() { s.get(t, "/") })
			}
			wg.Wait()

			if got := a.hits.Load() + b.hits.Load(); got != n {
				t.Fatalf("backends saw %d requests for %d client requests, want no hedge requests", got, n)
			}
			if fired, _ := s.metrics.Counter("hedge.r.fired"); fired != 0 {
				t.Fatalf("fired=%d, want 0", fired)
			}
		})
	}
}

func newSchemeBackend(name string, slowFor time.Duration, scheme string) (*httptest.Server, *atomic.Int64) {
	hits := &atomic.Int64{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/slow" {
			select {
			case <-time.After(slowFor):
			case <-r.Context().Done():
				return
			}
		}
		io.WriteString(w, name+" "+r.Proto)
	})

	switch scheme {
	case "https":
		return httptest.NewTLSServer(h), hits
	case "h2c":
		srv := httptest.NewUnstartedServer(h)
		srv.Config.Protocols = new(http.Protocols)
		srv.Config.Protocols.SetHTTP1(true)
		srv.Config.Protocols.SetUnencryptedHTTP2(true)
		srv.Start()
		return srv, hits
	default:
		return httptest.NewServer(h), hits
	}
}

func TestHedgeSchemes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scheme string
		flags  proxy.Flags
	}{
		{name: "https endpoints", scheme: "https", flags: proxy.Insecure},
		{name: "h2c endpoints", scheme: "h2c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slow, _ := newSchemeBackend("slow", time.Second, tc.scheme)
			defer slow.Close()
			fast, _ := newSchemeBackend("fast", 0, tc.scheme)
			defer fast.Close()

			url := func(s *httptest.Server) string {
				if tc.scheme == "h2c" {
					return "h2c" + s.URL[len("http"):]
				}
				return s.URL
			}

			m := &metricstest.MockMetrics{}
			fr := make(filters.Registry)
			routes := eskip.MustParse(fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, url(slow), url(fast)))
			p := proxytest.WithParams(fr, proxy.Params{
				CloseIdleConnsPeriod: -time.Second,
				Metrics:              m,
				Flags:                tc.flags,
				Hedge:                &proxy.HedgeParams{FixedDelay: 3 * hedgeDelay, RouteCounters: true},
			}, routes...)
			defer p.Close()
			s := &hedgeSetup{proxy: p, metrics: m}
			s.warmUp(t)

			for range 6 {
				body, code, d := s.get(t, "/slow")
				if code != http.StatusOK || !strings.HasPrefix(body, "fast ") {
					t.Fatalf("got %d %q in %v, want 200 from the fast endpoint; fired=%d won=%d throttled=%d budget_exhausted=%d no_alternate=%d",
						code, body, d, s.counter("r", "fired"), s.counter("r", "won"), s.counter("r", "throttled"),
						s.counter("r", "budget_exhausted"), s.counter("r", "no_alternate"))
				}
				if d > 700*time.Millisecond {
					t.Fatalf("response took %v, want the hedge to answer", d)
				}
			}
			if s.counter("r", "fired") == 0 || s.counter("r", "won") == 0 {
				t.Fatalf("fired=%d won=%d", s.counter("r", "fired"), s.counter("r", "won"))
			}
		})
	}
}

func TestHedgeShadowRequestIsNotHedgedAfterLoopback(t *testing.T) {
	slow := newHedgeBackend("slow", 300*time.Millisecond, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	m := &metricstest.MockMetrics{}
	fr := make(filters.Registry)
	fr.Register(builtin.NewSetRequestHeader())
	routes := eskip.MustParse(fmt.Sprintf(`
		entry: *
			-> setRequestHeader("X-Loop-Route", "1")
			-> <loopback>;
		looped: Header("X-Loop-Route", "1")
			-> <roundRobin, "%s", "%s">;
	`, slow.URL, fast.URL))

	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		Metrics:              m,
		Hedge:                &proxy.HedgeParams{FixedDelay: hedgeDelay, RouteCounters: true},
	}, routes...)
	defer p.Close()
	s := &hedgeSetup{proxy: p, metrics: m}

	for range 120 {
		if _, code, _ := s.get(t, "/"); code != http.StatusOK {
			t.Fatalf("warm-up request got %d, the loopback route does not reach a backend", code)
		}
	}
	s.base = map[string]int64{}

	for range 4 {
		if _, code, _ := s.get(t, "/slow", filters.ShadowTrafficHeader, "example.org"); code != http.StatusOK {
			t.Fatalf("got %d", code)
		}
	}

	if got := s.counter("looped", "fired"); got != 0 {
		t.Fatalf("a request that arrived with the shadow header and looped back fired %d hedges, want 0", got)
	}
}

func TestHedgeAfterLoopbackWithoutShadowHeader(t *testing.T) {
	slow := newHedgeBackend("slow", time.Second, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	m := &metricstest.MockMetrics{}
	fr := make(filters.Registry)
	fr.Register(builtin.NewSetRequestHeader())
	routes := eskip.MustParse(fmt.Sprintf(`
		entry: *
			-> setRequestHeader("X-Loop-Route", "1")
			-> <loopback>;
		looped: Header("X-Loop-Route", "1")
			-> <roundRobin, "%s", "%s">;
	`, slow.URL, fast.URL))

	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		Metrics:              m,
		Hedge:                &proxy.HedgeParams{FixedDelay: hedgeDelay, RouteCounters: true},
	}, routes...)
	defer p.Close()
	s := &hedgeSetup{proxy: p, metrics: m}
	s.warmUp(t)

	for range 6 {
		s.get(t, "/slow")
	}
	if got := s.counter("looped", "fired"); got == 0 {
		t.Fatal("a route reached through <loopback> is never hedged, so the previous test proves nothing")
	}
}

func newLatencyBackend(name string, d, stall time.Duration) *hedgeBackend {
	b := &hedgeBackend{}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.hits.Add(1)
		wait := d
		if r.URL.Path == "/stall" && stall > 0 {
			wait = stall
		}
		select {
		case <-time.After(wait):
		case <-r.Context().Done():
			return
		}
		io.WriteString(w, name)
	}))
	return b
}

func TestHedgeAdaptiveDelayFollowsObservedLatency(t *testing.T) {
	const typical = 60 * time.Millisecond
	stalling := newLatencyBackend("stalling", typical, 3*time.Second)
	defer stalling.Close()
	healthy := newLatencyBackend("healthy", typical, 0)
	defer healthy.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, stalling.URL, healthy.URL),
		&proxy.HedgeParams{FixedDelay: 5 * time.Second, Percentile: 0.9, MinDelay: 10 * time.Millisecond})

	var wg sync.WaitGroup
	for range 300 {
		wg.Go(func() { s.get(t, "/") })
	}
	wg.Wait()
	s.warmUp(t)

	for range 8 {
		body, code, d := s.get(t, "/stall")
		if code != http.StatusOK || body != "healthy" {
			t.Fatalf("got %d %q, want the healthy backend to answer", code, body)
		}
		if d > 1500*time.Millisecond {
			t.Fatalf("a stalled request took %v; the adaptive delay should have hedged long before the 5s fixed delay", d)
		}
	}
	if got := s.counter("r", "fired"); got == 0 {
		t.Fatal("no hedge was fired")
	}
}

func TestHedgeAdaptiveDelayNeedsWarmUp(t *testing.T) {
	stalling := newLatencyBackend("stalling", 60*time.Millisecond, 400*time.Millisecond)
	defer stalling.Close()
	healthy := newLatencyBackend("healthy", 60*time.Millisecond, 0)
	defer healthy.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, stalling.URL, healthy.URL),
		&proxy.HedgeParams{FixedDelay: 100 * time.Millisecond, Percentile: 0.9})

	for range 6 {
		s.get(t, "/stall")
	}
	if got := s.counter("r", "fired"); got != 0 {
		t.Fatalf("fired=%d before the route had enough samples; the cold start must not hedge", got)
	}
}

func TestHedgeAdaptiveWithoutFixedDelay(t *testing.T) {
	const typical = 60 * time.Millisecond
	stalling := newLatencyBackend("stalling", typical, 3*time.Second)
	defer stalling.Close()
	healthy := newLatencyBackend("healthy", typical, 0)
	defer healthy.Close()

	s := newHedgeProxy(t, fmt.Sprintf(`r: * -> <roundRobin, "%s", "%s">;`, stalling.URL, healthy.URL),
		&proxy.HedgeParams{Percentile: 0.9, MinDelay: 500 * time.Millisecond})

	start := time.Now()
	for range 4 {
		if _, code, _ := s.get(t, "/stall"); code != http.StatusOK {
			t.Fatalf("got %d", code)
		}
	}
	if got := s.counter("r", "fired"); got != 0 {
		t.Fatalf("fired=%d during warm-up without a fixed delay, want 0", got)
	}
	if time.Since(start) < 3*time.Second {
		t.Fatal("stalled requests returned early without a hedge")
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for range 300 {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			s.get(t, "/")
		})
	}
	wg.Wait()
	s.warmUp(t)

	for range 8 {
		body, code, d := s.get(t, "/stall")
		if code != http.StatusOK || body != "healthy" {
			t.Fatalf("got %d %q, want the healthy backend to answer", code, body)
		}
		if d > 1500*time.Millisecond {
			t.Fatalf("a stalled request took %v after warm-up, want a hedge", d)
		}
	}
	if got := s.counter("r", "fired"); got == 0 {
		t.Fatal("no hedge was fired after warm-up")
	}
}

func TestHedgeShadowHeaderSetByFilterBeforeLoopback(t *testing.T) {
	slow := newHedgeBackend("slow", 300*time.Millisecond, http.StatusOK)
	defer slow.Close()
	fast := newHedgeBackend("fast", 0, http.StatusOK)
	defer fast.Close()

	m := &metricstest.MockMetrics{}
	fr := make(filters.Registry)
	fr.Register(builtin.NewSetRequestHeader())
	routes := eskip.MustParse(fmt.Sprintf(`
		entry: *
			-> setRequestHeader("%s", "example.org")
			-> <loopback>;
		looped: Header("%s", "example.org")
			-> <roundRobin, "%s", "%s">;
	`, filters.ShadowTrafficHeader, filters.ShadowTrafficHeader, slow.URL, fast.URL))

	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		Metrics:              m,
		Hedge:                &proxy.HedgeParams{FixedDelay: hedgeDelay, RouteCounters: true},
	}, routes...)
	defer p.Close()
	s := &hedgeSetup{proxy: p, metrics: m}

	for range 120 {
		if _, code, _ := s.get(t, "/"); code != http.StatusOK {
			t.Fatalf("warm-up request got %d, the loopback route does not reach a backend", code)
		}
	}
	s.base = map[string]int64{}

	for range 4 {
		if _, code, _ := s.get(t, "/slow"); code != http.StatusOK {
			t.Fatalf("got %d", code)
		}
	}
	if got := s.counter("looped", "fired"); got != 0 {
		t.Fatalf("a request marked as shadow traffic by a filter before the loopback fired %d hedges, want 0", got)
	}
}
