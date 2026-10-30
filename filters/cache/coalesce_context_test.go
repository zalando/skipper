package cache

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/zalando/skipper/filters"
)

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
