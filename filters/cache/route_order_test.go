package cache

import (
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
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestProxy_ContentfulCache_ResponseHeaderOrder(t *testing.T) {
	for _, headerBeforeCache := range []bool{false, true} {
		name := "response header after cache is skipped"
		if headerBeforeCache {
			name = "response header before cache applies to miss hit and stale"
		}
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Cache-Control", "private, max-age=17")
				io.WriteString(w, "origin")
			}))
			defer backend.Close()
			spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
			t.Cleanup(func() { spec.(*cacheSpec).Close() })
			registry := make(filters.Registry)
			registry.Register(spec)
			registry.Register(builtin.NewSetResponseHeader())
			cache := &eskip.Filter{Name: spec.Name(), Args: []any{"5m", "15s", "5m", "24h", "Authorization"}}
			header := &eskip.Filter{Name: "setResponseHeader", Args: []any{"Cache-Control", "public, max-age=300"}}
			chain := []*eskip.Filter{cache, header}
			if headerBeforeCache {
				chain = []*eskip.Filter{header, cache}
			}
			proxy := proxytest.New(registry, &eskip.Route{
				Id: "response-order", Path: "/contentful", Backend: backend.URL, Filters: chain,
			})
			defer proxy.Close()
			var f *cacheFilter
			for _, instance := range spec.(*cacheSpec).filters {
				f = instance
			}
			if f == nil {
				t.Fatal("missing cache filter")
			}
			ctx := newCtx(http.MethodGet, proxy.URL+"/contentful", "")
			ctx.FRequest.URL.Scheme = ""
			key := cacheKey("response-order", ctx.Request(), f.keyHeaders)
			for i := range 3 {
				if i == 2 {
					entry, err := f.storage.Get(ctx.Request().Context(), key)
					if err != nil || entry == nil {
						t.Fatalf("missing stored entry: %+v, %v", entry, err)
					}
					if entry.Header.Get("Cache-Control") != "private, max-age=17" {
						t.Fatal("client header rewrite must not modify stored origin metadata")
					}
					entry.CreatedAt = time.Now().Add(-6 * time.Minute)
					if err := f.storage.Set(ctx.Request().Context(), key, entry); err != nil {
						t.Fatal(err)
					}
				}
				rsp, err := proxy.Client().Get(proxy.URL + "/contentful")
				if err != nil {
					t.Fatal(err)
				}
				io.Copy(io.Discard, rsp.Body)
				rsp.Body.Close()
				want := "private, max-age=17"
				if headerBeforeCache {
					want = "public, max-age=300"
				}
				if got := rsp.Header.Get("Cache-Control"); got != want {
					t.Fatalf("request %d: Cache-Control = %q, want %q", i, got, want)
				}
				wantStatus := []string{cacheStatusMiss, cacheStatusHit, cacheStatusStale}[i]
				if got := rsp.Header.Get(cacheStatusHeader); got != wantStatus {
					t.Fatalf("request %d: cache status = %q, want %q", i, got, wantStatus)
				}
			}
		})
	}
}

func TestProxy_ContentfulCache_RequestRewriteOrder(t *testing.T) {
	const originHost = "pegasus.contentful.example"
	for _, cacheFirst := range []bool{true, false} {
		name := "rewrites before cache preserve origin contract"
		if cacheFirst {
			name = "cache before rewrites reproduces origin contract violation"
		}
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Cache-Control", "public, max-age=300")
				io.WriteString(w, r.Host+" "+r.URL.RequestURI())
			}))
			defer backend.Close()
			spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
			t.Cleanup(func() { spec.(*cacheSpec).Close() })
			registry := make(filters.Registry)
			registry.Register(spec)
			registry.Register(builtin.NewSetRequestHeader())
			registry.Register(builtin.NewModPath())
			cache := &eskip.Filter{Name: spec.Name(), Args: []any{"5m", "15s", "5m", "24h", "Authorization"}}
			chain := []*eskip.Filter{
				{Name: "setRequestHeader", Args: []any{"Host", originHost}},
				{Name: "modPath", Args: []any{"^/api/contentful", ""}},
			}
			if cacheFirst {
				chain = append([]*eskip.Filter{cache}, chain...)
			} else {
				chain = append(chain, cache)
			}
			proxy := proxytest.New(registry, &eskip.Route{
				Id: "contentful-order", PathRegexps: []string{"^/api/contentful/"},
				Backend: backend.URL, Filters: chain,
			})
			defer proxy.Close()
			want := originHost + " /entries?locale=en"
			if cacheFirst {
				want = strings.TrimPrefix(backend.URL, "http://") + " /api/contentful/entries?locale=en"
			}
			for i := range 2 {
				req, _ := http.NewRequest(http.MethodGet, proxy.URL+"/api/contentful/entries?locale=en", nil)
				req.Header.Set("Authorization", "Bearer fixture")
				rsp, err := proxy.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(rsp.Body)
				rsp.Body.Close()
				if err != nil || string(body) != want {
					t.Fatalf("request %d: upstream request = %q, want %q, err=%v", i, body, want, err)
				}
				if i == 1 && rsp.Header.Get(cacheStatusHeader) != cacheStatusHit {
					t.Fatal("second request should be a warm cache hit")
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("origin calls = %d, want 1", calls.Load())
			}
		})
	}
}
