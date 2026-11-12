package proxy

import (
	stdlibcontext "context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/routing"
)

var hedgeEpoch = time.Unix(1_700_000_000, 0)

func feed(r *routeHedge, start time.Time, seconds, perSec, errsPerSec int) time.Time {
	now := start
	for range seconds {
		for i := range perSec {
			r.arrive(now)
			r.observe(now, i < errsPerSec)
		}
		now = now.Add(time.Second)
	}
	return now
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTestHedgeProxy(tb testing.TB, delay time.Duration) *Proxy {
	tb.Helper()

	quit := make(chan struct{})
	tb.Cleanup(func() { close(quit) })
	return &Proxy{hedger: newHedger(&HedgeParams{FixedDelay: delay}, quit)}
}

func newTestHedgeContext(method string, route eskip.Route, bag map[string]any) (*context, *http.Request) {
	req, err := http.NewRequest(method, "http://10.0.0.1:8080/", nil)
	if err != nil {
		panic(err)
	}
	if bag == nil {
		bag = map[string]any{}
	}
	return &context{
		route:    &routing.Route{Route: route},
		request:  req,
		stateBag: bag,
	}, req
}

var lbRoute = eskip.Route{Id: "r", BackendType: eskip.LBBackend, LBAlgorithm: "roundRobin"}

func TestRouteHedgeAcquire(t *testing.T) {
	for _, tc := range []struct {
		name string
		prep func(*routeHedge) time.Time
		want hedgeDecision
	}{{
		name: "cold start",
		prep: func(r *routeHedge) time.Time { return feed(r, hedgeEpoch, 1, hedgeColdStartMinReqs-1, 0) },
		want: hedgeColdStart,
	}, {
		name: "warm and idle",
		prep: func(r *routeHedge) time.Time { return feed(r, hedgeEpoch, 20, 10, 0) },
		want: hedgeAllowed,
	}, {
		name: "error ratio above the limit",
		prep: func(r *routeHedge) time.Time { return feed(r, hedgeEpoch, 10, 20, 5) },
		want: hedgeThrottled,
	}, {
		name: "error ratio at the limit is allowed",
		prep: func(r *routeHedge) time.Time { return feed(r, hedgeEpoch, 10, 20, 2) },
		want: hedgeAllowed,
	}, {
		name: "too few requests in the window to judge errors",
		prep: func(r *routeHedge) time.Time {
			now := feed(r, hedgeEpoch, 20, 10, 0)
			now = now.Add(hedgeWindowSeconds * time.Second)
			return feed(r, now, 1, 2, 2)
		},
		want: hedgeAllowed,
	}, {
		name: "request rate surge",
		prep: func(r *routeHedge) time.Time {
			now := feed(r, hedgeEpoch, 300, 20, 0)
			return feed(r, now, 5, 100, 0)
		},
		want: hedgeThrottled,
	}, {
		name: "steady high rate is not a surge",
		prep: func(r *routeHedge) time.Time { return feed(r, hedgeEpoch, 300, 100, 0) },
		want: hedgeAllowed,
	}, {
		name: "low baseline is not a surge",
		prep: func(r *routeHedge) time.Time {
			now := feed(r, hedgeEpoch, 60, 0, 0)
			r.arrive(now)
			return feed(r, now, 1, 150, 0)
		},
		want: hedgeAllowed,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			r := &routeHedge{tokens: hedgeBurst}
			now := tc.prep(r)
			if got := r.acquire(now, DefaultHedgeBudgetPercent); got != tc.want {
				t.Fatalf("got decision %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRouteHedgeErrorWindowRecovers(t *testing.T) {
	r := &routeHedge{tokens: hedgeBurst}
	now := feed(r, hedgeEpoch, 10, 20, 10)
	if got := r.acquire(now, 5); got != hedgeThrottled {
		t.Fatalf("got %d, want throttled", got)
	}

	now = feed(r, now, 2*hedgeWindowSeconds, 20, 0)
	if got := r.acquire(now, 5); got != hedgeAllowed {
		t.Fatalf("got %d after the error window passed, want allowed", got)
	}
}

func TestRouteHedgeBudget(t *testing.T) {
	r := &routeHedge{tokens: hedgeBurst}
	now := feed(r, hedgeEpoch, 60, 100, 0)

	allowed := 0
	for range 3 * hedgeBurst {
		if r.acquire(now, 5) == hedgeAllowed {
			allowed++
		}
	}
	if allowed < hedgeBurst-1 || allowed > hedgeBurst+1 {
		t.Fatalf("burst allowed %d hedges, want about %d", allowed, hedgeBurst)
	}
	if got := r.acquire(now, 5); got != hedgeBudgetExhausted {
		t.Fatalf("got %d, want budget exhausted", got)
	}

	now = feed(r, now, 2, 100, 0)
	allowed = 0
	for range 3 * hedgeBurst {
		if r.acquire(now, 5) == hedgeAllowed {
			allowed++
		}
	}
	if allowed < 8 || allowed > hedgeBurst {
		t.Fatalf("after 2s allowed %d hedges, want between 8 and %d", allowed, hedgeBurst)
	}
}

func TestRouteHedgeBudgetScalesWithPercent(t *testing.T) {
	refilled := func(percent float64) int {
		r := &routeHedge{tokens: hedgeBurst}
		now := feed(r, hedgeEpoch, 60, 100, 0)
		for r.acquire(now, percent) == hedgeAllowed {
		}
		now = feed(r, now, 1, 100, 0)
		n := 0
		for r.acquire(now, percent) == hedgeAllowed {
			n++
		}
		return n
	}

	low, high := refilled(1), refilled(10)
	if high <= low {
		t.Fatalf("a 10%% budget refilled %d tokens, a 1%% budget %d", high, low)
	}
}

func TestRouteHedgeRefund(t *testing.T) {
	r := &routeHedge{tokens: hedgeBurst}
	now := feed(r, hedgeEpoch, 60, 100, 0)

	for r.acquire(now, 1) == hedgeAllowed {
	}
	r.refund()
	if got := r.acquire(now, 1); got != hedgeAllowed {
		t.Fatalf("got %d after refund, want allowed", got)
	}

	r = &routeHedge{tokens: hedgeBurst}
	for range 50 {
		r.refund()
	}
	if r.tokens != hedgeBurst {
		t.Fatalf("refund exceeded the burst: %v", r.tokens)
	}
}

func TestRouteHedgeStateResetsAfterLongIdle(t *testing.T) {
	r := &routeHedge{tokens: hedgeBurst}
	now := feed(r, hedgeEpoch, 60, 100, 5)

	now = now.Add(time.Hour)
	r.arrive(now)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fast != 0 || r.slow != 0 {
		t.Fatalf("EWMAs not reset: fast=%v slow=%v", r.fast, r.slow)
	}
	for _, b := range r.window {
		if b.errs != 0 {
			t.Fatal("error window not reset")
		}
	}
}

func TestHedgerRouteState(t *testing.T) {
	quit := make(chan struct{})
	defer close(quit)
	h := newHedger(&HedgeParams{FixedDelay: time.Second}, quit)

	if h.budgetPercent != DefaultHedgeBudgetPercent {
		t.Fatalf("budget percent %v, want the default", h.budgetPercent)
	}
	first := h.route("a")
	if again := h.route("a"); again != first {
		t.Fatal("state is not stable per route")
	}
	if other := h.route("b"); other == first {
		t.Fatal("state is shared between routes")
	}

	h2 := newHedger(&HedgeParams{FixedDelay: time.Second, BudgetPercent: 12}, quit)
	if h2.budgetPercent != 12 {
		t.Fatalf("budget percent %v, want 12", h2.budgetPercent)
	}
}

func TestHedgerSweep(t *testing.T) {
	quit := make(chan struct{})
	defer close(quit)
	h := newHedger(&HedgeParams{FixedDelay: time.Second}, quit)

	h.route("idle").lastSeen = hedgeEpoch
	h.route("active").lastSeen = hedgeEpoch.Add(hedgeIdleTTL)

	h.sweep(hedgeEpoch.Add(hedgeIdleTTL + time.Minute))

	h.mu.RLock()
	defer h.mu.RUnlock()
	if _, ok := h.routes["idle"]; ok {
		t.Fatal("idle route state was not removed")
	}
	if _, ok := h.routes["active"]; !ok {
		t.Fatal("active route state was removed")
	}
}

func TestHedgeable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		route  func(eskip.Route) eskip.Route
		bag    map[string]any
		mutate func(*context, *http.Request)
		want   bool
	}{{
		name:   "get on lb route",
		method: http.MethodGet,
		want:   true,
	}, {
		name:   "head on lb route",
		method: http.MethodHead,
		want:   true,
	}, {
		name:   "post",
		method: http.MethodPost,
	}, {
		name:   "put",
		method: http.MethodPut,
	}, {
		name:   "options",
		method: http.MethodOptions,
	}, {
		name:   "network backend",
		method: http.MethodGet,
		route:  func(r eskip.Route) eskip.Route { r.BackendType = eskip.NetworkBackend; return r },
	}, {
		name:   "dynamic backend",
		method: http.MethodGet,
		route:  func(r eskip.Route) eskip.Route { r.BackendType = eskip.DynamicBackend; return r },
	}, {
		name:   "shunt",
		method: http.MethodGet,
		route:  func(r eskip.Route) eskip.Route { r.BackendType = eskip.ShuntBackend; return r },
	}, {
		name:   "consistent hash",
		method: http.MethodGet,
		route:  func(r eskip.Route) eskip.Route { r.LBAlgorithm = "consistentHash"; return r },
	}, {
		name:   "opted out",
		method: http.MethodGet,
		bag:    map[string]any{filters.BackendHedgeDisabled: true},
	}, {
		name:   "backend is a proxy",
		method: http.MethodGet,
		bag:    map[string]any{filters.BackendIsProxyKey: true},
	}, {
		name:   "arrived as shadow traffic",
		method: http.MethodGet,
		mutate: func(c *context, _ *http.Request) { c.arrivedAsShadow = true },
	}, {
		name:   "upgrade request",
		method: http.MethodGet,
		mutate: func(c *context, _ *http.Request) { c.request.Header.Set("Connection", "Upgrade") },
	}, {
		name:   "request with body",
		method: http.MethodGet,
		mutate: func(_ *context, r *http.Request) { r.Body = http.NoBody; r.Body = &nonEmptyBody{} },
	}, {
		name:   "empty body is fine",
		method: http.MethodGet,
		mutate: func(_ *context, r *http.Request) { r.Body = http.NoBody },
		want:   true,
	}, {
		name:   "unsupported scheme",
		method: http.MethodGet,
		mutate: func(_ *context, r *http.Request) { r.URL.Scheme = "fastcgi" },
	}} {
		t.Run(tc.name, func(t *testing.T) {
			route := lbRoute
			if tc.route != nil {
				route = tc.route(route)
			}
			ctx, req := newTestHedgeContext(tc.method, route, tc.bag)
			if tc.mutate != nil {
				tc.mutate(ctx, req)
			}

			p := &Proxy{}
			if got := p.hedgeable(ctx, req); got != tc.want {
				t.Fatalf("hedgeable=%v, want %v", got, tc.want)
			}
		})
	}
}

type nonEmptyBody struct{}

func (*nonEmptyBody) Read([]byte) (int, error) { return 0, nil }
func (*nonEmptyBody) Close() error             { return nil }

func TestHedgeAttemptRank(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    hedgeAttempt
		want int
	}{
		{"2xx", hedgeAttempt{resp: &http.Response{StatusCode: 200}}, 0},
		{"4xx", hedgeAttempt{resp: &http.Response{StatusCode: 404}}, 0},
		{"5xx", hedgeAttempt{resp: &http.Response{StatusCode: 503}}, 1},
		{"error", hedgeAttempt{err: http.ErrHandlerTimeout}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.rank(); got != tc.want {
				t.Fatalf("rank=%d, want %d", got, tc.want)
			}
		})
	}
}

type trackedBody struct {
	closed bool
}

func (b *trackedBody) Read([]byte) (int, error) { return 0, nil }
func (b *trackedBody) Close() error             { b.closed = true; return nil }

func TestHedgeAttemptRelease(t *testing.T) {
	t.Run("body keeps the context alive until closed", func(t *testing.T) {
		cancelled := false
		a := &hedgeAttempt{
			resp:   &http.Response{StatusCode: 200, Body: &trackedBody{}},
			cancel: func() { cancelled = true },
		}

		rsp, err := a.release()
		if err != nil || rsp == nil {
			t.Fatalf("rsp=%v err=%v", rsp, err)
		}
		if cancelled {
			t.Fatal("context cancelled before the body was closed")
		}
		rsp.Body.Close()
		if !cancelled {
			t.Fatal("context not cancelled after the body was closed")
		}
	})

	t.Run("empty body cancels at once", func(t *testing.T) {
		cancelled := false
		a := &hedgeAttempt{
			resp:   &http.Response{StatusCode: 200, Body: http.NoBody},
			cancel: func() { cancelled = true },
		}
		if _, err := a.release(); err != nil {
			t.Fatal(err)
		}
		if !cancelled {
			t.Fatal("context not cancelled")
		}
	})

	t.Run("error cancels and is returned", func(t *testing.T) {
		cancelled := false
		a := &hedgeAttempt{err: http.ErrHandlerTimeout, cancel: func() { cancelled = true }}
		rsp, err := a.release()
		if rsp != nil || err != http.ErrHandlerTimeout {
			t.Fatalf("rsp=%v err=%v", rsp, err)
		}
		if !cancelled {
			t.Fatal("context not cancelled")
		}
	})
}

func TestDiscardAttempt(t *testing.T) {
	body := &trackedBody{}
	cancelled := false
	discardAttempt(&hedgeAttempt{
		resp:   &http.Response{Body: body},
		cancel: func() { cancelled = true },
	})
	if !body.closed || !cancelled {
		t.Fatalf("body closed=%v cancelled=%v", body.closed, cancelled)
	}

	discardAttempt(&hedgeAttempt{err: http.ErrHandlerTimeout, cancel: func() {}})
}

func okRoundTripper() http.RoundTripper {
	return roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
}

func TestRoundTripPlainPathWhenNotHedged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		hedging bool
		method  string
	}{
		{"hedging disabled", false, http.MethodGet},
		{"ineligible method", true, http.MethodPost},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &Proxy{}
			if tc.hedging {
				p = newTestHedgeProxy(t, time.Hour)
			}
			ctx, req := newTestHedgeContext(tc.method, lbRoute, nil)

			calls := 0
			rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r != req {
					t.Error("the request was copied on the plain path")
				}
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			})

			rsp, err := p.roundTrip(ctx, rt, req, req)
			if err != nil || rsp.StatusCode != 200 || calls != 1 {
				t.Fatalf("rsp=%v err=%v calls=%d", rsp, err, calls)
			}
		})
	}
}

func BenchmarkHedgeable(b *testing.B) {
	for _, bm := range []struct {
		name   string
		method string
		route  eskip.Route
	}{
		{"eligible", http.MethodGet, lbRoute},
		{"ineligible method", http.MethodPost, lbRoute},
		{"ineligible backend", http.MethodGet, eskip.Route{Id: "r", BackendType: eskip.NetworkBackend}},
	} {
		b.Run(bm.name, func(b *testing.B) {
			ctx, req := newTestHedgeContext(bm.method, bm.route, nil)
			p := &Proxy{}

			b.ReportAllocs()
			for b.Loop() {
				p.hedgeable(ctx, req)
			}
		})
	}
}

func BenchmarkRoundTrip(b *testing.B) {
	for _, bm := range []struct {
		name    string
		hedging bool
		method  string
	}{
		{"hedging disabled", false, http.MethodGet},
		{"hedging enabled, ineligible", true, http.MethodPost},
		{"hedging enabled, eligible, no hedge sent", true, http.MethodGet},
	} {
		b.Run(bm.name, func(b *testing.B) {
			p := &Proxy{}
			if bm.hedging {
				p = newTestHedgeProxy(b, time.Hour)
			}
			ctx, req := newTestHedgeContext(bm.method, lbRoute, nil)
			rt := okRoundTripper()

			b.ReportAllocs()
			for b.Loop() {
				rsp, err := p.roundTrip(ctx, rt, req, req)
				if err != nil {
					b.Fatal(err)
				}
				rsp.Body.Close()
			}
		})
	}
}

func BenchmarkRoundTripParallel(b *testing.B) {
	for _, bm := range []struct {
		name    string
		hedging bool
	}{
		{"hedging disabled", false},
		{"hedging enabled, eligible, no hedge sent", true},
	} {
		b.Run(bm.name, func(b *testing.B) {
			p := &Proxy{}
			if bm.hedging {
				p = newTestHedgeProxy(b, time.Hour)
			}
			rt := okRoundTripper()

			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				ctx, req := newTestHedgeContext(http.MethodGet, lbRoute, nil)
				for pb.Next() {
					rsp, err := p.roundTrip(ctx, rt, req, req)
					if err != nil {
						b.Error(err)
						return
					}
					rsp.Body.Close()
				}
			})
		})
	}
}

func BenchmarkRouteHedge(b *testing.B) {
	b.Run("arrive", func(b *testing.B) {
		r := &routeHedge{tokens: hedgeBurst}
		now := hedgeEpoch

		b.ReportAllocs()
		for b.Loop() {
			r.arrive(now)
		}
	})

	b.Run("observe failed", func(b *testing.B) {
		r := &routeHedge{tokens: hedgeBurst}
		now := hedgeEpoch
		r.arrive(now)

		b.ReportAllocs()
		for b.Loop() {
			r.observe(now, true)
		}
	})

	b.Run("acquire", func(b *testing.B) {
		r := &routeHedge{tokens: hedgeBurst}
		now := feed(r, hedgeEpoch, 60, 100, 0)

		b.ReportAllocs()
		for b.Loop() {
			r.acquire(now, DefaultHedgeBudgetPercent)
		}
	})

	b.Run("arrive parallel", func(b *testing.B) {
		r := &routeHedge{tokens: hedgeBurst}
		now := hedgeEpoch

		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				r.arrive(now)
			}
		})
	})
}

func BenchmarkHedgerRoute(b *testing.B) {
	quit := make(chan struct{})
	defer close(quit)
	h := newHedger(&HedgeParams{FixedDelay: time.Second}, quit)
	h.route("r")

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			h.route("r")
		}
	})
}

func errorsInWindow(r *routeHedge) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, b := range r.window {
		n += b.errs
	}
	return n
}

func TestHedgeClientCancelIsNotCountedAsBackendFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		rt   func(*http.Request) (*http.Response, error)
		ctx  func() (stdlibcontext.Context, stdlibcontext.CancelFunc)
		want int
	}{{
		name: "client cancelled",
		rt: func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		},
		ctx: func() (stdlibcontext.Context, stdlibcontext.CancelFunc) {
			return stdlibcontext.WithCancel(stdlibcontext.Background())
		},
		want: 0,
	}, {
		name: "backend error",
		rt: func(*http.Request) (*http.Response, error) {
			return nil, http.ErrHandlerTimeout
		},
		ctx:  func() (stdlibcontext.Context, stdlibcontext.CancelFunc) { return stdlibcontext.Background(), func() {} },
		want: 1,
	}, {
		name: "backend 5xx",
		rt: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 503, Body: http.NoBody}, nil
		},
		ctx:  func() (stdlibcontext.Context, stdlibcontext.CancelFunc) { return stdlibcontext.Background(), func() {} },
		want: 1,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestHedgeProxy(t, time.Hour)
			ctx, req := newTestHedgeContext(http.MethodGet, lbRoute, nil)

			c, cancel := tc.ctx()
			defer cancel()
			req = req.WithContext(c)

			done := make(chan struct{})
			go func() {
				defer close(done)
				rsp, _ := p.hedgedRoundTrip(ctx, roundTripperFunc(tc.rt), req, req)
				if rsp != nil {
					rsp.Body.Close()
				}
			}()
			time.Sleep(10 * time.Millisecond)
			cancel()
			<-done

			if got := errorsInWindow(p.hedger.route(lbRoute.Id)); got != tc.want {
				t.Fatalf("errors in window=%d, want %d", got, tc.want)
			}
		})
	}
}

func newSelectionProxy(tb testing.TB, algorithm string, n int, fadeInDuration time.Duration, young map[int]bool) (*Proxy, *routing.Route, []string) {
	tb.Helper()
	return newSelectionProxyWithRegistry(tb, routing.RegistryOptions{}, algorithm, n, fadeInDuration, young)
}

func newSelectionProxyWithRegistry(tb testing.TB, ro routing.RegistryOptions, algorithm string, n int, fadeInDuration time.Duration, young map[int]bool) (*Proxy, *routing.Route, []string) {
	tb.Helper()

	registry := routing.NewEndpointRegistry(ro)
	tb.Cleanup(registry.Close)

	eskipRoute := eskip.Route{Id: "r", BackendType: eskip.LBBackend, LBAlgorithm: algorithm}
	var hosts []string
	for i := range n {
		host := fmt.Sprintf("ep-%d.test:80", i)
		hosts = append(hosts, host)
		eskipRoute.LBEndpoints = append(eskipRoute.LBEndpoints, &eskip.LBEndpoint{Address: "http://" + host})

		age := time.Hour
		if young[i] {
			age = time.Millisecond
		}
		registry.GetMetrics(host).SetDetected(time.Now().Add(-age))
	}

	route := &routing.Route{Route: eskipRoute, LBFadeInDuration: fadeInDuration, LBFadeInExponent: 1}
	route = loadbalancer.NewAlgorithmProvider().Do([]*routing.Route{route})[0]
	registry.Do([]*routing.Route{route})

	p := &Proxy{
		registry: registry,
		fadein:   &fadeIn{rnd: rand.New(rand.NewPCG(1, 2))},
		metrics:  metrics.Void,
	}
	return p, route, hosts
}

func TestSelectEndpointExcluding(t *testing.T) {
	for _, tc := range []struct {
		name      string
		algorithm string
		n         int
		fadeIn    time.Duration
		young     map[int]bool
		exclude   int
		wantNil   bool
	}{
		{name: "two endpoints", algorithm: "roundRobin", n: 2, exclude: 0},
		{name: "many endpoints", algorithm: "random", n: 5, exclude: 3},
		{name: "power of random choices", algorithm: "powerOfRandomNChoices", n: 3, exclude: 1},
		{name: "least requests", algorithm: "leastRequests", n: 3, exclude: 2},
		{name: "single endpoint has no alternate", algorithm: "roundRobin", n: 1, exclude: 0, wantNil: true},
		{
			name: "a fading-in endpoint does not stop an old alternate", algorithm: "roundRobin",
			n: 3, fadeIn: time.Hour, young: map[int]bool{1: true}, exclude: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, route, hosts := newSelectionProxy(t, tc.algorithm, tc.n, tc.fadeIn, tc.young)
			ctx := &context{route: route, request: &http.Request{}, stateBag: map[string]any{}}

			for range 200 {
				ep := p.selectEndpointExcluding(ctx, hosts[tc.exclude])
				if tc.wantNil {
					if ep != nil {
						t.Fatalf("got %q, want no alternate", ep.Host)
					}
					continue
				}
				if ep == nil {
					t.Fatal("no alternate endpoint although one exists")
				}
				if ep.Host == hosts[tc.exclude] {
					t.Fatalf("selected the excluded endpoint %q", ep.Host)
				}
			}
		})
	}
}

func TestSelectEndpointExcludingNothingBehavesLikeSelectEndpoint(t *testing.T) {
	p, route, hosts := newSelectionProxy(t, "roundRobin", 3, 0, nil)
	ctx := &context{route: route, request: &http.Request{}, stateBag: map[string]any{}}

	seen := map[string]bool{}
	for range 30 {
		seen[p.selectEndpoint(ctx).Host] = true
	}
	for _, h := range hosts {
		if !seen[h] {
			t.Fatalf("endpoint %q was never selected", h)
		}
	}
}

func TestSelectEndpointExcludingSkipsDroppedEndpoints(t *testing.T) {
	p, route, hosts := newSelectionProxyWithRegistry(t, routing.RegistryOptions{
		PassiveHealthCheckEnabled:     true,
		StatsResetPeriod:              50 * time.Millisecond,
		MinRequests:                   5,
		MinHealthCheckDropProbability: 0.01,
		MaxHealthCheckDropProbability: 1,
	}, "roundRobin", 3, 0, nil)
	p.healthyEndpoints = &healthyEndpoints{
		rnd:                        rand.New(rand.NewPCG(1, 2)),
		maxUnhealthyEndpointsRatio: 1,
	}
	ctx := &context{route: route, request: &http.Request{}, stateBag: map[string]any{}}

	failing := p.registry.GetMetrics(hosts[1])
	deadline := time.Now().Add(5 * time.Second)
	for failing.HealthCheckDropProbability() < 0.9 {
		if time.Now().After(deadline) {
			t.Fatal("the failing endpoint was never marked unhealthy")
		}
		for range 20 {
			failing.IncRequests(routing.IncRequestsOptions{FailedRoundTrip: true})
		}
		time.Sleep(10 * time.Millisecond)
	}

	for range 200 {
		ep := p.selectEndpointExcluding(ctx, hosts[0])
		if ep == nil {
			t.Fatal("no alternate endpoint although a healthy one remains")
		}
		if ep.Host == hosts[0] {
			t.Fatalf("selected the excluded endpoint %q", ep.Host)
		}
	}
}

func TestSelectEndpointExcludingDoesNotHedgeToFadingInEndpoint(t *testing.T) {
	p, route, hosts := newSelectionProxy(t, "roundRobin", 2, time.Hour, map[int]bool{1: true})
	ctx := &context{route: route, request: &http.Request{}, stateBag: map[string]any{}}

	const n = 500
	selected := 0
	for range n {
		ep := p.selectEndpointExcluding(ctx, hosts[0])
		if ep == nil {
			continue
		}
		if ep.Host == hosts[0] {
			t.Fatalf("selected the excluded endpoint %q", ep.Host)
		}
		selected++
	}
	if selected > n/20 {
		t.Fatalf("the fading-in endpoint was chosen for %d of %d hedges, want it to be rarely used", selected, n)
	}
}

type countingMetrics struct {
	routing.Metrics
	inflight atomic.Int64
	requests atomic.Int64
	failed   atomic.Int64
	maxSeen  atomic.Int64
}

func (m *countingMetrics) IncInflightRequest() {
	n := m.inflight.Add(1)
	for {
		cur := m.maxSeen.Load()
		if n <= cur || m.maxSeen.CompareAndSwap(cur, n) {
			return
		}
	}
}

func (m *countingMetrics) DecInflightRequest() { m.inflight.Add(-1) }

func (m *countingMetrics) IncRequests(o routing.IncRequestsOptions) {
	m.requests.Add(1)
	if o.FailedRoundTrip {
		m.failed.Add(1)
	}
}

func TestHedgeEndpointAccounting(t *testing.T) {
	const bound = 5 * time.Second

	blockUntilCancelled := func(r *http.Request) (*http.Response, error) {
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(bound):
			return nil, http.ErrHandlerTimeout
		}
	}
	okAfter := func(d time.Duration) func(*http.Request) (*http.Response, error) {
		return func(r *http.Request) (*http.Response, error) {
			select {
			case <-time.After(d):
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
	}

	for _, tc := range []struct {
		name       string
		primary    func(*http.Request) (*http.Response, error)
		hedge      func(*http.Request) (*http.Response, error)
		wantFailed int64
		wantErr    bool
	}{{
		name:    "hedge wins and the primary is cancelled",
		primary: blockUntilCancelled,
		hedge:   okAfter(0),
	}, {
		name:    "hedge fails with an error and the primary wins",
		primary: okAfter(100 * time.Millisecond),
		hedge: func(*http.Request) (*http.Response, error) {
			return nil, http.ErrHandlerTimeout
		},
		wantFailed: 1,
	}, {
		name:    "hedge loses and is cancelled",
		primary: okAfter(100 * time.Millisecond),
		hedge:   blockUntilCancelled,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			p, route, hosts := newSelectionProxy(t, "roundRobin", 2, 0, nil)

			primary := &countingMetrics{Metrics: route.LBEndpoints[0].Metrics}
			hedge := &countingMetrics{Metrics: route.LBEndpoints[1].Metrics}
			route.LBEndpoints[0].Metrics = primary
			route.LBEndpoints[1].Metrics = hedge

			rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == hosts[0] {
					return tc.primary(r)
				}
				return tc.hedge(r)
			})
			p.roundTripper = rt
			p.tracing = newProxyTracing(nil)

			quit := make(chan struct{})
			t.Cleanup(func() { close(quit) })
			p.hedger = newHedger(&HedgeParams{FixedDelay: 20 * time.Millisecond, RouteCounters: true}, quit)
			for range hedgeColdStartMinReqs {
				p.hedger.route("r").arrive(time.Now())
			}

			ctx := &context{route: route, request: &http.Request{}, stateBag: map[string]any{}}
			req, err := http.NewRequest(http.MethodGet, "http://"+hosts[0]+"/", nil)
			if err != nil {
				t.Fatal(err)
			}

			rsp, err := p.hedgedRoundTrip(ctx, rt, req, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			rsp.Body.Close()

			waitFor := func(what string, cond func() bool) {
				t.Helper()
				deadline := time.Now().Add(2 * time.Second)
				for !cond() {
					if time.Now().After(deadline) {
						t.Fatalf("timed out waiting for %s", what)
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
			waitFor("the hedge attempt to be counted", func() bool { return hedge.requests.Load() > 0 })
			waitFor("the hedge endpoint inflight counter to return to zero", func() bool { return hedge.inflight.Load() == 0 })

			if got := hedge.maxSeen.Load(); got != 1 {
				t.Fatalf("hedge endpoint inflight peaked at %d, want 1", got)
			}
			if got := hedge.requests.Load(); got != 1 {
				t.Fatalf("hedge endpoint IncRequests=%d, want 1", got)
			}
			if got := hedge.failed.Load(); got != tc.wantFailed {
				t.Fatalf("hedge endpoint failed round trips=%d, want %d", got, tc.wantFailed)
			}
			if got := primary.requests.Load() + primary.inflight.Load(); got != 0 {
				t.Fatalf("the hedge helper touched the primary endpoint metrics (%d); makeBackendRequest owns those", got)
			}
		})
	}
}

func TestHedgeThrottleCountsAndSkipsTheHedge(t *testing.T) {
	for _, tc := range []struct {
		name         string
		prep         func(*routeHedge, time.Time)
		wantMetric   string
		wantHedgeHit bool
	}{{
		name: "request rate surge",
		prep: func(r *routeHedge, now time.Time) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.seen = 10_000
			r.sec = now.Unix()
			r.slow = 10
			r.fast = 100
			r.tokens = hedgeBurst
		},
		wantMetric: "hedge.r.throttled",
	}, {
		name: "error ratio",
		prep: func(r *routeHedge, now time.Time) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.seen = 10_000
			r.sec = now.Unix()
			r.window[r.sec%hedgeWindowSeconds] = hedgeWindowBucket{reqs: 100, errs: 50}
			r.tokens = hedgeBurst
		},
		wantMetric: "hedge.r.throttled",
	}, {
		name: "steady state hedges",
		prep: func(r *routeHedge, now time.Time) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.seen = 10_000
			r.sec = now.Unix()
			r.slow = 10
			r.fast = 10
			r.tokens = hedgeBurst
		},
		wantHedgeHit: true,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			p, route, hosts := newSelectionProxy(t, "roundRobin", 2, 0, nil)
			m := &metricstest.MockMetrics{}
			p.metrics = m
			p.tracing = newProxyTracing(nil)

			var hedgeHits atomic.Int64
			rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != hosts[0] {
					hedgeHits.Add(1)
					return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
				}
				select {
				case <-time.After(150 * time.Millisecond):
					return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			})
			p.roundTripper = rt

			quit := make(chan struct{})
			t.Cleanup(func() { close(quit) })
			p.hedger = newHedger(&HedgeParams{FixedDelay: 20 * time.Millisecond, RouteCounters: true}, quit)
			tc.prep(p.hedger.route("r"), time.Now())

			ctx := &context{route: route, request: &http.Request{}, stateBag: map[string]any{}}
			req, err := http.NewRequest(http.MethodGet, "http://"+hosts[0]+"/", nil)
			if err != nil {
				t.Fatal(err)
			}
			rsp, err := p.hedgedRoundTrip(ctx, rt, req, req)
			if err != nil {
				t.Fatal(err)
			}
			rsp.Body.Close()

			if got := hedgeHits.Load() > 0; got != tc.wantHedgeHit {
				t.Fatalf("hedge request sent=%v, want %v", got, tc.wantHedgeHit)
			}
			if tc.wantMetric != "" {
				if n, _ := m.Counter(tc.wantMetric); n != 1 {
					t.Fatalf("%s=%d, want 1", tc.wantMetric, n)
				}
			}
			if n, _ := m.Counter("hedge.r.fired"); (n == 1) != tc.wantHedgeHit {
				t.Fatalf("fired=%d, want hedge=%v", n, tc.wantHedgeHit)
			}
		})
	}
}

func TestHedgeWinnerSelection(t *testing.T) {
	ok := func() (*http.Response, error) { return &http.Response{StatusCode: 200, Body: http.NoBody}, nil }
	serverError := func() (*http.Response, error) { return &http.Response{StatusCode: 503, Body: http.NoBody}, nil }
	failure := func() (*http.Response, error) { return nil, http.ErrHandlerTimeout }

	for _, tc := range []struct {
		name       string
		primary    func() (*http.Response, error)
		hedge      func() (*http.Response, error)
		hedgeFirst bool
		wantHedge  bool
		wantCode   int
		wantErr    bool
	}{
		{name: "hedge ok arrives first", primary: ok, hedge: ok, hedgeFirst: true, wantHedge: true, wantCode: 200},
		{name: "primary ok arrives first", primary: ok, hedge: ok, wantCode: 200},
		{name: "hedge 5xx first, primary ok later", primary: ok, hedge: serverError, hedgeFirst: true, wantCode: 200},
		{name: "primary 5xx first, hedge ok later", primary: serverError, hedge: ok, wantHedge: true, wantCode: 200},
		{name: "both 5xx, hedge first", primary: serverError, hedge: serverError, hedgeFirst: true, wantCode: 503},
		{name: "both 5xx, primary first", primary: serverError, hedge: serverError, wantCode: 503},
		{name: "both fail, hedge first", primary: failure, hedge: failure, hedgeFirst: true, wantErr: true},
		{name: "error then 5xx prefers the response", primary: failure, hedge: serverError, wantHedge: true, wantCode: 503},
		{name: "5xx then error prefers the response", primary: serverError, hedge: failure, hedgeFirst: true, wantCode: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, route, hosts := newSelectionProxy(t, "roundRobin", 2, 0, nil)
			m := &metricstest.MockMetrics{}
			p.metrics = m
			p.tracing = newProxyTracing(nil)

			hedgeSent := make(chan struct{})
			releasePrimary := make(chan struct{})
			rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == hosts[0] {
					if tc.hedgeFirst {
						<-hedgeSent
						select {
						case <-releasePrimary:
						case <-time.After(5 * time.Second):
						}
					} else {
						select {
						case <-hedgeSent:
						case <-time.After(5 * time.Second):
						}
					}
					return tc.primary()
				}
				close(hedgeSent)
				if !tc.hedgeFirst {
					time.Sleep(50 * time.Millisecond)
				} else {
					defer func() { go func() { time.Sleep(50 * time.Millisecond); close(releasePrimary) }() }()
				}
				return tc.hedge()
			})
			p.roundTripper = rt

			quit := make(chan struct{})
			t.Cleanup(func() { close(quit) })
			p.hedger = newHedger(&HedgeParams{FixedDelay: 20 * time.Millisecond, RouteCounters: true}, quit)
			for range hedgeColdStartMinReqs {
				p.hedger.route("r").arrive(time.Now())
			}

			ctx := &context{route: route, request: &http.Request{}, stateBag: map[string]any{}}
			req, err := http.NewRequest(http.MethodGet, "http://"+hosts[0]+"/", nil)
			if err != nil {
				t.Fatal(err)
			}

			rsp, err := p.hedgedRoundTrip(ctx, rt, req, req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tc.wantErr)
			}
			if rsp != nil {
				defer rsp.Body.Close()
				if rsp.StatusCode != tc.wantCode {
					t.Fatalf("status=%d, want %d", rsp.StatusCode, tc.wantCode)
				}
			}
			if tc.wantErr {
				return
			}

			won, _ := m.Counter("hedge.r.won")
			if (won == 1) != tc.wantHedge {
				t.Fatalf("hedge won=%d, want hedge to win=%v", won, tc.wantHedge)
			}
		})
	}
}

func TestHedgePrimaryEndpointAccounting(t *testing.T) {
	for _, tc := range []struct {
		name           string
		primaryFails   bool
		wantPrimaryReq int64
		wantPrimaryErr int64
	}{
		{name: "primary is cancelled by a winning hedge", wantPrimaryReq: 0},
		{name: "primary failed and the hedge won", primaryFails: true, wantPrimaryReq: 1, wantPrimaryErr: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, route, _ := newSelectionProxy(t, "roundRobin", 2, 0, nil)
			counters := map[string]*countingMetrics{}
			for i := range route.LBEndpoints {
				cm := &countingMetrics{Metrics: route.LBEndpoints[i].Metrics}
				route.LBEndpoints[i].Metrics = cm
				counters[route.LBEndpoints[i].Host] = cm
			}

			var calls atomic.Int64
			var primaryHost atomic.Value
			p.roundTripper = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if calls.Add(1) == 1 {
					primaryHost.Store(r.URL.Host)
					if tc.primaryFails {
						time.Sleep(50 * time.Millisecond)
						return nil, http.ErrHandlerTimeout
					}
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				if tc.primaryFails {
					time.Sleep(120 * time.Millisecond)
				}
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			})
			p.tracing = newProxyTracing(nil)
			p.metrics = metrics.Void

			quit := make(chan struct{})
			t.Cleanup(func() { close(quit) })
			p.hedger = newHedger(&HedgeParams{FixedDelay: 20 * time.Millisecond, RouteCounters: true}, quit)
			for range hedgeColdStartMinReqs {
				p.hedger.route("r").arrive(time.Now())
			}

			req, err := http.NewRequest(http.MethodGet, "http://example.org/", nil)
			require.NoError(t, err)
			ctx := &context{
				route:           route,
				request:         req,
				originalRequest: req,
				stateBag:        map[string]any{},
				outgoingHost:    "example.org",
				responseWriter:  nil,
			}

			rsp, perr := p.makeBackendRequest(ctx, req.Context())
			require.Nil(t, perr)
			rsp.Body.Close()

			ph, _ := primaryHost.Load().(string)
			require.NotEmpty(t, ph, "the primary never reached the backend")
			primary := counters[ph]

			waitUntil := time.Now().Add(2 * time.Second)
			for primary.inflight.Load() != 0 && time.Now().Before(waitUntil) {
				time.Sleep(5 * time.Millisecond)
			}
			assert.Equal(t, int64(0), primary.inflight.Load(), "primary inflight")
			assert.Equal(t, tc.wantPrimaryReq, primary.requests.Load(), "primary IncRequests")
			assert.Equal(t, tc.wantPrimaryErr, primary.failed.Load(), "primary failed round trips")
		})
	}
}

func TestHedgeCountersPerRouteAreOptIn(t *testing.T) {
	for _, tc := range []struct {
		name          string
		routeCounters bool
		wantRoute     int64
	}{
		{name: "route counters off", wantRoute: 0},
		{name: "route counters on", routeCounters: true, wantRoute: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, route, hosts := newSelectionProxy(t, "roundRobin", 1, 0, nil)
			m := &metricstest.MockMetrics{}
			p.metrics = m
			p.tracing = newProxyTracing(nil)

			quit := make(chan struct{})
			t.Cleanup(func() { close(quit) })
			p.hedger = newHedger(&HedgeParams{FixedDelay: 20 * time.Millisecond, RouteCounters: tc.routeCounters}, quit)
			for range hedgeColdStartMinReqs {
				p.hedger.route("r").arrive(time.Now())
			}
			p.roundTripper = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				time.Sleep(60 * time.Millisecond)
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			})

			ctx := &context{route: route, request: &http.Request{}, stateBag: map[string]any{}}
			req, err := http.NewRequest(http.MethodGet, "http://"+hosts[0]+"/", nil)
			require.NoError(t, err)
			rsp, err := p.hedgedRoundTrip(ctx, p.roundTripper, req, req)
			require.NoError(t, err)
			rsp.Body.Close()

			all, _ := m.Counter("hedge.no_alternate")
			assert.Equal(t, int64(1), all, "counter for all routes")
			perRoute, _ := m.Counter("hedge.r.no_alternate")
			assert.Equal(t, tc.wantRoute, perRoute, "counter for the route")
		})
	}
}

func BenchmarkHedgedRoundTripFiring(b *testing.B) {
	p, route, hosts := newSelectionProxy(b, "roundRobin", 2, 0, nil)
	p.tracing = newProxyTracing(nil)
	p.metrics = metrics.Void

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == hosts[0] {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
	p.roundTripper = rt

	quit := make(chan struct{})
	b.Cleanup(func() { close(quit) })
	p.hedger = newHedger(&HedgeParams{FixedDelay: time.Nanosecond, BudgetPercent: 100}, quit)
	state := p.hedger.route("r")
	state.mu.Lock()
	state.seen = 1 << 40
	state.mu.Unlock()

	ctx := &context{route: route, request: &http.Request{}, stateBag: map[string]any{}}
	req, err := http.NewRequest(http.MethodGet, "http://"+hosts[0]+"/", nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		state.mu.Lock()
		state.tokens = hedgeBurst
		state.fast, state.slow = 0, 0
		state.mu.Unlock()

		rsp, err := p.hedgedRoundTrip(ctx, rt, req, req)
		if err != nil {
			b.Fatal(err)
		}
		rsp.Body.Close()
	}
}

func BenchmarkMakeBackendRequest(b *testing.B) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer other.Close()

	for _, bm := range []struct {
		name  string
		hedge *HedgeParams
	}{
		{"hedging disabled", nil},
		{"hedging enabled, delay not reached", &HedgeParams{FixedDelay: time.Hour}},
	} {
		b.Run(bm.name, func(b *testing.B) {
			tp, err := newTestProxyWithParams(
				fmt.Sprintf(`* -> <roundRobin, "%s", "%s">`, backend.URL, other.URL),
				Params{CloseIdleConnsPeriod: -time.Second, Hedge: bm.hedge},
			)
			if err != nil {
				b.Fatal(err)
			}
			defer tp.close()

			ps := httptest.NewServer(tp.proxy)
			defer ps.Close()
			client := ps.Client()

			b.ReportAllocs()
			for b.Loop() {
				rsp, err := client.Get(ps.URL + "/")
				if err != nil {
					b.Fatal(err)
				}
				io.Copy(io.Discard, rsp.Body)
				rsp.Body.Close()
			}
		})
	}
}
