package cache

import (
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
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestProxy_CacheBypassBodyOwnership(t *testing.T) {
	for _, varyStar := range []bool{true, false} {
		t.Run(fmt.Sprintf("varyStar=%t", varyStar), func(t *testing.T) {
			const n = 4
			payload := strings.Repeat("body", 1024)
			var joined sync.WaitGroup
			joined.Add(n)
			var calls atomic.Int64
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				joined.Wait()
				if varyStar {
					w.Header().Set("Vary", "*")
				}
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
				fmt.Fprint(w, payload)
			}))
			defer backend.Close()
			spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
			defer spec.(*cacheSpec).Close()
			registry := make(filters.Registry)
			registry.Register(coalesceBarrierSpec{Spec: spec, joined: &joined})
			proxy := proxytest.New(registry, &eskip.Route{
				Id: "ownership", Path: "/contentful", Backend: backend.URL,
				Filters: []*eskip.Filter{{Name: spec.Name(), Args: []any{"4s", "2s", "4s", "12s"}}},
			})
			defer proxy.Close()
			client := proxy.Client()
			request := func() {
				req, _ := http.NewRequest("GET", proxy.URL+"/contentful", nil)
				req.Header.Set("Authorization", "fixture")
				resp, err := client.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil || string(body) != payload || resp.ContentLength != int64(len(payload)) {
					t.Errorf("status=%d bytes=%d contentLength=%d err=%v", resp.StatusCode, len(body), resp.ContentLength, err)
				}
			}
			var wg sync.WaitGroup
			for range n {
				wg.Add(1)
				go func() { defer wg.Done(); request() }()
			}
			wg.Wait()
			before := calls.Load()
			joined.Add(1)
			request()
			if calls.Load() != before+1 {
				t.Fatal("nonstoreable response cached")
			}
		})
	}
}

type bypassFailingBody struct{ closed atomic.Bool }

func (*bypassFailingBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (b *bypassFailingBody) Close() error           { b.closed.Store(true); return nil }

func TestCacheFilter_BypassBodyReadError(t *testing.T) {
	f := newTestFilter(t, time.Minute, time.Second, time.Minute)
	m := &metricstest.MockMetrics{}
	f.metrics = m
	var joined sync.WaitGroup
	joined.Add(2)
	var calls atomic.Int64
	bad := &bypassFailingBody{}
	f.fetch = func(*http.Request) (*http.Response, error) {
		joined.Wait()
		var body io.ReadCloser = io.NopCloser(strings.NewReader("leader"))
		if calls.Add(1) > 1 {
			body = bad
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Vary": {"*"}}, Body: body}, nil
	}
	served := make(chan bool, 2)
	for range 2 {
		go func() {
			ctx := newCtx("GET", "http://origin.example/body", "")
			ctx.FRequest = ctx.FRequest.WithContext(&coalesceBarrierContext{Context: ctx.FRequest.Context(), joined: &joined})
			f.Request(ctx)
			served <- ctx.FServed
		}()
	}
	a, b := <-served, <-served
	errors, _ := m.Counter("cache.coalesce_bypass_error")
	if a == b || !bad.closed.Load() || errors != 1 {
		t.Fatalf("served=%t,%t closed=%t errors=%d", a, b, bad.closed.Load(), errors)
	}
}
