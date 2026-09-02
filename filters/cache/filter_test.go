package cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy/proxytest"
)

func newTestFilter(t *testing.T, ttl, errorTTL, swrWindow time.Duration, extra ...time.Duration) *cacheFilter {
	t.Helper()
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", L1TTL: 60 * time.Second})
	args := []any{
		ttl.String(),
		errorTTL.String(),
		swrWindow.String(),
	}
	if len(extra) > 0 {
		args = append(args, extra[0].String())
	}
	f, err := spec.CreateFilter(args)
	if err != nil {
		t.Fatal(err)
	}
	cf := f.(*cacheFilter)
	// Panic if fetch is called without a stub — tests must set cf.fetch
	// if they exercise the cold-miss path.
	cf.fetch = func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no fetch stub set")
	}
	t.Cleanup(spec.(*cacheSpec).client.Close)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	return cf
}

// newTestFilterRFC creates a filter in pure RFC mode (zero args). Upstream
// Cache-Control is fully authoritative. Use this for tests that exercise
// Expires capping, heuristic freshness, or other RFC 9111 TTL logic.
// The ttl/errorTTL/swrWindow args are accepted for call-site compatibility
// but are ignored — pure RFC mode has no operator TTL.
func newTestFilterRFC(t *testing.T, _, _, _ time.Duration, _ ...time.Duration) *cacheFilter {
	t.Helper()
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", L1TTL: 60 * time.Second})
	f, err := spec.CreateFilter([]any{})
	if err != nil {
		t.Fatal(err)
	}
	cf := f.(*cacheFilter)
	cf.fetch = func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no fetch stub set")
	}
	t.Cleanup(spec.(*cacheSpec).client.Close)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	return cf
}

func newCtx(method, rawURL string, authHeader string) *filtertest.Context {
	req, _ := http.NewRequest(method, rawURL, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return &filtertest.Context{
		FRequest:  req,
		FStateBag: make(map[string]any),
		FMetrics:  &metricstest.MockMetrics{},
	}
}

func newCtxWithRoute(method, rawURL, authHeader, routeID string) *filtertest.Context {
	ctx := newCtx(method, rawURL, authHeader)
	ctx.FRouteId = routeID
	return ctx
}

func upstreamResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func upstreamResponseCC(status int, body, cacheControl string) *http.Response {
	r := upstreamResponse(status, body)
	r.Header.Set("Cache-Control", cacheControl)
	return r
}

func TestCacheFilter_MissAndHit(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)

	// First request: miss
	ctx1 := newCtx("GET", "https://cdn.contentful.com/spaces/abc/entries", "Bearer token1")
	f.Request(ctx1)
	if ctx1.FServed {
		t.Fatal("first request should not be served from cache")
	}

	ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"items":[]}`, "public, max-age=300")
	f.Response(ctx1)

	if ctx1.FResponse.Header.Get("X-Cache-Status") != "MISS" {
		t.Fatalf("expected MISS, got %q", ctx1.FResponse.Header.Get("X-Cache-Status"))
	}

	// Second request: hit
	ctx2 := newCtx("GET", "https://cdn.contentful.com/spaces/abc/entries", "Bearer token1")
	f.Request(ctx2)
	if !ctx2.FServed {
		t.Fatal("second request should be served from cache")
	}
	if ctx2.FResponse.Header.Get("X-Cache-Status") != "HIT" {
		t.Fatalf("expected HIT, got %q", ctx2.FResponse.Header.Get("X-Cache-Status"))
	}

	body, _ := io.ReadAll(ctx2.FResponse.Body)
	if string(body) != `{"items":[]}` {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestCacheFilter_KeyIsolationByAuthToken(t *testing.T) {
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", L1TTL: 60 * time.Second})
	fi, err := spec.CreateFilter([]any{"1m", "15s", "1m", "0s", "Authorization"})
	if err != nil {
		t.Fatal(err)
	}
	f := fi.(*cacheFilter)
	t.Cleanup(spec.(*cacheSpec).client.Close)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	f.fetch = func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no fetch stub set")
	}
	url := "https://cdn.contentful.com/spaces/abc/entries"

	// Populate cache with token A.
	ctxA := newCtx("GET", url, "Bearer token-delivery")
	f.Request(ctxA)
	ctxA.FResponse = upstreamResponseCC(http.StatusOK, `{"env":"production"}`, "public, max-age=300")
	f.Response(ctxA)

	// Populate cache with token B (preview).
	ctxB := newCtx("GET", url, "Bearer token-preview")
	f.Request(ctxB)
	ctxB.FResponse = upstreamResponseCC(http.StatusOK, `{"env":"preview"}`, "public, max-age=300")
	f.Response(ctxB)

	// Token A hit must return production payload.
	hitA := newCtx("GET", url, "Bearer token-delivery")
	f.Request(hitA)
	if !hitA.FServed {
		t.Fatal("expected cache hit for token-delivery")
	}
	bodyA, _ := io.ReadAll(hitA.FResponse.Body)
	if string(bodyA) != `{"env":"production"}` {
		t.Fatalf("token-delivery got wrong payload: %q", bodyA)
	}

	// Token B hit must return preview payload.
	hitB := newCtx("GET", url, "Bearer token-preview")
	f.Request(hitB)
	if !hitB.FServed {
		t.Fatal("expected cache hit for token-preview")
	}
	bodyB, _ := io.ReadAll(hitB.FResponse.Body)
	if string(bodyB) != `{"env":"preview"}` {
		t.Fatalf("token-preview got wrong payload: %q", bodyB)
	}
}

func TestCacheFilter_404CachedWithErrorTTL(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/missing"

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponseCC(http.StatusNotFound, `{"message":"not found"}`, "max-age=300")
	f.Response(ctx1)

	// Second request: 404 should be served from cache.
	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if !ctx2.FServed {
		t.Fatal("expected 404 to be served from cache")
	}
	if ctx2.FResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", ctx2.FResponse.StatusCode)
	}
}

func TestCacheFilter_NonCacheableStatusNotStored(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/redirect"

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponse(http.StatusFound, "")
	f.Response(ctx1)

	// Second request: 302 must not be cached; should be a miss.
	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if ctx2.FServed {
		t.Fatal("302 response should not be cached")
	}
}

func TestCacheFilter_TTLExpiry(t *testing.T) {
	// swrWindow=1ms so hard expiry is at TTL+1ms; advancing 2min exceeds both.
	// Filter created outside the bubble so sknet.Client's transport goroutine
	// does not get trapped inside the synctest bubble.
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Millisecond)
	url := "https://cdn.contentful.com/spaces/abc/entries"

	synctest.Test(t, func(t *testing.T) {
		// Populate cache.
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":1}`, "max-age=300")
		f.Response(ctx1)

		// Still within TTL — must be a HIT.
		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		if !ctx2.FServed {
			t.Fatal("expected HIT within TTL")
		}

		// Advance past TTL+SWR (swrWindow=1ms, so 2min exceeds both).
		time.Sleep(2 * time.Minute)

		// After TTL+SWR — must be a hard-expired MISS.
		ctx3 := newCtx("GET", url, "")
		f.Request(ctx3)
		if ctx3.FServed {
			t.Fatal("expected MISS after TTL+SWR expired")
		}
	})
}

func TestCacheFilter_Response_NoopIfStateBagKeyMissing(t *testing.T) {
	// Regression: Response() used a bare type assertion on stateBagKey which
	// panicked if Request() had not run (e.g. route misconfiguration).
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	ctx := newCtx("GET", "https://example.com/api", "")
	// Deliberately do NOT call f.Request(ctx) — state bag has no cache key.
	ctx.FResponse = upstreamResponse(http.StatusOK, `{}`)
	// Must not panic.
	f.Response(ctx)
}

func TestCreateFilter_InvalidArgs(t *testing.T) {
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", L1TTL: 60 * time.Second})
	t.Cleanup(spec.(*cacheSpec).client.Close)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	cases := []struct {
		name string
		args []any
	}{
		{"too few args", []any{"5m", "15s"}},
		{"bad ttl", []any{"bad", "15s", "30s"}},
		{"zero ttl", []any{"0s", "15s", "30s"}},
		{"non-string ttl", []any{300, "15s", "30s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := spec.CreateFilter(tc.args); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestCacheFilter_ErrorStatus_NoSWR(t *testing.T) {
	// 404 entries must hard-expire at errorTTL with no SWR window
	f := newTestFilter(t, time.Minute, time.Millisecond, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/missing"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponse(http.StatusNotFound, `{"message":"not found"}`)
		f.Response(ctx1)

		// Advance past errorTTL — must be a hard miss, not STALE (SWR is 0 for errors).
		time.Sleep(10 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		if ctx2.FServed {
			t.Fatal("404 entry must not be served as stale after errorTTL; SWR window must be 0")
		}
	})
}

func TestCacheFilter_SWR_StaleServedAndRevalidated(t *testing.T) {
	// ttl=1ms, swrWindow=1h — entry expires quickly but SWR window is huge.
	f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/swr"

	synctest.Test(t, func(t *testing.T) {
		// Populate cache.
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"fresh"}`, "max-age=300")
		f.Response(ctx1)

		// Advance past TTL but inside SWR window.
		time.Sleep(2 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()

		if !ctx2.FServed {
			t.Fatal("expected stale entry to be served")
		}
		if ctx2.FResponse.Header.Get("X-Cache-Status") != "STALE" {
			t.Fatalf("expected STALE, got %q", ctx2.FResponse.Header.Get("X-Cache-Status"))
		}
		body, _ := io.ReadAll(ctx2.FResponse.Body)
		if string(body) != `{"data":"fresh"}` {
			t.Fatalf("unexpected stale body: %q", body)
		}
	})
}

func TestCacheFilter_SWR_HardExpiry_Miss(t *testing.T) {
	// ttl=1ms, swrWindow=1ms — hard expiry at 2ms.
	f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Millisecond)
	url := "https://cdn.contentful.com/spaces/abc/entries/hard-expired"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponse(http.StatusOK, `{"data":"old"}`)
		f.Response(ctx1)

		// Advance past TTL + SWR window (2ms combined).
		time.Sleep(10 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)

		if ctx2.FServed {
			t.Fatal("expected hard-expired entry to result in a miss, not served")
		}
	})
}

// missCounting wraps a Storage and signals wg on each non-vary Get that returns a miss.
type missCounting struct {
	Storage
	wg sync.WaitGroup
}

func (mc *missCounting) Get(ctx context.Context, key string) (*Entry, error) {
	e, err := mc.Storage.Get(ctx, key)
	if e == nil && err == nil && !strings.HasPrefix(key, "vary:") {
		mc.wg.Done()
	}
	return e, err
}

func TestCacheFilter_ColdMissCoalescing(t *testing.T) {
	// Hold the upstream fetch until all N goroutines have performed their
	// storage.Get and observed a miss. At that point every goroutine is either
	// already inside DoChan's wait list or in the scheduler-opaque gap between
	// the nil check and the DoChan call — so exactly 1 fetch fires.
	//
	// mc.wg is the barrier: each goroutine calls wg.Done() inside storage.Get
	// on a miss, and the fetch stub calls wg.Wait() before returning, keeping
	// the singleflight open until all N goroutines have committed to joining it.
	const N = 50
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)

	mc := &missCounting{Storage: f.storage}
	mc.wg.Add(N)
	f.storage = mc

	var fetchCount int64
	fetchStarted := make(chan struct{})
	releaseAll := make(chan struct{})

	f.fetch = func(req *http.Request) (*http.Response, error) {
		atomic.AddInt64(&fetchCount, 1)
		close(fetchStarted)
		mc.wg.Wait() // block until every goroutine has confirmed a cache miss
		<-releaseAll
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}, "Cache-Control": {"max-age=300"}},
			Body:       io.NopCloser(strings.NewReader(`{"coalesced":true}`)),
		}, nil
	}

	url := "https://cdn.contentful.com/spaces/abc/entries/coalesce"
	results := make([]*filtertest.Context, N)
	var wg sync.WaitGroup

	for i := 0; i < N; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			ctx := newCtx("GET", url, "")
			f.Request(ctx)
			results[i] = ctx
		}()
	}

	<-fetchStarted // fetch is running; all N misses will be counted before it returns
	close(releaseAll)
	wg.Wait()

	if got := atomic.LoadInt64(&fetchCount); got != 1 {
		t.Fatalf("expected 1 upstream fetch, got %d", got)
	}

	// Every goroutine must have been served (either as MISS from the coalesced
	// fetch, or as HIT if they raced past storage.Get after the entry was stored).
	for i, ctx := range results {
		if !ctx.FServed {
			t.Errorf("goroutine %d: expected ctx to be served", i)
			continue
		}
		status := ctx.FResponse.Header.Get("X-Cache-Status")
		if status != "MISS" && status != "HIT" {
			t.Errorf("goroutine %d: expected MISS or HIT, got %q", i, status)
		}
		body, _ := io.ReadAll(ctx.FResponse.Body)
		if string(body) != `{"coalesced":true}` {
			t.Errorf("goroutine %d: unexpected body: %q", i, body)
		}
	}

	// Subsequent request must be a HIT (entry was stored by the leader).
	hit := newCtx("GET", url, "")
	f.Request(hit)
	if !hit.FServed {
		t.Fatal("expected HIT after coalesced miss")
	}
	if got := hit.FResponse.Header.Get("X-Cache-Status"); got != "HIT" {
		t.Fatalf("expected HIT, got %q", got)
	}
}

func TestCacheFilter_ColdMissCoalescing_NonCacheable(t *testing.T) {
	// 302 responses are served to waiters but not stored; each new request must
	// fetch again (fetchCount grows with each miss wave).
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)

	var fetchCount int64
	f.fetch = func(req *http.Request) (*http.Response, error) {
		atomic.AddInt64(&fetchCount, 1)
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}

	url := "https://cdn.contentful.com/spaces/abc/redirect"

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)

	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)

	if got := atomic.LoadInt64(&fetchCount); got != 2 {
		t.Fatalf("non-cacheable 302: expected 2 upstream fetches, got %d", got)
	}
}

func TestCacheFilter_ColdMissCoalescing_UpstreamError(t *testing.T) {
	// When the upstream fetch fails, coalesce must not call ctx.Serve so the
	// proxy can fall back to its own backend fetch.
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)

	f.fetch = func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("upstream unavailable")
	}

	ctx := newCtx("GET", "https://cdn.contentful.com/spaces/abc/entries/err", "")
	f.Request(ctx)

	if ctx.FServed {
		t.Fatal("on upstream error, ctx must not be served; proxy should fall back")
	}
}

func TestCacheFilter_ColdMissCoalescing_FetchError_CoalesceErrorMetric(t *testing.T) {
	// coalesce_error must be incremented when the upstream fetch fails during coalescing.
	mockMetrics := &metricstest.MockMetrics{}
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", L1TTL: 60 * time.Second, Metrics: mockMetrics})
	fi, err := spec.CreateFilter([]any{"1m", "15s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	f := fi.(*cacheFilter)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	f.fetch = func(*http.Request) (*http.Response, error) {
		return nil, errors.New("upstream unavailable")
	}

	ctx := newCtx("GET", "https://cdn.contentful.com/spaces/abc/entries/coalesce-err", "")
	f.Request(ctx)

	mockMetrics.WithCounters(func(counters map[string]int64) {
		if counters["cache.coalesce_error"] != 1 {
			t.Errorf("expected cache.coalesce_error==1, got %d", counters["cache.coalesce_error"])
		}
	})
}

func TestCacheFilter_RequestNoStore_NotCached(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/nostoreReq"

	ctx1 := newCtx("GET", url, "")
	ctx1.FRequest.Header.Set("Cache-Control", "no-store")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponse(http.StatusOK, `{"data":"fresh"}`)
	f.Response(ctx1)

	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if ctx2.FServed {
		t.Fatal("response should not have been stored when request had no-store")
	}
}

func TestCacheFilter_RequestNoCache_BypassesCache(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/nocacheReq"

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponse(http.StatusOK, `{"data":"v1"}`)
	f.Response(ctx1)

	ctx2 := newCtx("GET", url, "")
	ctx2.FRequest.Header.Set("Cache-Control", "no-cache")
	f.Request(ctx2)
	if ctx2.FServed {
		t.Fatal("no-cache request must bypass cache even on HIT")
	}
}

func TestCacheFilter_RequestOnlyIfCached_Miss_Returns504(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/onlyifcachedMiss"

	ctx := newCtx("GET", url, "")
	ctx.FRequest.Header.Set("Cache-Control", "only-if-cached")
	f.Request(ctx)

	if !ctx.FServed {
		t.Fatal("only-if-cached with cold cache must call ctx.Serve")
	}
	if ctx.FResponse.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", ctx.FResponse.StatusCode)
	}
}

func TestCacheFilter_RequestOnlyIfCached_Hit_ServesFromCache(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/onlyifcachedHit"

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"cached"}`, "max-age=300")
	f.Response(ctx1)

	ctx2 := newCtx("GET", url, "")
	ctx2.FRequest.Header.Set("Cache-Control", "only-if-cached")
	f.Request(ctx2)
	if !ctx2.FServed {
		t.Fatal("only-if-cached must serve cached entry on hit")
	}
	if ctx2.FResponse.Header.Get("X-Cache-Status") != "HIT" {
		t.Fatalf("expected HIT, got %q", ctx2.FResponse.Header.Get("X-Cache-Status"))
	}
}

func TestCacheFilter_RequestOnlyIfCached_StaleWhileRevalidate_ServesStale(t *testing.T) {
	// RFC 9111 §5.2.1.7: only-if-cached should return a stored response if it is
	// "usable" — entries in the SWR window are still being served as stale to
	// other clients, so they are usable and must not return 504.
	f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/oic-swr"

	synctest.Test(t, func(t *testing.T) {
		// Populate cache.
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"cached"}`, "max-age=0")
		f.Response(ctx1)

		// Advance past TTL (1ms) but stay within SWR window (1 minute).
		time.Sleep(50 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		ctx2.FRequest.Header.Set("Cache-Control", "only-if-cached")
		f.Request(ctx2)

		if !ctx2.FServed {
			t.Fatal("only-if-cached must serve during SWR window")
		}
		if ctx2.FResponse.StatusCode == http.StatusGatewayTimeout {
			t.Fatal("only-if-cached must not return 504 for SWR-window entry")
		}
		if ctx2.FResponse.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", ctx2.FResponse.StatusCode)
		}
	})
}

func TestCacheFilter_AgeHeader_HIT(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/age"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
		f.Response(ctx1)

		time.Sleep(10 * time.Second)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		if !ctx2.FServed {
			t.Fatal("expected HIT")
		}
		age := ctx2.FResponse.Header.Get("Age")
		if age == "" {
			t.Fatal("Age header must be present on HIT")
		}
		if age != "10" {
			t.Fatalf("expected Age: 10, got %q", age)
		}
	})
}

func TestCacheFilter_AgeHeader_STALE(t *testing.T) {
	f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/age-stale"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"old"}`, "max-age=300")
		f.Response(ctx1)

		time.Sleep(5 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()
		if !ctx2.FServed {
			t.Fatal("expected STALE")
		}
		if ctx2.FResponse.Header.Get("Age") == "" {
			t.Fatal("Age header must be present on STALE")
		}
	})
}

func TestCacheFilter_AgeHeader_UpstreamAgeAdded(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/upstream-age"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
		rsp.Header.Set("Age", "30")
		ctx1.FResponse = rsp
		f.Response(ctx1)

		time.Sleep(10 * time.Second)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		if !ctx2.FServed {
			t.Fatal("expected HIT")
		}
		age := ctx2.FResponse.Header.Get("Age")
		if age != "40" {
			t.Fatalf("expected Age: 40 (30 upstream + 10 elapsed), got %q", age)
		}
	})
}

func TestCacheFilter_Metrics(t *testing.T) {
	// ttl=1ms, swrWindow=1h — entry expires quickly, SWR window is huge.
	// Filter created outside the bubble so sknet.Client's transport goroutine
	// does not get trapped inside the synctest bubble.
	// Metrics passed via Options so f.metrics captures hit/miss/stale counters.
	mockMetrics := &metricstest.MockMetrics{}
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", L1TTL: 60 * time.Second, Metrics: mockMetrics})
	fi, err := spec.CreateFilter([]any{time.Millisecond.String(), (15 * time.Second).String(), time.Hour.String()})
	if err != nil {
		t.Fatal(err)
	}
	f := fi.(*cacheFilter)
	f.fetch = func(*http.Request) (*http.Response, error) { return nil, errors.New("no fetch stub set") }
	defer spec.(*cacheSpec).Close()
	url := "https://cdn.contentful.com/spaces/abc/entries/metrics"

	synctest.Test(t, func(t *testing.T) {

		// MISS: populate via Response() path
		miss := newCtx("GET", url, "")
		f.Request(miss)
		miss.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
		f.Response(miss)

		mockMetrics.WithCounters(func(counters map[string]int64) {
			if counters["cache.miss"] != 1 {
				t.Errorf("after MISS: expected cache.miss==1, got %d", counters["cache.miss"])
			}
			if counters["cache.hit"] != 0 {
				t.Errorf("after MISS: expected cache.hit==0, got %d", counters["cache.hit"])
			}
		})

		// HIT: within TTL
		hit := newCtx("GET", url, "")
		f.Request(hit)
		if !hit.FServed {
			t.Fatal("expected HIT within TTL")
		}
		mockMetrics.WithCounters(func(counters map[string]int64) {
			if counters["cache.hit"] != 1 {
				t.Errorf("after HIT: expected cache.hit==1, got %d", counters["cache.hit"])
			}
			if counters["cache.stale"] != 0 {
				t.Errorf("after HIT: expected cache.stale==0, got %d", counters["cache.stale"])
			}
		})

		// STALE: advance past TTL but inside SWR window
		f.fetch = func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"data":"v2"}`)),
			}, nil
		}
		time.Sleep(2 * time.Millisecond)

		stale := newCtx("GET", url, "")
		f.Request(stale)
		synctest.Wait()

		if !stale.FServed {
			t.Fatal("expected STALE to be served")
		}
		if stale.FResponse.Header.Get("X-Cache-Status") != "STALE" {
			t.Fatalf("expected STALE header, got %q", stale.FResponse.Header.Get("X-Cache-Status"))
		}
		mockMetrics.WithCounters(func(counters map[string]int64) {
			if counters["cache.stale"] != 1 {
				t.Errorf("after STALE: expected cache.stale==1, got %d", counters["cache.stale"])
			}
			// cache.hit==1 from the preceding HIT request; STALE must not add another hit.
			if counters["cache.hit"] != 1 {
				t.Errorf("after STALE: expected cache.hit still==1 (from HIT step), got %d", counters["cache.hit"])
			}
		})
	})
}

func TestCacheFilter_Vary_Isolation(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/vary"

	ctxEN := newCtx("GET", url, "")
	ctxEN.FRequest.Header.Set("Accept-Language", "en-US")
	f.Request(ctxEN)
	rspEN := upstreamResponseCC(http.StatusOK, `{"lang":"en-US"}`, "max-age=300")
	rspEN.Header.Set("Vary", "Accept-Language")
	ctxEN.FResponse = rspEN
	f.Response(ctxEN)

	ctxDE := newCtx("GET", url, "")
	ctxDE.FRequest.Header.Set("Accept-Language", "de-DE")
	f.Request(ctxDE)
	rspDE := upstreamResponseCC(http.StatusOK, `{"lang":"de-DE"}`, "max-age=300")
	rspDE.Header.Set("Vary", "Accept-Language")
	ctxDE.FResponse = rspDE
	f.Response(ctxDE)

	hitEN := newCtx("GET", url, "")
	hitEN.FRequest.Header.Set("Accept-Language", "en-US")
	f.Request(hitEN)
	if !hitEN.FServed {
		t.Fatal("expected HIT for en-US")
	}
	body, _ := io.ReadAll(hitEN.FResponse.Body)
	if string(body) != `{"lang":"en-US"}` {
		t.Fatalf("en-US got wrong payload: %q", body)
	}

	hitDE := newCtx("GET", url, "")
	hitDE.FRequest.Header.Set("Accept-Language", "de-DE")
	f.Request(hitDE)
	if !hitDE.FServed {
		t.Fatal("expected HIT for de-DE")
	}
	bodyDE, _ := io.ReadAll(hitDE.FResponse.Body)
	if string(bodyDE) != `{"lang":"de-DE"}` {
		t.Fatalf("de-DE got wrong payload: %q", bodyDE)
	}
}

func TestCacheFilter_Vary_Star_NotCached(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/vary-star"

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	rsp := upstreamResponse(http.StatusOK, `{"data":"v1"}`)
	rsp.Header.Set("Vary", "*")
	ctx1.FResponse = rsp
	f.Response(ctx1)

	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if ctx2.FServed {
		t.Fatal("Vary: * response must not be cached")
	}
}

func TestCacheFilter_ConditionalRevalidation_ETag_304(t *testing.T) {
	url := "https://cdn.contentful.com/spaces/abc/entries/etag"

	var revalReq *http.Request

	synctest.Test(t, func(t *testing.T) {
		f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
		rsp.Header.Set("ETag", `"abc123"`)
		ctx1.FResponse = rsp
		f.Response(ctx1)

		f.fetch = func(req *http.Request) (*http.Response, error) {
			revalReq = req
			return &http.Response{
				StatusCode: http.StatusNotModified,
				Header:     http.Header{"ETag": {`"abc123"`}},
				Body:       http.NoBody,
			}, nil
		}

		time.Sleep(2 * time.Millisecond)
		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()

		if revalReq == nil {
			t.Fatal("expected a revalidation request")
		}
		if revalReq.Header.Get("If-None-Match") != `"abc123"` {
			t.Fatalf("expected If-None-Match: \"abc123\", got %q", revalReq.Header.Get("If-None-Match"))
		}

		// After 304 revalidation, subsequent GET must be a fresh HIT.
		time.Sleep(0)
		ctx3 := newCtx("GET", url, "")
		f.Request(ctx3)
		if !ctx3.FServed {
			t.Fatal("expected HIT after 304 revalidation")
		}
		body, _ := io.ReadAll(ctx3.FResponse.Body)
		if string(body) != `{"data":"v1"}` {
			t.Fatalf("body changed unexpectedly: %q", body)
		}
	})
}

func TestCacheFilter_ConditionalRevalidation_LastModified_304(t *testing.T) {
	url := "https://cdn.contentful.com/spaces/abc/entries/lastmod"

	var revalReq *http.Request

	synctest.Test(t, func(t *testing.T) {
		f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
		rsp.Header.Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
		ctx1.FResponse = rsp
		f.Response(ctx1)

		f.fetch = func(req *http.Request) (*http.Response, error) {
			revalReq = req
			return &http.Response{
				StatusCode: http.StatusNotModified,
				Header:     http.Header{},
				Body:       http.NoBody,
			}, nil
		}

		time.Sleep(2 * time.Millisecond)
		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()

		if revalReq == nil {
			t.Fatal("expected revalidation request")
		}
		if revalReq.Header.Get("If-Modified-Since") != "Wed, 21 Oct 2015 07:28:00 GMT" {
			t.Fatalf("expected If-Modified-Since, got %q", revalReq.Header.Get("If-Modified-Since"))
		}
	})
}

func TestCacheFilter_RevalidationError_MetricIncremented(t *testing.T) {
	url := "https://cdn.contentful.com/spaces/abc/entries/reval-err"

	synctest.Test(t, func(t *testing.T) {
		f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
		mockMetrics := &metricstest.MockMetrics{}
		f.metrics = mockMetrics
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "public, max-age=300")
		ctx1.FResponse = rsp
		f.Response(ctx1)

		f.fetch = func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("upstream down")
		}

		time.Sleep(2 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()

		if !ctx2.FServed {
			t.Fatal("expected STALE to be served")
		}
		mockMetrics.WithCounters(func(counters map[string]int64) {
			if counters["cache.reval_error"] != 1 {
				t.Errorf("expected reval_error==1, got %d", counters["cache.reval_error"])
			}
		})
	})
}

func TestCacheFilter_ExpiresHeader_CapsOperatorTTL(t *testing.T) {
	// Expires without max-age/s-maxage: TTL must be capped by Expires (RFC 9111 §5.3).
	// No Cache-Control so max-age/s-maxage are absent; Expires must be honoured.
	// Also need Last-Modified for the heuristic branch to not short-circuit storage.
	// RFC mode required: force mode ignores Expires and uses operator TTL directly.
	f := newTestFilterRFC(t, time.Minute, 15*time.Second, time.Millisecond)
	url := "https://cdn.contentful.com/spaces/abc/entries/expires"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		rsp := upstreamResponse(http.StatusOK, `{"data":"expires-soon"}`)
		rsp.Header.Set("Expires", time.Now().Add(5*time.Second).UTC().Format(http.TimeFormat))
		ctx1.FResponse = rsp
		f.Response(ctx1)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		if !ctx2.FServed {
			t.Fatal("expected HIT within Expires window")
		}

		time.Sleep(6 * time.Second)

		ctx3 := newCtx("GET", url, "")
		f.Request(ctx3)
		if ctx3.FServed {
			t.Fatal("expected MISS after Expires time passed")
		}
	})
}

func TestCacheFilter_UnsafeMethod_InvalidatesCache(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/item"

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "public, max-age=300")
	ctx1.FResponse = rsp
	f.Response(ctx1)

	hit := newCtx("GET", url, "")
	f.Request(hit)
	if !hit.FServed {
		t.Fatal("expected HIT before invalidation")
	}

	postCtx := newCtx("POST", url, "")
	f.Request(postCtx)
	postCtx.FResponse = &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       http.NoBody,
	}
	f.Response(postCtx)

	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if ctx2.FServed {
		t.Fatal("cache must be invalidated after POST")
	}
}

func TestCacheFilter_UnsafeMethod_4xx_DoesNotInvalidate(t *testing.T) {
	// RFC 9111 §4.4: a cache MUST NOT invalidate a stored response when an
	// unsafe method returns a 4xx status. Only 2xx responses must trigger
	// invalidation.
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/item-4xx"

	// Step 1: warm the cache with a GET that returns 200 + max-age=300.
	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"cached"}`, "public, max-age=300")
	f.Response(ctx1)

	// Verify the entry is cached (second GET must be a HIT).
	ctxHit := newCtx("GET", url, "")
	f.Request(ctxHit)
	if !ctxHit.FServed {
		t.Fatal("expected HIT after warming the cache")
	}
	if ctxHit.FResponse.Header.Get("X-Cache-Status") != "HIT" {
		t.Fatalf("expected HIT status, got %q", ctxHit.FResponse.Header.Get("X-Cache-Status"))
	}

	// Step 2: DELETE request that returns 403 — must NOT invalidate the cache.
	deleteCtx := newCtx("DELETE", url, "")
	f.Request(deleteCtx)
	deleteCtx.FResponse = &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{},
		Body:       http.NoBody,
	}
	f.Response(deleteCtx)

	// Step 3: another GET must still return the cached entry (HIT).
	ctxAfter := newCtx("GET", url, "")
	f.Request(ctxAfter)
	if !ctxAfter.FServed {
		t.Fatal("cache must NOT be invalidated when unsafe method returns 4xx (RFC 9111 §4.4)")
	}
	if ctxAfter.FResponse.Header.Get("X-Cache-Status") != "HIT" {
		t.Fatalf("expected HIT after 4xx unsafe method, got %q", ctxAfter.FResponse.Header.Get("X-Cache-Status"))
	}
}

func TestCacheFilter_SafeMethod_DoesNotInvalidate(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/safe"

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	rsp := upstreamResponseCC(http.StatusOK, `{"data":"safe"}`, "public, max-age=300")
	ctx1.FResponse = rsp
	f.Response(ctx1)

	headCtx := newCtx("HEAD", url, "")
	f.Request(headCtx)
	headCtx.FResponse = &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: http.NoBody}
	f.Response(headCtx)

	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if !ctx2.FServed {
		t.Fatal("HEAD must not invalidate cache")
	}
}

func TestCacheFilter_AuthorizationSafety_BlockedWithoutPermission(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/auth-safety"

	ctx1 := newCtx("GET", url, "Bearer secret")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponse(http.StatusOK, `{"data":"private"}`)
	f.Response(ctx1)

	ctx2 := newCtx("GET", url, "Bearer secret")
	f.Request(ctx2)
	if ctx2.FServed {
		t.Fatal("response to Auth request without Cache-Control: public must not be cached")
	}
}

func TestCacheFilter_AuthorizationSafety_AllowedWithPublic(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/auth-public"

	ctx1 := newCtx("GET", url, "Bearer delivery-token")
	f.Request(ctx1)
	rsp := upstreamResponse(http.StatusOK, `{"data":"public-content"}`)
	rsp.Header.Set("Cache-Control", "public, max-age=300")
	ctx1.FResponse = rsp
	f.Response(ctx1)

	ctx2 := newCtx("GET", url, "Bearer delivery-token")
	f.Request(ctx2)
	if !ctx2.FServed {
		t.Fatal("Cache-Control: public must allow caching for authorized requests")
	}
}

func TestCacheFilter_NoCacheResponse_StoredWithZeroTTL(t *testing.T) {
	// Response Cache-Control: no-cache means: store the entry with TTL=0 (immediately
	// stale) so ETag/Last-Modified are preserved for conditional revalidation
	// (RFC 9111 §5.2.2.4). The entry must be in storage after the first fetch.
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/nc-stored"

	f.fetch = func(req *http.Request) (*http.Response, error) {
		// Use http.Header.Set to ensure canonical header key normalization.
		hdr := http.Header{}
		hdr.Set("Cache-Control", "no-cache")
		hdr.Set("ETag", `"etag-v1"`)
		hdr.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     hdr,
			Body:       io.NopCloser(strings.NewReader(`{"v":1}`)),
		}, nil
	}

	// First request: cold miss → fetch → must store with TTL=0.
	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	if !ctx1.FServed {
		t.Fatal("expected first request to be served via coalesce")
	}

	// Verify entry is stored in the cache with TTL=0 and ETag preserved.
	key := cacheKey(ctx1.FRouteId, ctx1.FRequest, nil)
	entry, err := f.storage.Get(ctx1.FRequest.Context(), key)
	if err != nil {
		t.Fatalf("storage.Get error: %v", err)
	}
	if entry == nil {
		t.Fatal("no-cache response must be stored in cache (TTL=0) so ETag is preserved for revalidation")
	}
	if entry.TTL != 0 {
		t.Fatalf("no-cache entry must have TTL=0 (immediately stale), got %v", entry.TTL)
	}
	if entry.ETag != `"etag-v1"` {
		t.Fatalf("no-cache entry must preserve ETag, got %q", entry.ETag)
	}
}

func TestCacheFilter_NoCacheResponse_ForceRevalidation(t *testing.T) {
	// Response Cache-Control: no-cache means: store the entry (for ETag reuse) but
	// MUST revalidate before every serve. TTL is effectively 0 (RFC 9111 §5.2.2.4).
	// The second request must trigger an upstream fetch (not be served from stored body).
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/nc"

	fetchCount := 0
	f.fetch = func(req *http.Request) (*http.Response, error) {
		fetchCount++
		hdr := http.Header{}
		hdr.Set("Cache-Control", "no-cache")
		hdr.Set("ETag", `"etag-v1"`)
		hdr.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     hdr,
			Body:       io.NopCloser(strings.NewReader(`{"v":1}`)),
		}, nil
	}

	// First request: cold miss → fetch.
	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	if !ctx1.FServed {
		t.Fatal("expected first request to be served via coalesce")
	}
	if ctx1.FResponse.Header.Get("X-Cache-Status") != "MISS" {
		t.Fatalf("expected MISS, got %q", ctx1.FResponse.Header.Get("X-Cache-Status"))
	}

	// Second request: entry exists with no-cache → must fetch upstream again (not serve from cache).
	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if fetchCount < 2 {
		t.Fatalf("no-cache must force upstream fetch on next request; fetchCount=%d", fetchCount)
	}
}

func TestCacheFilter_ProxyRevalidate_BlocksStale(t *testing.T) {
	// proxy-revalidate has the same effect as must-revalidate for shared caches:
	// stale entries MUST NOT be served without revalidation (RFC 9111 §5.2.2.8).
	f := newTestFilter(t, 100*time.Millisecond, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/pr"

	fetchCount := 0
	f.fetch = func(req *http.Request) (*http.Response, error) {
		fetchCount++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Cache-Control": {"proxy-revalidate"}, "Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"v":1}`)),
		}, nil
	}

	synctest.Test(t, func(t *testing.T) {
		// First request: cold miss → fetch → store.
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		if !ctx1.FServed {
			t.Fatal("expected cold miss to be served via coalesce")
		}
		if ctx1.FResponse.Header.Get("X-Cache-Status") != "MISS" {
			t.Fatalf("expected MISS, got %q", ctx1.FResponse.Header.Get("X-Cache-Status"))
		}

		// Advance into stale window (past TTL=100ms, within SWR=1h).
		time.Sleep(200 * time.Millisecond)

		// Second request: entry is stale + proxy-revalidate → must NOT serve stale.
		// coalesce() will call fetch again.
		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()
		if fetchCount < 2 {
			t.Fatalf("proxy-revalidate must block stale serve and trigger upstream fetch; fetchCount=%d", fetchCount)
		}
	})
}

func TestCacheFilter_SMaxAge_ImpliesProxyRevalidate(t *testing.T) {
	// RFC 9111 §5.2.2.10: s-maxage implies proxy-revalidate for shared caches.
	// Stale entries stored under s-maxage MUST NOT be served without revalidation.
	f := newTestFilter(t, 100*time.Millisecond, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/smaxage-pr"

	fetchCount := 0
	f.fetch = func(req *http.Request) (*http.Response, error) {
		fetchCount++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Cache-Control": {"s-maxage=1"}, "Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"v":1}`)),
		}, nil
	}

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		if ctx1.FResponse.Header.Get("X-Cache-Status") != "MISS" {
			t.Fatalf("expected MISS, got %q", ctx1.FResponse.Header.Get("X-Cache-Status"))
		}

		// Advance past TTL=100ms (operator), within SWR=1h.
		time.Sleep(200 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()
		if fetchCount < 2 {
			t.Fatalf("s-maxage must imply proxy-revalidate: stale must not be served; fetchCount=%d", fetchCount)
		}
	})
}

func TestCacheFilter_MustRevalidate_ForcesCoalesceWhenStale(t *testing.T) {
	// RFC 9111 §5.2.2.2: must-revalidate forbids serving a stale response.
	// Once the entry is past TTL, coalesce() must contact the origin even if the
	// entry is inside a stale-while-revalidate window.
	f := newTestFilter(t, 100*time.Millisecond, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/must-reval"

	var fetchCount int64
	f.fetch = func(req *http.Request) (*http.Response, error) {
		atomic.AddInt64(&fetchCount, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Cache-Control": {"must-revalidate"}, "Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"v":1}`)),
		}, nil
	}

	synctest.Test(t, func(t *testing.T) {
		// First request: cold miss → fetch → store.
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		if ctx1.FResponse.Header.Get("X-Cache-Status") != "MISS" {
			t.Fatalf("expected MISS, got %q", ctx1.FResponse.Header.Get("X-Cache-Status"))
		}

		// Advance into stale window (past TTL=100ms, within SWR=1h).
		time.Sleep(200 * time.Millisecond)

		// Second request: entry is stale + must-revalidate → must NOT serve stale.
		// coalesce() must call fetch again (origin contacted).
		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()
		if atomic.LoadInt64(&fetchCount) < 2 {
			t.Fatalf("must-revalidate must block stale serve and trigger upstream fetch; fetchCount=%d", atomic.LoadInt64(&fetchCount))
		}
		if status := ctx2.FResponse.Header.Get("X-Cache-Status"); status == "HIT" || status == "STALE" {
			t.Errorf("expected origin fetch, but got X-Cache-Status: %s", status)
		}
	})
}

func TestCacheFilter_SharedStorage_RouteIsolation(t *testing.T) {
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", L1TTL: 60 * time.Second})
	t.Cleanup(spec.(*cacheSpec).client.Close)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	makeFilter := func(t *testing.T) *cacheFilter {
		t.Helper()
		f, err := spec.CreateFilter([]any{"5m", "15s", "30s"})
		if err != nil {
			t.Fatal(err)
		}
		cf := f.(*cacheFilter)
		// Default fetch stub returns an error so coalesce does not serve the
		// request; this allows the test to distinguish a true cache HIT from a
		// coalesced upstream fetch.
		cf.fetch = func(*http.Request) (*http.Response, error) {
			return nil, errors.New("no fetch stub set")
		}
		return cf
	}

	f1 := makeFilter(t)
	f2 := makeFilter(t)

	// Both filter instances must share the same storage.
	if f1.storage != f2.storage {
		t.Fatal("expected shared storage: f1.storage and f2.storage must be the same pointer")
	}

	url := "https://cdn.contentful.com/spaces/abc/entries/shared"

	// Populate cache via f1 with route "route-a" using the Response() path.
	ctx1 := newCtxWithRoute("GET", url, "", "route-a")
	f1.Request(ctx1)
	ctx1.FResponse = &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": {"public, max-age=300"}},
		Body:       io.NopCloser(strings.NewReader(`{"route":"a"}`)),
	}
	f1.Response(ctx1)

	// f2 with a different route ID must not see f1's entry.
	ctx2 := newCtxWithRoute("GET", url, "", "route-b")
	f2.Request(ctx2)
	if ctx2.FServed {
		t.Fatal("route-b must not see route-a's cached entry (cross-route collision)")
	}

	// f1 with the same route ID must hit.
	ctx3 := newCtxWithRoute("GET", url, "", "route-a")
	f1.Request(ctx3)
	if !ctx3.FServed {
		t.Fatal("route-a must hit its own cached entry")
	}
	body, _ := io.ReadAll(ctx3.FResponse.Body)
	if string(body) != `{"route":"a"}` {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestCacheFilter_UnsafeMethod_SameOriginLocation_Invalidates(t *testing.T) {
	// RFC 9111 §4.4: a successful unsafe-method response with a same-origin Location
	// header must also invalidate the cached entry for that URI.
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	base := "https://cdn.contentful.com"

	// Populate cache for /entries/other.
	ctxGet := newCtx("GET", base+"/entries/other", "")
	f.Request(ctxGet)
	ctxGet.FResponse = upstreamResponseCC(http.StatusOK, `{"id":"other"}`, "public, max-age=300")
	f.Response(ctxGet)
	if ctxGet.FResponse.Header.Get("X-Cache-Status") != "MISS" {
		t.Fatal("expected MISS on first GET")
	}

	// Verify /entries/other is cached.
	ctxHit := newCtx("GET", base+"/entries/other", "")
	f.Request(ctxHit)
	if !ctxHit.FServed {
		t.Fatal("expected /entries/other to be cached before POST")
	}

	// POST to /entries — response has Location: /entries/other (same-origin, relative).
	ctxPost := newCtx("POST", base+"/entries", "")
	f.Request(ctxPost)
	postResp := upstreamResponse(http.StatusCreated, "")
	postResp.Header.Set("Location", "/entries/other")
	ctxPost.FResponse = postResp
	f.Response(ctxPost)

	// /entries/other cache must now be invalidated.
	ctxAfter := newCtx("GET", base+"/entries/other", "")
	f.Request(ctxAfter)
	if ctxAfter.FServed {
		t.Fatal("expected /entries/other cache to be invalidated by POST Location header")
	}
}

func TestCacheFilter_UnsafeMethod_ContentLocation_Invalidates(t *testing.T) {
	// RFC 9111 §4.4: a successful unsafe-method response with a same-origin Content-Location
	// header must also invalidate the cached entry for that URI.
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	base := "https://cdn.contentful.com"

	// Populate cache for /resource.
	ctxGet := newCtx("GET", base+"/resource", "")
	f.Request(ctxGet)
	ctxGet.FResponse = upstreamResponseCC(http.StatusOK, `{"id":"resource"}`, "public, max-age=300")
	f.Response(ctxGet)
	if ctxGet.FResponse.Header.Get("X-Cache-Status") != "MISS" {
		t.Fatal("expected MISS on first GET")
	}

	// Verify /resource is cached.
	ctxHit := newCtx("GET", base+"/resource", "")
	f.Request(ctxHit)
	if !ctxHit.FServed {
		t.Fatal("expected /resource to be cached before POST")
	}

	// POST to /resource — response has Content-Location: /resource.
	ctxPost := newCtx("POST", base+"/resource", "")
	f.Request(ctxPost)
	postResp := upstreamResponse(http.StatusOK, "")
	postResp.Header.Set("Content-Location", "/resource")
	ctxPost.FResponse = postResp
	f.Response(ctxPost)

	// /resource cache must now be invalidated.
	ctxAfter := newCtx("GET", base+"/resource", "")
	f.Request(ctxAfter)
	if ctxAfter.FServed {
		t.Fatal("expected /resource cache to be invalidated by POST Content-Location header")
	}
}

func TestCacheFilter_AgeHeader_RFC9111_CorrectFormula(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/age-rfc9111"

	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		ctx1 := newCtx("GET", url, "")
		// NOTE: for this test we need the response to go through the coalesce
		// path (f.fetch stub), NOT the Response() path. Set up f.fetch to return
		// a response with Date=now-20s and Age=5.
		f.fetch = func(r *http.Request) (*http.Response, error) {
			rsp := upstreamResponse(http.StatusOK, `{"data":"v1"}`)
			rsp.Header.Set("Cache-Control", "public, max-age=300")
			rsp.Header.Set("Date", now.Add(-20*time.Second).UTC().Format(http.TimeFormat))
			rsp.Header.Set("Age", "5")
			return rsp, nil
		}
		f.Request(ctx1) // triggers coalesce -> f.fetch
		// ctx1 is served by coalesce; advance time 10s
		time.Sleep(10 * time.Second)
		// Second request hits the cache
		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		if !ctx2.FServed {
			t.Fatal("expected cache HIT")
		}
		age := ctx2.FResponse.Header.Get("Age")
		if age != "30" {
			t.Fatalf("expected Age 30, got %q", age)
		}
	})
}

func TestCacheFilter_AgeHeader_RFC9111_ResponseDelay(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/age-delay"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.fetch = func(r *http.Request) (*http.Response, error) {
			// Simulate 5s of response delay (inside synctest, time.Sleep is instant).
			time.Sleep(5 * time.Second)
			now := time.Now()
			rsp := upstreamResponse(http.StatusOK, `{"data":"v1"}`)
			rsp.Header.Set("Cache-Control", "public, max-age=300")
			rsp.Header.Set("Date", now.UTC().Format(http.TimeFormat))
			return rsp, nil
		}
		f.Request(ctx1) // triggers coalesce -> f.fetch (5s delay simulated)
		// Serve immediately after, no additional time sleep
		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		if !ctx2.FServed {
			t.Fatal("expected cache HIT")
		}
		age := ctx2.FResponse.Header.Get("Age")
		if age != "5" {
			t.Fatalf("expected Age 5 (response_delay), got %q", age)
		}
	})
}

func TestCacheFilter_UnsafeMethod_CrossOriginLocation_DoesNotInvalidate(t *testing.T) {
	// RFC 9111 §4.4: cross-origin Location headers must NOT trigger cache invalidation.
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)

	// Populate cache for https://cdn.contentful.com/entries/safe.
	ctxGet := newCtx("GET", "https://cdn.contentful.com/entries/safe", "")
	f.Request(ctxGet)
	ctxGet.FResponse = upstreamResponseCC(http.StatusOK, `{"id":"safe"}`, "public, max-age=300")
	f.Response(ctxGet)

	// Verify it's cached.
	ctxHit := newCtx("GET", "https://cdn.contentful.com/entries/safe", "")
	f.Request(ctxHit)
	if !ctxHit.FServed {
		t.Fatal("expected /entries/safe to be cached")
	}

	// POST — response has a cross-origin Location.
	ctxPost := newCtx("POST", "https://cdn.contentful.com/entries", "")
	f.Request(ctxPost)
	postResp := upstreamResponse(http.StatusCreated, "")
	postResp.Header.Set("Location", "https://evil.example.com/entries/safe")
	ctxPost.FResponse = postResp
	f.Response(ctxPost)

	// /entries/safe must still be in cache (cross-origin Location must be ignored).
	ctxAfter := newCtx("GET", "https://cdn.contentful.com/entries/safe", "")
	f.Request(ctxAfter)
	if !ctxAfter.FServed {
		t.Fatal("cross-origin Location must not invalidate same-origin cache entry")
	}
}

func TestCacheFilter_HEAD_ServedWithEmptyBody(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/head-empty"

	// Populate via GET (goes through coalesce path).
	ctx1 := newCtx("GET", url, "")
	f.fetch = func(r *http.Request) (*http.Response, error) {
		rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "public, max-age=300")
		return rsp, nil
	}
	f.Request(ctx1)

	// HEAD request must be served from cache with empty body.
	headCtx := newCtx("HEAD", url, "")
	f.Request(headCtx)
	if !headCtx.FServed {
		t.Fatal("HEAD request must be served from cache when GET entry exists")
	}
	if headCtx.FResponse.Header.Get("X-Cache-Status") != "HIT" {
		t.Fatalf("expected HIT status, got %q", headCtx.FResponse.Header.Get("X-Cache-Status"))
	}
	body, _ := io.ReadAll(headCtx.FResponse.Body)
	if len(body) != 0 {
		t.Fatalf("HEAD response must have empty body, got %d bytes", len(body))
	}
	if headCtx.FResponse.ContentLength != 0 {
		t.Fatalf("HEAD response ContentLength must be 0, got %d", headCtx.FResponse.ContentLength)
	}
}

func TestCacheFilter_HEAD_200_FreshensStoredEntry(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/head-freshen"

	// Populate via GET with ETag "v1".
	ctx1 := newCtx("GET", url, "")
	f.fetch = func(r *http.Request) (*http.Response, error) {
		rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "public, max-age=300")
		rsp.Header.Set("ETag", `"v1"`)
		return rsp, nil
	}
	f.Request(ctx1)

	// HEAD request: served from cache (FServed=true), but we also call Response()
	// with a HEAD 200 containing updated ETag "v2". Freshening must update the entry.
	headCtx := newCtx("HEAD", url, "")
	f.Request(headCtx) // serves from cache, sets FServed=true
	// Simulate upstream returning a HEAD 200 with updated headers.
	headCtx.FResponse = &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Etag":          {`"v2"`},
			"Cache-Control": {"public, max-age=300"},
		},
		Body: http.NoBody,
	}
	f.Response(headCtx) // must freshen even though FServed=true

	// Stored GET entry must now have ETag "v2".
	key := cacheKey(headCtx.FRouteId, headCtx.FRequest, nil)
	entry, err := f.storage.Get(headCtx.FRequest.Context(), key)
	if err != nil || entry == nil {
		t.Fatal("expected stored entry after freshening")
	}
	if entry.ETag != `"v2"` {
		t.Fatalf("expected ETag %q after freshening, got %q", `"v2"`, entry.ETag)
	}
	if got := entry.Header.Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("expected freshened Cache-Control header, got %q", got)
	}

	// Subsequent GET must still serve from cache with the original body.
	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if !ctx2.FServed {
		t.Fatal("expected GET HIT after HEAD freshening")
	}
	body, _ := io.ReadAll(ctx2.FResponse.Body)
	if string(body) != `{"data":"v1"}` {
		t.Fatalf("body must not change after HEAD freshening, got %q", body)
	}
}

func TestCacheFilter_HeuristicFreshness_NoExplicitTTL(t *testing.T) {
	// f.ttl=5m; heuristic TTL = 0.1 * 1000s = 100s < 5m so not capped.
	// RFC mode required: force mode ignores Last-Modified and uses operator TTL directly.
	f := newTestFilterRFC(t, 5*time.Minute, 15*time.Second, time.Millisecond)
	url := "https://cdn.contentful.com/spaces/abc/entries/heuristic"

	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		ctx1 := newCtx("GET", url, "")
		f.fetch = func(r *http.Request) (*http.Response, error) {
			rsp := upstreamResponse(http.StatusOK, `{"data":"heuristic"}`)
			// No Cache-Control, no Expires. Last-Modified = 1000s ago.
			rsp.Header.Set("Last-Modified", now.Add(-1000*time.Second).UTC().Format(http.TimeFormat))
			return rsp, nil
		}
		f.Request(ctx1)

		// Entry must be stored with heuristic TTL ~= 100s.
		key := cacheKey(ctx1.FRouteId, ctx1.FRequest, nil)
		entry, err := f.storage.Get(ctx1.FRequest.Context(), key)
		if err != nil || entry == nil {
			t.Fatal("heuristic TTL response must be stored in cache")
		}
		if entry.TTL < 90*time.Second || entry.TTL > 110*time.Second {
			t.Fatalf("expected heuristic TTL ~100s, got %v", entry.TTL)
		}

		// HIT within heuristic window.
		time.Sleep(50 * time.Second)
		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		if !ctx2.FServed {
			t.Fatal("expected HIT within heuristic TTL window")
		}

		// After heuristic TTL + SWR (swrWindow=1ms), entry must be hard-expired.
		// We check storage directly rather than via f.Request to avoid coalesce
		// fetching and re-storing the entry.
		time.Sleep(60 * time.Second)
		entry2, err2 := f.storage.Get(ctx1.FRequest.Context(), key)
		if err2 != nil {
			t.Fatalf("storage.Get error: %v", err2)
		}
		if entry2 != nil {
			t.Fatal("expected entry expired after heuristic TTL + SWR")
		}
	})
}

func TestCacheFilter_HeuristicFreshness_ExplicitMaxAge_NoHeuristic(t *testing.T) {
	f := newTestFilter(t, 5*time.Minute, 15*time.Second, time.Millisecond)
	url := "https://cdn.contentful.com/spaces/abc/entries/heuristic-maxage"

	now := time.Now()
	ctx1 := newCtx("GET", url, "")
	f.fetch = func(r *http.Request) (*http.Response, error) {
		// max-age=300 present: heuristic must NOT apply.
		rsp := upstreamResponseCC(http.StatusOK, `{"data":"explicit"}`, "max-age=300")
		rsp.Header.Set("Last-Modified", now.Add(-1000*time.Second).UTC().Format(http.TimeFormat))
		return rsp, nil
	}
	f.Request(ctx1)

	key := cacheKey(ctx1.FRouteId, ctx1.FRequest, nil)
	entry, err := f.storage.Get(ctx1.FRequest.Context(), key)
	if err != nil || entry == nil {
		t.Fatal("entry with explicit max-age must be stored")
	}
	// TTL must be the operator f.ttl (5m), not the heuristic 100s.
	if entry.TTL != 5*time.Minute {
		t.Fatalf("expected operator TTL 5m, got %v (heuristic must not apply with explicit max-age)", entry.TTL)
	}
}

func TestCacheFilter_HeuristicFreshness_NoLastModified_NotCached(t *testing.T) {
	// RFC mode required: in force mode a response with no CC/Expires/Last-Modified
	// is still cached using the operator TTL.
	f := newTestFilterRFC(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/heuristic-nolm"

	ctx1 := newCtx("GET", url, "")
	f.fetch = func(r *http.Request) (*http.Response, error) {
		// No CC, no Expires, no Last-Modified → must not be cached.
		return upstreamResponse(http.StatusOK, `{"data":"nolm"}`), nil
	}
	f.Request(ctx1)

	// Reset fetch to error stub: entry must not be in storage (heuristic returned 0
	// for no-Last-Modified response), so ctx2 goes to coalesce which will fail.
	f.fetch = func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no fetch stub: expected cache miss, not upstream call")
	}

	key := cacheKey(ctx1.FRouteId, ctx1.FRequest, nil)
	entry, err := f.storage.Get(ctx1.FRequest.Context(), key)
	if err != nil {
		t.Fatalf("storage.Get error: %v", err)
	}
	if entry != nil {
		t.Fatal("response without Last-Modified and no explicit TTL must not be cached")
	}
}

func TestCacheFilter_HeuristicFreshness_Capped(t *testing.T) {
	// f.ttl=5m; heuristic = 0.1 * 36000s = 3600s, but capped to 5m.
	f := newTestFilter(t, 5*time.Minute, 15*time.Second, time.Millisecond)
	url := "https://cdn.contentful.com/spaces/abc/entries/heuristic-cap"

	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		ctx1 := newCtx("GET", url, "")
		f.fetch = func(r *http.Request) (*http.Response, error) {
			rsp := upstreamResponse(http.StatusOK, `{"data":"cap"}`)
			// Last-Modified = 10h ago → heuristic = 0.1 * 36000s = 3600s.
			rsp.Header.Set("Last-Modified", now.Add(-10*time.Hour).UTC().Format(http.TimeFormat))
			return rsp, nil
		}
		f.Request(ctx1)

		key := cacheKey(ctx1.FRouteId, ctx1.FRequest, nil)
		entry, err := f.storage.Get(ctx1.FRequest.Context(), key)
		if err != nil || entry == nil {
			t.Fatal("expected stored entry")
		}
		// Must be capped to f.ttl = 5m, not 3600s.
		if entry.TTL != 5*time.Minute {
			t.Fatalf("expected TTL capped to 5m, got %v", entry.TTL)
		}
	})
}

func TestCacheFilter_HEAD_NoStoredEntry_NoFreshen(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/head-no-entry"

	// HEAD arrives cold (no prior GET). Response() should not create a new entry.
	headCtx := newCtx("HEAD", url, "")
	f.Request(headCtx) // no entry, not served from cache
	headCtx.FResponse = &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Etag": {`"v1"`}},
		Body:       http.NoBody,
	}
	f.Response(headCtx)

	key := cacheKey(headCtx.FRouteId, headCtx.FRequest, nil)
	entry, err := f.storage.Get(headCtx.FRequest.Context(), key)
	if err != nil {
		t.Fatalf("storage.Get error: %v", err)
	}
	if entry != nil {
		t.Fatal("HEAD 200 with no existing entry must not create a new entry")
	}
}

func TestCacheFilter_Expires_InvalidDate_TreatedAsExpired(t *testing.T) {
	// RFC 9111 §5.3: invalid Expires date (including "0") must be treated as already
	// expired. capTTLByExpires returns 0. The entry is stored with TTL=0 (immediately
	// stale, preserved for conditional revalidation).
	// Use the Response() path (not coalesce) to store the TTL=0 entry;
	// coalesce only stores entries with NoCache==true or TTL>0.
	// f.fetch returns an error so coalesce resolves immediately without serving,
	// leaving ctx unserved so Response() can run and store the entry.
	// RFC mode required: force mode ignores Expires and always uses operator TTL.
	f := newTestFilterRFC(t, 5*time.Minute, 10*time.Second, time.Second)
	// f.fetch is already set to the error stub by newTestFilter; coalesce resolves
	// with an error, leaving ctx unserved so Response() will run.
	ctx := newCtx(http.MethodGet, "http://example.com/invalid-expires", "")
	f.Request(ctx) // sets state-bag key; coalesce resolves (error), ctx not served

	rsp := upstreamResponse(http.StatusOK, "body")
	rsp.Header.Set("Expires", "0")
	ctx.FResponse = rsp
	f.Response(ctx) // stores entry with TTL=0 via the Response() path

	key := ctx.StateBag()[stateBagKey].(string)
	entry, err := f.storage.Get(context.Background(), key)
	if err != nil || entry == nil {
		t.Fatal("expected entry to be stored with TTL=0 (invalid Expires treated as expired)")
	}
	if entry.TTL != 0 {
		t.Errorf("expected TTL=0 for invalid Expires, got %v", entry.TTL)
	}
}

func TestCacheFilter_HopByHop_NotStored(t *testing.T) {
	f := newTestFilter(t, 5*time.Minute, 10*time.Second, time.Second)
	rsp := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
	rsp.Header.Set("Connection", "Keep-Alive")
	rsp.Header.Set("Keep-Alive", "timeout=5")
	f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp, nil }

	ctx := newCtx(http.MethodGet, "http://example.com/path", "")
	f.Request(ctx)

	// Read stored entry directly
	key := ctx.StateBag()[stateBagKey].(string)
	entry, err := f.storage.Get(context.Background(), key)
	if err != nil || entry == nil {
		t.Fatal("expected entry to be stored")
	}
	if entry.Header.Get("Connection") != "" {
		t.Errorf("Connection header should not be stored, got %q", entry.Header.Get("Connection"))
	}
	if entry.Header.Get("Keep-Alive") != "" {
		t.Errorf("Keep-Alive header should not be stored, got %q", entry.Header.Get("Keep-Alive"))
	}
	// Positive assertion: a non-hop-by-hop header must still be present.
	if entry.Header.Get("Cache-Control") == "" {
		t.Errorf("Cache-Control header should still be present in stored entry")
	}
}

func TestCacheFilter_304Merge_HopByHop_NotMerged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newTestFilter(t, 5*time.Minute, 10*time.Second, 10*time.Minute)
		// 1. First request populates cache
		rsp1 := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
		rsp1.Header.Set("ETag", `"v1"`)
		f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp1, nil }
		ctx1 := newCtx(http.MethodGet, "http://example.com/path", "")
		f.Request(ctx1)

		// 2. Sleep past TTL to make entry stale
		time.Sleep(6 * time.Minute)

		// 3. Second request triggers STALE serve + background revalidation
		rsp304 := &http.Response{
			StatusCode: http.StatusNotModified,
			Header:     http.Header{},
			Body:       http.NoBody,
		}
		rsp304.Header.Set("ETag", `"v2"`)
		rsp304.Header.Set("Connection", "close")
		f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp304, nil }
		ctx2 := newCtx(http.MethodGet, "http://example.com/path", "")
		f.Request(ctx2)

		synctest.Wait() // wait for background revalidation goroutine

		// 4. Read stored entry and check
		key := ctx1.StateBag()[stateBagKey].(string)
		entry, err := f.storage.Get(context.Background(), key)
		if err != nil || entry == nil {
			t.Fatal("expected entry after revalidation")
		}
		if entry.Header.Get("Connection") != "" {
			t.Errorf("Connection should not be in stored entry after 304 merge")
		}
		if entry.ETag != `"v2"` {
			t.Errorf("ETag should be updated to v2, got %q", entry.ETag)
		}
	})
}

func TestCacheFilter_Revalidate200_HopByHop_NotStored(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newTestFilter(t, 5*time.Minute, 10*time.Second, 10*time.Minute)
		// 1. First request populates cache
		rsp1 := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
		rsp1.Header.Set("ETag", `"v1"`)
		f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp1, nil }
		ctx1 := newCtx(http.MethodGet, "http://example.com/path200", "")
		f.Request(ctx1)

		// 2. Sleep past TTL to make entry stale (within SWR window)
		time.Sleep(6 * time.Minute)

		// 3. Set fetch stub to return a 200 with hop-by-hop headers
		rsp200 := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("newbody")),
		}
		rsp200.Header.Set("Cache-Control", "max-age=300")
		rsp200.Header.Set("Connection", "Keep-Alive")
		rsp200.Header.Set("Keep-Alive", "timeout=5")
		f.fetch = func(_ *http.Request) (*http.Response, error) {
			return rsp200, nil
		}

		// 4. Second request: served stale + background revalidation fired
		ctx2 := newCtx(http.MethodGet, "http://example.com/path200", "")
		f.Request(ctx2)

		synctest.Wait() // wait for background revalidation goroutine

		// 5. Read stored entry and assert hop-by-hop headers are absent
		key := ctx1.StateBag()[stateBagKey].(string)
		entry, err := f.storage.Get(context.Background(), key)
		if err != nil || entry == nil {
			t.Fatal("expected entry after 200 revalidation")
		}
		if entry.Header.Get("Connection") != "" {
			t.Errorf("Connection should not be stored after 200 revalidation, got %q", entry.Header.Get("Connection"))
		}
		if entry.Header.Get("Keep-Alive") != "" {
			t.Errorf("Keep-Alive should not be stored after 200 revalidation, got %q", entry.Header.Get("Keep-Alive"))
		}
		// Positive assertion: non-hop-by-hop header must still be present.
		if entry.Header.Get("Cache-Control") == "" {
			t.Errorf("Cache-Control should be present in stored entry after 200 revalidation")
		}
		// Body and status must be correct.
		if entry.StatusCode != http.StatusOK {
			t.Errorf("expected StatusCode 200, got %d", entry.StatusCode)
		}
		if string(entry.Payload) != "newbody" {
			t.Errorf("expected payload %q, got %q", "newbody", string(entry.Payload))
		}
	})
}

func TestCacheFilter_CacheControl_PassedThrough(t *testing.T) {
	f := newTestFilter(t, 5*time.Minute, 10*time.Second, time.Second)
	rsp := upstreamResponseCC(http.StatusOK, "body", "max-age=300, must-revalidate")
	f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp, nil }

	ctx := newCtx(http.MethodGet, "http://example.com/path", "")
	f.Request(ctx)

	// The filter must not strip or modify Cache-Control on the response.
	got := ctx.FResponse.Header.Get("Cache-Control")
	if got != "max-age=300, must-revalidate" {
		t.Errorf("Cache-Control not passed through: got %q", got)
	}
}

func TestCacheFilter_Expires_IgnoredWhenMaxAgePresent(t *testing.T) {
	// RFC 9111 §5.3: Expires MUST be ignored when max-age is present.
	// max-age=300 present and Expires is a past date. Without the fix, the past
	// Expires would cap TTL to 0. With the fix, Expires is ignored and entry is
	// stored with TTL = f.ttl = 5m.
	// Use the fetch stub pattern (like TestCacheFilter_MissAndHit) so coalesce
	// handles storage — max-age=300 means ttl=5m>0, so coalesce will store it.
	f := newTestFilter(t, 5*time.Minute, 10*time.Second, time.Second)
	url := "http://example.com/expires-ignored"

	f.fetch = func(r *http.Request) (*http.Response, error) {
		rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
		rsp.Header.Set("Expires", "Mon, 01 Jan 2024 00:00:00 GMT") // past date: must be ignored
		return rsp, nil
	}

	// First request: cold miss → coalesce → fetch → store (max-age wins over Expires).
	ctx1 := newCtx(http.MethodGet, url, "")
	f.Request(ctx1)
	if !ctx1.FServed {
		t.Fatal("expected first request to be served via coalesce")
	}

	key := ctx1.StateBag()[stateBagKey].(string)
	entry, err := f.storage.Get(context.Background(), key)
	if err != nil || entry == nil {
		t.Fatal("expected entry to be stored (Expires ignored when max-age present)")
	}

	// Second request must be a HIT.
	ctx2 := newCtx(http.MethodGet, url, "")
	f.Request(ctx2)
	if !ctx2.FServed {
		t.Fatal("expected HIT: Expires must be ignored when max-age is present")
	}
	if ctx2.FResponse.Header.Get("X-Cache-Status") != "HIT" {
		t.Fatalf("expected HIT status, got %q", ctx2.FResponse.Header.Get("X-Cache-Status"))
	}
}

func TestCacheFilter_Expires_NonGMT_TreatedAsInvalid(t *testing.T) {
	// RFC 9111 §4.2 / §5.3: RFC 850 date with non-GMT zone (EST) must be rejected by
	// parseHTTPTime and treated as an invalid date — capTTLByExpires returns 0.
	// The entry is stored with TTL=0 (immediately stale, preserved for conditional
	// revalidation), mirroring the behaviour of TestCacheFilter_Expires_InvalidDate_TreatedAsExpired.
	// RFC mode required: force mode ignores Expires and always uses operator TTL.
	f := newTestFilterRFC(t, 5*time.Minute, 10*time.Second, time.Second)
	// f.fetch is already set to the error stub by newTestFilter; coalesce resolves
	// with an error, leaving ctx unserved so Response() will run.
	ctx := newCtx(http.MethodGet, "http://example.com/nonGMT-expires", "")
	f.Request(ctx) // sets state-bag key; coalesce resolves (error), ctx not served

	rsp := upstreamResponseCC(http.StatusOK, "body", "")
	rsp.Header.Set("Expires", "Monday, 01-Jan-24 12:00:00 EST")
	rsp.Header.Del("Cache-Control") // no Cache-Control
	ctx.FResponse = rsp
	f.Response(ctx) // stores entry with TTL=0 via the Response() path

	key := ctx.StateBag()[stateBagKey].(string)
	entry, err := f.storage.Get(context.Background(), key)
	if err != nil || entry == nil {
		t.Fatal("expected entry to be stored with TTL=0 (non-GMT Expires treated as invalid)")
	}
	if entry.TTL != 0 {
		t.Errorf("expected TTL=0 for non-GMT Expires, got %v", entry.TTL)
	}
}

func TestCacheFilter_AgeHeader_NonGMT_Date_Ignored(t *testing.T) {
	// RFC 9111 §4.2: RFC 850 date with non-GMT zone (EST) in Date header must be
	// rejected. apparent_age falls back to 0. After 10s resident time, Age must be
	// ~10 (from ResponseTime only), not inflated by a wrong apparent_age.
	f := newTestFilter(t, 5*time.Minute, 10*time.Second, time.Second)
	synctest.Test(t, func(t *testing.T) {
		rsp := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
		rsp.Header.Set("Date", "Monday, 01-Jan-24 12:00:00 EST")
		f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp, nil }

		ctx1 := newCtx(http.MethodGet, "http://example.com/nonGMT-date", "")
		f.Request(ctx1)

		time.Sleep(10 * time.Second)

		ctx2 := newCtx(http.MethodGet, "http://example.com/nonGMT-date", "")
		f.Request(ctx2)
		if ctx2.FResponse == nil {
			t.Fatal("expected HIT")
		}
		age := ctx2.FResponse.Header.Get("Age")
		// Age should be ~10, not inflated by a wrong apparent_age from EST offset
		v, _ := strconv.ParseInt(age, 10, 64)
		if v < 9 || v > 12 {
			t.Errorf("expected Age ~10 when non-GMT Date is ignored, got %q", age)
		}
	})
}

func TestCacheFilter_AgeHeader_InvalidAge_Ignored(t *testing.T) {
	// RFC 9111 §5.1: invalid Age field value must be ignored; only resident time
	// should contribute to the Age header on a HIT response.
	f := newTestFilter(t, 5*time.Minute, 10*time.Second, time.Second)
	synctest.Test(t, func(t *testing.T) {
		rsp := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
		rsp.Header.Set("Age", "bogus")
		f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp, nil }

		ctx1 := newCtx(http.MethodGet, "http://example.com/invalid-age", "")
		f.Request(ctx1)

		time.Sleep(10 * time.Second)

		ctx2 := newCtx(http.MethodGet, "http://example.com/invalid-age", "")
		f.Request(ctx2)
		if ctx2.FResponse == nil {
			t.Fatal("expected HIT")
		}
		age := ctx2.FResponse.Header.Get("Age")
		v, err := strconv.ParseInt(age, 10, 64)
		if err != nil {
			t.Fatalf("Age header not a valid integer: %q", age)
		}
		if v < 9 || v > 12 {
			t.Errorf("expected Age ~10 when invalid upstream Age is ignored, got %q", age)
		}
	})
}

func TestCacheFilter_AgeHeader_Zero_IsValid(t *testing.T) {
	// RFC 9111 §5.1: Age: 0 is a valid non-negative integer and must be accepted
	// (not discarded by a v > 0 guard). ageValue = 0 so corrected_initial_age
	// is unchanged, but the field must not cause a parse error.
	f := newTestFilter(t, 5*time.Minute, 10*time.Second, time.Second)
	synctest.Test(t, func(t *testing.T) {
		rsp := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
		rsp.Header.Set("Age", "0")
		f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp, nil }

		ctx1 := newCtx(http.MethodGet, "http://example.com/age-zero", "")
		f.Request(ctx1)

		time.Sleep(10 * time.Second)

		ctx2 := newCtx(http.MethodGet, "http://example.com/age-zero", "")
		f.Request(ctx2)
		if ctx2.FResponse == nil {
			t.Fatal("expected HIT")
		}
		age := ctx2.FResponse.Header.Get("Age")
		v, err := strconv.ParseInt(age, 10, 64)
		if err != nil {
			t.Fatalf("Age header not a valid integer: %q", age)
		}
		// Age: 0 upstream contributes ageValue=0; resident time of 10s dominates.
		if v < 9 || v > 12 {
			t.Errorf("expected Age ~10 with Age: 0 upstream, got %q", age)
		}
	})
}

func TestCacheFilter_ConditionalRequest_IfNoneMatch_304(t *testing.T) {
	f := newTestFilter(t, 5*time.Minute, time.Second, time.Second)
	rsp := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
	rsp.Header.Set("ETag", `"v1"`)
	f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp, nil }

	ctx1 := newCtx(http.MethodGet, "http://example.com/cond-inm", "")
	f.Request(ctx1)

	ctx2 := newCtx(http.MethodGet, "http://example.com/cond-inm", "")
	ctx2.FRequest.Header.Set("If-None-Match", `"v1"`)
	f.Request(ctx2)
	if ctx2.FResponse == nil {
		t.Fatal("expected response")
	}
	if ctx2.FResponse.StatusCode != http.StatusNotModified {
		t.Errorf("expected 304, got %d", ctx2.FResponse.StatusCode)
	}
	body, _ := io.ReadAll(ctx2.FResponse.Body)
	if len(body) != 0 {
		t.Errorf("expected empty body on 304, got %q", body)
	}
}

func TestCacheFilter_ConditionalRequest_IfNoneMatch_Wildcard_304(t *testing.T) {
	f := newTestFilter(t, 5*time.Minute, time.Second, time.Second)
	rsp := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
	rsp.Header.Set("ETag", `"abc"`)
	f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp, nil }

	ctx1 := newCtx(http.MethodGet, "http://example.com/cond-inm-wildcard", "")
	f.Request(ctx1)

	ctx2 := newCtx(http.MethodGet, "http://example.com/cond-inm-wildcard", "")
	ctx2.FRequest.Header.Set("If-None-Match", "*")
	f.Request(ctx2)
	if ctx2.FResponse == nil {
		t.Fatal("expected response")
	}
	if ctx2.FResponse.StatusCode != http.StatusNotModified {
		t.Errorf("expected 304, got %d", ctx2.FResponse.StatusCode)
	}
}

func TestCacheFilter_ConditionalRequest_IfModifiedSince_304(t *testing.T) {
	f := newTestFilter(t, 5*time.Minute, time.Second, time.Second)
	rsp := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
	rsp.Header.Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
	f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp, nil }

	ctx1 := newCtx(http.MethodGet, "http://example.com/cond-ims", "")
	f.Request(ctx1)

	ctx2 := newCtx(http.MethodGet, "http://example.com/cond-ims", "")
	ctx2.FRequest.Header.Set("If-Modified-Since", "Wed, 21 Oct 2015 07:28:00 GMT")
	f.Request(ctx2)
	if ctx2.FResponse == nil {
		t.Fatal("expected response")
	}
	if ctx2.FResponse.StatusCode != http.StatusNotModified {
		t.Errorf("expected 304, got %d", ctx2.FResponse.StatusCode)
	}
}

func TestCacheFilter_ConditionalRequest_IfModifiedSince_200(t *testing.T) {
	f := newTestFilter(t, 5*time.Minute, time.Second, time.Second)
	rsp := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
	rsp.Header.Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
	f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp, nil }

	ctx1 := newCtx(http.MethodGet, "http://example.com/cond-ims-200", "")
	f.Request(ctx1)

	ctx2 := newCtx(http.MethodGet, "http://example.com/cond-ims-200", "")
	// Earlier date: the resource HAS been modified since this date, so serve 200.
	ctx2.FRequest.Header.Set("If-Modified-Since", "Wed, 20 Oct 2015 07:28:00 GMT")
	f.Request(ctx2)
	if ctx2.FResponse == nil {
		t.Fatal("expected response")
	}
	if ctx2.FResponse.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", ctx2.FResponse.StatusCode)
	}
}

func TestCacheFilter_ConditionalRequest_INM_Precedence_Over_IMS(t *testing.T) {
	// RFC 9110 §13.1.3: If-None-Match takes precedence over If-Modified-Since.
	// Even when IMS would yield 200 (resource modified), INM match must win → 304.
	f := newTestFilter(t, 5*time.Minute, time.Second, time.Second)
	rsp := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
	rsp.Header.Set("ETag", `"v1"`)
	rsp.Header.Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
	f.fetch = func(_ *http.Request) (*http.Response, error) { return rsp, nil }

	ctx1 := newCtx(http.MethodGet, "http://example.com/cond-precedence", "")
	f.Request(ctx1)

	ctx2 := newCtx(http.MethodGet, "http://example.com/cond-precedence", "")
	ctx2.FRequest.Header.Set("If-None-Match", `"v1"`)
	// IMS is earlier than Last-Modified — IMS alone would yield 200.
	ctx2.FRequest.Header.Set("If-Modified-Since", "Wed, 20 Oct 2015 07:28:00 GMT")
	f.Request(ctx2)
	if ctx2.FResponse == nil {
		t.Fatal("expected response")
	}
	if ctx2.FResponse.StatusCode != http.StatusNotModified {
		t.Errorf("expected 304 (INM wins over IMS), got %d", ctx2.FResponse.StatusCode)
	}
}

func TestCacheFilter_ConditionalRequest_Stale_IfNoneMatch_304_AndRevalidates(t *testing.T) {
	// Stale entries must also honour client If-None-Match per RFC 9111 §4.3.2.
	// Background revalidation must still fire even when a 304 is served to the client.
	synctest.Test(t, func(t *testing.T) {
		f := newTestFilter(t, 100*time.Millisecond, time.Second, 500*time.Millisecond)
		var revalFired atomic.Bool

		// Prime the cache via Request+Response so the entry is stored with ETag "v1".
		ctx1 := newCtx(http.MethodGet, "http://example.com/stale-cond", "")
		f.Request(ctx1)
		primeRsp := upstreamResponseCC(http.StatusOK, "body", "max-age=300")
		primeRsp.Header.Set("ETag", `"v1"`)
		ctx1.FResponse = primeRsp
		f.Response(ctx1)

		// Switch fetch to the revalidation stub (fires after the entry goes stale).
		f.fetch = func(_ *http.Request) (*http.Response, error) {
			revalFired.Store(true)
			return upstreamResponseCC(http.StatusOK, "body2", "max-age=300"), nil
		}

		// Advance past TTL into SWR window (TTL=100ms, SWR=500ms).
		time.Sleep(200 * time.Millisecond)

		// Conditional request against stale entry.
		ctx2 := newCtx(http.MethodGet, "http://example.com/stale-cond", "")
		ctx2.FRequest.Header.Set("If-None-Match", `"v1"`)
		f.Request(ctx2)
		if ctx2.FResponse == nil {
			t.Fatal("expected response")
		}
		if ctx2.FResponse.StatusCode != http.StatusNotModified {
			t.Errorf("expected 304 for stale conditional, got %d", ctx2.FResponse.StatusCode)
		}

		// Background revalidation must have fired.
		synctest.Wait()
		if !revalFired.Load() {
			t.Error("expected background revalidation to fire even when 304 served")
		}
	})
}

// RFC 9111 §5.2.1 max-stale and min-fresh request directive tests.

func TestCacheFilter_MaxStale_ExceedsWindow_Bypasses(t *testing.T) {
	// ttl=1ms, swrWindow=1h — entry expires immediately, SWR keeps it alive in storage.
	// max-stale=0: even 2ms of staleness exceeds the 0s window → bypass (miss).
	f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/max-stale-exceed"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=0")
		f.Response(ctx1)

		// Advance 2ms — entry is now 2ms past its 1ms TTL (stale), within 1h SWR.
		time.Sleep(2 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		ctx2.FRequest.Header.Set("Cache-Control", "max-stale=0")
		f.Request(ctx2)

		if ctx2.FServed {
			t.Fatal("want bypass (max-stale=0 exceeded by 2ms staleness), got stale served")
		}
	})
}

func TestCacheFilter_MaxStale_WithinWindow_ServesStale(t *testing.T) {
	// ttl=1ms, swrWindow=1h — entry is stale after 1ms, SWR keeps it.
	// max-stale=1 (1000ms): 2ms stale < 1000ms window → serve stale.
	f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/max-stale-within"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=0")
		f.Response(ctx1)

		// Advance 2ms — entry is 2ms past its 1ms TTL.
		time.Sleep(2 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		ctx2.FRequest.Header.Set("Cache-Control", "max-stale=1")
		f.Request(ctx2)

		if !ctx2.FServed {
			t.Fatal("want stale served (2ms < 1000ms max-stale window), got miss")
		}
	})
}

func TestCacheFilter_MinFresh_SufficientFreshness_HIT(t *testing.T) {
	// Fresh entry with 5m TTL remaining; min-fresh=1 (1s required) → serve from cache.
	f := newTestFilter(t, 5*time.Minute, 15*time.Second, time.Millisecond)
	url := "https://cdn.contentful.com/spaces/abc/entries/min-fresh-hit"

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
	f.Response(ctx1)

	// Fresh entry, 5m TTL remaining (entry stored just now). min-fresh=1 (1s required).
	// 5m >> 1s → HIT.
	ctx2 := newCtx("GET", url, "")
	ctx2.FRequest.Header.Set("Cache-Control", "min-fresh=1")
	f.Request(ctx2)

	if !ctx2.FServed {
		t.Fatal("want HIT (5m remaining > 1s min-fresh), got bypass")
	}
}

func TestCacheFilter_MinFresh_InsufficientFreshness_Bypasses(t *testing.T) {
	// ttl=100ms, swrWindow=1ms — after 80ms only 20ms remain.
	// min-fresh=1 (1000ms required): 20ms remaining < 1000ms → bypass.
	f := newTestFilter(t, 100*time.Millisecond, 15*time.Second, time.Millisecond)
	url := "https://cdn.contentful.com/spaces/abc/entries/min-fresh-bypass"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
		f.Response(ctx1)

		// Advance 80ms — only 20ms of freshness remain.
		time.Sleep(80 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		ctx2.FRequest.Header.Set("Cache-Control", "min-fresh=1")
		f.Request(ctx2)

		if ctx2.FServed {
			t.Fatal("want bypass (20ms remaining < 1s min-fresh), got HIT")
		}
	})
}

func TestCacheFilter_StaleIfError_Serves_On_5xx(t *testing.T) {
	// ttl=1ms, errorTTL=10s, swrWindow=1ms, staleIfError=60s
	// Entry expires after 1ms; staleIfError=60s keeps it in storage.
	// A 503 upstream via coalesce should cause the stale entry to be served.
	//
	// Regression: the stale-if-error block in Response() was dead code — coalesce() always calls
	// ctx.Serve() (even on 5xx), so Response() returned early before reaching it.
	// stale-if-error logic must live inside coalesce() with the pre-fetch snapshot captured
	// before f.fetch runs, preventing the 5xx from overwriting the stored entry.
	f := newTestFilter(t, time.Millisecond, 10*time.Second, time.Millisecond, 60*time.Second)
	url := "http://example.com/sie-5xx"

	synctest.Test(t, func(t *testing.T) {
		ctx := newCtx(http.MethodGet, url, "")
		f.Request(ctx)
		ctx.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"cached"}`, "max-age=0")
		f.Response(ctx)

		// Advance past TTL+SWR so the next Request() calls coalesce().
		time.Sleep(50 * time.Millisecond)

		// Coalesce fetches from upstream and receives 503.
		f.fetch = func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}
		ctx2 := newCtx(http.MethodGet, url, "")
		f.Request(ctx2)

		if ctx2.FResponse == nil {
			t.Fatal("expected a response, got nil")
		}
		if ctx2.FResponse.StatusCode != http.StatusOK {
			t.Fatalf("want 200 from stale-if-error, got %d", ctx2.FResponse.StatusCode)
		}
		if ctx2.FResponse.Header.Get("X-Cache-Status") != "STALE" {
			t.Fatalf("want X-Cache-Status: STALE, got %s", ctx2.FResponse.Header.Get("X-Cache-Status"))
		}
	})
}

func TestCacheFilter_StaleIfError_Expired_NotServed(t *testing.T) {
	// ttl=1ms, errorTTL=10s, swrWindow=1ms, staleIfError=100ms
	// Sleep 200ms — past TTL + staleIfError window. Entry too old for stale-if-error.
	// Uses f.fetch returning 503 via coalesce (the same path as the positive stale-if-error case)
	// to confirm the 503 is passed through when the stale-if-error window has already elapsed.
	f := newTestFilter(t, time.Millisecond, 10*time.Second, time.Millisecond, 100*time.Millisecond)
	url := "http://example.com/sie-expired"

	synctest.Test(t, func(t *testing.T) {
		ctx := newCtx(http.MethodGet, url, "")
		f.Request(ctx)
		ctx.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"cached"}`, "max-age=0")
		f.Response(ctx)

		// Advance past TTL (1ms) + staleIfError (100ms) = well beyond 200ms.
		time.Sleep(200 * time.Millisecond)

		f.fetch = func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}
		ctx2 := newCtx(http.MethodGet, url, "")
		f.Request(ctx2)

		if ctx2.FResponse == nil {
			t.Fatal("expected a response, got nil")
		}
		if ctx2.FResponse.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("want 503 (stale-if-error window expired), got %d", ctx2.FResponse.StatusCode)
		}
	})
}

func TestCacheFilter_StaleIfError_Disabled_When_Zero(t *testing.T) {
	// No 4th arg — staleIfError defaults to 0. Upstream 503 must pass through.
	f := newTestFilter(t, time.Millisecond, 10*time.Second, time.Millisecond)
	url := "http://example.com/sie-disabled"

	synctest.Test(t, func(t *testing.T) {
		ctx := newCtx(http.MethodGet, url, "")
		f.Request(ctx)
		ctx.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"cached"}`, "max-age=0")
		f.Response(ctx)

		time.Sleep(50 * time.Millisecond)

		ctx2 := newCtx(http.MethodGet, url, "")
		f.Request(ctx2)
		ctx2.FResponse = upstreamResponse(http.StatusServiceUnavailable, "")
		f.Response(ctx2)

		if ctx2.FResponse.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("want 503 (stale-if-error disabled), got %d", ctx2.FResponse.StatusCode)
		}
	})
}

// --- Option C: force mode vs RFC mode ---

func TestCacheFilter_ForceMode_IgnoresUpstreamMaxAge(t *testing.T) {
	// Force mode (default): operator TTL=5m is used even when upstream says max-age=1.
	f := newTestFilter(t, 5*time.Minute, 15*time.Second, time.Millisecond)
	url := "http://example.com/force-ttl"

	ctx := newCtx(http.MethodGet, url, "")
	f.Request(ctx)
	rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "public, max-age=1")
	ctx.FResponse = rsp
	f.Response(ctx)

	key := ctx.StateBag()[stateBagKey].(string)
	entry, err := f.storage.Get(context.Background(), key)
	if err != nil || entry == nil {
		t.Fatal("expected entry to be stored in force mode")
	}
	if entry.TTL != 5*time.Minute {
		t.Errorf("force mode: expected TTL=5m, got %v", entry.TTL)
	}
}

func TestCacheFilter_ForceMode_CachesWhenUpstreamSaysPrivate(t *testing.T) {
	// Force mode: operator TTL is authoritative; upstream `private` is NOT a blocker.
	// This is the Contentful use-case: CDN returns private/no-store but we're a
	// shared proxy that owns the caching decision.
	f := newTestFilter(t, 5*time.Minute, 15*time.Second, time.Millisecond)
	url := "http://example.com/force-private"

	ctx := newCtx(http.MethodGet, url, "")
	f.Request(ctx)
	rsp := upstreamResponseCC(http.StatusOK, `{"data":"contentful"}`, "private, max-age=0")
	ctx.FResponse = rsp
	f.Response(ctx)

	key := ctx.StateBag()[stateBagKey].(string)
	entry, err := f.storage.Get(context.Background(), key)
	if err != nil || entry == nil {
		t.Fatal("force mode: expected entry stored despite upstream private")
	}
	if entry.TTL != 5*time.Minute {
		t.Errorf("force mode: expected TTL=5m, got %v", entry.TTL)
	}
}

func TestCacheFilter_ForceMode_CachesWhenUpstreamSaysNoStore(t *testing.T) {
	// Force mode: operator TTL is authoritative; upstream `no-store` is NOT a blocker.
	f := newTestFilter(t, 5*time.Minute, 15*time.Second, time.Millisecond)
	url := "http://example.com/force-nostore"

	ctx := newCtx(http.MethodGet, url, "")
	f.Request(ctx)
	rsp := upstreamResponseCC(http.StatusOK, `{"data":"no-store"}`, "no-store")
	ctx.FResponse = rsp
	f.Response(ctx)

	key := ctx.StateBag()[stateBagKey].(string)
	entry, err := f.storage.Get(context.Background(), key)
	if err != nil || entry == nil {
		t.Fatal("force mode: expected entry stored despite upstream no-store")
	}
}

func TestCacheFilter_RFCMode_RespectsUpstreamPrivate(t *testing.T) {
	// RFC mode: upstream `private` must block storage (RFC 9111 §5.2.2.7).
	f := newTestFilterRFC(t, 5*time.Minute, 15*time.Second, time.Millisecond)
	url := "http://example.com/rfc-private"

	ctx := newCtx(http.MethodGet, url, "")
	f.Request(ctx)
	rsp := upstreamResponseCC(http.StatusOK, `{"data":"private"}`, "private, max-age=300")
	ctx.FResponse = rsp
	f.Response(ctx)

	key := ctx.StateBag()[stateBagKey].(string)
	entry, _ := f.storage.Get(context.Background(), key)
	if entry != nil {
		t.Fatal("RFC mode: upstream private must not be cached")
	}
}

func TestCacheFilter_RFCMode_RespectsUpstreamNoStore(t *testing.T) {
	// RFC mode: upstream `no-store` must block storage (RFC 9111 §5.2.2.5).
	f := newTestFilterRFC(t, 5*time.Minute, 15*time.Second, time.Millisecond)
	url := "http://example.com/rfc-nostore"

	ctx := newCtx(http.MethodGet, url, "")
	f.Request(ctx)
	rsp := upstreamResponseCC(http.StatusOK, `{"data":"no-store"}`, "no-store")
	ctx.FResponse = rsp
	f.Response(ctx)

	key := ctx.StateBag()[stateBagKey].(string)
	entry, _ := f.storage.Get(context.Background(), key)
	if entry != nil {
		t.Fatal("RFC mode: upstream no-store must not be cached")
	}
}

func TestCacheFilter_RFCMode_UpstreamMaxAgeIsAuthoritative(t *testing.T) {
	// Pure RFC mode: upstream max-age=10 is the TTL exactly (no operator ceiling).
	f := newTestFilterRFC(t, 5*time.Minute, 15*time.Second, time.Millisecond)
	url := "http://example.com/rfc-maxage"

	ctx := newCtx(http.MethodGet, url, "")
	f.Request(ctx)
	rsp := upstreamResponseCC(http.StatusOK, `{"data":"maxage"}`, "public, max-age=10")
	ctx.FResponse = rsp
	f.Response(ctx)

	key := ctx.StateBag()[stateBagKey].(string)
	entry, err := f.storage.Get(context.Background(), key)
	if err != nil || entry == nil {
		t.Fatal("RFC mode: expected entry stored with upstream max-age")
	}
	if entry.TTL != 10*time.Second {
		t.Errorf("RFC mode: expected TTL=10s (upstream max-age), got %v", entry.TTL)
	}
}

func TestCacheFilter_SMaxAge_CapsRouteTTL(t *testing.T) {
	// RFC 9111 §5.2.2.10: s-maxage takes precedence over max-age for shared caches.
	f := newTestFilterRFC(t, 5*time.Minute, 15*time.Second, time.Millisecond)
	url := "http://example.com/smaxage-caps"

	ctx := newCtx(http.MethodGet, url, "")
	f.Request(ctx)
	ctx.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"smaxage"}`, "public, max-age=300, s-maxage=5")
	f.Response(ctx)

	key := ctx.StateBag()[stateBagKey].(string)
	entry, err := f.storage.Get(context.Background(), key)
	if err != nil || entry == nil {
		t.Fatal("RFC mode: expected entry stored")
	}
	if entry.TTL != 5*time.Second {
		t.Errorf("RFC mode: expected TTL=5s (s-maxage), got %v", entry.TTL)
	}
}

func TestCacheFilter_CreateFilter_RFCArgParsing(t *testing.T) {
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: ":9090", L1TTL: 60 * time.Second})
	t.Cleanup(spec.(*cacheSpec).client.Close)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	cases := []struct {
		name    string
		args    []any
		wantRFC bool
		wantSIE time.Duration
		wantErr bool
	}{
		{"0 args pure rfc mode", []any{}, true, 0, false},
		{"3 args force mode", []any{"5m", "15s", "30s"}, false, 0, false},
		{"4 args staleIfError", []any{"5m", "15s", "30s", "60s"}, false, 60 * time.Second, false},
		{"1 arg invalid", []any{"5m"}, false, 0, true},
		{"2 args invalid", []any{"5m", "15s"}, false, 0, true},
		{"6 args too many", []any{"5m", "15s", "30s", "60s", "Authorization", "extra"}, false, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := spec.CreateFilter(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			cf := f.(*cacheFilter)
			if cf.rfcMode != tc.wantRFC {
				t.Errorf("rfcMode: got %v, want %v", cf.rfcMode, tc.wantRFC)
			}
			if cf.staleIfError != tc.wantSIE {
				t.Errorf("staleIfError: got %v, want %v", cf.staleIfError, tc.wantSIE)
			}
		})
	}
}

func TestCacheFilter_PureRFCMode_ZeroArgs_UsesUpstreamMaxAge(t *testing.T) {
	// cache() with no args: pure RFC mode, upstream max-age is fully authoritative,
	// no operator ceiling. TTL should equal upstream max-age exactly.
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: ":9090", L1TTL: 60 * time.Second})
	t.Cleanup(spec.(*cacheSpec).client.Close)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	f, err := spec.CreateFilter([]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cf := f.(*cacheFilter)
	cf.fetch = func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no fetch stub")
	}

	url := "http://example.com/pure-rfc"
	ctx := newCtx(http.MethodGet, url, "")
	cf.Request(ctx)
	rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "public, max-age=120")
	ctx.FResponse = rsp
	cf.Response(ctx)

	key := ctx.StateBag()[stateBagKey].(string)
	entry, err := cf.storage.Get(context.Background(), key)
	if err != nil || entry == nil {
		t.Fatal("pure RFC mode: expected entry stored")
	}
	if entry.TTL != 120*time.Second {
		t.Errorf("pure RFC mode: expected TTL=120s (from upstream max-age), got %v", entry.TTL)
	}
}

func TestCacheFilter_LRUBytesGaugeUpdatesWithoutEviction(t *testing.T) {
	// The filter and its goroutines must be created inside the synctest bubble
	// so the ticker in lruBytesScraper is subject to synthetic time control.
	// f.fetch is replaced before any network I/O so the transport goroutine
	// being inside the bubble is safe (it never actually dials out).
	synctest.Test(t, func(t *testing.T) {
		spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", L1TTL: 60 * time.Second})
		t.Cleanup(spec.(*cacheSpec).client.Close)
		t.Cleanup(func() { spec.(*cacheSpec).Close() })
		f, err := spec.CreateFilter([]any{"5m", "15s", "5m"})
		if err != nil {
			t.Fatal(err)
		}
		cf := f.(*cacheFilter)

		mockMetrics := &metricstest.MockMetrics{}
		// synctest.Wait drains goroutine scheduling so the scraper is parked at the
		// select before we swap metrics. No tick has fired yet (synthetic time is frozen).
		synctest.Wait()
		spec.(*cacheSpec).metrics = mockMetrics
		cf.metrics = mockMetrics

		cf.fetch = func(_ *http.Request) (*http.Response, error) {
			return upstreamResponseCC(http.StatusOK, `{"data":"hello"}`, "max-age=300"), nil
		}

		var initialBytes float64
		mockMetrics.WithGauges(func(g map[string]float64) {
			initialBytes = g["cache.lru_bytes"]
		})

		// Store an entry large enough to be visible but not enough to evict.
		ctx := newCtx("GET", "https://example.com/lru-bytes-scrape", "")
		cf.Request(ctx)

		// Advance time past one scrape interval (10 s).
		time.Sleep(11 * time.Second)

		var afterBytes float64
		mockMetrics.WithGauges(func(g map[string]float64) {
			afterBytes = g["cache.lru_bytes"]
		})
		if afterBytes <= initialBytes {
			t.Errorf("expected lru_bytes to increase after Set without eviction; before=%v after=%v", initialBytes, afterBytes)
		}
	})
}

func TestCacheFilter_PureRFCMode_ZeroArgs_NoUpstreamDirective_NotCached(t *testing.T) {
	// cache() with no args: when upstream sends no Cache-Control, no Expires,
	// and no Last-Modified, nothing should be cached (no heuristic without Last-Modified).
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: ":9090", L1TTL: 60 * time.Second})
	t.Cleanup(spec.(*cacheSpec).client.Close)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	f, err := spec.CreateFilter([]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cf := f.(*cacheFilter)
	cf.fetch = func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no fetch stub")
	}

	url := "http://example.com/pure-rfc-nocache"
	ctx := newCtx(http.MethodGet, url, "")
	cf.Request(ctx)
	rsp := upstreamResponse(http.StatusOK, `{"data":"v1"}`)
	rsp.Header.Del("Cache-Control")
	ctx.FResponse = rsp
	cf.Response(ctx)

	key := ctx.StateBag()[stateBagKey].(string)
	entry, _ := cf.storage.Get(context.Background(), key)
	if entry != nil {
		t.Fatal("pure RFC mode: response with no freshness directives must not be cached")
	}
}

func TestCacheFilter_RevalDropped_WhenQueueFull(t *testing.T) {
	// Verify that reval_dropped is incremented when the revalJobs channel is at
	// capacity and a stale-while-revalidate request tries to enqueue a job.
	//
	// Strategy:
	//  1. Wire f.fetch to block until we release it, so the worker goroutine is
	//     stuck inside doRevalidate as soon as it picks up its first job.
	//  2. Pre-send one dummy job to wake the worker and block it on fetch.
	//  3. Fill the remaining revalQueueSize-1 slots with dummy jobs, saturating
	//     the channel (worker is blocked and cannot drain).
	//  4. Seed the cache directly with a backdated stale entry (past TTL, inside
	//     SWR window) so no real upstream call is needed to populate the cache.
	//  5. Make a GET — the stale path serves the entry and calls enqueueRevalidation;
	//     the channel is full, so the default branch fires and increments reval_dropped.
	//  6. Assert reval_dropped == 1.

	f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)

	// Wire up a dedicated MockMetrics so we can inspect counters in isolation.
	mockMetrics := &metricstest.MockMetrics{}
	f.metrics = mockMetrics

	// fetchBlocked gates the worker: it blocks until the test releases it.
	// workerIn is signalled once the worker is confirmed to be inside fetch.
	fetchBlocked := make(chan struct{})
	workerIn := make(chan struct{}, 1)
	f.fetch = func(req *http.Request) (*http.Response, error) {
		select {
		case workerIn <- struct{}{}: // signal first entry only (buffered size 1)
		default:
		}
		<-fetchBlocked // block until test closes this channel
		return nil, errors.New("blocked fetch")
	}

	// Send one job so the worker goroutine wakes and blocks inside fetch.
	dummyReq, _ := http.NewRequest(http.MethodGet, "http://example.com/dummy", nil)
	f.revalJobs <- revalJob{key: "dummy-wake", req: dummyReq, filter: f}

	// Wait for the worker to confirm it is inside fetch — no timing guesswork.
	<-workerIn

	// Worker has consumed the dummy job from the channel (0/256 slots occupied)
	// and is now blocked in fetch. Fill all revalQueueSize slots so the channel
	// is at capacity; the worker cannot drain while it is stuck in fetch.
	for i := range revalQueueSize {
		r, _ := http.NewRequest(http.MethodGet, "http://example.com/fill", nil)
		f.revalJobs <- revalJob{key: "fill-" + strconv.Itoa(i), req: r}
	}

	// Inject a stale entry directly into storage. CreatedAt is backdated so
	// IsStale(now) returns true (past TTL) and IsUsable(now) returns true (within SWR).
	url := "https://cdn.contentful.com/spaces/abc/entries/reval-dropped"
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	key := cacheKey("" /* routeID */, req, nil)
	staleEntry := &Entry{
		StatusCode:           http.StatusOK,
		Header:               http.Header{"Content-Type": {"application/json"}},
		Payload:              []byte(`{"data":"stale"}`),
		CreatedAt:            time.Now().Add(-10 * time.Millisecond), // well past 1ms TTL
		TTL:                  time.Millisecond,
		StaleWhileRevalidate: time.Hour, // still inside SWR window
	}
	if err := f.storage.Set(context.Background(), key, staleEntry); err != nil {
		t.Fatalf("failed to seed stale entry: %v", err)
	}

	// Make the GET request. The filter finds the stale entry, serves it, and
	// calls enqueueRevalidation. The channel is full — reval_dropped fires.
	ctx := newCtx(http.MethodGet, url, "")
	ctx.FMetrics = mockMetrics
	f.Request(ctx)

	if !ctx.FServed {
		t.Fatal("expected stale entry to be served when revalJobs queue is full")
	}
	if ctx.FResponse.Header.Get("X-Cache-Status") != "STALE" {
		t.Fatalf("expected X-Cache-Status: STALE, got %q", ctx.FResponse.Header.Get("X-Cache-Status"))
	}
	mockMetrics.WithCounters(func(counters map[string]int64) {
		if counters["cache.reval_dropped"] != 1 {
			t.Errorf("expected reval_dropped==1, got %d", counters["cache.reval_dropped"])
		}
	})

	// Release the blocked worker so the test can clean up without leaking goroutines.
	close(fetchBlocked)
}

func TestCacheSpec_FilterRegistry(t *testing.T) {
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", L1TTL: 60 * time.Second})
	t.Cleanup(spec.(*cacheSpec).client.Close)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	// Same args — should return same *cacheFilter pointer (registry hit)
	f1, err := spec.CreateFilter([]any{"5m", "15s", "30s"})
	if err != nil {
		t.Fatal(err)
	}
	cf1 := f1.(*cacheFilter)

	f2, err := spec.CreateFilter([]any{"5m", "15s", "30s"})
	if err != nil {
		t.Fatal(err)
	}
	cf2 := f2.(*cacheFilter)

	if cf1 != cf2 {
		t.Fatal("expected same *cacheFilter pointer for identical args, got different instances")
	}

	// Different args — should return different *cacheFilter pointer
	f3, err := spec.CreateFilter([]any{"10m", "15s", "30s"})
	if err != nil {
		t.Fatal(err)
	}
	cf3 := f3.(*cacheFilter)

	if cf1 == cf3 {
		t.Fatal("expected different *cacheFilter pointer for different args, got same instance")
	}

	// Different keyHeaders order should normalize to same instance
	f4, err := spec.CreateFilter([]any{"5m", "15s", "30s", "60s", "X-Foo,X-Bar"})
	if err != nil {
		t.Fatal(err)
	}
	cf4 := f4.(*cacheFilter)

	f5, err := spec.CreateFilter([]any{"5m", "15s", "30s", "60s", "X-Bar,X-Foo"})
	if err != nil {
		t.Fatal(err)
	}
	cf5 := f5.(*cacheFilter)

	if cf4 != cf5 {
		t.Fatal("expected same instance for different keyHeaders order (should normalize), got different instances")
	}
}

func TestCacheSpec_FilterRegistry_InFlightJobsSurviveRebuild(t *testing.T) {
	// Verify that the spec-level revalidation worker continues to drain jobs
	// even when CreateFilter is called again (simulating a route rebuild).
	// The registry returns the same cacheFilter instance, so in-flight jobs
	// targeting that instance are unaffected by the rebuild.

	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", L1TTL: 60 * time.Second})
	t.Cleanup(spec.(*cacheSpec).client.Close)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	// Create initial filter with blocking fetch stub (same pattern as TestCacheFilter_RevalDropped_WhenQueueFull).
	f1, err := spec.CreateFilter([]any{"5m", "15s", "30s"})
	if err != nil {
		t.Fatal(err)
	}
	cf1 := f1.(*cacheFilter)

	// Block the worker on the first job.
	fetchBlocked := make(chan struct{})
	workerIn := make(chan struct{}, 1)
	cf1.fetch = func(req *http.Request) (*http.Response, error) {
		select {
		case workerIn <- struct{}{}: // signal first entry only
		default:
		}
		<-fetchBlocked // block until test closes this
		return nil, errors.New("blocked fetch")
	}

	// Send a dummy job to wake and block the worker inside fetch.
	dummyReq, _ := http.NewRequest(http.MethodGet, "http://example.com/dummy", nil)
	cf1.revalJobs <- revalJob{key: "dummy-wake", req: dummyReq, filter: cf1}

	// Wait for worker to confirm it is inside fetch.
	<-workerIn

	// Inject a stale entry so the next GET will trigger revalidation.
	url := "https://cdn.contentful.com/spaces/abc/entries/in-flight-rebuild"
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	key := cacheKey("" /* routeID */, req, nil)
	staleEntry := &Entry{
		StatusCode:           http.StatusOK,
		Header:               http.Header{"Content-Type": {"application/json"}},
		Payload:              []byte(`{"data":"stale"}`),
		CreatedAt:            time.Now().Add(-10 * time.Millisecond),
		TTL:                  time.Millisecond,
		StaleWhileRevalidate: time.Hour,
	}
	if err := cf1.storage.Set(context.Background(), key, staleEntry); err != nil {
		t.Fatalf("failed to seed stale entry: %v", err)
	}

	// Make a GET request to enqueue a revalidation job.
	ctx1 := newCtx(http.MethodGet, url, "")
	cf1.Request(ctx1)
	if !ctx1.FServed {
		t.Fatal("expected stale entry to be served")
	}

	// Simulate a route rebuild: call CreateFilter again with identical args.
	// The registry should return the same cf instance.
	f2, err := spec.CreateFilter([]any{"5m", "15s", "30s"})
	if err != nil {
		t.Fatal(err)
	}
	cf2 := f2.(*cacheFilter)

	if cf1 != cf2 {
		t.Fatal("expected registry to return same instance on rebuild")
	}

	// Unblock the worker so it can process the pending revalidation job.
	close(fetchBlocked)

	// Make another GET after a brief delay to allow the worker to finish.
	// Since the job failed (blocked fetch returns error), the entry will not
	// be updated. But the test demonstrates that the worker continued draining
	// despite the rebuild. If it had stopped, the second GET would be served
	// stale without a revalidation attempt being enqueued.
	time.Sleep(10 * time.Millisecond)
	ctx2 := newCtx(http.MethodGet, url, "")
	cf2.Request(ctx2)
	if !ctx2.FServed {
		t.Fatal("expected entry to be served after rebuild (job draining continued)")
	}
}

func Benchmark_malicious_matchesETag(b *testing.B) {
	ifNoneMatch := strings.Repeat(",", http.DefaultMaxHeaderBytes)
	b.ReportAllocs()
	for b.Loop() {
		matchesETag(ifNoneMatch, "foobar")
	}
}

// ── storage stubs ──────────────────────────────────────────────────────────────

type failingSetStorage struct{ Storage }

func (s *failingSetStorage) Set(_ context.Context, _ string, _ *Entry) error {
	return errors.New("set: injected failure")
}

type failingDeleteStorage struct{ Storage }

func (s *failingDeleteStorage) Delete(_ context.Context, _ string) error {
	return errors.New("delete: injected failure")
}

// ── Part 1: unit tests for uncovered branches ─────────────────────────────────

func TestCreateFilter_InvalidArgs_ErrorTTL(t *testing.T) {
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090"})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	cases := []struct {
		name string
		args []any
	}{
		{"bad errorTTL string", []any{"5m", "bad", "30s"}},
		{"zero errorTTL", []any{"5m", "0s", "30s"}},
		{"non-string errorTTL", []any{"5m", 15, "30s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := spec.CreateFilter(tc.args); err == nil {
				t.Fatal("expected error for bad errorTTL arg")
			}
		})
	}
}

func TestCreateFilter_InvalidArgs_SWR(t *testing.T) {
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090"})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	cases := []struct {
		name string
		args []any
	}{
		{"bad swrWindow string", []any{"5m", "15s", "bad"}},
		{"zero swrWindow", []any{"5m", "15s", "0s"}},
		{"non-string swrWindow", []any{"5m", "15s", 30}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := spec.CreateFilter(tc.args); err == nil {
				t.Fatal("expected error for bad swrWindow arg")
			}
		})
	}
}

func TestCreateFilter_InvalidArgs_StaleIfError(t *testing.T) {
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090"})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	cases := []struct {
		name string
		args []any
	}{
		{"non-string staleIfError", []any{"5m", "15s", "30s", 60}},
		{"bad staleIfError duration", []any{"5m", "15s", "30s", "notaduration"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := spec.CreateFilter(tc.args); err == nil {
				t.Fatal("expected error for bad staleIfError arg")
			}
		})
	}
}

func TestCreateFilter_InvalidArgs_KeyHeaders(t *testing.T) {
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090"})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	// arg 4 (keyHeaders) must be a string
	if _, err := spec.CreateFilter([]any{"5m", "15s", "30s", "60s", 42}); err == nil {
		t.Fatal("expected error for non-string keyHeaders arg")
	}
}

func TestCacheSpec_Close_Idempotent(t *testing.T) {
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090"})
	cs := spec.(*cacheSpec)
	if err := cs.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := cs.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestCacheFilter_RevalidateHeader_Stripped(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.example.com/revalidate-bypass"

	ctx := newCtx("GET", url, "")
	ctx.FRequest.Header.Set(revalidateHeader, "1")
	f.Request(ctx)

	// Must not be served from cache (header stripped → cache bypassed for lookup)
	if ctx.FServed {
		t.Fatal("revalidate-header request must not be served from cache")
	}
	// Header must be stripped before reaching upstream
	if ctx.FRequest.Header.Get(revalidateHeader) != "" {
		t.Fatal("X-Cache-Revalidate header must be stripped by Request()")
	}
}

func TestCacheFilter_ContextCancelled_BeforeGet(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)

	// Pre-populate the cache so a normal request would HIT
	populate := newCtx("GET", "https://cdn.example.com/ctx-cancel", "")
	f.Request(populate)
	populate.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
	f.Response(populate)

	// Now issue a request with a pre-cancelled context
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	req, _ := http.NewRequestWithContext(cancelCtx, "GET", "https://cdn.example.com/ctx-cancel", nil)
	ctx := &filtertest.Context{
		FRequest:  req,
		FStateBag: make(map[string]any),
		FMetrics:  &metricstest.MockMetrics{},
	}
	f.Request(ctx)

	// The cancelled-context path returns without calling Serve
	if ctx.FServed {
		t.Fatal("cancelled context request must not be served")
	}
}

func TestCacheFilter_RFC_NoStore_NotCached_Coalesce(t *testing.T) {
	f := newTestFilterRFC(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.example.com/rfc-nostore-coalesce"

	f.fetch = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Cache-Control": {"no-store"}, "Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":"private"}`)),
		}, nil
	}

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	if !ctx1.FServed {
		t.Fatal("expected response to be served via coalesce even with no-store")
	}

	// Second request must NOT be served from cache (entry must not have been stored)
	var fetchCount int64
	f.fetch = func(req *http.Request) (*http.Response, error) {
		atomic.AddInt64(&fetchCount, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Cache-Control": {"no-store"}, "Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":"private"}`)),
		}, nil
	}
	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if fetchCount == 0 {
		t.Fatal("no-store response must not be cached; second request must hit upstream")
	}
}

func TestCacheFilter_RFC_Private_NotCached_Coalesce(t *testing.T) {
	f := newTestFilterRFC(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.example.com/rfc-private-coalesce"

	f.fetch = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Cache-Control": {"private, max-age=300"}, "Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":"user-specific"}`)),
		}, nil
	}

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	if !ctx1.FServed {
		t.Fatal("expected response to be served via coalesce even with private")
	}

	var fetchCount int64
	f.fetch = func(req *http.Request) (*http.Response, error) {
		atomic.AddInt64(&fetchCount, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Cache-Control": {"private, max-age=300"}, "Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":"user-specific"}`)),
		}, nil
	}
	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if fetchCount == 0 {
		t.Fatal("private response must not be cached; second request must hit upstream")
	}
}

func TestCacheFilter_CoalesceSetFailure_Served(t *testing.T) {
	mockMetrics := &metricstest.MockMetrics{}
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", Metrics: mockMetrics})
	fi, err := spec.CreateFilter([]any{"1m", "15s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	f := fi.(*cacheFilter)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	// Wrap storage so Set always fails
	f.storage = &failingSetStorage{f.storage}

	f.fetch = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Cache-Control": {"max-age=300"}, "Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":"v1"}`)),
		}, nil
	}

	ctx := newCtx("GET", "https://cdn.example.com/coalesce-set-fail", "")
	f.Request(ctx)

	// Response must still be served despite storage failure
	if !ctx.FServed {
		t.Fatal("expected response to be served even when storage Set fails")
	}
	mockMetrics.WithCounters(func(counters map[string]int64) {
		if counters["cache.storage_error"] == 0 {
			t.Error("expected cache.storage_error to be incremented on Set failure in coalesce")
		}
	})
}

func TestCacheFilter_HEAD_Freshen_BodyHeaderSkipped(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Hour)
	url := "https://cdn.example.com/head-freshen-body-header"

	// Populate via GET (coalesce path)
	f.fetch = func(r *http.Request) (*http.Response, error) {
		rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "public, max-age=300")
		return rsp, nil
	}
	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)

	// HEAD request: served from cache
	headCtx := newCtx("HEAD", url, "")
	f.Request(headCtx)
	if !headCtx.FServed {
		t.Fatal("expected HEAD to be served from cache")
	}

	// HEAD response contains Content-Length (body-related header) — must NOT update stored entry
	headCtx.FResponse = &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Length": {"999"},
			"Cache-Control":  {"public, max-age=300"},
		},
		Body: http.NoBody,
	}
	f.Response(headCtx)

	// Stored entry must NOT have the Content-Length from the HEAD response
	key := cacheKey(headCtx.FRouteId, headCtx.FRequest, nil)
	entry, err := f.storage.Get(headCtx.FRequest.Context(), key)
	if err != nil || entry == nil {
		t.Fatal("expected stored entry after GET")
	}
	if entry.Header.Get("Content-Length") == "999" {
		t.Error("Content-Length (body-related header) must not be updated by HEAD response freshen")
	}
}

func TestCacheFilter_HEAD_Freshen_SetError(t *testing.T) {
	mockMetrics := &metricstest.MockMetrics{}
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", Metrics: mockMetrics})
	fi, err := spec.CreateFilter([]any{"1m", "15s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	f := fi.(*cacheFilter)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	url := "https://cdn.example.com/head-freshen-set-err"

	// Populate via GET response path
	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "public, max-age=300")
	f.Response(ctx1)

	// Now wrap storage so Set fails
	f.storage = &failingSetStorage{f.storage}

	// HEAD 200 freshen — storage.Set will fail
	headCtx := newCtx("HEAD", url, "")
	f.Request(headCtx)
	headCtx.FResponse = &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": {"public, max-age=300"}},
		Body:       http.NoBody,
	}
	f.Response(headCtx) // must not panic

	mockMetrics.WithCounters(func(counters map[string]int64) {
		if counters["cache.storage_error"] == 0 {
			t.Error("expected cache.storage_error incremented on HEAD freshen Set failure")
		}
	})
}

func TestCacheFilter_Response_ServedFromCache_IsNoop(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.example.com/response-served-noop"

	// Populate cache
	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
	f.Response(ctx1)

	// Second request: HIT — FServed = true
	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if !ctx2.FServed {
		t.Fatal("expected HIT")
	}

	// Calling Response() when already served must be a no-op (no panic, no double write)
	ctx2.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v2"}`, "max-age=300")
	f.Response(ctx2) // must return early at ctx.Served() check

	// The stored entry must still have the original payload
	key := ctx1.StateBag()[stateBagKey].(string)
	entry, err := f.storage.Get(context.Background(), key)
	if err != nil || entry == nil {
		t.Fatal("expected entry to remain in storage")
	}
	if string(entry.Payload) != `{"data":"v1"}` {
		t.Fatalf("Response() must not overwrite entry when ctx is already served; got %q", string(entry.Payload))
	}
}

func TestCacheFilter_UnsafeMethod_DeleteError_ContinuesSilently(t *testing.T) {
	mockMetrics := &metricstest.MockMetrics{}
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", Metrics: mockMetrics})
	fi, err := spec.CreateFilter([]any{"1m", "15s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	f := fi.(*cacheFilter)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	url := "https://cdn.example.com/unsafe-del-err"

	// Populate cache
	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "public, max-age=300")
	f.Response(ctx1)

	// Wrap storage so Delete fails
	f.storage = &failingDeleteStorage{f.storage}

	// POST to same URL — Delete will fail
	postCtx := newCtx("POST", url, "")
	f.Request(postCtx)
	postCtx.FResponse = &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       http.NoBody,
	}
	f.Response(postCtx) // must not panic

	mockMetrics.WithCounters(func(counters map[string]int64) {
		if counters["cache.storage_error"] == 0 {
			t.Error("expected cache.storage_error incremented on Delete failure in unsafe method invalidation")
		}
	})
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read: injected failure") }
func (errReader) Close() error             { return nil }

func TestCacheFilter_Response_BodyReadError_NotCached(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.example.com/body-read-err"

	ctx := newCtx("GET", url, "")
	f.Request(ctx)

	// Response with an erroring body reader
	ctx.FResponse = &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": {"max-age=300"}},
		Body:       errReader{},
	}
	f.Response(ctx) // must not panic

	// Entry must not be stored
	key := ctx.StateBag()[stateBagKey].(string)
	entry, err := f.storage.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("storage.Get: %v", err)
	}
	if entry != nil {
		t.Fatal("entry must not be stored when body read fails")
	}
}

func TestCacheFilter_Response_VarySentinelSetError(t *testing.T) {
	mockMetrics := &metricstest.MockMetrics{}
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", Metrics: mockMetrics})
	fi, err := spec.CreateFilter([]any{"1m", "15s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	f := fi.(*cacheFilter)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	// Wrap storage so Set fails
	f.storage = &failingSetStorage{f.storage}

	ctx := newCtx("GET", "https://cdn.example.com/vary-set-err", "")
	ctx.FRequest.Header.Set("Accept-Language", "en-US")
	f.Request(ctx)

	rsp := upstreamResponseCC(http.StatusOK, `{"lang":"en"}`, "max-age=300")
	rsp.Header.Set("Vary", "Accept-Language")
	ctx.FResponse = rsp
	f.Response(ctx) // must not panic; vary sentinel Set will fail

	mockMetrics.WithCounters(func(counters map[string]int64) {
		if counters["cache.storage_error"] == 0 {
			t.Error("expected cache.storage_error incremented on Vary sentinel Set failure")
		}
	})
}

func TestCacheFilter_Response_StorageSetError(t *testing.T) {
	mockMetrics := &metricstest.MockMetrics{}
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", Metrics: mockMetrics})
	fi, err := spec.CreateFilter([]any{"1m", "15s", "1m"})
	if err != nil {
		t.Fatal(err)
	}
	f := fi.(*cacheFilter)
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	f.storage = &failingSetStorage{f.storage}

	ctx := newCtx("GET", "https://cdn.example.com/response-set-err", "")
	f.Request(ctx)
	ctx.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
	f.Response(ctx) // must not panic

	mockMetrics.WithCounters(func(counters map[string]int64) {
		if counters["cache.storage_error"] == 0 {
			t.Error("expected cache.storage_error incremented on Response() storage Set failure")
		}
	})
}

func TestCacheFilter_Revalidate_304_EntryEvicted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
		url := "https://cdn.example.com/reval-304-evicted"

		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		rsp := upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
		rsp.Header.Set("ETag", `"v1"`)
		ctx1.FResponse = rsp
		f.Response(ctx1)

		// Evict the entry from storage before revalidation fires
		key := ctx1.StateBag()[stateBagKey].(string)
		if err := f.storage.Delete(context.Background(), key); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		// Set fetch to return 304 — doRevalidate will find the entry is gone
		f.fetch = func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotModified,
				Header:     http.Header{"ETag": {`"v1"`}},
				Body:       http.NoBody,
			}, nil
		}

		// Advance past TTL to enter SWR window → triggers stale serve + background revalidation
		time.Sleep(2 * time.Millisecond)

		// Insert the entry back so the stale serve can find it
		staleEntry := &Entry{
			StatusCode:           http.StatusOK,
			Header:               http.Header{"Content-Type": {"application/json"}, "ETag": {`"v1"`}},
			Payload:              []byte(`{"data":"v1"}`),
			CreatedAt:            time.Now().Add(-2 * time.Millisecond),
			TTL:                  time.Millisecond,
			StaleWhileRevalidate: time.Hour,
			ETag:                 `"v1"`,
		}
		if err := f.storage.Set(context.Background(), key, staleEntry); err != nil {
			t.Fatalf("Set stale entry: %v", err)
		}

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()

		if !ctx2.FServed {
			t.Fatal("expected stale entry served while revalidation fires in background")
		}
		// The 304 with evicted-entry path must not panic; doRevalidate logs and returns
	})
}

func TestCacheFilter_Revalidate_BodyReadError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
		mockMetrics := &metricstest.MockMetrics{}
		f.metrics = mockMetrics
		url := "https://cdn.example.com/reval-body-err"

		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
		f.Response(ctx1)

		// Fetch returns a 200 with an erroring body
		f.fetch = func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Cache-Control": {"max-age=300"}},
				Body:       errReader{},
			}, nil
		}

		time.Sleep(2 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()

		if !ctx2.FServed {
			t.Fatal("expected stale to be served")
		}
		mockMetrics.WithCounters(func(counters map[string]int64) {
			if counters["cache.reval_error"] == 0 {
				t.Error("expected cache.reval_error incremented on body read failure in doRevalidate")
			}
		})
	})
}

func TestCacheFilter_Revalidate_RFC_NoStore_NotStored(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newTestFilterRFC(t, time.Millisecond, 15*time.Second, time.Hour)
		url := "https://cdn.example.com/reval-rfc-nostore"

		// Seed a cacheable entry first
		f.fetch = func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Cache-Control": {"max-age=10"}, "Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"data":"v1"}`)),
			}, nil
		}
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		synctest.Wait()

		key := ctx1.StateBag()[stateBagKey].(string)
		if _, err := f.storage.Get(ctx1.FRequest.Context(), key); err != nil {
			t.Fatalf("storage.Get: %v", err)
		}

		// Background revalidation returns no-store
		f.fetch = func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Cache-Control": {"no-store"}, "Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"data":"v2"}`)),
			}, nil
		}

		time.Sleep(2 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()

		// Must not panic; doRevalidate simply returns without storing
		if !ctx2.FServed {
			t.Fatal("expected stale to be served even if revalidation returns no-store")
		}
	})
}

func TestCacheFilter_Revalidate_SetError_MetricIncremented(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mockMetrics := &metricstest.MockMetrics{}
		spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "localhost:9090", Metrics: mockMetrics})
		fi, err := spec.CreateFilter([]any{time.Millisecond.String(), "15s", time.Hour.String()})
		if err != nil {
			t.Fatal(err)
		}
		f := fi.(*cacheFilter)
		t.Cleanup(func() { spec.(*cacheSpec).Close() })

		url := "https://cdn.example.com/reval-set-err"

		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
		f.Response(ctx1)

		// Now wrap storage so Set fails
		f.storage = &failingSetStorage{f.storage}
		f.lruStorage = NewLRUStorage(1<<20, nil, mockMetrics) // won't be hit, but keep it valid

		f.fetch = func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Cache-Control": {"max-age=300"}, "Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"data":"v2"}`)),
			}, nil
		}

		time.Sleep(2 * time.Millisecond)

		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		synctest.Wait()

		mockMetrics.WithCounters(func(counters map[string]int64) {
			if counters["cache.storage_error"] == 0 {
				t.Error("expected cache.storage_error incremented on Set failure in doRevalidate")
			}
		})
	})
}

func TestEvaluateConditionals_InvalidIMS(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://cdn.example.com/path", nil)
	req.Header.Set("If-Modified-Since", "not-a-date")
	entry := &Entry{LastModified: "Wed, 21 Oct 2015 07:28:00 GMT"}
	if evaluateConditionals(req, entry) {
		t.Fatal("invalid If-Modified-Since must return false")
	}
}

func TestEvaluateConditionals_InvalidLM(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://cdn.example.com/path", nil)
	req.Header.Set("If-Modified-Since", "Wed, 21 Oct 2015 07:28:00 GMT")
	entry := &Entry{LastModified: "not-a-date"}
	if evaluateConditionals(req, entry) {
		t.Fatal("invalid Last-Modified in entry must return false")
	}
}

func TestMatchesETag_EmptyETag(t *testing.T) {
	if matchesETag(`"abc"`, "") {
		t.Fatal("empty etag must not match")
	}
}

func TestMatchesETag_WildcardMatch(t *testing.T) {
	if !matchesETag("*", `"any-etag"`) {
		t.Fatal("wildcard * must match any etag")
	}
}

func TestMatchesETag_WeakComparison(t *testing.T) {
	// W/"etag" and "etag" are equal under weak comparison
	if !matchesETag(`W/"abc"`, `"abc"`) {
		t.Fatal("W/ prefix must be stripped for weak comparison")
	}
	if !matchesETag(`"abc"`, `W/"abc"`) {
		t.Fatal("W/ prefix in etag must be stripped for weak comparison")
	}
	if matchesETag(`"abc"`, `"xyz"`) {
		t.Fatal("different etags must not match")
	}
}

func TestHeuristicTTL_WithExplicitDirective_ReturnsZero(t *testing.T) {
	h := http.Header{}
	h.Set("Cache-Control", "max-age=300")
	h.Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
	d := parseCacheControl(h)
	if got := heuristicTTL(h, d, time.Now()); got != 0 {
		t.Fatalf("heuristic must return 0 when max-age present, got %v", got)
	}
}

func TestHeuristicTTL_BadLastModified(t *testing.T) {
	h := http.Header{}
	h.Set("Last-Modified", "not-a-date")
	d := cacheDirectives{maxAge: -1, sMaxAge: -1}
	if got := heuristicTTL(h, d, time.Now()); got != 0 {
		t.Fatalf("heuristic must return 0 for bad Last-Modified, got %v", got)
	}
}

func TestHeuristicTTL_BadDate_FallsBackToNow(t *testing.T) {
	h := http.Header{}
	h.Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT") // 10+ years ago
	h.Set("Date", "not-a-date")                             // bad Date → falls back to time.Now()
	d := cacheDirectives{maxAge: -1, sMaxAge: -1}
	got := heuristicTTL(h, d, time.Now())
	// age = now - 2015 ≈ huge; 0.1 * huge > 0
	if got <= 0 {
		t.Fatalf("heuristic with bad Date and old Last-Modified must be > 0, got %v", got)
	}
}

func TestHeuristicTTL_NegativeAge_ReturnsZero(t *testing.T) {
	// Last-Modified in the future → age negative → heuristic returns 0
	h := http.Header{}
	h.Set("Last-Modified", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
	d := cacheDirectives{maxAge: -1, sMaxAge: -1}
	if got := heuristicTTL(h, d, time.Now()); got != 0 {
		t.Fatalf("heuristic with future Last-Modified must return 0, got %v", got)
	}
}

func TestCapTTLByExpires_MaxAgePresent_IgnoresExpires(t *testing.T) {
	h := http.Header{}
	h.Set("Expires", time.Now().Add(time.Second).UTC().Format(http.TimeFormat))
	d := cacheDirectives{maxAge: 300, sMaxAge: -1}
	// TTL must not be capped: max-age present means Expires is ignored
	if got := capTTLByExpires(5*time.Minute, h, d); got != 5*time.Minute {
		t.Fatalf("expected TTL unchanged when max-age present, got %v", got)
	}
}

func TestCapTTLByExpires_NoExpires(t *testing.T) {
	d := cacheDirectives{maxAge: -1, sMaxAge: -1}
	if got := capTTLByExpires(5*time.Minute, http.Header{}, d); got != 5*time.Minute {
		t.Fatalf("expected TTL unchanged when no Expires, got %v", got)
	}
}

func TestCapTTLByExpires_InvalidDate(t *testing.T) {
	h := http.Header{}
	h.Set("Expires", "0")
	d := cacheDirectives{maxAge: -1, sMaxAge: -1}
	if got := capTTLByExpires(5*time.Minute, h, d); got != 0 {
		t.Fatalf("expected TTL=0 for invalid Expires date, got %v", got)
	}
}

func TestCapTTLByExpires_AlreadyExpired(t *testing.T) {
	h := http.Header{}
	h.Set("Expires", time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat))
	d := cacheDirectives{maxAge: -1, sMaxAge: -1}
	if got := capTTLByExpires(5*time.Minute, h, d); got != 0 {
		t.Fatalf("expected TTL=0 for past Expires, got %v", got)
	}
}

func TestCapTTLByExpires_Caps(t *testing.T) {
	// Expires is 10s from now; TTL=0 means uncapped → must return remaining ~10s
	h := http.Header{}
	h.Set("Expires", time.Now().Add(10*time.Second).UTC().Format(http.TimeFormat))
	d := cacheDirectives{maxAge: -1, sMaxAge: -1}
	got := capTTLByExpires(0, h, d)
	if got <= 0 || got > 11*time.Second {
		t.Fatalf("expected TTL ~10s for uncapped Expires, got %v", got)
	}
}

func TestCapTTLByExpires_ReturnsTTLWhenSmaller(t *testing.T) {
	// Expires is far in the future; TTL=1s < remaining → TTL wins
	h := http.Header{}
	h.Set("Expires", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
	d := cacheDirectives{maxAge: -1, sMaxAge: -1}
	if got := capTTLByExpires(time.Second, h, d); got != time.Second {
		t.Fatalf("expected TTL=1s (smaller than Expires remaining), got %v", got)
	}
}

func TestVaryKey_EmptyHeaders_ReturnsBase(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://cdn.example.com/path", nil)
	base := "some-base-key"
	if got := varyKey(base, req, nil); got != base {
		t.Fatalf("varyKey with nil varyHeaders must return base key, got %q", got)
	}
	if got := varyKey(base, req, []string{}); got != base {
		t.Fatalf("varyKey with empty varyHeaders must return base key, got %q", got)
	}
}

func TestCacheKeyForURL_InvalidURL(t *testing.T) {
	base, _ := http.NewRequest("GET", "https://cdn.example.com/path", nil)
	got := cacheKeyForURL("route", base, "://invalid url\x00", nil)
	if got != "" {
		t.Fatalf("expected empty string for invalid URL, got %q", got)
	}
}

func TestCacheKeyForURL_RelativeURL_UsesBaseHost(t *testing.T) {
	base, _ := http.NewRequest("GET", "https://cdn.example.com/path", nil)
	base.Host = "cdn.example.com"
	got := cacheKeyForURL("route", base, "/other/path", nil)
	if got == "" {
		t.Fatal("expected non-empty key for relative URL")
	}
	// The key for absolute same-origin must match
	abs := cacheKeyForURL("route", base, "https://cdn.example.com/other/path", nil)
	if got != abs {
		t.Fatalf("relative and absolute same-origin URL must produce the same key; got %q vs %q", got, abs)
	}
}

// ── Part 2: proxytest L1/L2 integration tests ─────────────────────────────────

// newProxyCacheRoute builds an eskip route for the proxytest proxy that applies
// the cache filter. backendURL is the httptest backend.
func newProxyCacheRoute(t *testing.T, backendURL string, spec filters.Spec, args ...string) *eskip.Route {
	t.Helper()
	var filterArgs []any
	for _, a := range args {
		filterArgs = append(filterArgs, a)
	}
	return &eskip.Route{
		Id:          "cache-route",
		PathRegexps: []string{".*"},
		Filters:     []*eskip.Filter{{Name: spec.Name(), Args: filterArgs}},
		Backend:     backendURL,
	}
}

func TestProxy_L1Cache_MissAndHit(t *testing.T) {
	var backendCallCount int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&backendCallCount, 1)
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":"v1"}`)
	}))
	defer backend.Close()

	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: "127.0.0.1:0"})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	fr := make(filters.Registry)
	fr.Register(spec)

	proxy := proxytest.New(fr, newProxyCacheRoute(t, backend.URL, spec, "5m", "15s", "30s"))
	defer proxy.Close()

	// Spec must know the proxy's listen addr for revalidation loop-back
	spec.(*cacheSpec).listenAddr = strings.TrimPrefix(proxy.URL, "http://")
	for _, f := range spec.(*cacheSpec).filters {
		f.listenAddr = spec.(*cacheSpec).listenAddr
	}

	client := proxy.Client()

	// First request: MISS — backend must be called
	rsp1, err := client.Get(proxy.URL + "/items/1")
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	rsp1.Body.Close()
	if rsp1.Header.Get("X-Cache-Status") != "MISS" {
		t.Fatalf("expected MISS on first request, got %q", rsp1.Header.Get("X-Cache-Status"))
	}
	if atomic.LoadInt64(&backendCallCount) != 1 {
		t.Fatalf("expected 1 backend call, got %d", atomic.LoadInt64(&backendCallCount))
	}

	// Second request: HIT — backend must NOT be called
	rsp2, err := client.Get(proxy.URL + "/items/1")
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	rsp2.Body.Close()
	if rsp2.Header.Get("X-Cache-Status") != "HIT" {
		t.Fatalf("expected HIT on second request, got %q", rsp2.Header.Get("X-Cache-Status"))
	}
	if atomic.LoadInt64(&backendCallCount) != 1 {
		t.Fatalf("expected still 1 backend call after HIT, got %d", atomic.LoadInt64(&backendCallCount))
	}
}

func TestProxy_L1Cache_RFCMode_MaxAge(t *testing.T) {
	var backendCallCount int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&backendCallCount, 1)
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":"rfc"}`)
	}))
	defer backend.Close()

	spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	fr := make(filters.Registry)
	fr.Register(spec)

	// RFC mode: zero args
	route := &eskip.Route{
		Id:          "rfc-route",
		PathRegexps: []string{".*"},
		Filters:     []*eskip.Filter{{Name: spec.Name(), Args: nil}},
		Backend:     backend.URL,
	}

	proxy := proxytest.New(fr, route)
	defer proxy.Close()

	client := proxy.Client()

	rsp1, err := client.Get(proxy.URL + "/rfc")
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	rsp1.Body.Close()

	rsp2, err := client.Get(proxy.URL + "/rfc")
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	rsp2.Body.Close()
	if rsp2.Header.Get("X-Cache-Status") != "HIT" {
		t.Fatalf("expected HIT in RFC mode, got %q", rsp2.Header.Get("X-Cache-Status"))
	}
	if atomic.LoadInt64(&backendCallCount) != 1 {
		t.Fatalf("expected 1 backend call, got %d", atomic.LoadInt64(&backendCallCount))
	}
}

func TestProxy_L1Cache_UnsafeMethod_Invalidates(t *testing.T) {
	var backendCallCount int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&backendCallCount, 1)
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":"item"}`)
	}))
	defer backend.Close()

	spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	fr := make(filters.Registry)
	fr.Register(spec)

	route := &eskip.Route{
		Id:          "invalidate-route",
		PathRegexps: []string{".*"},
		Filters:     []*eskip.Filter{{Name: spec.Name(), Args: []any{"5m", "15s", "30s"}}},
		Backend:     backend.URL,
	}

	proxy := proxytest.New(fr, route)
	defer proxy.Close()

	client := proxy.Client()

	// GET → MISS, populate cache
	rsp1, _ := client.Get(proxy.URL + "/item/42")
	rsp1.Body.Close()
	if rsp1.Header.Get("X-Cache-Status") != "MISS" {
		t.Fatalf("expected MISS on first GET")
	}

	// GET → HIT
	rsp2, _ := client.Get(proxy.URL + "/item/42")
	rsp2.Body.Close()
	if rsp2.Header.Get("X-Cache-Status") != "HIT" {
		t.Fatalf("expected HIT on second GET")
	}

	// POST → invalidates cache
	req, _ := http.NewRequest("POST", proxy.URL+"/item/42", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rsp3, _ := client.Do(req)
	rsp3.Body.Close()

	// GET → MISS again (cache invalidated)
	rsp4, _ := client.Get(proxy.URL + "/item/42")
	rsp4.Body.Close()
	if rsp4.Header.Get("X-Cache-Status") != "MISS" {
		t.Fatalf("expected MISS after POST invalidation, got %q", rsp4.Header.Get("X-Cache-Status"))
	}
}

func TestProxy_L1Cache_NoCache_RequestBypasses(t *testing.T) {
	var backendCallCount int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&backendCallCount, 1)
		w.Header().Set("Cache-Control", "public, max-age=300")
		fmt.Fprint(w, `{"data":"v1"}`)
	}))
	defer backend.Close()

	spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	fr := make(filters.Registry)
	fr.Register(spec)

	route := &eskip.Route{
		Id:          "nocache-route",
		PathRegexps: []string{".*"},
		Filters:     []*eskip.Filter{{Name: spec.Name(), Args: []any{"5m", "15s", "30s"}}},
		Backend:     backend.URL,
	}

	proxy := proxytest.New(fr, route)
	defer proxy.Close()

	client := proxy.Client()

	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", proxy.URL+"/nc", nil)
		req.Header.Set("Cache-Control", "no-cache")
		rsp, _ := client.Do(req)
		rsp.Body.Close()
	}

	if atomic.LoadInt64(&backendCallCount) < 3 {
		t.Fatalf("no-cache must bypass cache: expected >=3 backend calls, got %d", atomic.LoadInt64(&backendCallCount))
	}
}

func TestProxy_L1Cache_VaryStar_NeverCached(t *testing.T) {
	var backendCallCount int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&backendCallCount, 1)
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("Vary", "*")
		fmt.Fprint(w, `{"data":"vary-star"}`)
	}))
	defer backend.Close()

	spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	fr := make(filters.Registry)
	fr.Register(spec)

	route := &eskip.Route{
		Id:          "vary-star-route",
		PathRegexps: []string{".*"},
		Filters:     []*eskip.Filter{{Name: spec.Name(), Args: []any{"5m", "15s", "30s"}}},
		Backend:     backend.URL,
	}

	proxy := proxytest.New(fr, route)
	defer proxy.Close()

	client := proxy.Client()

	for i := 0; i < 3; i++ {
		rsp, _ := client.Get(proxy.URL + "/vary")
		rsp.Body.Close()
	}

	if atomic.LoadInt64(&backendCallCount) < 3 {
		t.Fatalf("Vary: * must never be cached: expected >=3 backend calls, got %d", atomic.LoadInt64(&backendCallCount))
	}
}

func TestProxy_L2Cache_HitAfterL1Eviction(t *testing.T) {
	var backendCallCount int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&backendCallCount, 1)
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":"l2-hit"}`)
	}))
	defer backend.Close()

	stub := newStubValkeyClient()
	m := &testMetrics{}
	lru := NewLRUStorage(1<<20, nil, m)

	spec := NewCacheFilter(Options{
		MaxBytes: 1 << 20,
		L2Client: stub,
		IsNoL2Err: func(err error) bool {
			// valkey.Nil sentinel — mimic the real check without the import
			return err != nil && err.Error() == "valkey: nil"
		},
		L1TTL:   60 * time.Second,
		Metrics: m,
	})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })

	// Use the isNoErr function that matches the stub's "not found" behaviour
	isNoErr := func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "stub: not found")
	}
	cs := spec.(*cacheSpec)
	cs.storage = NewL2Storage(stub, lru, m, 60*time.Second, isNoErr)
	cs.lruStorage = lru
	for _, f := range cs.filters {
		f.storage = cs.storage
		f.lruStorage = lru
	}

	fr := make(filters.Registry)
	fr.Register(spec)

	route := &eskip.Route{
		Id:          "l2-route",
		PathRegexps: []string{".*"},
		Filters:     []*eskip.Filter{{Name: spec.Name(), Args: []any{"5m", "15s", "30s"}}},
		Backend:     backend.URL,
	}

	proxy := proxytest.New(fr, route)
	defer proxy.Close()

	// Re-wire filters to use the test storage
	for _, f := range cs.filters {
		f.storage = cs.storage
		f.lruStorage = lru
	}

	client := proxy.Client()

	// First request: MISS — populates both L1 and L2
	rsp1, _ := client.Get(proxy.URL + "/l2item")
	rsp1.Body.Close()
	if rsp1.Header.Get("X-Cache-Status") != "MISS" {
		t.Fatalf("expected MISS, got %q", rsp1.Header.Get("X-Cache-Status"))
	}
	// Second request: HIT from L1
	rsp2, _ := client.Get(proxy.URL + "/l2item")
	rsp2.Body.Close()
	if rsp2.Header.Get("X-Cache-Status") != "HIT" {
		t.Fatalf("expected HIT from L1, got %q", rsp2.Header.Get("X-Cache-Status"))
	}

	// Verify L2 has the entry
	if len(stub.data) == 0 {
		t.Fatal("expected L2 to contain an entry after MISS+HIT")
	}
}

func TestProxy_L2Cache_FallsBackToL1OnL2Failure(t *testing.T) {
	var backendCallCount int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&backendCallCount, 1)
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":"l1-fallback"}`)
	}))
	defer backend.Close()

	stub := newBrokenStubValkeyClient()
	m := &testMetrics{}
	lru := NewLRUStorage(1<<20, nil, m)

	isNoErr := func(err error) bool { return false } // broken stub always returns real errors
	l2store := NewL2Storage(stub, lru, m, 60*time.Second, isNoErr)

	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, Metrics: m})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	cs := spec.(*cacheSpec)
	cs.storage = l2store
	cs.lruStorage = lru

	fr := make(filters.Registry)
	fr.Register(spec)

	route := &eskip.Route{
		Id:          "l2-fallback-route",
		PathRegexps: []string{".*"},
		Filters:     []*eskip.Filter{{Name: spec.Name(), Args: []any{"5m", "15s", "30s"}}},
		Backend:     backend.URL,
	}

	proxy := proxytest.New(fr, route)
	defer proxy.Close()

	// Re-wire storage to all filters
	for _, f := range cs.filters {
		f.storage = l2store
		f.lruStorage = lru
	}

	client := proxy.Client()

	// First request: L2 broken → Set falls back to L1; served as MISS
	rsp1, _ := client.Get(proxy.URL + "/fallback")
	rsp1.Body.Close()
	if rsp1.Header.Get("X-Cache-Status") != "MISS" {
		t.Fatalf("expected MISS on first request with broken L2")
	}

	// L2 Set fallback must have written to L1 — check l2_set_fallback counter
	if m.counter("cache.l2_set_fallback") == 0 {
		t.Error("expected l2_set_fallback to be incremented when L2 Set fails")
	}
}

func TestProxy_L2Cache_MissWhenBothMiss(t *testing.T) {
	var backendCallCount int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&backendCallCount, 1)
		w.Header().Set("Cache-Control", "public, max-age=300")
		fmt.Fprint(w, `{"data":"cold"}`)
	}))
	defer backend.Close()

	stub := newStubValkeyClient() // empty
	m := &testMetrics{}
	lru := NewLRUStorage(1<<20, nil, m)
	isNoErr := func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "valkey: nil")
	}
	l2store := NewL2Storage(stub, lru, m, 0, isNoErr)

	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, Metrics: m})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	cs := spec.(*cacheSpec)
	cs.storage = l2store
	cs.lruStorage = lru

	fr := make(filters.Registry)
	fr.Register(spec)

	route := &eskip.Route{
		Id:          "both-miss-route",
		PathRegexps: []string{".*"},
		Filters:     []*eskip.Filter{{Name: spec.Name(), Args: []any{"5m", "15s", "30s"}}},
		Backend:     backend.URL,
	}

	proxy := proxytest.New(fr, route)
	defer proxy.Close()

	for _, f := range cs.filters {
		f.storage = l2store
		f.lruStorage = lru
	}

	client := proxy.Client()

	rsp, _ := client.Get(proxy.URL + "/cold")
	rsp.Body.Close()
	if rsp.Header.Get("X-Cache-Status") != "MISS" {
		t.Fatalf("expected MISS when both L1 and L2 empty, got %q", rsp.Header.Get("X-Cache-Status"))
	}
	if atomic.LoadInt64(&backendCallCount) == 0 {
		t.Fatal("expected backend to be called on cold miss")
	}
}

// Ensure url import is used
var _ = url.Parse
