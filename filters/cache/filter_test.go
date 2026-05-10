package cache

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
)

func newTestFilter(t *testing.T, ttl, errorTTL, swrWindow time.Duration) *cacheFilter {
	t.Helper()
	spec := NewCacheFilter(1<<20, "localhost:9090")
	f, err := spec.CreateFilter([]interface{}{
		ttl.String(),
		errorTTL.String(),
		swrWindow.String(),
	})
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
	fi, err := spec.CreateFilter([]interface{}{"1m", "15s", "1m", "Authorization"})
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
	ctx1.FResponse = upstreamResponse(http.StatusNotFound, `{"message":"not found"}`)
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
		ctx1.FResponse = upstreamResponse(http.StatusOK, `{"data":1}`)
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
		ctx1.FResponse = upstreamResponse(http.StatusOK, `{"data":"fresh"}`)
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
			Header:     http.Header{"Content-Type": {"application/json"}},
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

func TestCacheFilter_ColdMissCoalescing_UpstreamError_FetchErrorMetric(t *testing.T) {
	// fetch_error counter must be incremented when the upstream fetch fails during coalescing.
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	mockMetrics := &metricstest.MockMetrics{}

	f.fetch = func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("upstream unavailable")
	}

	ctx := newCtx("GET", "https://cdn.contentful.com/spaces/abc/entries/fetch-err-metric", "")
	ctx.FMetrics = mockMetrics
	f.Request(ctx)

	mockMetrics.WithCounters(func(counters map[string]int64) {
		if counters["fetch_error"] != 1 {
			t.Errorf("expected fetch_error==1, got %d", counters["fetch_error"])
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
	ctx1.FResponse = upstreamResponse(http.StatusOK, `{"data":"cached"}`)
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
		ctx1.FResponse = upstreamResponse(http.StatusOK, `{"data":"v1"}`)
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
		ctx1.FResponse = upstreamResponse(http.StatusOK, `{"data":"old"}`)
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
		rsp := upstreamResponse(http.StatusOK, `{"data":"v1"}`)
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
		miss.FResponse = upstreamResponse(http.StatusOK, `{"data":"v1"}`)
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
	rspEN := upstreamResponse(http.StatusOK, `{"lang":"en-US"}`)
	rspEN.Header.Set("Vary", "Accept-Language")
	ctxEN.FResponse = rspEN
	f.Response(ctxEN)

	ctxDE := newCtx("GET", url, "")
	ctxDE.FRequest.Header.Set("Accept-Language", "de-DE")
	f.Request(ctxDE)
	rspDE := upstreamResponse(http.StatusOK, `{"lang":"de-DE"}`)
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
		rsp := upstreamResponse(http.StatusOK, `{"data":"v1"}`)
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
		rsp := upstreamResponse(http.StatusOK, `{"data":"v1"}`)
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

		mockMetrics := &metricstest.MockMetrics{}
		ctx2 := newCtx("GET", url, "")
		ctx2.FMetrics = mockMetrics
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
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Millisecond)
	url := "https://cdn.contentful.com/spaces/abc/entries/expires"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		rsp := upstreamResponseCC(http.StatusOK, `{"data":"expires-soon"}`, "public, max-age=300")
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

func TestCacheFilter_PartialContent_NotStored(t *testing.T) {
	// 206 Partial Content must never be stored — storing it as a full 200 is data corruption.
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/partial"

	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	ctx1.FResponse = &http.Response{
		StatusCode: http.StatusPartialContent,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`partial`)),
	}
	f.Response(ctx1)

	ctx2 := newCtx("GET", url, "")
	f.Request(ctx2)
	if ctx2.FServed {
		t.Fatal("206 Partial Content must not be stored in cache")
	}
}

func TestCacheFilter_NonGetMethod_NotStored(t *testing.T) {
	// HEAD and OPTIONS responses must not be stored even if the status is 200.
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	url := "https://cdn.contentful.com/spaces/abc/entries/non-get"

	for _, method := range []string{http.MethodHead, http.MethodOptions} {
		ctx1 := newCtx(method, url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponse(http.StatusOK, `{"data":"v1"}`)
		f.Response(ctx1)

		ctx2 := newCtx(http.MethodGet, url, "")
		f.Request(ctx2)
		if ctx2.FServed {
			t.Fatalf("%s response must not be stored in cache", method)
		}
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

// evictAfterFirstGetStorage wraps a Storage and deletes the entry after the
// first successful Get, simulating eviction between two consecutive Get calls.
type evictAfterFirstGetStorage struct {
	Storage
	mu      sync.Mutex
	evicted map[string]bool
}

func (s *evictAfterFirstGetStorage) Get(ctx context.Context, key string) (*Entry, error) {
	entry, err := s.Storage.Get(ctx, key)
	if entry != nil && err == nil {
		s.mu.Lock()
		already := s.evicted[key]
		if !already {
			s.evicted[key] = true
		}
		s.mu.Unlock()
		if !already {
			// Evict after returning — next Get will miss.
			_ = s.Storage.Delete(ctx, key)
		}
	}
	return entry, err
}

// TestCacheFilter_OnlyIfCached_NoSecondGet proves that a valid only-if-cached hit
// is served from the entry found on the first Get, without a second Get that
// could race with eviction.
func TestCacheFilter_OnlyIfCached_NoSecondGet(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, 30*time.Second)
	url := "https://cdn.contentful.com/spaces/abc/entries/oic-evict"

	// Populate the cache normally.
	ctx1 := newCtx("GET", url, "")
	f.Request(ctx1)
	ctx1.FResponse = upstreamResponse(http.StatusOK, `{"id":"oic"}`)
	f.Response(ctx1)

	// Swap in a storage that evicts the entry after the first Get.
	f.storage = &evictAfterFirstGetStorage{
		Storage: f.storage,
		evicted: make(map[string]bool),
	}

	// The only-if-cached path must reuse the entry from the first Get rather than
	// issuing a second Get that could race with eviction.
	ctx2 := newCtx("GET", url, "")
	ctx2.FRequest.Header.Set("Cache-Control", "only-if-cached")
	f.Request(ctx2)
	if !ctx2.FServed {
		t.Fatal("only-if-cached hit must be served using the entry from the first Get, not a second Get that can race with eviction")
	}
}

// TestCacheSpec_SharedStorage proves that two filter instances created from the
// same spec share one LRU. A response stored via f1 must be a HIT when f2
// serves the same URL.
// TestCacheFilter_SMaxAge_CapsRouteTTL verifies that s-maxage in the upstream
// response overrides the operator-configured TTL for shared caches.
func TestCacheFilter_SMaxAge_CapsRouteTTL(t *testing.T) {
	// Route TTL is 10 minutes; upstream says s-maxage=2s.
	// Filter created outside bubble so sknet transport goroutines don't get trapped.
	f := newTestFilter(t, 10*time.Minute, 15*time.Second, time.Millisecond)
	url := "https://cdn.contentful.com/spaces/abc/smaxage"

	synctest.Test(t, func(t *testing.T) {
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"ok":true}`, "s-maxage=2")
		f.Response(ctx1)

		// Immediately after storage: should be a HIT.
		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)
		if !ctx2.FServed {
			t.Fatal("expected HIT immediately after storage")
		}

		// Advance past s-maxage=2s; entry must have expired.
		time.Sleep(3 * time.Second)

		ctx3 := newCtx("GET", url, "")
		f.Request(ctx3)
		if ctx3.FServed {
			t.Fatal("expected MISS after s-maxage expired — route TTL must not override s-maxage")
		}
	})
}

// TestCacheFilter_SMaxAge_ImpliesProxyRevalidate verifies that s-maxage implies
// proxy-revalidate: once stale the cache must not serve stale and must revalidate.
func TestCacheFilter_SMaxAge_ImpliesProxyRevalidate(t *testing.T) {
	// Route TTL=1ms (minimal), SWR=10s; upstream has s-maxage=2s.
	// After 3s the entry is stale (TTL=1ms gone) but inside SWR=10s.
	// Without s-maxage support: serveEntry would return STALE.
	// With s-maxage=2s + implied proxy-revalidate: STALE must be blocked.
	f := newTestFilter(t, time.Millisecond, 15*time.Second, 10*time.Second)
	f.fetch = func(req *http.Request) (*http.Response, error) {
		return upstreamResponseCC(http.StatusOK, `{"v":2}`, "s-maxage=2"), nil
	}
	url := "https://cdn.contentful.com/spaces/abc/smaxage-reval"

	synctest.Test(t, func(t *testing.T) {
		// Prime the cache.
		ctx1 := newCtx("GET", url, "")
		f.Request(ctx1)
		ctx1.FResponse = upstreamResponseCC(http.StatusOK, `{"v":1}`, "s-maxage=2")
		f.Response(ctx1)

		// Advance past route TTL (1ms) but inside SWR (10s) — stale territory.
		time.Sleep(3 * time.Second)

		// s-maxage implies proxy-revalidate — must NOT serve STALE.
		ctx2 := newCtx("GET", url, "")
		f.Request(ctx2)

		if ctx2.FServed && ctx2.FResponse.Header.Get("X-Cache-Status") == "STALE" {
			t.Fatal("s-maxage implies proxy-revalidate — must not serve stale")
		}
	})
}

func TestCacheSpec_SharedStorage(t *testing.T) {
	spec := NewCacheFilter(1<<20, "localhost:9090")
	url := "https://cdn.contentful.com/spaces/abc/entries/shared"

	mustFilter := func() *cacheFilter {
		f, err := spec.CreateFilter([]interface{}{"1m", "15s", "30s"})
		if err != nil {
			t.Fatal(err)
		}
		cf := f.(*cacheFilter)
		cf.fetch = func(*http.Request) (*http.Response, error) {
			return nil, errors.New("no fetch stub")
		}
		return cf
	}

	f1 := mustFilter()
	f2 := mustFilter()

	// Populate cache through f1.
	ctx1 := newCtx("GET", url, "")
	f1.Request(ctx1)
	ctx1.FResponse = upstreamResponse(http.StatusOK, `{"id":"shared"}`)
	f1.Response(ctx1)

	// f2 must find the entry stored by f1.
	ctx2 := newCtx("GET", url, "")
	f2.Request(ctx2)
	if !ctx2.FServed {
		t.Fatal("f2 did not get a HIT for an entry stored by f1 — storage is not shared")
	}
}
