package cache

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/metrics/metricstest"
)

func TestCacheOriginStatusClass(t *testing.T) {
	for _, tc := range []struct {
		code int
		err  error
		want string
	}{
		{200, nil, "2xx"}, {299, nil, "2xx"}, {300, nil, "3xx"}, {399, nil, "3xx"},
		{400, nil, "4xx"}, {499, nil, "4xx"}, {500, nil, "5xx"}, {599, nil, "5xx"},
		{0, nil, "other"}, {199, nil, "other"}, {600, nil, "other"}, {200, assert.AnError, "error"},
	} {
		t.Run(fmt.Sprintf("%d/%s", tc.code, tc.want), func(t *testing.T) {
			assert.Equal(t, tc.want, originStatusClass(&http.Response{StatusCode: tc.code}, tc.err))
		})
	}
	assert.Equal(t, "error", originStatusClass(nil, nil))
	assert.Equal(t, "error", originStatusClass(nil, assert.AnError))
}

func TestCacheRecordCacheControlBoundedLabels(t *testing.T) {
	m := &metricstest.MockMetrics{}
	f := &cacheFilter{metrics: m}
	h := http.Header{}
	h.Add("Cache-Control", ` PUBLIC , private="user-id", no-store, NO-CACHE="Authorization", max-age=60`)
	h.Add("Cache-Control", `s-maxage=120, stale-while-revalidate=30, stale-if-error=300, vendor-ext=secret, /private/path=token, , =ignored`)
	f.recordCacheControl(h)
	m.WithCounters(func(got map[string]int64) {
		require.Equal(t, map[string]int64{
			"cache_origin_cache_control.public": 1, "cache_origin_cache_control.private": 1,
			"cache_origin_cache_control.no-store": 1, "cache_origin_cache_control.no-cache": 1,
			"cache_origin_cache_control.max-age": 1, "cache_origin_cache_control.s-maxage": 1,
			"cache_origin_cache_control.stale-while-revalidate": 1, "cache_origin_cache_control.stale-if-error": 1,
			"cache_origin_cache_control.other": 1,
		}, got)
	})
}

func TestCacheRecordCacheControlOncePerResponse(t *testing.T) {
	m := &metricstest.MockMetrics{}
	f := &cacheFilter{metrics: m}
	h := http.Header{"Cache-Control": {"max-age=10, MAX-AGE=20, unknown=1", "max-age=30, another-unknown, unknown=2"}}
	for i := int64(1); i <= 2; i++ {
		f.recordCacheControl(h)
		m.WithCounters(func(got map[string]int64) {
			assert.Equal(t, map[string]int64{"cache_origin_cache_control.max-age": i, "cache_origin_cache_control.other": i}, got)
		})
	}
	f.recordCacheControl(http.Header{})
	m.WithCounters(func(got map[string]int64) { assert.Len(t, got, 2) })
}

func TestCacheFilterOriginMetrics(t *testing.T) {
	for _, mode := range []struct {
		name    string
		rfc     bool
		backend string
	}{
		{name: "direct", backend: "https://origin.example"},
		{name: "pipeline", rfc: true, backend: "https://origin.example"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			for _, background := range []bool{false, true} {
				for _, failed := range []bool{false, true} {
					t.Run(fmt.Sprintf("background=%t/error=%t", background, failed), func(t *testing.T) {
						synctest.Test(t, func(t *testing.T) {
							var f *cacheFilter
							var cleanup func()
							if mode.rfc {
								f, cleanup = newTestFilterRFCWithCleanup(t, 0, 0, 0)
							} else {
								f, cleanup = newTestFilterWithCleanup(t, time.Minute, 15*time.Second, 5*time.Minute)
							}
							defer cleanup()
							synctest.Wait()
							m := &metricstest.MockMetrics{}
							f.metrics = m
							fetches := 0
							f.fetch = func(req *http.Request) (*http.Response, error) {
								fetches++
								wantHost := f.listenAddr
								if !mode.rfc {
									wantHost = "origin.example"
								}
								assert.Equal(t, wantHost, req.URL.Host)
								assert.Equal(t, mode.rfc, req.Header.Get(revalidateHeader) == "1")
								if failed {
									return nil, assert.AnError
								}
								return upstreamResponseCC(http.StatusOK, "fresh", "public, max-age=60, stale-while-revalidate=300"), nil
							}
							ctx := newCtxWithRoute(http.MethodGet, "http://client.example/entries", "", "product__en")
							ctx.FBackendUrl = mode.backend
							ctx.FOutgoingHost = "origin.example"
							if background {
								key := cacheKey(ctx.RouteId(), ctx.Request(), f.keyHeaders)
								require.NoError(t, f.storage.Set(context.Background(), key, &Entry{
									StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": {"public, max-age=60"}}, Payload: []byte("stale"),
									CreatedAt: time.Now().Add(-90 * time.Second), TTL: time.Minute, StaleWhileRevalidate: 5 * time.Minute,
								}))
							}
							f.Request(ctx)
							synctest.Wait()
							if ctx.FResponse != nil && ctx.FResponse.Body != nil {
								defer ctx.FResponse.Body.Close()
							}
							assert.Equal(t, 1, fetches)
							statusClass := "2xx"
							if failed {
								statusClass = "error"
							}
							want := map[string]int64{"cache_origin_duration.product__en." + statusClass + "." + mode.name: 1}
							if background {
								require.True(t, ctx.FServed)
								assert.Equal(t, cacheStatusStale, ctx.FResponse.Header.Get(cacheStatusHeader))
								want["cache_route_reval.product__en.started"] = 1
								outcome := "success"
								if failed {
									outcome = "failed"
								}
								want["cache_route_reval.product__en."+outcome] = 1
							} else if !failed {
								require.True(t, ctx.FServed)
								assert.Equal(t, cacheStatusMiss, ctx.FResponse.Header.Get(cacheStatusHeader))
								count, ok := m.Counter("cache_origin_cache_control.max-age")
								assert.True(t, ok)
								assert.Equal(t, int64(1), count)
							}
							m.WithCounters(func(counters map[string]int64) {
								got := map[string]int64{}
								for name, count := range counters {
									if strings.HasPrefix(name, "cache_origin_duration.") || strings.HasPrefix(name, "cache_route_reval.") {
										got[name] = count
									}
								}
								assert.Equal(t, want, got)
							})
						})
					})
				}
			}
		})
	}
}

func TestCacheRouteRevalDropped(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancelled=%t", cancelled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelled {
				cancel()
			}
			m := &metricstest.MockMetrics{}
			f := &cacheFilter{metrics: m, ctx: ctx, revalJobs: make(chan revalJob)}
			f.enqueueRevalidation("key", newCtx(http.MethodGet, "http://client.example/entries", "").Request(), "", "", "product")
			m.WithCounters(func(got map[string]int64) {
				assert.Equal(t, map[string]int64{"cache.reval_dropped": 1, "cache_route_reval.product.dropped": 1}, got)
			})
		})
	}
}

func TestCacheResponseOriginCacheControl(t *testing.T) {
	f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
	m := &metricstest.MockMetrics{}
	f.metrics = m
	ctx := newCtx(http.MethodGet, "http://client.example/entries", "")
	f.Request(ctx)
	ctx.FResponse = upstreamResponseCC(http.StatusOK, "body", "public, max-age=60, max-age=120, vendor=secret")
	defer ctx.FResponse.Body.Close()
	f.Response(ctx)
	m.WithCounters(func(counters map[string]int64) {
		got := map[string]int64{}
		for name, count := range counters {
			if strings.HasPrefix(name, "cache_origin_cache_control.") {
				got[name] = count
			}
		}
		assert.Equal(t, map[string]int64{"cache_origin_cache_control.public": 1, "cache_origin_cache_control.max-age": 1, "cache_origin_cache_control.other": 1}, got)
	})
}

func TestCacheRouteRevalFailurePaths(t *testing.T) {
	for _, failure := range []string{"dispatch", "storage"} {
		t.Run(failure, func(t *testing.T) {
			f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
			m := &metricstest.MockMetrics{}
			f.metrics = m
			fetches := 0
			f.fetch = func(*http.Request) (*http.Response, error) {
				fetches++
				return upstreamResponseCC(http.StatusOK, "fresh", "max-age=60"), nil
			}
			backend := "https://origin.example"
			if failure == "dispatch" {
				backend = "not-an-absolute-url"
			} else {
				f.storage = &failingSetStorage{f.storage}
			}
			f.doRevalidateQueued(revalJob{
				key: "key", baseKey: "key", req: newCtx(http.MethodGet, "http://client.example/entries", "").Request(),
				backendURL: backend, outgoingHost: "origin.example", route: "product", enqueuedAt: time.Now(),
			})
			want := map[string]int64{"cache_route_reval.product.started": 1, "cache_route_reval.product.failed": 1}
			if failure == "storage" {
				want["cache_origin_duration.product.2xx.direct"] = 1
				assert.Equal(t, 1, fetches)
			} else {
				assert.Zero(t, fetches)
			}
			m.WithCounters(func(counters map[string]int64) {
				got := map[string]int64{}
				for name, count := range counters {
					if strings.HasPrefix(name, "cache_origin_duration.") || strings.HasPrefix(name, "cache_route_reval.") {
						got[name] = count
					}
				}
				assert.Equal(t, want, got)
			})
		})
	}
}

func TestCacheRouteRevalTerminalOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		sie          bool
		rfc          bool
		cacheControl string
		vary         string
		readError    bool
		fetchError   error
		evict        bool
		setError     bool
		wantOutcome  string
		wantError    int64
		wantStatus   int
	}{
		{name: "success", status: 200, wantOutcome: "success", wantStatus: 200},
		{name: "not modified", status: 304, wantOutcome: "success", wantStatus: 200},
		{name: "SIE 500", status: 500, sie: true, wantOutcome: "failed", wantError: 1, wantStatus: 200},
		{name: "SIE 502", status: 502, sie: true, wantOutcome: "failed", wantError: 1, wantStatus: 200},
		{name: "SIE 503", status: 503, sie: true, wantOutcome: "failed", wantError: 1, wantStatus: 200},
		{name: "SIE 504", status: 504, sie: true, wantOutcome: "failed", wantError: 1, wantStatus: 200},
		{name: "stored 503", status: 503, wantOutcome: "failed", wantError: 1, wantStatus: 503},
		{name: "stored 501 outside SIE statuses", status: 501, sie: true, wantOutcome: "failed", wantError: 1, wantStatus: 501},
		{name: "non-cacheable 503", status: 503, rfc: true, cacheControl: "no-store", wantOutcome: "failed", wantError: 1, wantStatus: 200},
		{name: "transport error", fetchError: assert.AnError, wantOutcome: "failed", wantError: 1, wantStatus: 200},
		{name: "timeout", fetchError: context.DeadlineExceeded, wantOutcome: "failed", wantError: 1, wantStatus: 200},
		{name: "body error", status: 200, readError: true, wantOutcome: "failed", wantError: 1, wantStatus: 200},
		{name: "503 body error counted once", status: 503, readError: true, wantOutcome: "failed", wantError: 1, wantStatus: 200},
		{name: "304 evicted", status: 304, evict: true, wantOutcome: "dropped"},
		{name: "no-store", status: 200, rfc: true, cacheControl: "no-store", wantOutcome: "dropped", wantStatus: 200},
		{name: "private", status: 200, rfc: true, cacheControl: "private", wantOutcome: "dropped", wantStatus: 200},
		{name: "vary star", status: 200, vary: "*", wantOutcome: "dropped", wantStatus: 200},
		{name: "uncacheable status", status: 302, wantOutcome: "dropped", wantStatus: 200},
		{name: "storage error", status: 200, setError: true, wantOutcome: "failed", wantStatus: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTestFilter(t, time.Minute, 10*time.Second, time.Minute)
			f.rfcMode = tc.rfc
			if tc.sie {
				f.staleIfError = time.Minute
			}
			m := &metricstest.MockMetrics{}
			f.metrics = m
			stored := &Entry{
				StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": {"max-age=60"}},
				Payload: []byte("stale"), CreatedAt: time.Now().Add(-90 * time.Second),
				TTL: time.Minute, StaleWhileRevalidate: time.Minute, ETag: `"original"`,
			}
			require.NoError(t, f.storage.Set(context.Background(), "key", stored))
			if tc.setError {
				f.storage = &failingSetStorage{f.storage}
			}
			f.fetch = func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, `"original"`, req.Header.Get("If-None-Match"))
				if tc.evict {
					require.NoError(t, f.storage.Delete(context.Background(), "key"))
				}
				if tc.fetchError != nil {
					return nil, tc.fetchError
				}
				resp := upstreamResponseCC(tc.status, "fresh", tc.cacheControl)
				resp.Header.Set("Vary", tc.vary)
				if tc.readError {
					resp.Body = errReader{}
				}
				return resp, nil
			}
			f.doRevalidateQueued(revalJob{
				key: "key", baseKey: "key", route: "product", enqueuedAt: time.Now(),
				req: newCtx(http.MethodGet, "http://client.example/entries", "").Request(),
			})
			m.WithCounters(func(counters map[string]int64) {
				got := map[string]int64{}
				for name, count := range counters {
					if strings.HasPrefix(name, "cache_route_reval.") {
						got[name] = count
					}
				}
				assert.Equal(t, map[string]int64{
					"cache_route_reval.product.started":           1,
					"cache_route_reval.product." + tc.wantOutcome: 1,
				}, got)
				assert.Equal(t, tc.wantError, counters["cache.reval_error"])
				assert.Zero(t, counters["cache.reval_dropped"])
			})
			got, err := f.storage.GetSilent(context.Background(), "key")
			if tc.evict {
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tc.wantStatus, got.StatusCode)
			if tc.wantOutcome == "success" || tc.wantStatus >= 500 {
				assert.True(t, got.CreatedAt.After(stored.CreatedAt))
				if tc.wantStatus >= 500 {
					assert.Equal(t, 10*time.Second, got.TTL)
					assert.Zero(t, got.StaleWhileRevalidate)
					assert.Equal(t, []byte("fresh"), got.Payload)
				}
			} else {
				assert.True(t, stored.CreatedAt.Equal(got.CreatedAt))
				assert.Equal(t, stored.Payload, got.Payload)
			}
		})
	}
}

func TestCacheRouteRevalSuperseded(t *testing.T) {
	f := newTestFilter(t, time.Minute, time.Second, time.Minute)
	m := &metricstest.MockMetrics{}
	f.metrics = m
	enqueuedAt := time.Now().Add(-time.Second)
	require.NoError(t, f.storage.Set(context.Background(), "key", &Entry{
		CreatedAt: time.Now(), TTL: time.Minute, StatusCode: http.StatusOK,
	}))
	f.fetch = func(*http.Request) (*http.Response, error) {
		t.Fatal("superseded job must not fetch")
		return nil, assert.AnError
	}
	f.doRevalidateQueued(revalJob{
		key: "key", baseKey: "key", route: "product", enqueuedAt: enqueuedAt,
		req: newCtx(http.MethodGet, "http://client.example/entries", "").Request(),
	})
	m.WithCounters(func(counters map[string]int64) {
		for name := range counters {
			assert.False(t, strings.HasPrefix(name, "cache_route_reval."), name)
		}
	})
}
