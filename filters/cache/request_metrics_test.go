package cache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valkey-io/valkey-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy/proxytest"
)

func requestCounters(m *metricstest.MockMetrics) map[string]int64 {
	got := make(map[string]int64)
	m.WithCounters(func(counters map[string]int64) {
		for key, count := range counters {
			if strings.HasPrefix(key, "cache_request.") {
				got[strings.TrimPrefix(key, "cache_request.")] = count
			}
		}
	})
	return got
}

func requestMetricsFilter(t *testing.T, tier string) (*cacheFilter, *metricstest.MockMetrics) {
	t.Helper()
	m := &metricstest.MockMetrics{}
	lru := NewLRUStorage(1<<20, nil, m)
	var storage Storage = lru
	if tier != "lru" {
		l1TTL := time.Duration(0)
		if tier == "warm" {
			l1TTL = time.Minute
		}
		storage = NewL2Storage(newStubValkeyClient(), lru, m, l1TTL, valkey.IsValkeyNil)
	}
	return &cacheFilter{
		storage: storage, lruStorage: lru, metrics: m, listenAddr: "localhost:9090",
		ttl: time.Minute, errorTTL: time.Second, swrWindow: time.Minute,
		staleIfError: 5 * time.Minute, ctx: context.Background(), revalJobs: make(chan revalJob, 10),
		fetch: func(*http.Request) (*http.Response, error) {
			return upstreamResponseCC(http.StatusOK, "origin", "public, max-age=60"), nil
		},
	}, m
}

func TestCacheRequestMetrics(t *testing.T) {
	for _, tier := range []string{"lru", "l2", "warm"} {
		for _, scenario := range []struct {
			name, method, directive, want string
			age                           time.Duration
			seed, conditional, fail       bool
		}{
			{name: "fresh", seed: true, want: "fresh"},
			{name: "head", method: http.MethodHead, seed: true, want: "fresh"},
			{name: "conditional", seed: true, conditional: true, want: "fresh"},
			{name: "cold", want: "origin"},
			{name: "stale", seed: true, age: 90 * time.Second, want: "stale"},
			{name: "stale conditional", seed: true, conditional: true, age: 90 * time.Second, want: "stale"},
			{name: "stale if error", seed: true, age: 3 * time.Minute, fail: true, want: "stale"},
			{name: "expired", seed: true, age: 10 * time.Minute, want: "origin"},
			{name: "min fresh", seed: true, directive: "min-fresh=120", want: "origin"},
			{name: "max stale", seed: true, age: 90 * time.Second, directive: "max-stale=10", want: "origin"},
			{name: "no cache", seed: true, directive: "no-cache", want: "bypass"},
			{name: "no store", directive: "no-store", want: "bypass"},
			{name: "unsafe", method: http.MethodPost, want: "bypass"},
			{name: "only if cached hit", seed: true, directive: "only-if-cached", want: "fresh"},
			{name: "only if cached miss", directive: "only-if-cached", want: "cache_only_miss"},
			{name: "only if cached expired", seed: true, age: 3 * time.Minute, directive: "only-if-cached", want: "cache_only_miss"},
			{name: "fetch failure", fail: true, want: "error"},
		} {
			t.Run(tier+"/"+scenario.name, func(t *testing.T) {
				f, m := requestMetricsFilter(t, tier)
				method := scenario.method
				if method == "" {
					method = http.MethodGet
				}
				ctx := newCtx(method, "http://cache.example/item", "")
				ctx.Request().Header.Set("Cache-Control", scenario.directive)
				key := cacheKey(ctx.RouteId(), ctx.Request(), nil)
				if scenario.seed {
					entry := &Entry{
						StatusCode: http.StatusOK, Header: http.Header{}, Payload: []byte("cached"),
						ETag: `"tag"`, CreatedAt: time.Now().Add(-scenario.age),
						TTL: time.Minute, StaleWhileRevalidate: time.Minute, StaleIfError: 5 * time.Minute,
					}
					require.NoError(t, f.storage.Set(context.Background(), key, entry))
					if tier == "warm" && scenario.age > 0 {
						require.NoError(t, f.lruStorage.Set(context.Background(), key, entry))
					}
				}
				if scenario.conditional {
					ctx.Request().Header.Set("If-None-Match", `"tag"`)
				}
				if scenario.fail {
					f.fetch = func(*http.Request) (*http.Response, error) { return nil, assert.AnError }
				}
				f.Request(ctx)
				if ctx.FResponse == nil {
					ctx.FResponse = upstreamResponse(http.StatusOK, "pipeline")
				}
				defer ctx.FResponse.Body.Close()
				f.Response(ctx)
				wantTier := "none"
				if scenario.want == "fresh" || scenario.want == "stale" {
					wantTier = "l1"
					if tier == "l2" || tier == "warm" && scenario.age > time.Minute && scenario.age < 2*time.Minute {
						wantTier = "l2"
					}
				}
				require.Equal(t, map[string]int64{scenario.want + "." + wantTier: 1}, requestCounters(m))
				if scenario.conditional {
					assert.Equal(t, http.StatusNotModified, ctx.Response().StatusCode)
				}
				if scenario.want == "fresh" || scenario.want == "stale" {
					if method == http.MethodHead || scenario.conditional {
						body, err := io.ReadAll(ctx.Response().Body)
						require.NoError(t, err)
						assert.Empty(t, body)
					}
				}
			})
		}
	}
}

func TestCacheRequestMetricsVaryAndWarming(t *testing.T) {
	f, m := requestMetricsFilter(t, "warm")
	ctx := newCtx(http.MethodGet, "http://cache.example/item", "")
	ctx.Request().Header.Set("Accept-Language", "en")
	base := cacheKey(ctx.RouteId(), ctx.Request(), nil)
	require.NoError(t, f.storeEntry(context.Background(), base, base, ctx.Request(), &Entry{
		StatusCode: http.StatusOK, Header: http.Header{"Vary": {"Accept-Language"}},
		Payload: []byte("en"), CreatedAt: time.Now(), TTL: time.Minute,
	}))
	require.NoError(t, f.lruStorage.Delete(context.Background(), "vary:"+base))
	require.NoError(t, f.lruStorage.Delete(context.Background(), varyKey(base, ctx.Request(), []string{"Accept-Language"})))
	f.Request(ctx)
	require.True(t, ctx.Served())
	ctx.Response().Body.Close()
	assert.Equal(t, map[string]int64{"fresh.l2": 1}, requestCounters(m))
	next := newCtx(http.MethodGet, "http://cache.example/item", "")
	next.Request().Header.Set("Accept-Language", "en")
	f.Request(next)
	require.True(t, next.Served())
	next.Response().Body.Close()
	assert.Equal(t, map[string]int64{"fresh.l2": 1, "fresh.l1": 1}, requestCounters(m))
}

func TestCacheRequestMetricsCoalescing(t *testing.T) {
	for _, failure := range []string{"", "transport", "body", "unshareable", "vary", "stale transport", "stale 5xx"} {
		t.Run(failure, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f, m := requestMetricsFilter(t, "lru")
				release := make(chan struct{})
				variantRelease := make(chan struct{})
				var fetches atomic.Int64
				f.fetch = func(r *http.Request) (*http.Response, error) {
					n := fetches.Add(1)
					<-release
					if failure == "vary" && n > 1 {
						<-variantRelease
					}
					switch failure {
					case "transport", "stale transport":
						return nil, assert.AnError
					case "stale 5xx":
						return upstreamResponse(http.StatusServiceUnavailable, "unavailable"), nil
					case "body":
						rsp := upstreamResponse(http.StatusOK, "")
						rsp.Body = &errReader{}
						return rsp, nil
					case "unshareable":
						return upstreamResponse(http.StatusNotFound, "unshareable"), nil
					case "vary":
						rsp := upstreamResponse(http.StatusOK, r.Header.Get("Accept-Language"))
						rsp.Header.Set("Vary", "Accept-Language")
						return rsp, nil
					default:
						return upstreamResponse(http.StatusOK, "shared"), nil
					}
				}
				if failure == "unshareable" {
					f.errorTTL = 0
				}
				if strings.HasPrefix(failure, "stale ") {
					ctx := newCtx(http.MethodGet, "http://cache.example/item", "")
					key := cacheKey(ctx.RouteId(), ctx.Request(), nil)
					require.NoError(t, f.storage.Set(context.Background(), key, &Entry{
						StatusCode: http.StatusOK, Header: http.Header{}, Payload: []byte("stale"),
						CreatedAt: time.Now().Add(-3 * time.Minute), TTL: time.Minute,
						StaleWhileRevalidate: time.Minute, StaleIfError: 5 * time.Minute,
					}))
				}
				const requests = 8
				contexts := make([]*filtertest.Context, requests)
				for i := range contexts {
					ctx := newCtx(http.MethodGet, "http://cache.example/item", "")
					ctx.Request().Header.Set("Accept-Language", fmt.Sprint(i%2))
					contexts[i] = ctx
					go f.Request(ctx)
				}
				synctest.Wait()
				assert.Equal(t, int64(1), fetches.Load())
				close(release)
				synctest.Wait()
				close(variantRelease)
				synctest.Wait()
				for _, ctx := range contexts {
					if ctx.FResponse != nil {
						ctx.Response().Body.Close()
					}
				}
				want := map[string]int64{"origin.none": 1, "coalesced.none": requests - 1}
				switch failure {
				case "transport", "body":
					want = map[string]int64{"error.none": requests}
				case "stale transport", "stale 5xx":
					want = map[string]int64{"stale.l1": requests}
				case "unshareable":
					want = map[string]int64{"origin.none": requests}
					assert.Equal(t, int64(requests), fetches.Load())
				case "vary":
					want = map[string]int64{"origin.none": 2, "coalesced.none": requests - 2}
					assert.Equal(t, int64(2), fetches.Load())
				}
				assert.Equal(t, want, requestCounters(m))
			})
		})
	}
}

func TestCacheRequestMetricsStorageErrors(t *testing.T) {
	for _, failure := range []string{"l1 decode", "l2 decode", "l2 get", "l2 set", "only if cached"} {
		t.Run(failure, func(t *testing.T) {
			tier := "l2"
			if failure == "l1 decode" {
				tier = "lru"
			}
			f, m := requestMetricsFilter(t, tier)
			ctx := newCtx(http.MethodGet, "http://cache.example/item", "")
			key := cacheKey(ctx.RouteId(), ctx.Request(), nil)
			switch failure {
			case "l1 decode":
				f.lruStorage.lru.Set(key, []byte("{"))
			case "l2 decode":
				client := f.storage.(*L2Storage).l2client.(*stubValkeyClient)
				client.data[key] = "{"
			case "l2 get", "only if cached":
				f.storage.(*L2Storage).l2client = newBrokenStubValkeyClient()
			case "l2 set":
				f.fetch = func(*http.Request) (*http.Response, error) {
					f.storage.(*L2Storage).l2client = newBrokenStubValkeyClient()
					return upstreamResponse(http.StatusOK, "origin"), nil
				}
			}
			if failure == "only if cached" {
				ctx.Request().Header.Set("Cache-Control", "only-if-cached")
			}
			f.Request(ctx)
			require.True(t, ctx.Served())
			ctx.Response().Body.Close()
			want := "error.none"
			if failure == "l2 set" {
				// Persistence failure does not change the source of a served response.
				want = "origin.none"
			}
			assert.Equal(t, map[string]int64{want: 1}, requestCounters(m))
			next := newCtx(http.MethodGet, "http://cache.example/item", "")
			f.Request(next)
			require.True(t, next.Served())
			next.Response().Body.Close()
			wantTier := "l2"
			if tier == "lru" || failure == "l2 get" || failure == "l2 set" {
				wantTier = "l1"
			}
			if failure == "only if cached" {
				assert.Equal(t, map[string]int64{"error.none": 2}, requestCounters(m))
			} else {
				assert.Equal(t, map[string]int64{want: 1, "fresh." + wantTier: 1}, requestCounters(m))
			}
		})
	}
}

func TestCacheRequestMetricsInternalTraffic(t *testing.T) {
	for _, background := range []bool{false, true} {
		t.Run(fmt.Sprint(background), func(t *testing.T) {
			f, m := requestMetricsFilter(t, "l2")
			ctx := newCtx(http.MethodGet, "http://cache.example/item", "")
			key := cacheKey(ctx.RouteId(), ctx.Request(), nil)
			if background {
				require.NoError(t, f.storage.Set(context.Background(), key, &Entry{
					StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": {"public, max-age=60"}},
					Payload: []byte("cached"), ETag: `"tag"`, TTL: time.Minute,
					CreatedAt: time.Now().Add(-90 * time.Second), StaleWhileRevalidate: time.Minute,
				}))
			}
			fetches := 0
			f.fetch = func(r *http.Request) (*http.Response, error) {
				fetches++
				require.Equal(t, "1", r.Header.Get(revalidateHeader))
				inner := newCtx(http.MethodGet, r.URL.String(), "")
				inner.FRequest = r
				f.Request(inner)
				require.False(t, inner.Served())
				assert.Empty(t, inner.Request().Header.Get(revalidateHeader))
				if background {
					require.Equal(t, `"tag"`, inner.Request().Header.Get("If-None-Match"))
					inner.FResponse = upstreamResponse(http.StatusNotModified, "")
				} else {
					inner.FResponse = upstreamResponseCC(http.StatusOK, "origin", "public, max-age=60")
				}
				f.Response(inner)
				return inner.Response(), nil
			}
			f.Request(ctx)
			require.True(t, ctx.Served())
			ctx.Response().Body.Close()
			f.Response(ctx)
			want := map[string]int64{"origin.none": 1}
			if background {
				f.doRevalidateQueued(<-f.revalJobs)
				want = map[string]int64{"stale.l2": 1}
				entry, err := f.storage.Get(context.Background(), key)
				require.NoError(t, err)
				require.NotNil(t, entry)
				assert.Equal(t, "cached", string(entry.Payload))
				assert.False(t, entry.IsStale(time.Now()))
			}
			assert.Equal(t, 1, fetches)
			assert.Equal(t, want, requestCounters(m))
		})
	}
}

func TestCacheRequestMetricsCancellation(t *testing.T) {
	for _, duringFetch := range []bool{false, true} {
		t.Run(fmt.Sprint(duringFetch), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f, m := requestMetricsFilter(t, "lru")
				ctx := newCtx(http.MethodGet, "http://cache.example/item", "")
				reqCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ctx.FRequest = ctx.Request().WithContext(reqCtx)
				release := make(chan struct{})
				f.fetch = func(*http.Request) (*http.Response, error) {
					<-release
					return upstreamResponse(http.StatusOK, "origin"), nil
				}
				if !duringFetch {
					cancel()
				}
				go f.Request(ctx)
				synctest.Wait()
				cancel()
				synctest.Wait()
				assert.Equal(t, map[string]int64{"error.none": 1}, requestCounters(m))
				close(release)
				synctest.Wait()
				assert.Equal(t, map[string]int64{"error.none": 1}, requestCounters(m))
			})
		})
	}
}

func TestProxyCacheRequestMetrics(t *testing.T) {
	for _, tier := range []string{"lru", "l2", "warm"} {
		t.Run(tier, func(t *testing.T) {
			var originCalls atomic.Int64
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				originCalls.Add(1)
				assert.Empty(t, r.Header.Get(revalidateHeader))
				w.Header().Set("Cache-Control", "public, max-age=60")
				w.Header().Set("ETag", `"tag"`)
				fmt.Fprint(w, "representation")
			}))
			defer backend.Close()
			m := &metricstest.MockMetrics{}
			opts := Options{MaxBytes: 1 << 20, Metrics: m}
			if tier != "lru" {
				opts.L2Client = newStubValkeyClient()
				opts.IsNoL2Err = valkey.IsValkeyNil
			}
			if tier == "warm" {
				opts.L1TTL = time.Minute
			}
			spec := NewCacheFilter(opts).(*cacheSpec)
			defer spec.Close()
			registry := make(filters.Registry)
			registry.Register(spec)
			proxy := proxytest.New(registry, &eskip.Route{
				Id: "request", Backend: backend.URL, Filters: []*eskip.Filter{{Name: spec.Name()}},
			})
			defer proxy.Close()
			filter, err := spec.CreateFilter(nil)
			require.NoError(t, err)
			filter.(*cacheFilter).listenAddr = strings.TrimPrefix(proxy.URL, "http://")
			for _, request := range []struct {
				method, directive, etag string
				status                  int
			}{
				{method: http.MethodGet, status: http.StatusOK},
				{method: http.MethodHead, status: http.StatusOK},
				{method: http.MethodGet, etag: `"tag"`, status: http.StatusNotModified},
				{method: http.MethodGet, directive: "no-store", status: http.StatusOK},
				{method: http.MethodPost, status: http.StatusOK},
			} {
				req, err := http.NewRequest(request.method, proxy.URL+"/item", nil)
				require.NoError(t, err)
				req.Header.Set("Cache-Control", request.directive)
				req.Header.Set("If-None-Match", request.etag)
				rsp, err := proxy.Client().Do(req)
				require.NoError(t, err)
				body, err := io.ReadAll(rsp.Body)
				rsp.Body.Close()
				require.NoError(t, err)
				assert.Equal(t, request.status, rsp.StatusCode)
				if request.method == http.MethodHead || request.status == http.StatusNotModified {
					assert.Empty(t, body)
				} else {
					assert.Equal(t, "representation", string(body))
				}
			}
			wantTier := "l1"
			if tier == "l2" {
				wantTier = "l2"
			}
			assert.Equal(t, int64(3), originCalls.Load())
			assert.Equal(t, map[string]int64{
				"origin.none": 1, "fresh." + wantTier: 2, "bypass.none": 2,
			}, requestCounters(m))
		})
	}
}

func BenchmarkCacheRequestFreshHit(b *testing.B) {
	m := metrics.NewPrometheus(metrics.Options{})
	lru := NewLRUStorage(1<<20, nil, m)
	f := &cacheFilter{storage: lru, lruStorage: lru, metrics: m}
	ctx := newCtx(http.MethodGet, "http://cache.example/item", "")
	key := cacheKey(ctx.RouteId(), ctx.Request(), nil)
	if err := lru.Set(context.Background(), key, &Entry{
		StatusCode: http.StatusOK, Header: http.Header{}, Payload: []byte("body"),
		CreatedAt: time.Now(), TTL: time.Hour,
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		f.Request(ctx)
		ctx.Response().Body.Close()
	}
}
