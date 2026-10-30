package cache

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestProxyCacheOriginMetrics(t *testing.T) {
	for _, dispatch := range []string{"direct", "pipeline"} {
		for _, outcome := range []string{"success", "503", "disconnect"} {
			t.Run(dispatch+"/"+outcome, func(t *testing.T) {
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if outcome == "disconnect" {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						conn.Close()
						return
					}
					w.Header().Set("Cache-Control", "public, max-age=60")
					if outcome == "503" {
						w.WriteHeader(http.StatusServiceUnavailable)
					}
					fmt.Fprint(w, "origin")
				}))
				defer backend.Close()
				m := &metricstest.MockMetrics{}
				spec := NewCacheFilter(Options{MaxBytes: 1 << 20, Metrics: m}).(*cacheSpec)
				defer spec.Close()
				args := []any{"1m", "15s", "1m"}
				if dispatch == "pipeline" {
					args = nil
				}
				registry := make(filters.Registry)
				registry.Register(spec)
				p := proxytest.WithParams(registry, proxy.Params{Metrics: m}, &eskip.Route{
					Id: "product__en", Path: "/contentful", Backend: backend.URL,
					Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}},
				})
				defer p.Close()
				instance, err := spec.CreateFilter(args)
				require.NoError(t, err)
				instance.(*cacheFilter).listenAddr = strings.TrimPrefix(p.URL, "http://")
				resp, err := p.Client().Get(p.URL + "/contentful")
				require.NoError(t, err)
				_, err = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				require.NoError(t, err)
				statusClass := "2xx"
				if outcome == "503" {
					statusClass = "5xx"
					assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
				} else if outcome == "disconnect" {
					statusClass = "error"
					if dispatch == "pipeline" {
						// The listener returns an HTTP error for its backend transport failure.
						statusClass = "5xx"
					}
					assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
				} else {
					assert.Equal(t, http.StatusOK, resp.StatusCode)
				}
				m.WithCounters(func(counters map[string]int64) {
					assert.Equal(t, int64(1), counters["client.requests"])
					var fetches int64
					for name, count := range counters {
						if strings.HasPrefix(name, "cache_origin_duration.") {
							fetches += count
						}
					}
					assert.Equal(t, int64(1), fetches)
					assert.Equal(t, int64(1), counters["cache_origin_duration.product__en."+statusClass+"."+dispatch])
					if outcome != "disconnect" {
						assert.Equal(t, int64(1), counters["cache_origin_cache_control.public"])
						assert.Equal(t, int64(1), counters["cache_origin_cache_control.max-age"])
						assert.Equal(t, int64(1), counters["cache_route_result.product__en.miss"])
					}
				})
			})
		}
	}
}

func BenchmarkCacheRecordCacheControl(b *testing.B) {
	f := &cacheFilter{metrics: &metricstest.MockMetrics{}}
	header := http.Header{"Cache-Control": {"public, max-age=60, stale-while-revalidate=30"}}
	b.ReportAllocs()
	for b.Loop() {
		f.recordCacheControl(header)
	}
}

func TestCacheInternalMarkerDoesNotFollowExternalRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Skipper-Internal-Request") != "" {
			t.Error("internal request marker leaked to redirected origin")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, target.URL, http.StatusFound)
	}))
	defer listener.Close()
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, ListenAddr: strings.TrimPrefix(listener.URL, "http://")}).(*cacheSpec)
	defer spec.Close()
	instance, err := spec.CreateFilter(nil)
	require.NoError(t, err)
	f := instance.(*cacheFilter)
	req, err := http.NewRequest(http.MethodGet, listener.URL, nil)
	require.NoError(t, err)
	dispatched, err := f.prepareDispatch(req, "", "")
	require.NoError(t, err)
	resp, err := f.fetch(dispatched)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
