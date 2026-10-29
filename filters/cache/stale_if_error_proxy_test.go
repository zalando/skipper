package cache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestProxy_CacheLoopbackSIEPreservesVariant(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		for _, background := range []bool{false, true} {
			t.Run(fmt.Sprintf("status=%d/background=%t", status, background), func(t *testing.T) {
				var recovering atomic.Bool
				var calls atomic.Int64
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Vary", "Accept-Language")
					if !recovering.Load() {
						w.WriteHeader(status)
						io.WriteString(w, "unavailable")
						return
					}
					io.WriteString(w, r.Header.Get("Accept-Language")+"-recovered")
				}))
				defer backend.Close()
				spec := NewCacheFilter(Options{MaxBytes: 1 << 20}).(*cacheSpec)
				defer spec.Close()
				registry := make(filters.Registry)
				registry.Register(spec)
				registry.Register(builtin.NewSetDynamicBackendUrl())
				args := []any{"5m", "15s", "5m", "24h", "Authorization"}
				p := proxytest.New(registry, &eskip.Route{
					Id: "loopback-sie", Path: "/contentful", BackendType: eskip.DynamicBackend,
					Filters: []*eskip.Filter{
						{Name: spec.Name(), Args: args},
						{Name: "setDynamicBackendUrl", Args: []any{backend.URL}},
					},
				})
				defer p.Close()
				filter, err := spec.CreateFilter(args)
				if err != nil {
					t.Fatal(err)
				}
				f := filter.(*cacheFilter)
				f.listenAddr = strings.TrimPrefix(p.URL, "http://")
				req, err := http.NewRequest(http.MethodGet, p.URL+"/contentful", nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "fixture")
				req.Header.Set("Accept-Language", "en")
				keyReq := req.Clone(context.Background())
				keyReq.URL.Scheme = ""
				base := cacheKey("loopback-sie", keyReq, f.keyHeaders)
				key := varyKey(base, keyReq, []string{"Accept-Language"})
				before := &Entry{
					StatusCode: http.StatusOK, Header: http.Header{"Vary": {"Accept-Language"}},
					Payload: []byte("last-good"), CreatedAt: time.Now().Add(-11 * time.Minute),
					TTL: 5 * time.Minute, StaleWhileRevalidate: 5 * time.Minute, StaleIfError: 24 * time.Hour,
				}
				if err := f.storeEntry(context.Background(), base, key, keyReq, before); err != nil {
					t.Fatal(err)
				}
				before, err = f.storage.GetSilent(context.Background(), key)
				if err != nil || before == nil {
					t.Fatalf("seed absent: %v", err)
				}
				request := func(wantBody, wantCache string) {
					t.Helper()
					resp, err := p.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(resp.Body)
					resp.Body.Close()
					if err != nil || resp.StatusCode != http.StatusOK || string(body) != wantBody ||
						resp.Header.Get(cacheStatusHeader) != wantCache {
						t.Fatalf("status=%d body=%q cache=%q err=%v", resp.StatusCode, body, resp.Header.Get(cacheStatusHeader), err)
					}
				}
				for range 2 {
					if background {
						f.doRevalidate(key, keyReq, "", "", base)
					} else {
						request("last-good", cacheStatusStale)
					}
					after, err := f.storage.GetSilent(context.Background(), key)
					if err != nil || !reflect.DeepEqual(before, after) {
						t.Fatalf("failed loopback refresh overwrote last-good variant: before=%+v after=%+v err=%v", before, after, err)
					}
				}
				recovering.Store(true)
				if background {
					f.doRevalidate(key, keyReq, "", "", base)
				} else {
					request("en-recovered", cacheStatusMiss)
				}
				request("en-recovered", cacheStatusHit)
				if calls.Load() != 3 {
					t.Fatalf("want two failures and one recovery fetch, got %d", calls.Load())
				}
			})
		}
	}
}
