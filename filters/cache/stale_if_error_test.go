package cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

type sieTimeoutReader struct{}

func (sieTimeoutReader) Read([]byte) (int, error) { return 0, context.DeadlineExceeded }

func seedSIEStale(t *testing.T, f *cacheFilter, age time.Duration, status int) string {
	t.Helper()
	req := newCtx(http.MethodGet, "http://origin.example/contentful", "Bearer same")
	key := cacheKey(req.RouteId(), req.Request(), f.keyHeaders)
	entry := &Entry{
		StatusCode:           status,
		Header:               http.Header{"Etag": {`"original"`}},
		Payload:              []byte("last-good"),
		CreatedAt:            time.Now().Add(-age),
		TTL:                  5 * time.Minute,
		StaleWhileRevalidate: 5 * time.Minute,
		StaleIfError:         24 * time.Hour,
	}

	if err := f.storage.Set(context.Background(), key, entry); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestCacheFilter_SIE_WindowExpiresDuringFetch(t *testing.T) {
	for _, transport := range []bool{false, true} {
		t.Run(fmt.Sprintf("transport=%t", transport), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newTestFilter(t, 5*time.Minute, 15*time.Second, 5*time.Minute, 24*time.Hour)
				f.keyHeaders = []string{"Authorization"}
				seedSIEStale(t, f, 24*time.Hour+5*time.Minute-time.Second, http.StatusOK)
				f.fetch = func(*http.Request) (*http.Response, error) {
					time.Sleep(2 * time.Second)
					if transport {
						return nil, context.DeadlineExceeded
					}
					return &http.Response{
						StatusCode: http.StatusServiceUnavailable,
						Header:     http.Header{},
						Body:       io.NopCloser(strings.NewReader("unavailable")),
					}, nil
				}
				ctx := newCtx(http.MethodGet, "http://origin.example/contentful", "Bearer same")
				f.Request(ctx)
				if transport {
					if ctx.FServed {
						t.Fatal("stale response served beyond SIE window")
					}
				} else if !ctx.FServed || ctx.FResponse.StatusCode != http.StatusServiceUnavailable {
					t.Fatal("want 503 after SIE window expires during fetch")
				}
			})
		})
	}
}

func TestCacheFilter_SIE_FailureClassesPreserveStoredResponse(t *testing.T) {
	for _, failure := range []struct {
		name      string
		status    int
		err       error
		bodyError bool
	}{
		{name: "500", status: 500},
		{name: "502", status: 502},
		{name: "503", status: 503},
		{name: "504", status: 504},
		{name: "network", err: errors.New("connection reset by peer")},
		{name: "timeout", err: context.DeadlineExceeded},
		{name: "body timeout", status: 200, bodyError: true},
	} {
		t.Run(failure.name, func(t *testing.T) {
			for _, background := range []bool{false, true} {
				t.Run(fmt.Sprintf("background=%t", background), func(t *testing.T) {
					f := newTestFilter(t, 5*time.Minute, 15*time.Second, 5*time.Minute, 24*time.Hour)
					f.keyHeaders = []string{"Authorization"}
					key := seedSIEStale(t, f, 11*time.Minute, http.StatusOK)
					before, _ := f.storage.Get(context.Background(), key)
					fetches := 0
					f.fetch = func(*http.Request) (*http.Response, error) {
						fetches++
						if failure.err != nil {
							return nil, failure.err
						}
						if failure.bodyError {
							return &http.Response{
								StatusCode: failure.status,
								Header:     http.Header{},
								Body:       io.NopCloser(sieTimeoutReader{}),
							}, nil
						}
						return &http.Response{
							StatusCode: failure.status,
							Header:     http.Header{},
							Body:       io.NopCloser(strings.NewReader("unavailable")),
						}, nil
					}
					for range 2 {
						ctx := newCtx(http.MethodGet, "http://origin.example/contentful", "Bearer same")
						if background {
							f.doRevalidate(key, ctx.Request(), "http://origin.example", "")
						} else {
							f.Request(ctx)
							if !ctx.FServed || ctx.FResponse == nil || ctx.FResponse.StatusCode != http.StatusOK {
								t.Fatal("eligible stale response not served")
							}
							body, _ := io.ReadAll(ctx.FResponse.Body)
							if string(body) != "last-good" || ctx.FResponse.Header.Get(cacheStatusHeader) != cacheStatusStale {
								t.Fatalf("unexpected stale response: %q, %v", body, ctx.FResponse.Header)
							}
						}
						after, err := f.storage.Get(context.Background(), key)
						if err != nil || !reflect.DeepEqual(before, after) {
							t.Fatalf("stored response changed: before=%+v after=%+v err=%v", before, after, err)
						}
					}
					if fetches != 2 {
						t.Fatalf("want two upstream attempts, got %d", fetches)
					}
				})
			}
		})
	}
}

func TestCacheFilter_SIE_EligibilityAndFailureBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name        string
		age         time.Duration
		storedCode  int
		failureCode int
		disabled    bool
		transport   bool
	}{
		{name: "expired", age: 25 * time.Hour, storedCode: 200, failureCode: 503},
		{name: "expired transport", age: 25 * time.Hour, storedCode: 200, transport: true},
		{name: "disabled", age: 11 * time.Minute, storedCode: 200, failureCode: 503, disabled: true},
		{name: "cached error", age: 11 * time.Minute, storedCode: 503, failureCode: 503},
		{name: "501 excluded", age: 11 * time.Minute, storedCode: 200, failureCode: 501},
		{name: "505 excluded", age: 11 * time.Minute, storedCode: 200, failureCode: 505},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFilter(t, 5*time.Minute, 15*time.Second, 5*time.Minute, 24*time.Hour)
			f.keyHeaders = []string{"Authorization"}
			key := seedSIEStale(t, f, tt.age, tt.storedCode)
			if tt.disabled {
				f.staleIfError = 0
			}
			f.fetch = func(*http.Request) (*http.Response, error) {
				if tt.transport {
					return nil, errors.New("origin down")
				}
				return &http.Response{
					StatusCode: tt.failureCode,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader("unavailable")),
				}, nil
			}
			ctx := newCtx(http.MethodGet, "http://origin.example/contentful", "Bearer same")
			f.Request(ctx)
			if tt.transport {
				if ctx.FServed {
					t.Fatal("ineligible stale response served on transport error")
				}
				return
			}
			if !ctx.FServed || ctx.FResponse.StatusCode != tt.failureCode {
				t.Fatal("want original upstream failure")
			}
			stored, err := f.storage.Get(context.Background(), key)
			if err != nil || stored == nil || stored.StatusCode != tt.failureCode || stored.TTL != 15*time.Second {
				t.Fatalf("normal negative caching changed: %+v, %v", stored, err)
			}
		})
	}
}

func TestCacheFilter_QueuedRefresh(t *testing.T) {
	for _, failure := range []string{"none", "503", "timeout"} {
		t.Run(failure, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				spec := NewCacheFilter(Options{
					MaxBytes: 1 << 20, L2Client: newStubValkeyClient(),
					L1TTL: time.Second, IsNoL2Err: valkey.IsValkeyNil,
				}).(*cacheSpec)
				defer spec.Close()
				filter, err := spec.CreateFilter([]any{"4s", "2s", "4s", "12s", "Authorization"})
				if err != nil {
					t.Fatal(err)
				}
				f := filter.(*cacheFilter)
				jobs := make(chan revalJob, 8)
				f.revalJobs = jobs
				calls := 0
				f.fetch = func(req *http.Request) (*http.Response, error) {
					calls++
					if req.Header.Get("Authorization") != "fixture" || req.Header.Get("Accept-Language") != "en" {
						t.Fatalf("request identity lost: %+v", req.Header)
					}
					if calls == 2 && failure == "timeout" {
						return nil, context.DeadlineExceeded
					}
					status, body := http.StatusOK, "B"
					if calls == 1 {
						body = "A"
					} else if calls == 2 && failure == "503" {
						status, body = http.StatusServiceUnavailable, "failure"
					}
					return &http.Response{
						StatusCode: status, Header: http.Header{"Vary": {"Accept-Language"}},
						Body: io.NopCloser(strings.NewReader(body)),
					}, nil
				}
				request := func(wantBody, wantStatus string) {
					t.Helper()
					ctx := newCtx(http.MethodGet, "http://origin.example/contentful", "fixture")
					ctx.Request().Header.Set("Accept-Language", "en")
					f.Request(ctx)
					if !ctx.FServed {
						t.Fatal("not served")
					}
					body, _ := io.ReadAll(ctx.FResponse.Body)
					if string(body) != wantBody || ctx.FResponse.Header.Get(cacheStatusHeader) != wantStatus {
						t.Fatalf("body=%s cache status=%s", body, ctx.FResponse.Header.Get(cacheStatusHeader))
					}
				}
				request("A", cacheStatusMiss)
				time.Sleep(4*time.Second + time.Millisecond)
				request("A", cacheStatusStale)
				request("A", cacheStatusStale)
				if len(jobs) != 2 {
					t.Fatalf("queued jobs=%d", len(jobs))
				}
				first, second := <-jobs, <-jobs
				if first.key != second.key || first.key == first.baseKey {
					t.Fatalf("expected same isolated Vary variant: %+v %+v", first, second)
				}
				f.doRevalidateQueued(first)
				if failure != "none" {
					// Failure preserves A; the already queued second job must retry.
					request("A", cacheStatusStale)
				} else {
					request("B", cacheStatusHit)
				}
				f.doRevalidateQueued(second)
				request("B", cacheStatusHit)
				for len(jobs) > 0 {
					f.doRevalidateQueued(<-jobs)
				}
				wantCalls := 2
				if failure != "none" {
					wantCalls = 3
				}
				if calls != wantCalls {
					t.Fatalf("origin calls=%d, want %d", calls, wantCalls)
				}
			})
		})
	}
}

func TestCacheFilter_SIE_RetentionAfterProcessAndVolatileL2Restart(t *testing.T) {
	for _, resetL2 := range []bool{false, true} {
		name := "retained L2 survives process restart"
		if resetL2 {
			name = "volatile L2 reset after process restart loses seed"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				l2 := newStubValkeyClient()
				newFilter := func() (*cacheFilter, func()) {
					spec := NewCacheFilter(Options{
						MaxBytes: 1 << 20, L2Client: l2,
						L1TTL: time.Second, IsNoL2Err: valkey.IsValkeyNil,
					})
					filter, err := spec.CreateFilter([]any{"3s", "1s", "3s", "24s", "Authorization"})
					if err != nil {
						t.Fatal(err)
					}
					return filter.(*cacheFilter), func() { spec.(*cacheSpec).Close() }
				}
				f, closeSeedProcess := newFilter()
				f.fetch = func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Vary": {"Accept-Language"}},
						Body:       io.NopCloser(strings.NewReader("A")),
					}, nil
				}
				newRequest := func() *http.Request {
					req, _ := http.NewRequest(http.MethodGet, "http://origin.example/retention", nil)
					req.Header.Set("Authorization", "Bearer fixture")
					req.Header.Set("Accept-Language", "en")
					return req
				}
				seed := newCtx(http.MethodGet, "http://origin.example/retention", "Bearer fixture")
				seed.FRequest = newRequest()
				f.Request(seed)
				if !seed.FServed {
					t.Fatal("seed not served")
				}
				baseKey := cacheKey(seed.RouteId(), seed.Request(), f.keyHeaders)
				key := varyKey(baseKey, seed.Request(), []string{"Accept-Language"})
				before, err := f.storage.GetSilent(context.Background(), key)
				if err != nil || before == nil || l2.lastExpire != 27*time.Second {
					t.Fatalf("wrong initial retention: %+v err=%v TTL=%v", before, err, l2.lastExpire)
				}
				closeSeedProcess()
				if resetL2 {
					l2.mu.Lock()
					clear(l2.data)
					l2.mu.Unlock()
				}
				f, closeProcess := newFilter()
				defer closeProcess()
				time.Sleep(6*time.Second + time.Millisecond)
				f.fetch = func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Header:     http.Header{"Vary": {"Accept-Language"}},
						Body:       io.NopCloser(strings.NewReader("failure")),
					}, nil
				}
				ctx := newCtx(http.MethodGet, "http://origin.example/retention", "Bearer fixture")
				ctx.FRequest = newRequest()
				f.Request(ctx)
				wantCode, wantCache := http.StatusOK, cacheStatusStale
				if resetL2 {
					wantCode, wantCache = http.StatusInternalServerError, cacheStatusMiss
				}
				if !ctx.FServed || ctx.FResponse.StatusCode != wantCode || ctx.FResponse.Header.Get(cacheStatusHeader) != wantCache {
					t.Fatalf("resetL2=%t response=%+v", resetL2, ctx.FResponse)
				}
				if !resetL2 {
					after, err := f.storage.GetSilent(context.Background(), key)
					if err != nil || after == nil || !after.CreatedAt.Equal(before.CreatedAt) || string(after.Payload) != "A" {
						t.Fatalf("retained entry mutated: %+v, %v", after, err)
					}
				}
			})
		})
	}
}

func TestProxy_CacheLoopbackSIEPreservesVariant(t *testing.T) {
	const status = http.StatusServiceUnavailable
	for _, background := range []bool{false, true} {
		t.Run(fmt.Sprintf("background=%t", background), func(t *testing.T) {
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
