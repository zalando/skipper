package cache

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/zalando/skipper/metrics/metricstest"
	skpnet "github.com/zalando/skipper/net"
)

type bypassCountingBody struct {
	io.ReadCloser
	closes atomic.Int64
}

func (b *bypassCountingBody) Close() error {
	b.closes.Add(1)
	return b.ReadCloser.Close()
}

func TestCacheFilter_BypassBodyCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &metricstest.MockMetrics{}
		leaderStarted := make(chan struct{})
		releaseLeader := make(chan struct{})
		releaseFollower := make(chan struct{})
		var fetches atomic.Int64
		transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
			client, server := net.Pipe()
			go func() {
				defer server.Close()
				req, err := http.ReadRequest(bufio.NewReader(server))
				if err != nil {
					t.Error(err)
					return
				}
				req.Body.Close()
				if req.Header.Get("Authorization") == "leader" {
					close(leaderStarted)
					<-releaseLeader
					io.WriteString(server, "HTTP/1.1 200 OK\r\nCache-Control: private, no-store\r\nContent-Length: 6\r\nConnection: close\r\n\r\nleader")
				} else {
					io.WriteString(server, "HTTP/1.1 200 OK\r\nCache-Control: private, no-store\r\nContent-Length: 8\r\nConnection: close\r\n\r\n")
					<-releaseFollower
					io.WriteString(server, "follower")
				}
			}()
			return client, nil
		}}
		spec := NewCacheFilter(Options{
			MaxBytes: 1 << 20, ListenAddr: "example.org", Metrics: m,
			NetOpts: skpnet.Options{Transport: transport},
		})
		defer spec.(*cacheSpec).Close()
		fi, err := spec.CreateFilter(nil)
		if err != nil {
			t.Fatal(err)
		}
		f := fi.(*cacheFilter)
		fetch := f.fetch
		var followerBody *bypassCountingBody
		f.fetch = func(req *http.Request) (*http.Response, error) {
			fetches.Add(1)
			rsp, err := fetch(req)
			if err == nil && req.Header.Get("Authorization") == "follower" {
				followerBody = &bypassCountingBody{ReadCloser: rsp.Body}
				rsp.Body = followerBody
			}
			return rsp, err
		}
		leader := newCtx(http.MethodGet, "http://example.org/resource", "leader")
		follower := newCtx(http.MethodGet, "http://example.org/resource", "follower")
		ctx, cancel := context.WithCancel(follower.FRequest.Context())
		defer cancel()
		follower.FRequest = follower.FRequest.WithContext(ctx)
		leaderDone := make(chan struct{})
		go func() { defer close(leaderDone); f.Request(leader) }()
		<-leaderStarted
		followerDone := make(chan struct{})
		go func() { defer close(followerDone); f.Request(follower) }()
		synctest.Wait()
		close(releaseLeader)
		<-leaderDone
		synctest.Wait()
		if fetches.Load() != 2 || followerBody == nil {
			t.Fatal("follower did not independently fetch response headers")
		}
		cancel()
		synctest.Wait()
		select {
		case <-followerDone:
		default:
			t.Error("cancelled follower Request remains blocked on the upstream body")
		}
		close(releaseFollower)
		<-followerDone
		if followerBody.closes.Load() != 1 {
			t.Errorf("original follower body closed %d times, want 1", followerBody.closes.Load())
		}
		if follower.FServed {
			follower.FResponse.Body.Close()
			t.Error("cancelled follower was served")
		}
		if !leader.FServed {
			t.Fatal("shared leader response was not preserved")
		}
		body, err := io.ReadAll(leader.FResponse.Body)
		leader.FResponse.Body.Close()
		if err != nil || string(body) != "leader" {
			t.Errorf("leader body = %q, %v", body, err)
		}
		m.WithCounters(func(counters map[string]int64) {
			if got := counters["cache.miss"]; got != 1 {
				t.Errorf("success misses = %d, want leader only", got)
			}
		})
		if got, _ := m.CacheResultCount("miss", "2xx"); got != 1 {
			t.Errorf("labelled misses = %d, want leader only", got)
		}
	})
}

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
	testCacheFilterBypassBody(t, http.MethodGet, nil, nil, http.StatusOK)
}

func TestCacheFilter_BypassBodyReadError(t *testing.T) {
	testCacheFilterBypassBody(t, http.MethodGet, io.ErrUnexpectedEOF, nil, http.StatusOK)
}

func TestCacheFilter_BypassFetchError(t *testing.T) {
	testCacheFilterBypassBody(t, http.MethodGet, nil, errors.New("origin connection failed"), http.StatusOK)
}

func TestCacheFilter_BypassMissCodeClass(t *testing.T) {
	testCacheFilterBypassBody(t, http.MethodGet, nil, nil, http.StatusNotFound)
}

// Hold the leader until the follower has joined singleflight, then inspect both
// responses only after Request has returned and closed the original streams.
func testCacheFilterBypassBody(t *testing.T, method string, readErr, fetchErr error, followerStatus int) {
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
			status := followerStatus
			if req.Header.Get("Authorization") == "leader" {
				close(started)
				<-release
				body = leaderBody
				status = http.StatusOK
			} else if fetchErr != nil {
				return nil, fetchErr
			}
			return &http.Response{
				StatusCode: status,
				Header:     http.Header{"Cache-Control": {"private, no-store"}},
				Body:       body,
			}, nil
		}

		leader := newCtx(method, "http://example.org/resource", "leader")
		follower := newCtx(method, "http://example.org/resource", "follower")
		span := mocktracer.New().StartSpan("follower").(*mocktracer.MockSpan)
		defer span.Finish()
		follower.FRequest = follower.FRequest.WithContext(opentracing.ContextWithSpan(follower.FRequest.Context(), span))
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
			want := 1
			if name == "follower" && fetchErr != nil {
				want = 0
			}
			if body.closes != want {
				t.Errorf("%s original stream closed %d times, want %d", name, body.closes, want)
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
		if readErr != nil || fetchErr != nil {
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
			if follower.FResponse.StatusCode != followerStatus || span.Tag("cache_status") != cacheStatusMiss {
				t.Errorf("follower status/span = %d/%v", follower.FResponse.StatusCode, span.Tag("cache_status"))
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
			for _, class := range []string{"2xx", "4xx"} {
				wantClass := int64(0)
				if class == "2xx" {
					wantClass = 1
				}
				if wantErrors == 0 && class == statusCodeClass(followerStatus) {
					wantClass++
				}
				if got, _ := m.CacheResultCount("miss", class); got != wantClass {
					t.Errorf("labelled %s miss count = %d, want %d", class, got, wantClass)
				}
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
