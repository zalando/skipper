package cache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
	skipperproxy "github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
)

// Done is evaluated in coalesce's select only after joining singleflight.
// Holding the backend until every waiter joins avoids scheduler-dependent sleeps.
type coalesceBarrierContext struct {
	context.Context
	joined *sync.WaitGroup
	once   sync.Once
}

func (c *coalesceBarrierContext) Done() <-chan struct{} {
	c.once.Do(c.joined.Done)
	return c.Context.Done()
}

type coalesceBarrierSpec struct {
	filters.Spec
	joined *sync.WaitGroup
}

func (s coalesceBarrierSpec) CreateFilter(args []any) (filters.Filter, error) {
	f, err := s.Spec.CreateFilter(args)
	if err != nil {
		return nil, err
	}
	return coalesceBarrierFilter{Filter: f, joined: s.joined}, nil
}

type coalesceBarrierFilter struct {
	filters.Filter
	joined *sync.WaitGroup
}

type dispatchContext struct {
	filters.FilterContext
	req *http.Request
}

func (c dispatchContext) Request() *http.Request { return c.req }

func (f coalesceBarrierFilter) Request(ctx filters.FilterContext) {
	ctx.SetOutgoingHost("virtual.origin.example")
	req := ctx.Request()
	req = req.WithContext(&coalesceBarrierContext{Context: req.Context(), joined: f.joined})
	f.Filter.Request(dispatchContext{FilterContext: ctx, req: req})
}

func TestCacheFilter_DispatchSelection(t *testing.T) {
	for _, tt := range []struct {
		name    string
		backend string
		rfc     bool
		direct  bool
		invalid bool
	}{
		{name: "http", backend: "http://origin.example", direct: true},
		{name: "https", backend: "https://origin.example:443/base?ignored=1", direct: true},
		{name: "dynamic"},
		{name: "malformed", backend: "http://[::1", invalid: true},
		{name: "relative", backend: "/origin", invalid: true},
		{name: "no host", backend: "http:///path", invalid: true},
		{name: "empty hostname", backend: "http://:80", invalid: true},
		{name: "unsupported scheme", backend: "ftp://origin.example", invalid: true},
		{name: "RFC static", backend: "https://origin.example", rfc: true},
		{name: "RFC invalid", backend: "http://[::1", rfc: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
			f.rfcMode = tt.rfc
			m := &metricstest.MockMetrics{}
			f.metrics = m
			original := httptest.NewRequest("GET", "http://client.example/public/item?q=1", nil)
			filtered := original.Clone(context.Background())
			filtered.URL.Path = "/item"
			ctx := &filtertest.Context{
				FRequest: filtered, FOriginalRequest: original,
				FBackendUrl: tt.backend, FOutgoingHost: "virtual.origin.example",
			}
			for _, metric := range []string{"cache.coalesce_error", "cache.reval_error"} {
				source, backend, host := f.requestDispatch(ctx, metric)
				wantSource := original
				if tt.direct {
					wantSource = filtered
					if backend != tt.backend || host != ctx.FOutgoingHost {
						t.Fatalf("direct destination = %q, Host = %q", backend, host)
					}
				} else if backend != "" || host != "" {
					t.Fatalf("pipeline destination = %q, Host = %q", backend, host)
				}
				if source != wantSource {
					t.Fatal("wrong source request")
				}
				m.WithCounters(func(counters map[string]int64) {
					want := int64(0)
					if tt.invalid {
						want = 1
					}
					if counters[metric] != want {
						t.Errorf("%s = %d, want %d", metric, counters[metric], want)
					}
				})
			}
		})
	}
}

func TestCacheFilter_PrepareDispatch_NoMutation(t *testing.T) {
	for _, backend := range []string{"", "https://origin.example:443/ignored?ignored=1"} {
		t.Run(backend, func(t *testing.T) {
			f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
			source := httptest.NewRequest("GET", "http://client.example/filtered%2Fpath?q=a%2Fb", strings.NewReader("payload"))
			source.Header.Set("X-Filtered", "value")
			before := source.Clone(source.Context())
			req, err := f.prepareDispatch(source, backend, "virtual.origin.example")
			if err != nil {
				t.Fatal(err)
			}
			if req == source || req.URL == source.URL || req.RequestURI != "" {
				t.Fatal("dispatch must clone the request and URL and clear RequestURI")
			}
			if req.Method != source.Method || req.Body != source.Body || req.ContentLength != source.ContentLength ||
				req.URL.EscapedPath() != source.URL.EscapedPath() || req.URL.RawQuery != source.URL.RawQuery {
				t.Fatal("dispatch changed method, body, path or query")
			}
			if backend == "" {
				if req.URL.Scheme != "http" || req.URL.Host != f.listenAddr || req.Host != source.Host || req.Header.Get(revalidateHeader) != "1" {
					t.Fatalf("wrong pipeline request: %+v", req)
				}
			} else if req.URL.Scheme != "https" || req.URL.Host != "origin.example:443" ||
				req.Host != "virtual.origin.example" || req.Header.Get(revalidateHeader) != "" {
				t.Fatalf("wrong direct request: %+v", req)
			}
			req.Header.Set("X-Filtered", "changed")
			req.URL.Path = "/changed"
			if source.RequestURI != before.RequestURI || source.Host != before.Host ||
				!reflect.DeepEqual(source.URL, before.URL) || !reflect.DeepEqual(source.Header, before.Header) {
				t.Fatal("dispatch mutated inbound request")
			}
		})
	}
}

func TestCacheFilter_Coalesce_PipelineFallback(t *testing.T) {
	for _, tt := range []struct {
		name    string
		backend string
		rfc     bool
	}{
		{name: "dynamic"},
		{name: "invalid", backend: "http://[::1"},
		{name: "relative", backend: "/not-an-origin"},
		{name: "RFC", backend: "https://must-not-be-contacted.example", rfc: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
			f.rfcMode = tt.rfc
			var calls atomic.Int64
			pipeline := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Host != "client.example" || r.URL.RequestURI() != "/public/item?q=1" ||
					r.Header.Get(revalidateHeader) != "1" || r.Header.Get("X-Original") != "yes" {
					t.Errorf("wrong pipeline replay: %+v", r)
				}
				inner := &filtertest.Context{FRequest: r, FStateBag: make(map[string]any)}
				f.Request(inner)
				if inner.FServed || r.Header.Get(revalidateHeader) != "" {
					t.Error("pipeline replay did not bypass and strip cache lookup marker")
				}
				w.Header().Set("Cache-Control", "public, max-age=60")
				w.Header().Set("X-Response-Filter", "applied")
				io.WriteString(w, "pipeline response")
			}))
			defer pipeline.Close()
			f.listenAddr = strings.TrimPrefix(pipeline.URL, "http://")
			f.fetch = pipeline.Client().Do
			original := httptest.NewRequest("GET", "http://client.example/public/item?q=1", nil)
			original.Header.Set("X-Original", "yes")
			originalURI := original.RequestURI
			filtered := original.Clone(context.Background())
			filtered.URL.Path = "/item"
			filtered.Header.Del("X-Original")
			ctx := &filtertest.Context{
				FRequest: filtered, FOriginalRequest: original, FStateBag: make(map[string]any),
				FBackendUrl: tt.backend, FOutgoingHost: "must-not-be-used.example",
			}
			f.Request(ctx)
			if !ctx.FServed {
				t.Fatal("cold miss was not served through the pipeline")
			}
			defer ctx.FResponse.Body.Close()
			body, err := io.ReadAll(ctx.FResponse.Body)
			if err != nil || string(body) != "pipeline response" || ctx.FResponse.Header.Get("X-Response-Filter") != "applied" {
				t.Fatalf("pipeline response = %q, err = %v", body, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("pipeline calls = %d, want 1", calls.Load())
			}
			if original.RequestURI != originalURI || original.Header.Get(revalidateHeader) != "" ||
				filtered.URL.Path != "/item" || filtered.Header.Get(revalidateHeader) != "" {
				t.Fatal("pipeline fallback mutated inbound request")
			}
		})
	}
}

func TestProxy_Coalesce_PipelineFallback(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		name := "RFC static backend"
		if dynamic {
			name = "force dynamic backend"
		}
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.RequestURI() != "/item?q=1" || r.Header.Get(revalidateHeader) != "" {
					t.Errorf("wrong backend request: %+v", r)
				}
				w.Header().Set("Cache-Control", "no-store")
				io.WriteString(w, "pipeline response")
			}))
			defer backend.Close()
			spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
			t.Cleanup(func() { spec.(*cacheSpec).Close() })
			fr := make(filters.Registry)
			fr.Register(spec)
			fr.Register(builtin.NewModPath())
			fr.Register(builtin.NewSetDynamicBackendUrl())
			fr.Register(builtin.NewSetResponseHeader())
			route := &eskip.Route{
				Id: "pipeline", Path: "/public/item", Backend: backend.URL,
				Filters: []*eskip.Filter{
					{Name: "modPath", Args: []any{"^/public", ""}},
					{Name: spec.Name()},
					{Name: "setResponseHeader", Args: []any{"Cache-Control", "public, max-age=60"}},
				},
			}
			if dynamic {
				route.Backend = ""
				route.BackendType = eskip.DynamicBackend
				route.Filters[1].Args = []any{"5m", "15s", "30s"}
				// The dynamic destination is deliberately selected after cache().
				route.Filters = append(route.Filters, &eskip.Filter{Name: "setDynamicBackendUrl", Args: []any{backend.URL}})
			}
			proxy := proxytest.WithParams(fr, skipperproxy.Params{Flags: skipperproxy.PreserveOriginal}, route)
			defer proxy.Close()
			for _, f := range spec.(*cacheSpec).filters {
				f.listenAddr = strings.TrimPrefix(proxy.URL, "http://")
			}
			for i := range 2 {
				resp, err := proxy.Client().Get(proxy.URL + "/public/item?q=1")
				if err != nil {
					t.Fatal(err)
				}
				body, readErr := io.ReadAll(resp.Body)
				resp.Body.Close()
				wantStatus := cacheStatusMiss
				if i == 1 {
					wantStatus = cacheStatusHit
				}
				if readErr != nil || resp.StatusCode != http.StatusOK || string(body) != "pipeline response" ||
					resp.Header.Get(cacheStatusHeader) != wantStatus || resp.Header.Get("Cache-Control") != "public, max-age=60" {
					t.Fatalf("response %d: status=%d body=%q headers=%v err=%v", i, resp.StatusCode, body, resp.Header, readErr)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("backend calls = %d, want 1", calls.Load())
			}
		})
	}
}

func TestCacheFilter_DispatchParity(t *testing.T) {
	for _, backend := range []string{"", "https://origin.example", "http://[::1"} {
		t.Run(backend, func(t *testing.T) {
			f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
			ctx := &filtertest.Context{
				FOriginalRequest: httptest.NewRequest("GET", "http://client.example/public/item?q=1", nil),
				FRequest:         httptest.NewRequest("GET", "http://client.example/item?q=1", nil),
				FBackendUrl:      backend, FOutgoingHost: "virtual.origin.example",
				FStateBag: make(map[string]any),
			}
			var requests []*http.Request
			f.fetch = func(req *http.Request) (*http.Response, error) {
				requests = append(requests, req.Clone(context.Background()))
				return upstreamResponseCC(http.StatusOK, "body", "public, max-age=60"), nil
			}
			f.coalesce(ctx, "key")
			if !ctx.FServed {
				t.Fatal("coalesce did not serve")
			}
			ctx.FResponse.Body.Close()
			source, destination, host := f.requestDispatch(ctx, "cache.reval_error")
			before := source.Clone(source.Context())
			f.doRevalidate("key", source, destination, host)
			if len(requests) != 2 {
				t.Fatalf("fetches = %d, want 2", len(requests))
			}
			if !reflect.DeepEqual(requests[0], requests[1]) {
				t.Fatalf("cold miss and revalidation dispatch differ:\n%+v\n%+v", requests[0], requests[1])
			}
			if !reflect.DeepEqual(source, before) {
				t.Fatal("dispatch mutated source request")
			}
		})
	}
}

func TestCacheFilter_Coalesce_VaryStar_NotShared(t *testing.T) {
	for _, keyHeaders := range [][]string{nil, {"X-Tenant"}} {
		for _, vary := range [][]string{{"*"}, {"Accept-Language", "*"}, {"Accept-Language, *"}} {
			t.Run(strings.Join(keyHeaders, ",")+"/"+strings.Join(vary, ";"), func(t *testing.T) {
				const n = 5
				f := newTestFilter(t, time.Minute, 15*time.Second, time.Minute)
				f.keyHeaders = keyHeaders
				var joined sync.WaitGroup
				joined.Add(n)
				var calls atomic.Int64
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					joined.Wait()
					w.Header()["Vary"] = vary
					io.WriteString(w, "unique")
				}))
				defer backend.Close()
				f.fetch = backend.Client().Do
				var wg sync.WaitGroup
				for range n {
					wg.Go(func() {
						req := httptest.NewRequest("GET", "/item", nil)
						req.Header.Set("X-Tenant", "same")
						req = req.WithContext(&coalesceBarrierContext{Context: req.Context(), joined: &joined})
						ctx := &filtertest.Context{FRequest: req, FBackendUrl: backend.URL, FStateBag: make(map[string]any)}
						f.Request(ctx)
						if !ctx.FServed {
							t.Error("request was not served")
							return
						}
						io.Copy(io.Discard, ctx.FResponse.Body)
						ctx.FResponse.Body.Close()
					})
				}
				wg.Wait()
				if calls.Load() != n {
					t.Fatalf("backend calls = %d, want %d", calls.Load(), n)
				}
			})
		}
	}
}

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
		body := io.NopCloser(strings.NewReader("leader"))
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

type coalesceLifetimeContext struct {
	filters.FilterContext
	returned          atomic.Bool
	accessAfterReturn atomic.Bool
}

func (c *coalesceLifetimeContext) StateBag() map[string]any {
	if c.returned.Load() {
		c.accessAfterReturn.Store(true)
	}
	return c.FilterContext.StateBag()
}

type coalesceCompletionBody struct {
	io.ReadCloser
	done chan struct{}
}

func (b *coalesceCompletionBody) Close() error {
	defer close(b.done)
	return b.ReadCloser.Close()
}

func TestCacheFilter_Coalesce_CancelledLeaderDoesNotAccessContextAfterReturn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newTestFilter(t, time.Minute, time.Second, time.Minute)
		started := make(chan struct{})
		release := make(chan struct{})
		completed := make(chan struct{})
		f.fetch = func(*http.Request) (*http.Response, error) {
			close(started)
			<-release
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Vary": {"Accept-Language"}},
				Body: &coalesceCompletionBody{
					ReadCloser: io.NopCloser(strings.NewReader("cached after cancellation")),
					done:       completed,
				},
			}, nil
		}
		inner := newCtx(http.MethodGet, "http://origin.example/cancelled", "")
		inner.FRequest.Header.Set("Accept-Language", "en")
		requestContext, cancel := context.WithCancel(inner.Request().Context())
		defer cancel()
		inner.FRequest = inner.Request().WithContext(requestContext)
		ctx := &coalesceLifetimeContext{FilterContext: inner}
		baseKey := cacheKey(inner.RouteId(), inner.Request(), f.keyHeaders)
		returned := make(chan struct{})
		go func() {
			f.Request(ctx)
			ctx.returned.Store(true)
			close(returned)
		}()
		<-started
		cancel()
		<-returned
		if inner.FServed {
			t.Fatal("cancelled request should return without serving")
		}
		// The proxy may reuse the state bag once Request returns.
		clear(inner.FStateBag)
		close(release)
		<-completed
		synctest.Wait()
		if ctx.accessAfterReturn.Load() {
			t.Fatal("singleflight callback accessed the filter context after Request returned")
		}
		key := varyKey(baseKey, inner.Request(), []string{"Accept-Language"})
		entry, err := f.storage.GetSilent(context.Background(), key)
		if err != nil || entry == nil || string(entry.Payload) != "cached after cancellation" {
			t.Fatalf("fetch did not retain the captured base key: entry=%+v err=%v", entry, err)
		}
	})
}

// Regression test for coalesce() passing a server-received *http.Request
// (RequestURI set, relative URL) straight to the real http.Client: that
// combination makes every fetch fail client-side ("Request.RequestURI can't
// be set in client requests"), so coalesce() silently falls through to
// skipper's normal backend path on every call instead of deduplicating
// concurrent cold misses. The other Coalesce* tests in this file stub out
// f.fetch directly, which bypasses net/http's client-side validation
// entirely and can't catch this class of bug.
func TestProxy_Coalesce_RealFetch_ConcurrentColdMiss_SingleBackendCall(t *testing.T) {
	const n = 10
	var joined sync.WaitGroup
	joined.Add(n)
	var backendCallCount int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&backendCallCount, 1)
		joined.Wait()
		if r.Host != "virtual.origin.example" {
			t.Errorf("backend Host = %q, want virtual.origin.example", r.Host)
		}
		if r.URL.RequestURI() != "/concurrent-cold-miss?key=value" {
			t.Errorf("backend request URI = %q", r.URL.RequestURI())
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		fmt.Fprint(w, `{"data":"cold-miss"}`)
	}))
	defer backend.Close()

	spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
	t.Cleanup(func() { spec.(*cacheSpec).Close() })
	fr := make(filters.Registry)
	fr.Register(coalesceBarrierSpec{Spec: spec, joined: &joined})

	route := &eskip.Route{
		Id:          "coalesce-real-fetch-route",
		PathRegexps: []string{".*"},
		Filters: []*eskip.Filter{
			{Name: spec.Name(), Args: []any{"5m", "15s", "30s"}},
		},
		Backend: backend.URL,
	}

	proxy := proxytest.New(fr, route)
	defer proxy.Close()

	client := proxy.Client()

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			rsp, err := client.Get(proxy.URL + "/concurrent-cold-miss?key=value")
			if err != nil {
				t.Errorf("request failed: %v", err)
				return
			}
			defer rsp.Body.Close()
			if rsp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", rsp.StatusCode)
			}
			body, err := io.ReadAll(rsp.Body)
			if err != nil || string(body) != `{"data":"cold-miss"}` {
				t.Errorf("response body = %q, err = %v", body, err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&backendCallCount); got != 1 {
		t.Fatalf("expected exactly 1 real backend call for %d concurrent cold misses on the same key, got %d", n, got)
	}
}
