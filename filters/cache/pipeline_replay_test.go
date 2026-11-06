package cache

import (
	"context"
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
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/sed"
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
			originalName := "original_unavailable"
			if preserve {
				originalName = "original_preserved"
			}
			backendName := "static_backend"
			if dynamic {
				backendName = "dynamic_backend"
			}
			t.Run(originalName+"/"+backendName, func(t *testing.T) {
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

func newReplayProxy(t *testing.T, route *eskip.Route, extra ...filters.Spec) *proxytest.TestProxy {
	t.Helper()
	p, _ := newReplayProxyWithSpec(t, route, extra...)
	return p
}

func newReplayProxyWithSpec(t *testing.T, route *eskip.Route, extra ...filters.Spec) (*proxytest.TestProxy, *cacheSpec) {
	t.Helper()
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20, Metrics: &metricstest.MockMetrics{}})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	registry := make(filters.Registry)
	registry.Register(spec)
	for _, s := range extra {
		registry.Register(s)
	}
	p := proxytest.WithParams(registry, skipperproxy.Params{Flags: skipperproxy.PreserveOriginal}, route)
	t.Cleanup(func() { p.Close() })
	for _, f := range spec.(*cacheSpec).filters {
		f.listenAddr = strings.TrimPrefix(p.URL, "http://")
	}
	return p, spec.(*cacheSpec)
}

func TestProxy_CacheReplayHonoursFilterAddedAuthorization(t *testing.T) {
	var calls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Cache-Control", "max-age=60")
		fmt.Fprintf(w, "private:%s", r.Header.Get("Authorization"))
	}))
	defer backend.Close()
	p := newReplayProxy(t, &eskip.Route{
		Id: "auth", Path: "/resource", Backend: backend.URL,
		Filters: []*eskip.Filter{
			{Name: "copyRequestHeader", Args: []any{"X-User-Token", "Authorization"}},
			{Name: Name},
		},
	}, builtin.NewCopyRequestHeader())

	get := func(token string) string {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, p.URL+"/resource", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-User-Token", token)
		rsp, err := p.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()
		body, err := io.ReadAll(rsp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	for _, token := range []string{"A", "B"} {
		if got, want := get(token), "private:"+token; got != want {
			t.Fatalf("token %s got %q want %q", token, got, want)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("origin calls=%d want=2: credential-specific response was shared", calls.Load())
	}
}

func TestProxy_CacheReplayAppliesResponseFiltersOnce(t *testing.T) {
	var calls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Cache-Control", "public, max-age=60")
		io.WriteString(w, "a")
	}))
	defer backend.Close()
	p := newReplayProxy(t, &eskip.Route{
		Id: "transform", Path: "/resource", Backend: backend.URL,
		Filters: []*eskip.Filter{
			{Name: "sed", Args: []any{"a", "aa"}},
			{Name: Name},
		},
	}, sed.New())

	for _, want := range []string{cacheStatusMiss, cacheStatusHit} {
		rsp, err := p.Client().Get(p.URL + "/resource")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rsp.Body)
		rsp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if got := rsp.Header.Get(cacheStatusHeader); got != want || string(body) != "aa" {
			t.Fatalf("cache=%s body=%q want cache=%s body=%q", got, body, want, "aa")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("origin calls=%d want=1", calls.Load())
	}
}

func TestProxy_CacheReplayHeadersNeverReachOrigin(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers map[string]string
	}{
		{name: "cold_replay"},
		{name: "client_capture", headers: map[string]string{replayCaptureHeader: "forged"}},
		{name: "client_capture_and_revalidate", headers: map[string]string{replayCaptureHeader: "forged", revalidateHeader: "1"}},
		{name: "client_revalidate", headers: map[string]string{revalidateHeader: "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				for _, h := range []string{replayCaptureHeader, revalidateHeader} {
					if v := r.Header.Get(h); v != "" {
						t.Errorf("origin received %s=%q", h, v)
					}
				}
				w.Header().Set("Cache-Control", "public, max-age=60")
				fmt.Fprintf(w, "payload-%d", n)
			}))
			defer backend.Close()
			p := newReplayProxy(t, &eskip.Route{
				Id: "isolation", Path: "/public/resource", Backend: backend.URL,
				Filters: []*eskip.Filter{
					{Name: "modPath", Args: []any{"^/public", ""}},
					{Name: Name},
				},
			}, builtin.NewModPath())

			get := func(headers map[string]string) (string, string) {
				t.Helper()
				req, err := http.NewRequest(http.MethodGet, p.URL+"/public/resource", nil)
				if err != nil {
					t.Fatal(err)
				}
				for k, v := range headers {
					req.Header.Set(k, v)
				}
				rsp, err := p.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(rsp.Body)
				rsp.Body.Close()
				if err != nil || rsp.StatusCode != http.StatusOK {
					t.Fatalf("status=%d err=%v", rsp.StatusCode, err)
				}
				return string(body), rsp.Header.Get(cacheStatusHeader)
			}

			if body, _ := get(tc.headers); body != "payload-1" {
				t.Fatalf("first body=%q want payload-1", body)
			}
			if body, status := get(nil); body != "payload-1" || status != cacheStatusHit {
				t.Fatalf("second body=%q cache=%s want payload-1 hit", body, status)
			}
			if calls.Load() != 1 {
				t.Fatalf("origin calls=%d want=1", calls.Load())
			}
		})
	}
}

func TestProxy_CacheReplayConcurrentCredentialsStayIsolated(t *testing.T) {
	const clients = 8
	start := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-start
		w.Header().Set("Cache-Control", "max-age=60")
		fmt.Fprintf(w, "private:%s", r.Header.Get("Authorization"))
	}))
	defer backend.Close()
	p, spec := newReplayProxyWithSpec(t, &eskip.Route{
		Id: "concurrent-auth", Path: "/resource", Backend: backend.URL,
		Filters: []*eskip.Filter{
			{Name: "copyRequestHeader", Args: []any{"X-User-Token", "Authorization"}},
			{Name: Name},
		},
	}, builtin.NewCopyRequestHeader())

	client := p.Client()
	var wg sync.WaitGroup
	var issued sync.WaitGroup
	issued.Add(clients)
	for i := range clients {
		token := fmt.Sprintf("token-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, p.URL+"/resource", nil)
			if err != nil {
				issued.Done()
				t.Error(err)
				return
			}
			req.Header.Set("X-User-Token", token)
			issued.Done()
			rsp, err := client.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			body, err := io.ReadAll(rsp.Body)
			rsp.Body.Close()
			if err != nil || string(body) != "private:"+token {
				t.Errorf("token %s got %q err=%v", token, body, err)
			}
		}()
	}
	issued.Wait()
	close(start)
	wg.Wait()

	for _, f := range spec.filters {
		f.replays.Range(func(k, _ any) bool {
			t.Errorf("replay registry retains %v", k)
			return true
		})
	}
}

func TestProxy_CacheRefreshAppliesResponseFiltersOnce(t *testing.T) {
	var calls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=3600")
		io.WriteString(w, "a")
	}))
	defer backend.Close()
	p, spec := newReplayProxyWithSpec(t, &eskip.Route{
		Id: "refresh", Path: "/resource", Backend: backend.URL,
		Filters: []*eskip.Filter{
			{Name: "sed", Args: []any{"a", "aa"}},
			{Name: Name},
		},
	}, sed.New())

	get := func(wantStatus string) {
		t.Helper()
		rsp, err := p.Client().Get(p.URL + "/resource")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rsp.Body)
		rsp.Body.Close()
		if got := rsp.Header.Get(cacheStatusHeader); err != nil || got != wantStatus || string(body) != "aa" {
			t.Fatalf("cache=%s body=%q err=%v want cache=%s body=aa", got, body, err, wantStatus)
		}
	}

	get(cacheStatusMiss)

	req := httptest.NewRequest("GET", p.URL+"/resource", nil)
	req.URL.Scheme = ""
	key := cacheKey("refresh", req, nil)
	done := make(chan struct{})
	for _, f := range spec.filters {
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
	get(cacheStatusStale)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not complete")
	}
	get(cacheStatusHit)
	if calls.Load() != 2 {
		t.Fatalf("origin calls=%d want=2", calls.Load())
	}
}
