package cache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestProxy_CacheHEADPreservesGET(t *testing.T) {
	for _, expired := range []bool{false, true} {
		name := "cold_entry"
		if expired {
			name = "expired_entry"
		}
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Length", "7")
				w.Header().Set("X-Metadata", r.Method)
				fmt.Fprint(w, "payload")
			}))
			defer backend.Close()
			spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
			defer spec.(*cacheSpec).Close()
			registry := make(filters.Registry)
			registry.Register(spec)
			proxy := proxytest.New(registry, &eskip.Route{
				Id: "head", Path: "/resource", Backend: backend.URL,
				Filters: []*eskip.Filter{{Name: Name, Args: []any{"1m", "15s", "1s", "1h"}}},
			})
			defer proxy.Close()
			client := proxy.Client()
			client.Timeout = 2 * time.Second
			request := func(method string) {
				t.Helper()
				req, err := http.NewRequest(method, proxy.URL+"/resource", nil)
				if err != nil {
					t.Fatal(err)
				}
				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body, readErr := io.ReadAll(resp.Body)
				resp.Body.Close()
				want := "payload"
				if method == http.MethodHead {
					want = ""
				}
				if readErr != nil || resp.StatusCode != http.StatusOK || string(body) != want ||
					resp.ContentLength != 7 || req.Method != method {
					t.Fatalf("%s status=%d body=%q length=%d err=%v", method, resp.StatusCode, body, resp.ContentLength, readErr)
				}
			}
			if expired {
				request(http.MethodGet)
				req := httptest.NewRequest("GET", proxy.URL+"/resource", nil)
				req.URL.Scheme = ""
				for _, f := range spec.(*cacheSpec).filters {
					key := cacheKey("head", req, nil)
					entry, err := f.storage.Get(context.Background(), key)
					if err != nil || entry == nil {
						t.Fatalf("seed missing: %v", err)
					}
					entry.CreatedAt = time.Now().Add(-2 * time.Minute)
					if err := f.storage.Set(context.Background(), key, entry); err != nil {
						t.Fatal(err)
					}
				}
			}
			request(http.MethodHead)
			request(http.MethodGet)
			request(http.MethodGet)
			wantCalls := int64(2)
			if expired {
				wantCalls = 3
			}
			if got := calls.Load(); got != wantCalls {
				t.Fatalf("origin calls=%d want=%d", got, wantCalls)
			}
		})
	}
}

func TestProxy_CacheConcurrentHEADAndGETPreservesGETBody(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			once.Do(func() { close(started) })
			<-release
		}
		w.Header().Set("Content-Length", "7")
		fmt.Fprint(w, "payload")
	}))
	defer backend.Close()
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
	defer spec.(*cacheSpec).Close()
	registry := make(filters.Registry)
	var joined sync.WaitGroup
	joined.Add(2)
	registry.Register(coalesceBarrierSpec{Spec: spec, joined: &joined})
	proxy := proxytest.New(registry, &eskip.Route{
		Id: "head", Path: "/resource", Backend: backend.URL,
		Filters: []*eskip.Filter{{Name: Name, Args: []any{"1m", "15s", "1s"}}},
	})
	defer proxy.Close()
	client := proxy.Client()
	client.Timeout = 2 * time.Second
	headDone := make(chan struct{})
	go func() {
		defer close(headDone)
		resp, err := client.Head(proxy.URL + "/resource")
		if err != nil {
			t.Error(err)
			return
		}
		resp.Body.Close()
	}()
	<-started
	getDone := make(chan struct{})
	go func() {
		defer close(getDone)
		resp, err := client.Get(proxy.URL + "/resource")
		if err != nil {
			t.Error(err)
			return
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil || string(body) != "payload" {
			t.Errorf("GET body=%q err=%v", body, readErr)
		}
	}()
	joined.Wait()
	close(release)
	<-headDone
	<-getDone
}

func TestCacheFilter_HEADOrdinaryHandling(t *testing.T) {
	for _, seeded := range []bool{false, true} {
		name := "no_stored_entry"
		if seeded {
			name = "stored_entry"
		}
		t.Run(name, func(t *testing.T) {
			f := newTestFilter(t, time.Minute, time.Second, time.Second)
			ctx := newCtx(http.MethodHead, "http://example.org/resource", "")
			key := cacheKey("", ctx.Request(), nil)
			if seeded {
				if err := f.storage.Set(context.Background(), key, &Entry{
					StatusCode: http.StatusOK, Payload: []byte("payload"),
					Header:    http.Header{"X-Metadata": {"old"}, "Content-Length": {"7"}},
					CreatedAt: time.Now().Add(-time.Hour), TTL: time.Minute, StaleIfError: 2 * time.Hour,
				}); err != nil {
					t.Fatal(err)
				}
			}
			f.fetch = func(*http.Request) (*http.Response, error) {
				t.Fatal("HEAD miss must follow ordinary proxy handling")
				return nil, nil
			}
			f.Request(ctx)
			if ctx.FServed || ctx.Request().Method != http.MethodHead {
				t.Fatal("HEAD miss served or method changed")
			}
			ctx.FResponse = &http.Response{
				StatusCode: http.StatusOK, Body: http.NoBody,
				Header: http.Header{"X-Metadata": {"new"}, "Content-Length": {"7"}},
			}
			f.Response(ctx)
			entry, err := f.storage.Get(context.Background(), key)
			if err != nil {
				t.Fatal(err)
			}
			if !seeded {
				if entry != nil {
					t.Fatal("cold HEAD created a bodyless GET entry")
				}
			} else if entry == nil || string(entry.Payload) != "payload" || entry.Header.Get("X-Metadata") != "new" {
				t.Fatalf("HEAD metadata freshening lost representation: %+v", entry)
			}
		})
	}
}

func TestProxy_CacheHEADMethodRoute(t *testing.T) {
	var calls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodHead {
			t.Errorf("HEAD route origin method=%s", r.Method)
		}
		w.Header().Set("Content-Length", "7")
		fmt.Fprint(w, "payload")
	}))
	defer backend.Close()
	spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
	defer spec.(*cacheSpec).Close()
	registry := make(filters.Registry)
	registry.Register(spec)
	proxy := proxytest.New(registry,
		&eskip.Route{
			Id: "head-only", Path: "/resource", Method: http.MethodHead, Backend: backend.URL,
			Filters: []*eskip.Filter{{Name: Name}},
		},
		&eskip.Route{Id: "get-only", Path: "/resource", Method: http.MethodGet, BackendType: eskip.ShuntBackend},
	)
	defer proxy.Close()
	resp, err := proxy.Client().Head(proxy.URL + "/resource")
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil || len(body) != 0 || resp.StatusCode != http.StatusOK || resp.ContentLength != 7 || calls.Load() != 1 {
		t.Fatalf("HEAD status=%d body=%q calls=%d err=%v", resp.StatusCode, body, calls.Load(), readErr)
	}
}

func TestCacheFilter_HEADRefreshSkipped(t *testing.T) {
	f := newTestFilter(t, time.Minute, time.Second, time.Second)
	m := &metricstest.MockMetrics{}
	f.metrics = m
	f.fetch = func(*http.Request) (*http.Response, error) {
		t.Fatal("HEAD refresh must not fetch a bodyless representation")
		return nil, nil
	}
	req := httptest.NewRequest(http.MethodHead, "http://example.org/resource", nil)
	f.enqueueRevalidation("key", req, "http://origin.example", "")
	f.doRevalidate("key", req, "", "")
	m.WithCounters(func(counters map[string]int64) {
		if counters["cache.reval_dropped"] != 2 {
			t.Errorf("dropped refreshes=%d want=2", counters["cache.reval_dropped"])
		}
	})
	if req.Method != http.MethodHead || req.Header.Get(revalidateHeader) != "" {
		t.Fatal("skipped refresh mutated request")
	}
}

func TestProxy_CacheHEADErrorDoesNotReplaceGETRepresentation(t *testing.T) {
	var calls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Length", "7")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "missing")
	}))
	defer backend.Close()
	p := newReplayProxy(t, &eskip.Route{
		Id: "head-error", Path: "/resource", Backend: backend.URL,
		Filters: []*eskip.Filter{{Name: Name, Args: []any{"1m", "15s", "1s"}}},
	})

	for _, method := range []string{http.MethodHead, http.MethodGet} {
		req, err := http.NewRequest(method, p.URL+"/resource", nil)
		if err != nil {
			t.Fatal(err)
		}
		rsp, err := p.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(rsp.Body)
		rsp.Body.Close()
		if method == http.MethodGet && (readErr != nil || string(body) != "missing" || rsp.Header.Get(cacheStatusHeader) == cacheStatusHit) {
			t.Fatalf("GET after HEAD error: cache=%s body=%q err=%v", rsp.Header.Get(cacheStatusHeader), body, readErr)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("origin calls=%d want=2: HEAD error was stored as the GET representation", calls.Load())
	}
}

func TestProxy_CacheHEADErrorKeepsExpiredRetainedGET(t *testing.T) {
	var gets atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		gets.Add(1)
		w.Header().Set("Content-Length", "7")
		io.WriteString(w, "payload")
	}))
	defer backend.Close()
	p, spec := newReplayProxyWithSpec(t, &eskip.Route{
		Id: "head-retained", Path: "/resource", Backend: backend.URL,
		Filters: []*eskip.Filter{{Name: Name, Args: []any{"1m", "15s", "1s", "1h"}}},
	})
	do := func(method string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(method, p.URL+"/resource", nil)
		if err != nil {
			t.Fatal(err)
		}
		rsp, err := p.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rsp.Body)
		rsp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return rsp.StatusCode, string(body)
	}

	if status, body := do(http.MethodGet); status != http.StatusOK || body != "payload" {
		t.Fatalf("seed status=%d body=%q", status, body)
	}
	req := httptest.NewRequest("GET", p.URL+"/resource", nil)
	req.URL.Scheme = ""
	key := cacheKey("head-retained", req, nil)
	for _, f := range spec.filters {
		entry, err := f.storage.Get(context.Background(), key)
		if err != nil || entry == nil {
			t.Fatalf("seed missing: %v", err)
		}
		entry.CreatedAt = time.Now().Add(-2 * time.Minute)
		if err := f.storage.Set(context.Background(), key, entry); err != nil {
			t.Fatal(err)
		}
	}
	if status, _ := do(http.MethodHead); status != http.StatusInternalServerError {
		t.Fatalf("HEAD status=%d want=500", status)
	}
	for _, f := range spec.filters {
		entry, err := f.storage.Get(context.Background(), key)
		if err != nil || entry == nil || string(entry.Payload) != "payload" {
			t.Fatalf("HEAD 500 replaced retained GET: %+v err=%v", entry, err)
		}
	}
	if status, body := do(http.MethodGet); status != http.StatusOK || body != "payload" {
		t.Fatalf("GET after HEAD 500 status=%d body=%q", status, body)
	}
}
