package skipper

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/cache"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/proxy"
)

func TestCacheNetOptions_BackendTimeoutMapping(t *testing.T) {
	o := &Options{
		TimeoutBackend:               time.Second,
		ResponseHeaderTimeoutBackend: 2 * time.Second,
		TLSHandshakeTimeoutBackend:   3 * time.Second,
		ExpectContinueTimeoutBackend: 4 * time.Second,
		CloseIdleConnsPeriod:         5 * time.Second,
	}
	opts := o.cacheNetOptions()
	if opts.Timeout != 0 {
		t.Fatal("TCP dial timeout must not become a whole-request/body timeout")
	}
	tr := opts.Transport
	if tr == nil || tr.DialContext == nil || tr.ResponseHeaderTimeout != 2*time.Second ||
		tr.TLSHandshakeTimeout != 3*time.Second || tr.ExpectContinueTimeout != 4*time.Second ||
		tr.IdleConnTimeout != 5*time.Second {
		t.Fatalf("backend transport settings not propagated: %+v", tr)
	}
	defaults := (&Options{}).cacheNetOptions().Transport
	if defaults.ResponseHeaderTimeout != proxy.DefaultResponseHeaderTimeout ||
		defaults.ExpectContinueTimeout != proxy.DefaultExpectContinueTimeout {
		t.Fatalf("proxy timeout defaults not preserved: %+v", defaults)
	}
	negative := (&Options{ResponseHeaderTimeoutBackend: -time.Second}).cacheNetOptions().Transport
	if negative.ResponseHeaderTimeout != -time.Second {
		t.Fatal("negative timeout should retain transport's disabled semantics")
	}
}

type timeoutTestL2 struct {
	mu    sync.Mutex
	value string
	sets  int
}

func (s *timeoutTestL2) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.HasPrefix(key, "vary:") {
		return "", errors.New("test cache miss")
	}
	return s.value, nil
}

func (s *timeoutTestL2) SetWithExpire(_ context.Context, _ string, value string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value = value
	s.sets++
	return nil
}

func (*timeoutTestL2) Del(context.Context, string) (int64, error) { return 0, nil }

func TestCacheBackendHeaderTimeout_PreservesStaleAndRecovers(t *testing.T) {
	var recovering atomic.Bool
	var calls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !recovering.Load() {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
			}
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		io.WriteString(w, "recovered")
	}))
	defer backend.Close()
	stale := &cache.Entry{
		StatusCode:           http.StatusOK,
		Header:               http.Header{"Etag": {`"last-good"`}},
		Payload:              []byte("last-good"),
		CreatedAt:            time.Now().Add(-11 * time.Minute),
		TTL:                  5 * time.Minute,
		StaleWhileRevalidate: 5 * time.Minute,
		StaleIfError:         24 * time.Hour,
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	store := &timeoutTestL2{value: string(data)}
	o := &Options{ResponseHeaderTimeoutBackend: 50 * time.Millisecond, TimeoutBackend: time.Second}
	spec := cache.NewCacheFilter(cache.Options{
		MaxBytes: 1 << 20, NetOpts: o.cacheNetOptions(), L2Client: store,
		IsNoL2Err: func(error) bool { return true },
	})
	defer spec.(io.Closer).Close()
	filter, err := spec.CreateFilter([]any{"5m", "15s", "5m", "24h", "Authorization"})
	if err != nil {
		t.Fatal(err)
	}
	request := func() *filtertest.Context {
		req := httptest.NewRequest(http.MethodGet, "http://client.example/contentful", nil)
		req.Header.Set("Authorization", "Bearer fixture")
		ctx := &filtertest.Context{
			FRequest: req, FStateBag: make(map[string]any),
			FRouteId: "timeout", FBackendUrl: backend.URL,
		}
		filter.Request(ctx)
		return ctx
	}
	for range 2 {
		start := time.Now()
		ctx := request()
		elapsed := time.Since(start)
		if elapsed >= 500*time.Millisecond {
			t.Fatalf("configured 50ms response-header timeout ignored: %v", elapsed)
		}
		if !ctx.FServed || ctx.FResponse == nil || ctx.FResponse.Header.Get("X-Cache-Status") != "STALE" {
			t.Fatal("header timeout must serve eligible stale response")
		}
		body, _ := io.ReadAll(ctx.FResponse.Body)
		if string(body) != "last-good" {
			t.Fatalf("header timeout renewed cache with delayed origin body: %q", body)
		}
		store.mu.Lock()
		unchanged := store.value == string(data) && store.sets == 0
		store.mu.Unlock()
		if !unchanged {
			t.Fatal("timeout changed last-good payload or timestamps")
		}
	}
	recovering.Store(true)
	ctx := request()
	if !ctx.FServed || ctx.FResponse.Header.Get("X-Cache-Status") != "MISS" {
		t.Fatal("successful recovery must replace stale response")
	}
	body, _ := io.ReadAll(ctx.FResponse.Body)
	if string(body) != "recovered" {
		t.Fatalf("unexpected recovery response: %q", body)
	}
	store.mu.Lock()
	var fresh cache.Entry
	err = json.Unmarshal([]byte(store.value), &fresh)
	sets := store.sets
	store.mu.Unlock()
	if err != nil || sets != 1 || !fresh.CreatedAt.After(stale.CreatedAt) {
		t.Fatalf("successful recovery not stored once with fresh timestamps: %+v, sets=%d, err=%v", fresh, sets, err)
	}
	ctx = request()
	if !ctx.FServed || ctx.FResponse.Header.Get("X-Cache-Status") != "HIT" || calls.Load() != 3 {
		t.Fatalf("recovered object should be a hit without origin retry; calls=%d", calls.Load())
	}
}
