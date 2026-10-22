package cache

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

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
