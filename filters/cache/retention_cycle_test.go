package cache

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/valkey-io/valkey-go"
)

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
