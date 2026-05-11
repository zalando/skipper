package cache

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
)

func newTestFilter(t *testing.T, ttl, errorTTL, swrWindow time.Duration, extra ...time.Duration) *cacheFilter {
	t.Helper()
	spec := NewCacheFilter(1<<20, "localhost:9090")
	args := []interface{}{
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
	return cf
}

// newTestFilterRFC creates a filter in pure RFC mode (zero args). Upstream
// Cache-Control is fully authoritative. Use this for tests that exercise
// Expires capping, heuristic freshness, or other RFC 9111 TTL logic.
// The ttl/errorTTL/swrWindow args are accepted for call-site compatibility
// but are ignored — pure RFC mode has no operator TTL.
func newTestFilterRFC(t *testing.T, _, _, _ time.Duration, _ ...time.Duration) *cacheFilter {
	t.Helper()
	spec := NewCacheFilter(1<<20, "localhost:9090")
	f, err := spec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	cf := f.(*cacheFilter)
	cf.fetch = func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no fetch stub set")
	}
	return cf
}

func newCtx(method, rawURL string, authHeader string) *filtertest.Context {
	req, _ := http.NewRequest(method, rawURL, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return &filtertest.Context{
		FRequest:  req,
		FStateBag: make(map[string]interface{}),
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
	spec := NewCacheFilter(1<<20, "localhost:9090")
	fi, err := spec.CreateFilter([]interface{}{"1m", "15s", "1m", "0s", "Authorization"})
	if err != nil {
		t.Fatal(err)
	}
	f := fi.(*cacheFilter)
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

func TestCreateFilter_InvalidArgs(t *testing.T) {
	spec := NewCacheFilter(1<<20, "localhost:9090")
	cases := []struct {
		name string
		args []interface{}
	}{
		{"too few args", []interface{}{"5m", "15s"}},
		{"bad ttl", []interface{}{"bad", "15s", "30s"}},
		{"zero ttl", []interface{}{"0s", "15s", "30s"}},
		{"non-string ttl", []interface{}{300, "15s", "30s"}},
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

func TestCacheFilter_ColdMissCoalescing(t *testing.T) {
	// Hold the upstream fetch until all N goroutines have checked in, proving
	// that exactly 1 fetch fires for the whole cohort.
	const N = 50
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)

	var fetchStarted sync.Once
	fetchStartedCh := make(chan struct{})
	releaseAll := make(chan struct{})
	var fetchCount int64

	// The leader waits for all N goroutines to signal (wgIn.Done) before the
	// fetch returns. Each goroutine signals just before calling f.Request so
	// it is either already in DoChan's wait list or about to enter it while
	// the leader is still blocked on <-releaseAll.
	var wgIn sync.WaitGroup
	wgIn.Add(N)

	f.fetch = func(req *http.Request) (*http.Response, error) {
		atomic.AddInt64(&fetchCount, 1)
		fetchStarted.Do(func() { close(fetchStartedCh) })
		wgIn.Wait()
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
			wgIn.Done()
			f.Request(ctx)
			results[i] = ctx
		}()
	}

	<-fetchStartedCh
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
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	f.fetch = func(*http.Request) (*http.Response, error) {
		return nil, errors.New("upstream unavailable")
	}

	mockMetrics := &metricstest.MockMetrics{}
	ctx := newCtx("GET", "https://cdn.contentful.com/spaces/abc/entries/coalesce-err", "")
	ctx.FMetrics = mockMetrics
	f.Request(ctx)

	mockMetrics.WithCounters(func(counters map[string]int64) {
		if counters["coalesce_error"] != 1 {
			t.Errorf("expected coalesce_error==1, got %d", counters["coalesce_error"])
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
	f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/metrics"

	synctest.Test(t, func(t *testing.T) {

		// MISS: populate via Response() path
		miss := newCtx("GET", url, "")
		f.Request(miss)
		miss.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"v1"}`, "max-age=300")
		f.Response(miss)

		miss.FMetrics.(*metricstest.MockMetrics).WithCounters(func(counters map[string]int64) {
			if counters["miss"] != 1 {
				t.Errorf("after MISS: expected miss==1, got %d", counters["miss"])
			}
			if counters["hit"] != 0 {
				t.Errorf("after MISS: expected hit==0, got %d", counters["hit"])
			}
		})

		// HIT: within TTL
		hit := newCtx("GET", url, "")
		f.Request(hit)
		if !hit.FServed {
			t.Fatal("expected HIT within TTL")
		}
		hit.FMetrics.(*metricstest.MockMetrics).WithCounters(func(counters map[string]int64) {
			if counters["hit"] != 1 {
				t.Errorf("after HIT: expected hit==1, got %d", counters["hit"])
			}
			if counters["stale"] != 0 {
				t.Errorf("after HIT: expected stale==0, got %d", counters["stale"])
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
		stale.FMetrics.(*metricstest.MockMetrics).WithCounters(func(counters map[string]int64) {
			if counters["stale"] != 1 {
				t.Errorf("after STALE: expected stale==1, got %d", counters["stale"])
			}
			if counters["hit"] != 0 {
				t.Errorf("after STALE: expected hit==0, got %d", counters["hit"])
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
	f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/etag"

	var revalReq *http.Request

	synctest.Test(t, func(t *testing.T) {
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
	f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
	url := "https://cdn.contentful.com/spaces/abc/entries/lastmod"

	var revalReq *http.Request

	synctest.Test(t, func(t *testing.T) {
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
	f := newTestFilter(t, time.Millisecond, 15*time.Second, time.Hour)
	mockMetrics := &metricstest.MockMetrics{}
	f.metrics = mockMetrics
	url := "https://cdn.contentful.com/spaces/abc/entries/reval-err"

	synctest.Test(t, func(t *testing.T) {
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
			if counters["reval_error"] != 1 {
				t.Errorf("expected reval_error==1, got %d", counters["reval_error"])
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

func TestCacheFilter_SharedStorage_RouteIsolation(t *testing.T) {
	spec := NewCacheFilter(1<<20, "localhost:9090")

	makeFilter := func(t *testing.T) *cacheFilter {
		t.Helper()
		f, err := spec.CreateFilter([]interface{}{"5m", "15s", "30s"})
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
	// filter created OUTSIDE synctest bubble
	f := newTestFilter(t, 5*time.Minute, 10*time.Second, 10*time.Minute)

	synctest.Test(t, func(t *testing.T) {
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
	// filter created OUTSIDE synctest bubble
	f := newTestFilter(t, 5*time.Minute, 10*time.Second, 10*time.Minute)

	synctest.Test(t, func(t *testing.T) {
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
	f := newTestFilter(t, 100*time.Millisecond, time.Second, 500*time.Millisecond)
	synctest.Test(t, func(t *testing.T) {
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
	// A 503 upstream should cause the stale entry to be served.
	f := newTestFilter(t, time.Millisecond, 10*time.Second, time.Millisecond, 60*time.Second)
	url := "http://example.com/sie-5xx"

	synctest.Test(t, func(t *testing.T) {
		ctx := newCtx(http.MethodGet, url, "")
		f.Request(ctx)
		ctx.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"cached"}`, "max-age=0")
		f.Response(ctx)

		// Advance past TTL+SWR (SWR=0) so the entry is stale, but within staleIfError window.
		time.Sleep(50 * time.Millisecond)

		ctx2 := newCtx(http.MethodGet, url, "")
		f.Request(ctx2)
		// ctx2 is a MISS (past TTL+SWR); upstream returns 503.
		ctx2.FResponse = upstreamResponse(http.StatusServiceUnavailable, "")
		f.Response(ctx2)

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
	f := newTestFilter(t, time.Millisecond, 10*time.Second, time.Millisecond, 100*time.Millisecond)
	url := "http://example.com/sie-expired"

	synctest.Test(t, func(t *testing.T) {
		ctx := newCtx(http.MethodGet, url, "")
		f.Request(ctx)
		ctx.FResponse = upstreamResponseCC(http.StatusOK, `{"data":"cached"}`, "max-age=0")
		f.Response(ctx)

		// Advance past TTL (1ms) + staleIfError (100ms) = well beyond 200ms.
		time.Sleep(200 * time.Millisecond)

		ctx2 := newCtx(http.MethodGet, url, "")
		f.Request(ctx2)
		ctx2.FResponse = upstreamResponse(http.StatusServiceUnavailable, "")
		f.Response(ctx2)

		if ctx2.FResponse.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("want 503 (SIE window expired), got %d", ctx2.FResponse.StatusCode)
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
			t.Fatalf("want 503 (SIE disabled), got %d", ctx2.FResponse.StatusCode)
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
	spec := NewCacheFilter(1<<20, ":9090")

	cases := []struct {
		name    string
		args    []interface{}
		wantRFC bool
		wantSIE time.Duration
		wantErr bool
	}{
		{"0 args pure rfc mode", []interface{}{}, true, 0, false},
		{"3 args force mode", []interface{}{"5m", "15s", "30s"}, false, 0, false},
		{"4 args staleIfError", []interface{}{"5m", "15s", "30s", "60s"}, false, 60 * time.Second, false},
		{"1 arg invalid", []interface{}{"5m"}, false, 0, true},
		{"2 args invalid", []interface{}{"5m", "15s"}, false, 0, true},
		{"6 args too many", []interface{}{"5m", "15s", "30s", "60s", "Authorization", "extra"}, false, 0, true},
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
	spec := NewCacheFilter(1<<20, ":9090")
	f, err := spec.CreateFilter([]interface{}{})
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

func TestCacheFilter_PureRFCMode_ZeroArgs_NoUpstreamDirective_NotCached(t *testing.T) {
	// cache() with no args: when upstream sends no Cache-Control, no Expires,
	// and no Last-Modified, nothing should be cached (no heuristic without Last-Modified).
	spec := NewCacheFilter(1<<20, ":9090")
	f, err := spec.CreateFilter([]interface{}{})
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
