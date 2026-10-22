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
