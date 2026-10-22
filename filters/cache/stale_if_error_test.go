package cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"
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
