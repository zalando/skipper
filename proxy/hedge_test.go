package proxy_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	hedgefilters "github.com/zalando/skipper/filters/hedge"
	"github.com/zalando/skipper/filters/tee"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/predicates/primitive"
	teepred "github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/predicates/traffic"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
)

const hedgeDelay = 50 * time.Millisecond

type hedgeBackend struct {
	*httptest.Server
	hits atomic.Int64
}

// newHedgeBackend serves name as body. The path /slow takes slowFor, the
// path /err responds with 500, and the delay is cut short when the request is cancelled.
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
}

func newHedgeProxy(t *testing.T, doc string, hp *proxy.HedgeParams) *hedgeSetup {
	t.Helper()

	m := &metricstest.MockMetrics{}
	fr := make(filters.Registry)
	fr.Register(hedgefilters.NewDisableHedge())
	routes := eskip.MustParse(doc)
	p := proxytest.WithParams(fr, proxy.Params{
		CloseIdleConnsPeriod: -time.Second,
		Metrics:              m,
		Hedge:                hp,
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
	for range 120 {
		wg.Go(func() {
			s.get(t, "/")
		})
	}
	wg.Wait()
}

func (s *hedgeSetup) counter(route, name string) int64 {
	v, _ := s.metrics.Counter(fmt.Sprintf("hedge.%s.%s", route, name))
	return v
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
		Hedge:                &proxy.HedgeParams{FixedDelay: hedgeDelay},
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
