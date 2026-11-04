package cache

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/zalando/skipper/metrics/metricstest"
)

type bypassResponseBody struct {
	reader io.Reader
	err    error
	closes int
}

func (b *bypassResponseBody) Read(p []byte) (int, error) {
	if b.closes != 0 {
		return 0, errors.New("read after response body closed")
	}
	n, err := b.reader.Read(p)
	if err == io.EOF && b.err != nil {
		err = b.err
	}
	return n, err
}

func (b *bypassResponseBody) Close() error {
	b.closes++
	return nil
}

func TestCacheFilter_BypassBodyOwnership(t *testing.T) {
	testCacheFilterBypassBody(t, http.MethodGet, nil)
}

func TestCacheFilter_BypassBodyReadError(t *testing.T) {
	testCacheFilterBypassBody(t, http.MethodGet, io.ErrUnexpectedEOF)
}

// Hold the leader until the follower has joined singleflight, then inspect both
// responses only after Request has returned and closed the original streams.
func testCacheFilterBypassBody(t *testing.T, method string, readErr error) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		m := &metricstest.MockMetrics{}
		spec := NewCacheFilter(Options{MaxBytes: 1 << 20, Metrics: m})
		defer spec.(*cacheSpec).Close()
		fi, err := spec.CreateFilter(nil)
		if err != nil {
			t.Fatal(err)
		}
		f := fi.(*cacheFilter)
		started := make(chan struct{})
		release := make(chan struct{})
		var fetches atomic.Int64
		leaderBody := &bypassResponseBody{reader: strings.NewReader("leader payload")}
		followerBody := &bypassResponseBody{reader: strings.NewReader("follower payload"), err: readErr}
		f.fetch = func(req *http.Request) (*http.Response, error) {
			fetches.Add(1)
			body := followerBody
			if req.Header.Get("Authorization") == "leader" {
				close(started)
				<-release
				body = leaderBody
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Cache-Control": {"private, no-store"}},
				Body:       body,
			}, nil
		}

		leader := newCtx(method, "http://example.org/resource", "leader")
		follower := newCtx(method, "http://example.org/resource", "follower")
		leaderDone := make(chan struct{})
		go func() {
			defer close(leaderDone)
			f.Request(leader)
		}()
		<-started
		followerDone := make(chan struct{})
		go func() {
			defer close(followerDone)
			f.Request(follower)
		}()
		synctest.Wait()
		close(release)
		<-leaderDone
		<-followerDone

		if got := fetches.Load(); got != 2 {
			t.Fatalf("expected leader and independent follower fetches, got %d", got)
		}
		for name, body := range map[string]*bypassResponseBody{"leader": leaderBody, "follower": followerBody} {
			if body.closes != 1 {
				t.Errorf("%s original stream closed %d times, want 1", name, body.closes)
			}
		}
		if !leader.FServed {
			t.Fatal("leader response was not served")
		}
		b, err := io.ReadAll(leader.FResponse.Body)
		leader.FResponse.Body.Close()
		want := "leader payload"
		if method == http.MethodHead {
			want = ""
		}
		if err != nil || string(b) != want {
			t.Errorf("leader body = %q, %v; want %q", b, err, want)
		}

		wantErrors := int64(0)
		if readErr != nil {
			wantErrors = 1
			if follower.FServed {
				follower.FResponse.Body.Close()
				t.Error("body-read failure was served as a successful response")
			}
		} else {
			if !follower.FServed {
				t.Fatal("independent follower response was not served")
			}
			b, err := io.ReadAll(follower.FResponse.Body)
			follower.FResponse.Body.Close()
			want := "follower payload"
			if method == http.MethodHead {
				want = ""
				if follower.FResponse.Body != http.NoBody || follower.FResponse.ContentLength != 0 {
					t.Error("HEAD response did not omit body and content length")
				}
			}
			if err != nil || string(b) != want {
				t.Errorf("independent follower body = %q, %v; want %q", b, err, want)
			}
			if got := follower.FResponse.Header.Get(cacheStatusHeader); got != cacheStatusMiss {
				t.Errorf("independent follower cache status = %q, want %q", got, cacheStatusMiss)
			}
		}
		m.WithCounters(func(counters map[string]int64) {
			if got := counters["cache.coalesce_bypass"]; got != 1 {
				t.Errorf("bypass count = %d, want 1", got)
			}
			if got := counters["cache.coalesce_bypass_error"]; got != wantErrors {
				t.Errorf("bypass error count = %d, want %d", got, wantErrors)
			}
		})
		if got, _ := m.CacheResultCount("coalesce_bypass_error", "n/a"); got != wantErrors {
			t.Errorf("labelled bypass error count = %d, want %d", got, wantErrors)
		}
		if got, _ := m.CacheResultCount("coalesce_bypass", "n/a"); got != 1 {
			t.Errorf("labelled bypass count = %d, want 1", got)
		}
		assertMisses := func() {
			t.Helper()
			want := 2 - wantErrors
			m.WithCounters(func(counters map[string]int64) {
				if got := counters["cache.miss"]; got != want {
					t.Errorf("miss count = %d, want %d", got, want)
				}
			})
			if got, _ := m.CacheResultCount("miss", "2xx"); got != want {
				t.Errorf("labelled miss count = %d, want %d", got, want)
			}
		}
		assertMisses()
		f.Response(leader)
		if follower.FServed {
			f.Response(follower)
		}
		assertMisses()
	})
}
