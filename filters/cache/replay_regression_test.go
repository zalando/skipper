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
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/metrics/metricstest"
	skipperproxy "github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
)

type refreshCompletionBody struct {
	io.ReadCloser
	done chan struct{}
}

func (b refreshCompletionBody) Close() error {
	err := b.ReadCloser.Close()
	close(b.done)
	return err
}

func TestProxy_CacheReplayRequiresOriginal(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		for _, dynamic := range []bool{false, true} {
			t.Run(fmt.Sprintf("preserve=%t/dynamic=%t", preserve, dynamic), func(t *testing.T) {
				var calls atomic.Int64
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					if r.Method != http.MethodGet || r.URL.RequestURI() != "/item?q=1" || r.Header.Get(revalidateHeader) != "" {
						t.Errorf("origin method=%s uri=%s replay=%s", r.Method, r.URL.RequestURI(), r.Header.Get(revalidateHeader))
					}
					w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=3600")
					fmt.Fprintf(w, "payload-%d", n)
				}))
				defer backend.Close()
				m := &metricstest.MockMetrics{}
				spec := NewCacheFilter(Options{MaxBytes: 1 << 20, Metrics: m})
				defer spec.(*cacheSpec).Close()
				registry := make(filters.Registry)
				registry.Register(spec)
				registry.Register(builtin.NewModPath())
				registry.Register(builtin.NewSetDynamicBackendUrl())
				route := &eskip.Route{
					Id: "replay", Path: "/public/item", Backend: backend.URL,
					Filters: []*eskip.Filter{
						{Name: "modPath", Args: []any{"^/public", ""}},
						{Name: Name},
					},
				}
				if dynamic {
					route.Backend = ""
					route.BackendType = eskip.DynamicBackend
					route.Filters[1].Args = []any{"1m", "15s", "1h"}
					route.Filters = append(route.Filters, &eskip.Filter{Name: "setDynamicBackendUrl", Args: []any{backend.URL}})
				}
				params := skipperproxy.Params{}
				if preserve {
					params.Flags = skipperproxy.PreserveOriginal
				}
				proxy := proxytest.WithParams(registry, params, route)
				defer proxy.Close()
				client := proxy.Client()
				client.Timeout = 2 * time.Second
				for _, f := range spec.(*cacheSpec).filters {
					f.listenAddr = strings.TrimPrefix(proxy.URL, "http://")
				}
				request := func(wantBody, wantStatus string) {
					t.Helper()
					resp, err := client.Get(proxy.URL + "/public/item?q=1")
					if err != nil {
						t.Fatal(err)
					}
					body, readErr := io.ReadAll(resp.Body)
					resp.Body.Close()
					if readErr != nil || resp.StatusCode != http.StatusOK || string(body) != wantBody ||
						resp.Header.Get(cacheStatusHeader) != wantStatus {
						t.Fatalf("status=%d cache=%s body=%q err=%v", resp.StatusCode, resp.Header.Get(cacheStatusHeader), body, readErr)
					}
				}
				request("payload-1", cacheStatusMiss)
				request("payload-1", cacheStatusHit)
				if calls.Load() != 1 {
					t.Fatalf("cold/warm origin calls=%d want=1", calls.Load())
				}
				req := httptest.NewRequest("GET", proxy.URL+"/item?q=1", nil)
				req.URL.Scheme = ""
				key := cacheKey("replay", req, nil)
				done := make(chan struct{})
				var active *cacheFilter
				for _, f := range spec.(*cacheSpec).filters {
					active = f
					entry, err := f.storage.Get(context.Background(), key)
					if err != nil || entry == nil {
						t.Fatalf("entry missing: %v", err)
					}
					entry.CreatedAt = time.Now().Add(-2 * time.Minute)
					if err := f.storage.Set(context.Background(), key, entry); err != nil {
						t.Fatal(err)
					}
					fetch := f.fetch
					f.fetch = func(r *http.Request) (*http.Response, error) {
						resp, err := fetch(r)
						if err == nil {
							resp.Body = refreshCompletionBody{ReadCloser: resp.Body, done: done}
						}
						return resp, err
					}
				}
				request("payload-1", cacheStatusStale)
				if preserve {
					select {
					case <-done:
					case <-time.After(2 * time.Second):
						t.Fatal("refresh did not complete")
					}
					request("payload-2", cacheStatusHit)
					if calls.Load() != 2 {
						t.Fatalf("refresh origin calls=%d want=2", calls.Load())
					}
				} else {
					entry, err := active.storage.Get(context.Background(), key)
					if err != nil || entry == nil || string(entry.Payload) != "payload-1" || !entry.IsStale(time.Now()) {
						t.Fatalf("unavailable replay overwrote stale representation: %+v err=%v", entry, err)
					}
					m.WithCounters(func(counters map[string]int64) {
						if counters["cache.reval_dropped"] != 1 {
							t.Errorf("unavailable refresh drops=%d want=1", counters["cache.reval_dropped"])
						}
					})
					if calls.Load() != 1 {
						t.Fatalf("unavailable refresh contacted origin %d times", calls.Load())
					}
				}
			})
		}
	}
}
