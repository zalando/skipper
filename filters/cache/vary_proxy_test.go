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
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestCacheFilter_Vary_RevalidationPreservesVariantKey(t *testing.T) {
	f := newTestFilter(t, 5*time.Minute, 15*time.Second, 5*time.Minute, 24*time.Hour)
	f.keyHeaders = []string{"Authorization"}
	f.fetch = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Vary": {"Accept-Language"}},
			Body:       io.NopCloser(strings.NewReader(req.Header.Get("Accept-Language") + "-refreshed")),
		}, nil
	}
	for _, language := range []string{"en", "de"} {
		ctx := newCtx(http.MethodGet, "http://origin.example/contentful", "Bearer fixture")
		ctx.FRequest.Header.Set("Accept-Language", language)
		baseKey := cacheKey(ctx.RouteId(), ctx.Request(), f.keyHeaders)
		key := varyKey(baseKey, ctx.Request(), []string{"Accept-Language"})
		f.doRevalidate(key, ctx.Request(), "http://origin.example", "", baseKey)
		entry, err := f.storage.Get(context.Background(), key)
		if err != nil || entry == nil || string(entry.Payload) != language+"-refreshed" {
			t.Fatalf("variant not refreshed at original key: %+v, %v", entry, err)
		}
		sentinel, err := f.storage.GetSilent(context.Background(), "vary:"+baseKey)
		if err != nil || sentinel == nil || sentinel.StaleIfError != 24*time.Hour {
			t.Fatalf("Vary sentinel must retain SIE discoverability: %+v, %v", sentinel, err)
		}
	}
	for _, language := range []string{"en", "de"} {
		ctx := newCtx(http.MethodGet, "http://origin.example/contentful", "Bearer fixture")
		ctx.FRequest.Header.Set("Accept-Language", language)
		f.Request(ctx)
		if !ctx.FServed || ctx.FResponse.Header.Get(cacheStatusHeader) != cacheStatusHit {
			t.Fatal("refreshed variant not found by Request")
		}
		body, _ := io.ReadAll(ctx.FResponse.Body)
		if string(body) != language+"-refreshed" {
			t.Fatalf("wrong refreshed variant: %q", body)
		}
	}
}

func TestProxy_CacheableVary_ConcurrentColdAndWarmIsolation(t *testing.T) {
	for _, keyAuthorization := range []bool{false, true} {
		t.Run(fmt.Sprintf("authorization=%t", keyAuthorization), func(t *testing.T) {
			const n = 12
			var joined sync.WaitGroup
			joined.Add(n)
			var calls atomic.Int64
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				joined.Wait()
				w.Header().Set("Vary", "Accept-Language")
				w.Header().Set("Cache-Control", "public, max-age=300")
				fmt.Fprint(w, r.Header.Get("Accept-Language"))
			}))
			defer backend.Close()
			spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
			t.Cleanup(func() { spec.(*cacheSpec).Close() })
			registry := make(filters.Registry)
			registry.Register(coalesceBarrierSpec{Spec: spec, joined: &joined})
			args := []any{"5m", "15s", "5m", "24h"}
			if keyAuthorization {
				args = append(args, "Authorization")
			}
			proxy := proxytest.New(registry, &eskip.Route{
				Id: "vary", Path: "/contentful", Backend: backend.URL,
				Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}},
			})
			defer proxy.Close()
			client := proxy.Client()
			client.Timeout = 10 * time.Second
			request := func(language string, warm bool) {
				req, _ := http.NewRequest(http.MethodGet, proxy.URL+"/contentful", nil)
				req.Header.Set("Accept-Language", language)
				if keyAuthorization {
					req.Header.Set("Authorization", "Bearer fixture")
				}
				rsp, err := client.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				defer rsp.Body.Close()
				body, err := io.ReadAll(rsp.Body)
				if err != nil || string(body) != language {
					t.Errorf("want language %q, got %q, err=%v", language, body, err)
				}
				if warm && rsp.Header.Get(cacheStatusHeader) != cacheStatusHit {
					t.Errorf("warm %s: want HIT, got %q", language, rsp.Header.Get(cacheStatusHeader))
				}
			}
			var wg sync.WaitGroup
			for i := range n {
				wg.Add(1)
				go func() {
					defer wg.Done()
					language := "en"
					if i%2 != 0 {
						language = "de"
					}
					request(language, false)
				}()
			}
			wg.Wait()
			coldCalls := calls.Load()
			for range 3 {
				request("en", true)
				request("de", true)
			}
			if got := calls.Load(); got != coldCalls {
				t.Fatalf("warm variants fetched origin: before=%d after=%d", coldCalls, got)
			}
		})
	}
}
