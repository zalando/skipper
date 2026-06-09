package shedder

import (
	"math"
	mrand "math/rand/v2"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/tracing/tracingtest"
)

func TestPhysicsShedderCreateFilter(t *testing.T) {
	spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)

	cases := []struct {
		name    string
		args    []interface{}
		wantErr bool
	}{
		{"ok minimal", []interface{}{"app", "active", "200ms"}, false},
		{"ok with window", []interface{}{"app", "active", "200ms", "3s"}, false},
		{"ok inactive", []interface{}{"app", "inactive", "200ms"}, false},
		{"ok logInactive", []interface{}{"app", "logInactive", "200ms"}, false},
		{"too few args", []interface{}{"app", "active"}, true},
		{"too many args", []interface{}{"app", "active", "200ms", "3s", "extra"}, true},
		{"empty suffix", []interface{}{"", "active", "200ms"}, true},
		{"non-string suffix", []interface{}{123, "active", "200ms"}, true},
		{"bad mode", []interface{}{"app", "bogus", "200ms"}, true},
		{"bad latency target", []interface{}{"app", "active", "not-a-duration"}, true},
		{"zero latency target", []interface{}{"app", "active", "0ms"}, true},
		{"negative latency target", []interface{}{"app", "active", "-100ms"}, true},
		{"bad window", []interface{}{"app", "active", "200ms", "nope"}, true},
		{"window too small", []interface{}{"app", "active", "200ms", "50ms"}, true},
		{"window too large", []interface{}{"app", "active", "200ms", "120s"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := spec.CreateFilter(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got filter %v", f)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			f.(*physicsShedder).Close()
		})
	}
}

func TestPhysicsShedderComputeResistance(t *testing.T) {
	ps := &physicsShedder{latencyTarget: 200 * time.Millisecond}

	t.Run("no traffic", func(t *testing.T) {
		if got := ps.computeResistance(0, 0, 0); got != 0 {
			t.Fatalf("want 0, got %v", got)
		}
	})

	t.Run("on target no errors", func(t *testing.T) {
		// 100 reqs averaging exactly latencyTarget → latencyRatio = 1, errorRate = 0
		r := ps.computeResistance(100, 0, int64(100*200*time.Millisecond))
		if math.Abs(r-1.0) > 1e-9 {
			t.Fatalf("want 1.0, got %v", r)
		}
	})

	t.Run("double latency", func(t *testing.T) {
		// 100 reqs averaging 2 * latencyTarget
		r := ps.computeResistance(100, 0, int64(100*400*time.Millisecond))
		if math.Abs(r-2.0) > 1e-9 {
			t.Fatalf("want 2.0, got %v", r)
		}
	})

	t.Run("errors contribute via errorWeight", func(t *testing.T) {
		// 100 reqs at target with 10% errors → 1.0 + 5.0*0.1 = 1.5
		r := ps.computeResistance(100, 10, int64(100*200*time.Millisecond))
		if math.Abs(r-1.5) > 1e-9 {
			t.Fatalf("want 1.5, got %v", r)
		}
	})
}

func TestPhysicsShedderUpdateBaselinePrimes(t *testing.T) {
	ps := &physicsShedder{}
	threshold := ps.updateBaseline(1.0)
	if !ps.ewmaPrimed {
		t.Fatal("expected primed after first update")
	}
	if ps.ewmaMu != 1.0 {
		t.Fatalf("want mu 1.0, got %v", ps.ewmaMu)
	}
	if ps.ewmaVar != 0 {
		t.Fatalf("want var 0 on prime, got %v", ps.ewmaVar)
	}
	if threshold != 1.0 {
		t.Fatalf("want threshold 1.0, got %v", threshold)
	}
}

func TestPhysicsShedderUpdateBaselineConverges(t *testing.T) {
	ps := &physicsShedder{}
	// Feed steady R=1.0 for many ticks; mu should converge to 1.0 and var to 0.
	for range 500 {
		ps.updateBaseline(1.0)
	}
	if math.Abs(ps.ewmaMu-1.0) > 1e-6 {
		t.Fatalf("mu not converged: %v", ps.ewmaMu)
	}
	if ps.ewmaVar > 1e-6 {
		t.Fatalf("var not converged: %v", ps.ewmaVar)
	}
}

func TestPhysicsShedderComputeRejectProbability(t *testing.T) {
	ps := &physicsShedder{}

	t.Run("warmup blocks shedding", func(t *testing.T) {
		p := ps.computeRejectProbability(10.0, 1.0, physicsWarmupTicks-1)
		if p != 0 {
			t.Fatalf("want 0 during warmup, got %v", p)
		}
	})

	t.Run("below threshold no shed", func(t *testing.T) {
		p := ps.computeRejectProbability(0.5, 1.0, physicsWarmupTicks)
		if p != 0 {
			t.Fatalf("want 0 below threshold, got %v", p)
		}
	})

	t.Run("above threshold sheds", func(t *testing.T) {
		// R=2, threshold=1 → (2-1)/2 = 0.5
		p := ps.computeRejectProbability(2.0, 1.0, physicsWarmupTicks)
		if math.Abs(p-0.5) > 1e-9 {
			t.Fatalf("want 0.5, got %v", p)
		}
	})

	t.Run("clamped to max", func(t *testing.T) {
		// Very high R should saturate at physicsMaxRejectProb
		p := ps.computeRejectProbability(1e9, 1.0, physicsWarmupTicks)
		if p != physicsMaxRejectProb {
			t.Fatalf("want %v, got %v", physicsMaxRejectProb, p)
		}
	})
}

// TestPhysicsShedderTickPipeline drives the tick loop directly and verifies
// the full pipeline (counters → ring buffer → R → EWMA → pReject).
func TestPhysicsShedderTickPipeline(t *testing.T) {
	ps := makeTestPhysicsShedder(t, "200ms")
	defer ps.Close()

	// Healthy phase: 100 reqs at 50ms (well below target) for windowSize ticks.
	// R ≈ 0.25, no errors, baseline primes low.
	for range ps.windowSize {
		ps.counter.Store(10)
		ps.errorCounter.Store(0)
		ps.latencyNs.Store(int64(10 * 50 * time.Millisecond))
		ps.runOneTick()
	}
	if ps.pReject() != 0 {
		t.Fatalf("healthy baseline should not shed, got p=%v", ps.pReject())
	}

	// Continue healthy for warmup to elapse.
	for ps.ticksSeen < physicsWarmupTicks {
		ps.counter.Store(10)
		ps.errorCounter.Store(0)
		ps.latencyNs.Store(int64(10 * 50 * time.Millisecond))
		ps.runOneTick()
	}
	if ps.pReject() != 0 {
		t.Fatalf("after warmup on healthy traffic should not shed, got p=%v", ps.pReject())
	}

	// Latency spike: 100 reqs averaging 2s (10x target) — pure gray failure.
	for range 5 {
		ps.counter.Store(10)
		ps.errorCounter.Store(0)
		ps.latencyNs.Store(int64(10 * 2 * time.Second))
		ps.runOneTick()
	}
	if ps.pReject() <= 0 {
		t.Fatalf("gray failure should shed, got p=%v", ps.pReject())
	}
	if ps.pReject() > physicsMaxRejectProb {
		t.Fatalf("p exceeds clamp: %v", ps.pReject())
	}
}

func TestPhysicsShedderErrorBurstSheds(t *testing.T) {
	ps := makeTestPhysicsShedder(t, "200ms")
	defer ps.Close()

	// Prime with healthy traffic past warmup.
	for ps.ticksSeen < physicsWarmupTicks {
		ps.counter.Store(10)
		ps.errorCounter.Store(0)
		ps.latencyNs.Store(int64(10 * 50 * time.Millisecond))
		ps.runOneTick()
	}

	// Fast 5xx burst: latency is fine but everything fails.
	for range 5 {
		ps.counter.Store(10)
		ps.errorCounter.Store(10)
		ps.latencyNs.Store(int64(10 * 50 * time.Millisecond))
		ps.runOneTick()
	}
	if ps.pReject() <= 0 {
		t.Fatalf("error burst should shed, got p=%v", ps.pReject())
	}
}

func TestPhysicsShedderRecoveryDropsRejects(t *testing.T) {
	ps := makeTestPhysicsShedder(t, "200ms")
	defer ps.Close()

	// Prime with healthy traffic past warmup.
	for ps.ticksSeen < physicsWarmupTicks {
		ps.counter.Store(10)
		ps.latencyNs.Store(int64(10 * 50 * time.Millisecond))
		ps.runOneTick()
	}

	// Spike.
	for range 5 {
		ps.counter.Store(10)
		ps.latencyNs.Store(int64(10 * 2 * time.Second))
		ps.runOneTick()
	}
	if ps.pReject() <= 0 {
		t.Fatalf("spike should shed first")
	}

	// Recovery: latency back to target for many ticks.
	for range 200 {
		ps.counter.Store(10)
		ps.latencyNs.Store(int64(10 * 50 * time.Millisecond))
		ps.runOneTick()
	}
	if ps.pReject() > 0 {
		t.Fatalf("expected 0 rejects after recovery, got p=%v", ps.pReject())
	}
}

// TestPhysicsShedderModesDoNotServe503 verifies that inactive/logInactive
// modes never serve a 503 even when shouldReject returns true.
func TestPhysicsShedderModesDoNotServe503(t *testing.T) {
	for _, md := range []mode{inactive, logInactive} {
		t.Run(md.String(), func(t *testing.T) {
			spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)
			f, err := spec.CreateFilter([]interface{}{"app", md.String(), "200ms"})
			if err != nil {
				t.Fatalf("CreateFilter: %v", err)
			}
			ps := f.(*physicsShedder)
			defer ps.Close()

			// Force the filter to "want to reject".
			ps.pRejectBits.Store(math.Float64bits(1.0))

			ctx := newFakeFilterContext()
			ps.Request(ctx)
			if ctx.FServed {
				t.Fatalf("%s mode should not serve a response", md)
			}
		})
	}
}

func TestPhysicsShedderActiveServes503(t *testing.T) {
	spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)
	f, err := spec.CreateFilter([]interface{}{"app", "active", "200ms"})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	ps := f.(*physicsShedder)
	defer ps.Close()

	ps.pRejectBits.Store(math.Float64bits(1.0))

	ctx := newFakeFilterContext()
	ps.Request(ctx)
	if !ctx.FServed {
		t.Fatal("active mode should serve a 503")
	}
	if ctx.FResponse.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", ctx.FResponse.StatusCode)
	}
	if ctx.FResponse.Header.Get(admissionSignalHeaderKey) != admissionSignalHeaderValue {
		t.Fatal("active mode must set Admission-Control header")
	}
	if ctx.FStateBag[physicsStateBagKey] != physicsStateBagReject {
		t.Fatal("state bag should mark reject")
	}
}

func TestPhysicsShedderResponseSkipsShortcut(t *testing.T) {
	ps := makeTestPhysicsShedder(t, "200ms")
	defer ps.Close()

	ctx := newFakeFilterContext()
	ctx.FStateBag[physicsStateBagKey] = physicsStateBagReject
	ctx.FResponse = &http.Response{StatusCode: 503, Header: http.Header{}}
	ps.Response(ctx)

	if ps.counter.Load() != 0 {
		t.Fatalf("shortcut response should not be counted, got %d", ps.counter.Load())
	}
}

func TestPhysicsShedderResponseSkipsUpstreamShedder(t *testing.T) {
	ps := makeTestPhysicsShedder(t, "200ms")
	defer ps.Close()

	ctx := newFakeFilterContext()
	ctx.FResponse = &http.Response{StatusCode: 503, Header: http.Header{}}
	ctx.FResponse.Header.Set(admissionSignalHeaderKey, admissionSignalHeaderValue)
	ps.Response(ctx)

	if ps.counter.Load() != 0 {
		t.Fatalf("upstream shedder response should not be counted, got %d", ps.counter.Load())
	}
}

func TestPhysicsShedderResponseCountsLatency(t *testing.T) {
	ps := makeTestPhysicsShedder(t, "200ms")
	defer ps.Close()

	ctx := newFakeFilterContext()
	ctx.FStateBag[physicsStartTimeKey] = time.Now().Add(-50 * time.Millisecond)
	ctx.FResponse = &http.Response{StatusCode: 200, Header: http.Header{}}
	ps.Response(ctx)

	if ps.counter.Load() != 1 {
		t.Fatalf("expected 1 req counted, got %d", ps.counter.Load())
	}
	if ps.latencyNs.Load() <= 0 {
		t.Fatal("expected latency counted from StateBag start time")
	}
}

func TestPhysicsShedderCreateFilterStartsAndStopsGoroutine(t *testing.T) {
	spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)
	f, err := spec.CreateFilter([]interface{}{"app", "inactive", "200ms"})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	ps := f.(*physicsShedder)

	// Let the tick goroutine run a bit then close.
	time.Sleep(3 * physicsTickDuration)
	if err := ps.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Double close must be safe.
	if err := ps.Close(); err != nil {
		t.Fatalf("Close again: %v", err)
	}
}

func TestPhysicsShedderPreProcessorDedupes(t *testing.T) {
	spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)
	pre := spec.PreProcessor()

	route := &eskip.Route{
		Filters: []*eskip.Filter{
			{Name: "setRequestHeader", Args: []interface{}{"X-A", "a"}},
			{Name: filters.PhysicsShedderName, Args: []interface{}{"a", "active", "200ms"}},
			{Name: filters.PhysicsShedderName, Args: []interface{}{"b", "active", "200ms"}},
			{Name: "setRequestHeader", Args: []interface{}{"X-B", "b"}},
			{Name: filters.PhysicsShedderName, Args: []interface{}{"c", "active", "200ms"}},
		},
	}
	out := pre.Do([]*eskip.Route{route})
	if len(out) != 1 {
		t.Fatalf("expected 1 route, got %d", len(out))
	}
	count := 0
	for _, f := range out[0].Filters {
		if f.Name == filters.PhysicsShedderName {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 physicsShedder remaining after dedupe, got %d", count)
	}
}

func TestPhysicsShedderPostProcessorClosesStale(t *testing.T) {
	spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)

	f1, _ := spec.CreateFilter([]interface{}{"a", "inactive", "200ms"})
	f2, _ := spec.CreateFilter([]interface{}{"b", "inactive", "200ms"})
	ps1 := f1.(*physicsShedder)
	ps2 := f2.(*physicsShedder)

	post := spec.PostProcessor()
	routes := []*routing.Route{
		{
			Route:   eskip.Route{Id: "r1"},
			Filters: []*routing.RouteFilter{{Filter: ps1, Name: filters.PhysicsShedderName}},
		},
	}
	post.Do(routes)

	// Replace r1's filter with ps2; postprocessor should close ps1.
	routes[0].Filters[0].Filter = ps2
	post.Do(routes)
	if !ps1.closed {
		t.Fatal("old filter should be closed after replacement")
	}

	// Drop the route entirely; postprocessor should close ps2.
	post.Do(nil)
	if !ps2.closed {
		t.Fatal("filter should be closed when route removed")
	}
}

// TestPhysicsShedderEndToEnd wires the filter into a real proxytest and
// checks a small traffic run completes successfully in inactive mode.
// The goal here is wiring/smoke, not algorithm correctness.
func TestPhysicsShedderEndToEnd(t *testing.T) {
	prev := metrics.Default
	m := &metricstest.MockMetrics{}
	metrics.Default = m
	t.Cleanup(func() { metrics.Default = prev; m.Close() })

	var backendHits int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&backendHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)

	args := []interface{}{"e2e", "inactive", "200ms"}
	route := &eskip.Route{
		Id:      "r1",
		Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}},
		Backend: backend.URL,
	}

	fr := make(filters.Registry)
	fr.Register(spec)
	fr.Register(builtin.NewSetRequestHeader())

	dc := testdataclient.New([]*eskip.Route{route})
	defer dc.Close()

	proxy := proxytest.WithRoutingOptions(fr, routing.Options{
		DataClients:    []routing.DataClient{dc},
		PreProcessors:  []routing.PreProcessor{spec.PreProcessor()},
		PostProcessors: []routing.PostProcessor{spec.PostProcessor()},
	})
	defer proxy.Close()

	client := proxy.Client()
	req, err := http.NewRequest("GET", proxy.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	const N = 50
	for range N {
		rsp, err := client.Do(req)
		if err != nil {
			t.Fatalf("roundtrip: %v", err)
		}
		rsp.Body.Close()
		if rsp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", rsp.StatusCode)
		}
	}

	if got := atomic.LoadInt64(&backendHits); got != N {
		t.Fatalf("expected all %d requests to hit backend, got %d", N, got)
	}

	var total int64
	m.WithCounters(func(c map[string]int64) {
		total = c["shedder.physics.total.e2e"]
	})
	if total != N {
		t.Fatalf("expected total counter %d, got %d", N, total)
	}
}

// TestPhysicsShedderConcurrentHotPath hammers Request/Response from many
// goroutines while the real tick goroutine runs. Intended to catch data
// races (go test -race) and to smoke-test lock contention on the reject
// path. It also forces pReject non-zero so the shed branch (StateBag +
// Serve) is exercised.
func TestPhysicsShedderConcurrentHotPath(t *testing.T) {
	spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)
	f, err := spec.CreateFilter([]interface{}{"race", "active", "200ms"})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	ps := f.(*physicsShedder)
	ps.metrics = nil // avoid metrics.Default side effects
	defer ps.Close()

	const workers = 16
	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Keep pReject non-zero so the shed branch (StateBag write + Serve)
	// races against the tick goroutine's pRejectBits.Store.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			ps.pRejectBits.Store(math.Float64bits(0.3))
			time.Sleep(time.Millisecond)
		}
	}()

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				ctx := newFakeFilterContext()
				ctx.FResponse = &http.Response{StatusCode: 200, Header: http.Header{}}
				ps.Request(ctx)
				if !ctx.FServed {
					ps.Response(ctx)
				}
			}
		}()
	}

	// 3 full tick cycles is enough to cover swap + ring buffer rotation.
	time.Sleep(3*physicsTickDuration + 50*time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestPhysicsShedderSustainedGrayFailure verifies shedding engages
// strongly when a backend turns slow-but-200. v1 has no scar/momentum
// term, so EWMA + 3-sigma eventually normalizes a sustained failure to
// the new baseline — by design, the filter focuses on the transition,
// not on punishing a steady state. This test asserts that the transition
// shedding is strong (peak ≥ 0.5) and lasts long enough to matter
// (≥ 10 ticks above 0.1), which is what catches a degrading deployment.
func TestPhysicsShedderSustainedGrayFailure(t *testing.T) {
	ps := makeTestPhysicsShedder(t, "200ms", "500ms") // 5-bucket window
	defer ps.Close()

	// Warmup at healthy 50ms latency.
	for ps.ticksSeen < physicsWarmupTicks {
		ps.counter.Store(10)
		ps.latencyNs.Store(int64(10 * 50 * time.Millisecond))
		ps.runOneTick()
	}
	if ps.pReject() != 0 {
		t.Fatalf("warmup should leave pReject=0, got %v", ps.pReject())
	}

	// Gray failure: latency jumps to 5x target (1s vs 200ms).
	var peakP float64
	engagedTicks := 0
	for range 100 {
		ps.counter.Store(10)
		ps.latencyNs.Store(int64(10 * 1 * time.Second))
		ps.runOneTick()
		p := ps.pReject()
		if p > peakP {
			peakP = p
		}
		if p > 0.1 {
			engagedTicks++
		}
	}
	if peakP < 0.5 {
		t.Fatalf("expected peak shedding >= 0.5 during gray failure, got %v", peakP)
	}
	if engagedTicks < 10 {
		t.Fatalf("expected >=10 ticks of pReject>0.1 during gray failure, got %d", engagedTicks)
	}
}

// TestPhysicsShedderBaselineAdaptsToNewNormal verifies the opposite side
// of the tradeoff: when the "new normal" is still healthy (below target),
// shedding from the transient eventually abates as EWMA catches up.
func TestPhysicsShedderBaselineAdaptsToNewNormal(t *testing.T) {
	ps := makeTestPhysicsShedder(t, "200ms")
	defer ps.Close()

	// Warmup at 50ms (R ≈ 0.25).
	for ps.ticksSeen < physicsWarmupTicks {
		ps.counter.Store(10)
		ps.latencyNs.Store(int64(10 * 50 * time.Millisecond))
		ps.runOneTick()
	}

	// Step up to a new, slower but still-healthy baseline of 150ms
	// (R ≈ 0.75, still below target). Run enough ticks for EWMA to
	// track (~5x time constant).
	for range 500 {
		ps.counter.Store(10)
		ps.latencyNs.Store(int64(10 * 150 * time.Millisecond))
		ps.runOneTick()
	}

	if ps.ewmaMu < 0.6 {
		t.Fatalf("expected mu to track up toward 0.75, got %v", ps.ewmaMu)
	}
	if ps.pReject() > 0.05 {
		t.Fatalf("shedding should abate after baseline adapts, got p=%v", ps.pReject())
	}
}

// TestPhysicsShedderMetrics verifies every metric the filter emits has the
// right name and value. Downstream dashboards depend on these exact keys.
func TestPhysicsShedderMetrics(t *testing.T) {
	t.Run("active mode increments reject counter", func(t *testing.T) {
		spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)
		f, err := spec.CreateFilter([]interface{}{"m", "active", "200ms"})
		if err != nil {
			t.Fatalf("CreateFilter: %v", err)
		}
		ps := f.(*physicsShedder)
		m := &metricstest.MockMetrics{}
		ps.metrics = m
		defer ps.Close()

		ps.pRejectBits.Store(math.Float64bits(1.0))
		ctx := newFakeFilterContext()
		ps.Request(ctx)

		m.WithCounters(func(c map[string]int64) {
			if got := c["shedder.physics.total.m"]; got != 1 {
				t.Errorf("total: got %d, want 1", got)
			}
			if got := c["shedder.physics.reject.m"]; got != 1 {
				t.Errorf("reject: got %d, want 1", got)
			}
			if got := c["shedder.physics.would_reject.m"]; got != 0 {
				t.Errorf("would_reject in active mode: got %d, want 0", got)
			}
		})
	})

	t.Run("inactive mode increments would_reject counter", func(t *testing.T) {
		spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)
		f, err := spec.CreateFilter([]interface{}{"m", "inactive", "200ms"})
		if err != nil {
			t.Fatalf("CreateFilter: %v", err)
		}
		ps := f.(*physicsShedder)
		m := &metricstest.MockMetrics{}
		ps.metrics = m
		defer ps.Close()

		ps.pRejectBits.Store(math.Float64bits(1.0))
		ctx := newFakeFilterContext()
		ps.Request(ctx)

		m.WithCounters(func(c map[string]int64) {
			if got := c["shedder.physics.would_reject.m"]; got != 1 {
				t.Errorf("would_reject: got %d, want 1", got)
			}
			if got := c["shedder.physics.reject.m"]; got != 0 {
				t.Errorf("reject in inactive mode: got %d, want 0", got)
			}
		})
	})

	t.Run("gauges emit resistance baseline threshold", func(t *testing.T) {
		ps := makeTestPhysicsShedder(t, "200ms")
		defer ps.Close()
		m := ps.metrics.(*metricstest.MockMetrics)

		ps.counter.Store(10)
		ps.latencyNs.Store(int64(10 * 100 * time.Millisecond))
		ps.runOneTick()

		m.WithGauges(func(g map[string]float64) {
			if _, ok := g["shedder.physics.resistance.test"]; !ok {
				t.Error("resistance gauge missing")
			}
			if _, ok := g["shedder.physics.baseline.test"]; !ok {
				t.Error("baseline gauge missing")
			}
			if _, ok := g["shedder.physics.threshold.test"]; !ok {
				t.Error("threshold gauge missing")
			}
			// After one tick with R = 100ms/200ms = 0.5, mu primes to 0.5.
			if got := g["shedder.physics.resistance.test"]; math.Abs(got-0.5) > 1e-9 {
				t.Errorf("resistance: got %v, want 0.5", got)
			}
			if got := g["shedder.physics.baseline.test"]; math.Abs(got-0.5) > 1e-9 {
				t.Errorf("baseline: got %v, want 0.5", got)
			}
		})
	})
}

// TestPhysicsShedderWindowRotation verifies that the ring buffer actually
// evicts old data when it wraps. A stale spike stuck in the buffer would
// be silent in every other test but disastrous in production.
func TestPhysicsShedderWindowRotation(t *testing.T) {
	ps := makeTestPhysicsShedder(t, "200ms", "500ms") // 5-bucket window
	defer ps.Close()

	// Fill the whole window with 1s-latency spike.
	for range ps.windowSize {
		ps.counter.Store(10)
		ps.latencyNs.Store(int64(10 * 1 * time.Second))
		ps.runOneTick()
	}
	ps.mu.Lock()
	r1 := ps.computeResistance(sum(ps.totals), sum(ps.errors), sum(ps.latencySumNs))
	ps.mu.Unlock()
	if math.Abs(r1-5.0) > 1e-9 {
		t.Fatalf("window full of spike: want R=5.0, got %v", r1)
	}

	// Fill the window with healthy 50ms traffic — old spike must be gone.
	for range ps.windowSize {
		ps.counter.Store(10)
		ps.latencyNs.Store(int64(10 * 50 * time.Millisecond))
		ps.runOneTick()
	}
	ps.mu.Lock()
	r2 := ps.computeResistance(sum(ps.totals), sum(ps.errors), sum(ps.latencySumNs))
	ps.mu.Unlock()
	if math.Abs(r2-0.25) > 1e-9 {
		t.Fatalf("window refilled healthy: want R=0.25 (old spike evicted), got %v", r2)
	}
}

// TestPhysicsShedderResponseCountsErrors checks that only 5xx responses
// feed the error arm of R. 2xx/3xx/4xx increment the total counter but
// not the error counter.
func TestPhysicsShedderResponseCountsErrors(t *testing.T) {
	cases := []struct {
		status     int
		wantErrors int64
	}{
		{200, 0},
		{302, 0},
		{404, 0},
		{499, 0},
		{500, 1},
		{502, 1},
		{503, 1},
		{504, 1},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			ps := makeTestPhysicsShedder(t, "200ms")
			defer ps.Close()
			ctx := newFakeFilterContext()
			ctx.FResponse = &http.Response{StatusCode: tc.status, Header: http.Header{}}
			ps.Response(ctx)
			if got := ps.counter.Load(); got != 1 {
				t.Fatalf("status %d: total got %d, want 1", tc.status, got)
			}
			if got := ps.errorCounter.Load(); got != tc.wantErrors {
				t.Fatalf("status %d: errors got %d, want %d", tc.status, got, tc.wantErrors)
			}
		})
	}
}

// FuzzPhysicsShedderMath drives computeResistance, updateBaseline, and
// computeRejectProbability with arbitrary non-negative inputs. Asserts
// outputs stay finite and pReject stays in [0, physicsMaxRejectProb].
func FuzzPhysicsShedderMath(f *testing.F) {
	f.Add(int64(100), int64(10), int64(20_000_000_000), 100)
	f.Add(int64(0), int64(0), int64(0), 0)
	f.Add(int64(1), int64(1), int64(1), physicsWarmupTicks)
	f.Add(int64(1_000_000), int64(0), int64(math.MaxInt32), 1000)
	f.Fuzz(func(t *testing.T, sumReqs, sumErrs, sumLatNs int64, ticksSeen int) {
		// Production inputs are all non-negative; skip invalid shapes.
		if sumReqs < 0 || sumErrs < 0 || sumLatNs < 0 || ticksSeen < 0 {
			t.Skip()
		}
		ps := &physicsShedder{latencyTarget: 200 * time.Millisecond}
		r := ps.computeResistance(sumReqs, sumErrs, sumLatNs)
		if math.IsNaN(r) || math.IsInf(r, 0) {
			t.Fatalf("R non-finite: reqs=%d errs=%d lat=%d → %v", sumReqs, sumErrs, sumLatNs, r)
		}
		threshold := ps.updateBaseline(r)
		if math.IsNaN(threshold) || math.IsInf(threshold, 0) {
			t.Fatalf("threshold non-finite: R=%v → %v", r, threshold)
		}
		p := ps.computeRejectProbability(r, threshold, ticksSeen)
		if math.IsNaN(p) {
			t.Fatalf("pReject NaN: R=%v threshold=%v", r, threshold)
		}
		if p < 0 || p > physicsMaxRejectProb {
			t.Fatalf("pReject %v out of range [0, %v]", p, physicsMaxRejectProb)
		}
	})
}

// TestPhysicsShedderInvariants runs randomized traffic for many ticks and
// asserts pipeline invariants always hold: pReject is in [0, max] and
// shedding never activates during warmup. Complements the fuzz test by
// exercising sequential state (EWMA + ring buffer).
func TestPhysicsShedderInvariants(t *testing.T) {
	ps := makeTestPhysicsShedder(t, "200ms")
	defer ps.Close()

	rng := mrand.New(mrand.NewPCG(1, 2))
	for range 2000 {
		reqs := int64(rng.IntN(100))
		var errs int64
		if reqs > 0 {
			errs = int64(rng.IntN(int(reqs) + 1))
		}
		latPerReq := time.Duration(rng.Int64N(int64(2 * time.Second)))
		ps.counter.Store(reqs)
		ps.errorCounter.Store(errs)
		ps.latencyNs.Store(reqs * int64(latPerReq))
		ps.runOneTick()

		p := ps.pReject()
		if math.IsNaN(p) {
			t.Fatalf("pReject NaN at tick %d", ps.ticksSeen)
		}
		if p < 0 || p > physicsMaxRejectProb {
			t.Fatalf("pReject %v out of range at tick %d", p, ps.ticksSeen)
		}
		if ps.ticksSeen < physicsWarmupTicks && p != 0 {
			t.Fatalf("shed during warmup at tick %d: p=%v", ps.ticksSeen, p)
		}
	}
}

// TestPhysicsShedderTracingOnReject verifies that rejecting a request
// emits a physics_shedder span with the expected tags (including error).
func TestPhysicsShedderTracingOnReject(t *testing.T) {
	tracer := tracingtest.NewTracer()
	spec := NewPhysicsShedder(PhysicsShedderOptions{Tracer: tracer, testRand: true}).(*PhysicsShedderSpec)
	f, err := spec.CreateFilter([]interface{}{"trace", "active", "200ms"})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	ps := f.(*physicsShedder)
	ps.metrics = &metricstest.MockMetrics{}
	defer ps.Close()

	ps.pRejectBits.Store(math.Float64bits(1.0))

	parent := tracer.StartSpan("parent")
	req, _ := http.NewRequest("GET", "http://example/", nil)
	req = req.WithContext(opentracing.ContextWithSpan(req.Context(), parent))
	ctx := &filtertest.Context{FRequest: req, FStateBag: map[string]interface{}{}}

	ps.Request(ctx)
	parent.Finish()

	span := tracer.FindSpan(physicsSpanName)
	if span == nil {
		t.Fatal("physics_shedder span not created")
	}
	tags := span.Tags()
	wantTags := map[string]interface{}{
		"component":                    "skipper",
		"mode":                         "active",
		"physicsShedder.group":         "trace",
		"physicsShedder.mode":          "active",
		"physicsShedder.latencyTarget": "200ms",
		"physicsShedder.pReject":       1.0,
		"error":                        true,
	}
	for k, want := range wantTags {
		got, ok := tags[k]
		if !ok {
			t.Errorf("tag %q missing", k)
			continue
		}
		if got != want {
			t.Errorf("tag %q: got %v, want %v", k, got, want)
		}
	}
}

// TestPhysicsShedderTracingNoShedNoErrorTag verifies that when pReject is
// zero, the span is created but the error tag is NOT set.
func TestPhysicsShedderTracingNoShedNoErrorTag(t *testing.T) {
	tracer := tracingtest.NewTracer()
	spec := NewPhysicsShedder(PhysicsShedderOptions{Tracer: tracer, testRand: true}).(*PhysicsShedderSpec)
	f, err := spec.CreateFilter([]interface{}{"trace", "active", "200ms"})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	ps := f.(*physicsShedder)
	ps.metrics = &metricstest.MockMetrics{}
	defer ps.Close()

	parent := tracer.StartSpan("parent")
	req, _ := http.NewRequest("GET", "http://example/", nil)
	req = req.WithContext(opentracing.ContextWithSpan(req.Context(), parent))
	ctx := &filtertest.Context{FRequest: req, FStateBag: map[string]interface{}{}}

	ps.Request(ctx)
	parent.Finish()

	span := tracer.FindSpan(physicsSpanName)
	if span == nil {
		t.Fatal("physics_shedder span not created")
	}
	if _, hasErr := span.Tags()["error"]; hasErr {
		t.Error("error tag should not be set when not rejecting")
	}
	if got := span.Tags()["physicsShedder.pReject"]; got != 0.0 {
		t.Errorf("pReject tag: got %v, want 0", got)
	}
}

// TestPhysicsShedderTracingNoParentNoSpan verifies that without a parent
// span in the request context, physicsShedder creates no span (avoiding
// orphaned spans and noise in traces).
func TestPhysicsShedderTracingNoParentNoSpan(t *testing.T) {
	tracer := tracingtest.NewTracer()
	spec := NewPhysicsShedder(PhysicsShedderOptions{Tracer: tracer, testRand: true}).(*PhysicsShedderSpec)
	f, err := spec.CreateFilter([]interface{}{"trace", "inactive", "200ms"})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	ps := f.(*physicsShedder)
	ps.metrics = &metricstest.MockMetrics{}
	defer ps.Close()

	ctx := newFakeFilterContext()
	ps.Request(ctx)

	finished := tracer.FinishedSpans()
	if len(finished) != 0 {
		t.Fatalf("expected no spans without parent, got %d", len(finished))
	}
}

// TestPhysicsShedderCoexistsWithAdmissionControl verifies that both
// filters can share a route, wired with their own pre/post processors,
// without breaking traffic. Healthy traffic flows through and each
// filter's counters advance independently.
func TestPhysicsShedderCoexistsWithAdmissionControl(t *testing.T) {
	prev := metrics.Default
	m := &metricstest.MockMetrics{}
	metrics.Default = m
	t.Cleanup(func() { metrics.Default = prev; m.Close() })

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	physSpec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)
	acSpec := NewAdmissionControl(Options{testRand: true}).(*AdmissionControlSpec)

	route := &eskip.Route{
		Id: "r1",
		Filters: []*eskip.Filter{
			{Name: filters.AdmissionControlName, Args: []interface{}{
				"ac", "inactive", "100ms", 5, 10, 0.9, 0.95, 1.0,
			}},
			{Name: filters.PhysicsShedderName, Args: []interface{}{
				"ph", "inactive", "200ms",
			}},
		},
		Backend: backend.URL,
	}

	fr := make(filters.Registry)
	fr.Register(physSpec)
	fr.Register(acSpec)
	fr.Register(builtin.NewSetRequestHeader())

	dc := testdataclient.New([]*eskip.Route{route})
	defer dc.Close()

	proxy := proxytest.WithRoutingOptions(fr, routing.Options{
		DataClients: []routing.DataClient{dc},
		PreProcessors: []routing.PreProcessor{
			physSpec.PreProcessor(),
			acSpec.PreProcessor(),
		},
		PostProcessors: []routing.PostProcessor{
			physSpec.PostProcessor(),
			acSpec.PostProcessor(),
		},
	})
	defer proxy.Close()

	client := proxy.Client()
	req, err := http.NewRequest("GET", proxy.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	const N = 20
	for range N {
		rsp, err := client.Do(req)
		if err != nil {
			t.Fatalf("roundtrip: %v", err)
		}
		rsp.Body.Close()
		if rsp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", rsp.StatusCode)
		}
	}

	var physTotal, acTotal int64
	m.WithCounters(func(c map[string]int64) {
		physTotal = c["shedder.physics.total.ph"]
		acTotal = c["shedder.admission_control.total.ac"]
	})
	if physTotal != N {
		t.Errorf("physics total: got %d, want %d", physTotal, N)
	}
	if acTotal == 0 {
		t.Errorf("admissionControl total: got 0, expected > 0 (filters may not both have run)")
	}
}

// --- helpers --------------------------------------------------------------

// makeTestPhysicsShedder builds a filter without starting its tick goroutine
// and attaches a mock metrics sink. Tests drive ticks manually via runOneTick
// for determinism. An optional window argument overrides the default (5s).
func makeTestPhysicsShedder(t *testing.T, latencyTarget string, window ...string) *physicsShedder {
	t.Helper()
	spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)
	args := []interface{}{"test", "inactive", latencyTarget}
	if len(window) == 1 {
		args = append(args, window[0])
	}
	f, err := spec.CreateFilter(args)
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	ps := f.(*physicsShedder)
	ps.metrics = &metricstest.MockMetrics{}
	// Stop the auto tick goroutine so we can drive it deterministically.
	// Close() now blocks until the goroutine actually exits, so it's safe
	// to replace quit/done here without racing.
	ps.Close()
	ps.closed = false
	ps.quit = make(chan struct{})
	ps.done = make(chan struct{})
	close(ps.done) // no goroutine running; make Close() a no-op wait.
	ps.once = sync.Once{}
	return ps
}

// runOneTick performs exactly one tick cycle — swapping counters into the
// ring buffer and recomputing R, baseline, and reject probability.
func (ps *physicsShedder) runOneTick() {
	reqs := ps.counter.Swap(0)
	errs := ps.errorCounter.Swap(0)
	lat := ps.latencyNs.Swap(0)

	ps.mu.Lock()
	ps.totals[ps.bucketIdx] = reqs
	ps.errors[ps.bucketIdx] = errs
	ps.latencySumNs[ps.bucketIdx] = lat
	ps.bucketIdx = (ps.bucketIdx + 1) % ps.windowSize
	if ps.ticksSeen < math.MaxInt32 {
		ps.ticksSeen++
	}
	sumReqs := sum(ps.totals)
	sumErrs := sum(ps.errors)
	sumLat := sum(ps.latencySumNs)
	ticksSeen := ps.ticksSeen
	ps.mu.Unlock()

	r := ps.computeResistance(sumReqs, sumErrs, sumLat)
	threshold := ps.updateBaseline(r)
	rejectP := ps.computeRejectProbability(r, threshold, ticksSeen)
	ps.pRejectBits.Store(math.Float64bits(rejectP))
	ps.publishGauges(r, ps.ewmaMu, threshold)
}

func newFakeFilterContext() *filtertest.Context {
	req, _ := http.NewRequest("GET", "http://example/", nil)
	return &filtertest.Context{
		FRequest:  req,
		FStateBag: map[string]interface{}{},
	}
}

// --- benchmarks -----------------------------------------------------------

// BenchmarkPhysicsShedderHotPath measures Request+Response under concurrent
// load. The filter is configured in inactive mode so no 503s are served,
// isolating the counter/atomic cost.
func BenchmarkPhysicsShedderHotPath(b *testing.B) {
	spec := NewPhysicsShedder(PhysicsShedderOptions{testRand: true}).(*PhysicsShedderSpec)
	f, err := spec.CreateFilter([]interface{}{"bench", "inactive", "200ms"})
	if err != nil {
		b.Fatalf("CreateFilter: %v", err)
	}
	ps := f.(*physicsShedder)
	defer ps.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := newFakeFilterContext()
		ctx.FResponse = &http.Response{StatusCode: 200, Header: http.Header{}}
		for pb.Next() {
			ps.Request(ctx)
			ps.Response(ctx)
		}
	})
}

// BenchmarkPhysicsShedderTickLoop measures the cost of one tick-cycle
// (swap + ring buffer + R + EWMA + probability publish).
func BenchmarkPhysicsShedderTickLoop(b *testing.B) {
	ps := &physicsShedder{
		windowSize:    50,
		latencyTarget: 200 * time.Millisecond,
		totals:        make([]int64, 50),
		errors:        make([]int64, 50),
		latencySumNs:  make([]int64, 50),
	}
	// Pre-fill counters so each tick has work to do.
	ps.counter.Store(1000)
	ps.latencyNs.Store(int64(1000 * 100 * time.Millisecond))

	b.ResetTimer()
	for range b.N {
		ps.counter.Store(1000)
		ps.latencyNs.Store(int64(1000 * 100 * time.Millisecond))
		ps.runOneTick()
	}
}
