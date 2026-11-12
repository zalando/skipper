package proxy

import (
	stdlibcontext "context"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

var hedgeEpoch = time.Unix(1_700_000_000, 0)

// feed simulates seconds of traffic: perSec requests per second of which
// errsPerSec failed. It returns the time after the last simulated second.
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

	// 100 req/s at 5% refill 5 tokens per second.
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
