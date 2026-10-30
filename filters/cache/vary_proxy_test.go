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
	skipperproxy "github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
)

type varyRewriteSpec struct{}

func TestSnapshotCacheIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/identity", strings.NewReader("body")).WithContext(ctx)
	req.Header["Accept-Language"] = []string{"en", "de"}
	identity := snapshotCacheIdentity(req)
	req.Header["Accept-Language"][0] = "changed"
	req.Header.Set("X-Later", "mutation")
	cancel()
	if identity.Header.Get("Accept-Language") != "en" || identity.Header.Get("X-Later") != "" {
		t.Fatal("cache identity retained mutable request headers")
	}
	if identity.Body != nil || identity.Context().Done() != nil {
		t.Fatal("cache identity retained the live request body or context")
	}
}

func BenchmarkSnapshotCacheIdentity(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/identity", nil)
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("Authorization", "fixture")
	state := make(map[string]any)
	b.ReportAllocs()
	for b.Loop() {
		state[stateBagIdentity] = snapshotCacheIdentity(req)
	}
}

func (varyRewriteSpec) Name() string { return "varyRewrite" }
func (varyRewriteSpec) CreateFilter([]any) (filters.Filter, error) {
	return varyRewriteFilter{}, nil
}

type varyRewriteFilter struct{}

func (varyRewriteFilter) Request(ctx filters.FilterContext) {
	if ctx.Request().Header.Get("X-Rewrite") == "yes" {
		ctx.Request().Header.Set("Accept-Language", "de")
	}
}
func (varyRewriteFilter) Response(filters.FilterContext) {}

type varyIdentityBarrierSpec struct {
	filters.Spec
	joined *sync.WaitGroup
	cold   *atomic.Bool
}

func (s varyIdentityBarrierSpec) CreateFilter(args []any) (filters.Filter, error) {
	f, err := s.Spec.CreateFilter(args)
	if err != nil {
		return nil, err
	}
	return varyIdentityBarrierFilter{Filter: f, joined: s.joined, cold: s.cold}, nil
}

type varyIdentityBarrierFilter struct {
	filters.Filter
	joined *sync.WaitGroup
	cold   *atomic.Bool
}

func (f varyIdentityBarrierFilter) Request(ctx filters.FilterContext) {
	if f.cold.Load() && ctx.Request().Header.Get(revalidateHeader) == "" {
		req := ctx.Request().WithContext(&coalesceBarrierContext{Context: ctx.Request().Context(), joined: f.joined})
		ctx = dispatchContext{FilterContext: ctx, req: req}
	}
	f.Filter.Request(ctx)
}

func TestProxy_CacheVary_FilteredIdentityLoopback(t *testing.T) {
	for _, mode := range []string{"rfc", "force-dynamic"} {
		t.Run(mode, func(t *testing.T) {
			const n = 10
			var joined sync.WaitGroup
			joined.Add(n)
			var cold atomic.Bool
			cold.Store(true)
			var generation atomic.Int64
			var calls atomic.Int64
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if cold.Load() {
					joined.Wait()
				}
				if r.URL.Path != "/identity" || r.Header.Get(revalidateHeader) != "" {
					t.Errorf("loopback replay changed routing or leaked its marker: %s %v", r.URL.Path, r.Header)
				}
				w.Header().Set("Vary", "Accept-Language")
				w.Header().Set("Cache-Control", "public, max-age=300, stale-while-revalidate=300")
				fmt.Fprintf(w, "%s-%d", r.Header.Get("Accept-Language"), generation.Load())
			}))
			defer backend.Close()
			spec := NewCacheFilter(Options{MaxBytes: 1 << 20}).(*cacheSpec)
			defer spec.Close()
			registry := make(filters.Registry)
			registry.Register(varyRewriteSpec{})
			registry.Register(varyIdentityBarrierSpec{Spec: spec, joined: &joined, cold: &cold})
			registry.Register(builtin.NewSetDynamicBackendUrl())
			args := []any{}
			route := &eskip.Route{
				Id: "identity", Path: "/identity", Backend: backend.URL,
				Filters: []*eskip.Filter{{Name: "varyRewrite"}, {Name: spec.Name(), Args: args}},
			}
			if mode == "force-dynamic" {
				args = []any{"5m", "15s", "5m"}
				route.Filters[1].Args = args
				route.Backend = ""
				route.BackendType = eskip.DynamicBackend
				route.Filters = append(route.Filters, &eskip.Filter{Name: "setDynamicBackendUrl", Args: []any{backend.URL}})
			}
			p := proxytest.WithParams(registry, skipperproxy.Params{Flags: skipperproxy.PreserveOriginal}, route)
			defer p.Close()
			created, err := spec.CreateFilter(args)
			if err != nil {
				t.Fatal(err)
			}
			f := created.(*cacheFilter)
			f.listenAddr = strings.TrimPrefix(p.URL, "http://")
			jobs := make(chan revalJob, n)
			f.revalJobs = jobs
			client := p.Client()
			client.Timeout = 10 * time.Second
			request := func(rewrite bool, wantGeneration int64, status string) {
				t.Helper()
				req, err := http.NewRequest(http.MethodGet, p.URL+"/identity", nil)
				if err != nil {
					t.Error(err)
					return
				}
				req.Header.Set("Accept-Language", "en")
				language := "en"
				if rewrite {
					req.Header.Set("X-Rewrite", "yes")
					language = "de"
				}
				rsp, err := client.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				body, err := io.ReadAll(rsp.Body)
				rsp.Body.Close()
				if err != nil || rsp.StatusCode != http.StatusOK || string(body) != fmt.Sprintf("%s-%d", language, wantGeneration) {
					t.Errorf("rewrite=%t status=%d body=%q err=%v", rewrite, rsp.StatusCode, body, err)
				}
				if status != "" && rsp.Header.Get(cacheStatusHeader) != status {
					t.Errorf("rewrite=%t cache=%q want=%q", rewrite, rsp.Header.Get(cacheStatusHeader), status)
				}
			}
			var wg sync.WaitGroup
			for i := range n {
				wg.Go(func() { request(i%2 != 0, 0, "") })
			}
			wg.Wait()
			cold.Store(false)
			beforeWarm := calls.Load()
			request(false, 0, cacheStatusHit)
			request(true, 0, cacheStatusHit)
			if calls.Load() != beforeWarm {
				t.Error("warm variants must not fetch origin")
			}
			keyReq, _ := http.NewRequest(http.MethodGet, p.URL+"/identity", nil)
			keyReq.URL.Scheme = ""
			baseKey := cacheKey("identity", keyReq, f.keyHeaders)
			for _, language := range []string{"en", "de"} {
				keyReq.Header.Set("Accept-Language", language)
				key := varyKey(baseKey, keyReq, []string{"Accept-Language"})
				entry, err := f.storage.GetSilent(context.Background(), key)
				if err != nil || entry == nil {
					t.Fatalf("missing %s variant before refresh: %v", language, err)
				}
				entry.CreatedAt = time.Now().Add(-6 * time.Minute)
				if err := f.storage.Set(context.Background(), key, entry); err != nil {
					t.Fatal(err)
				}
			}
			request(false, 0, cacheStatusStale)
			request(true, 0, cacheStatusStale)
			if len(jobs) != 2 {
				t.Fatalf("want two queued variant refreshes, got %d", len(jobs))
			}
			generation.Store(1)
			refreshed := make(chan struct{})
			go func() {
				defer close(refreshed)
				for range 2 {
					f.doRevalidateQueued(<-jobs)
				}
			}()
			<-refreshed
			afterRefresh := calls.Load()
			if afterRefresh != beforeWarm+2 {
				t.Fatalf("want two origin refreshes, before=%d after=%d", beforeWarm, afterRefresh)
			}
			request(false, 1, cacheStatusHit)
			request(true, 1, cacheStatusHit)
			if calls.Load() != afterRefresh {
				t.Error("refreshed variants must not fetch origin")
			}
		})
	}
}

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
	for _, tt := range []struct {
		keyAuthorization bool
		multipleFields   bool
	}{{}, {keyAuthorization: true}, {multipleFields: true}, {keyAuthorization: true, multipleFields: true}} {
		t.Run(fmt.Sprintf("authorization=%t/multipleFields=%t", tt.keyAuthorization, tt.multipleFields), func(t *testing.T) {
			const n = 12
			var joined sync.WaitGroup
			joined.Add(n)
			var calls atomic.Int64
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				joined.Wait()
				if tt.multipleFields {
					w.Header().Add("Vary", "X-Shared")
				}
				w.Header().Add("Vary", "Accept-Language")
				w.Header().Set("Cache-Control", "public, max-age=300")
				fmt.Fprint(w, r.Header.Get("Accept-Language"))
			}))
			defer backend.Close()
			spec := NewCacheFilter(Options{MaxBytes: 1 << 20})
			t.Cleanup(func() { spec.(*cacheSpec).Close() })
			registry := make(filters.Registry)
			registry.Register(coalesceBarrierSpec{Spec: spec, joined: &joined})
			args := []any{"5m", "15s", "5m", "24h"}
			if tt.keyAuthorization {
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
				if tt.keyAuthorization {
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
