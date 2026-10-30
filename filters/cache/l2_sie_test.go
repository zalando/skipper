package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
)

func TestL2StorageWarmingPreservesSIEWindow(t *testing.T) {
	for _, readWarm := range []bool{false, true} {
		for _, age := range []time.Duration{0, 30 * time.Second} {
			for _, warmTTL := range []time.Duration{time.Second, 5 * time.Minute} {
				t.Run(fmt.Sprintf("read=%t/age=%s/warm=%s", readWarm, age, warmTTL), func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						ctx := context.Background()
						client := newStubValkeyClient()
						l1 := NewLRUStorage(1<<20, nil, metrics.Void)
						s := NewL2Storage(client, l1, metrics.Void, warmTTL, valkey.IsValkeyNil)
						entry := &Entry{
							StatusCode: http.StatusOK, Payload: []byte("original"), Header: make(http.Header),
							CreatedAt: time.Now().Add(-age), ResponseTime: time.Now().Add(-age),
							TTL: time.Minute, StaleIfError: 10 * time.Minute,
						}
						if readWarm {
							remote := NewL2Storage(client, NewLRUStorage(1<<20, nil, metrics.Void), metrics.Void, 0, valkey.IsValkeyNil)
							if err := remote.Set(ctx, "key", entry); err != nil {
								t.Fatal(err)
							}
							if _, err := s.Get(ctx, "key"); err != nil {
								t.Fatal(err)
							}
						} else if err := s.Set(ctx, "key", entry); err != nil {
							t.Fatal(err)
						}
						warmed, err := l1.Get(ctx, "key")
						if err != nil || warmed == nil {
							t.Fatalf("no warmed entry: %v", err)
						}
						wantDeadline := time.Now().Add(min(warmTTL, entry.TTL-age))
						if !warmed.L1FreshUntil.Equal(wantDeadline) {
							t.Fatalf("warming deadline=%v, want %v", warmed.L1FreshUntil, wantDeadline)
						}
						preserved := *warmed
						preserved.L1FreshUntil = time.Time{}
						if !reflect.DeepEqual(&preserved, entry) {
							t.Fatalf("warming changed original metadata: got=%+v want=%+v", preserved, entry)
						}
						data, err := client.Get(ctx, "key")
						if err != nil {
							t.Fatal(err)
						}
						var shared Entry
						if err := json.Unmarshal([]byte(data), &shared); err != nil || !shared.L1FreshUntil.IsZero() {
							t.Fatalf("L1 deadline leaked to shared storage: %+v err=%v", shared, err)
						}
						client.broken = true
						time.Sleep(time.Until(wantDeadline))
						candidate, err := s.Get(ctx, "key")
						if candidate == nil || err == nil {
							t.Fatalf("expired warming must return SIE candidate with error, not HIT: candidate=%+v err=%v", candidate, err)
						}
						expiry := entry.CreatedAt.Add(entry.TTL + entry.StaleIfError)
						time.Sleep(time.Until(expiry))
						candidate, err = s.GetSilent(ctx, "key")
						if candidate == nil || err == nil || !candidate.CreatedAt.Equal(entry.CreatedAt) || candidate.TTL != entry.TTL {
							t.Fatalf("original SIE boundary shortened or changed: candidate=%+v err=%v", candidate, err)
						}
						time.Sleep(time.Nanosecond)
						candidate, err = s.Get(ctx, "key")
						if candidate != nil || err == nil {
							t.Fatalf("expired original SIE window extended: candidate=%+v err=%v", candidate, err)
						}
					})
				})
			}
		}
	}
}

func TestCacheFilterL2FailureSIE(t *testing.T) {
	cases := []struct {
		scenario string
		status   int
	}{}
	for _, status := range []int{0, http.StatusServiceUnavailable} {
		for _, scenario := range []string{"error", "expires during fetch", "original window expires during fetch"} {
			cases = append(cases, struct {
				scenario string
				status   int
			}{scenario, status})
		}
	}
	for _, scenario := range []string{"decode error", "miss", "expired", "fresh L2", "origin success", "only-if-cached"} {
		cases = append(cases, struct {
			scenario string
			status   int
		}{scenario, http.StatusServiceUnavailable})
	}
	for _, tt := range cases {
		scenario, status := tt.scenario, tt.status
		t.Run(fmt.Sprintf("%s/status=%d", scenario, status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f, cleanup := newTestFilterWithCleanup(t, time.Minute, time.Second, time.Second, 10*time.Minute)
				defer cleanup()
				client := newStubValkeyClient()
				m := &metricstest.MockMetrics{}
				s := NewL2Storage(client, f.lruStorage, m, time.Second, valkey.IsValkeyNil)
				f.storage = s
				ctx := newCtx(http.MethodGet, "http://cache.test/resource", "")
				key := cacheKey(ctx.RouteId(), ctx.Request(), f.keyHeaders)
				original := &Entry{
					StatusCode: http.StatusOK, Payload: []byte("last-good"),
					Header:    http.Header{"Etag": {`"original"`}, "X-Representation": {"original"}},
					CreatedAt: time.Now(), ResponseTime: time.Now(), TTL: time.Minute,
					StaleIfError: 10 * time.Minute, CorrectedInitialAge: 3 * time.Second,
				}
				if scenario == "original window expires during fetch" {
					original.StaleIfError = time.Minute
				}
				if err := s.Set(context.Background(), key, original); err != nil {
					t.Fatal(err)
				}
				time.Sleep(2 * time.Minute)
				if scenario == "expired" {
					time.Sleep(10 * time.Minute)
				}
				before, err := f.lruStorage.Get(context.Background(), key)
				if err != nil {
					t.Fatal(err)
				}
				if scenario != "expired" && before == nil {
					t.Fatal("warming discarded the original SIE window")
				}
				client.broken = true
				if scenario == "decode error" {
					client.broken = false
					if err := client.SetWithExpire(context.Background(), key, "invalid JSON", time.Minute); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "miss" {
					client.broken = false
					if _, err := client.Del(context.Background(), key); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "fresh L2" {
					client.broken = false
					fresh := *original
					fresh.CreatedAt = time.Now()
					fresh.Payload = []byte("fresh L2")
					// Write around L1 to model a refresh by another process.
					remote := NewL2Storage(client, NewLRUStorage(1<<20, nil, metrics.Void), metrics.Void, 0, valkey.IsValkeyNil)
					if err := remote.Set(context.Background(), key, &fresh); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "only-if-cached" {
					ctx.Request().Header.Set("Cache-Control", "only-if-cached")
				}
				calls := 0
				f.fetch = func(*http.Request) (*http.Response, error) {
					calls++
					if scenario == "expires during fetch" {
						time.Sleep(10 * time.Minute)
					}
					if scenario == "original window expires during fetch" {
						time.Sleep(time.Nanosecond)
					}
					if scenario == "origin success" {
						return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("recovered"))}, nil
					}
					if status == 0 {
						return nil, errors.New("origin transport failure")
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("origin failure"))}, nil
				}
				f.Request(ctx)
				wantCalls := 1
				if scenario == "fresh L2" || scenario == "only-if-cached" {
					wantCalls = 0
				}
				if calls != wantCalls {
					t.Fatalf("origin calls=%d, want %d", calls, wantCalls)
				}
				if scenario == "error" || scenario == "decode error" {
					if !ctx.FServed || ctx.FResponse == nil {
						t.Fatal("eligible local SIE response not served")
					}
					rsp := ctx.FResponse
					body, err := io.ReadAll(rsp.Body)
					rsp.Body.Close()
					if err != nil || rsp.StatusCode != original.StatusCode || string(body) != string(original.Payload) ||
						rsp.Header.Get(cacheStatusHeader) != cacheStatusStale ||
						rsp.Header.Get("ETag") != original.Header.Get("ETag") ||
						rsp.Header.Get("X-Representation") != "original" || rsp.Header.Get("Age") != "123" {
						t.Fatalf("SIE response: status=%d header=%v body=%q err=%v", rsp.StatusCode, rsp.Header, body, err)
					}
					after, err := f.lruStorage.Get(context.Background(), key)
					if err != nil || !reflect.DeepEqual(before, after) {
						t.Fatalf("failed refresh changed last-good: before=%+v after=%+v err=%v", before, after, err)
					}
				} else if ctx.FResponse != nil {
					defer ctx.FResponse.Body.Close()
					body, err := io.ReadAll(ctx.FResponse.Body)
					if err != nil {
						t.Fatal(err)
					}
					switch scenario {
					case "fresh L2":
						if string(body) != "fresh L2" || ctx.FResponse.Header.Get(cacheStatusHeader) != cacheStatusHit {
							t.Fatalf("fresh L2 not preferred: %q %v", body, ctx.FResponse.Header)
						}
					case "origin success":
						if string(body) != "recovered" || ctx.FResponse.Header.Get(cacheStatusHeader) != cacheStatusMiss {
							t.Fatalf("successful origin not preferred: %q %v", body, ctx.FResponse.Header)
						}
					case "only-if-cached":
						if ctx.FResponse.StatusCode != http.StatusGatewayTimeout {
							t.Fatalf("local SIE candidate reported as a hit: %v", ctx.FResponse)
						}
					default:
						if string(body) == "last-good" || ctx.FResponse.Header.Get(cacheStatusHeader) == cacheStatusStale {
							t.Fatalf("ineligible local response served: %q %v", body, ctx.FResponse.Header)
						}
					}
				} else if scenario == "fresh L2" || scenario == "origin success" || scenario == "only-if-cached" {
					t.Fatal("expected response not served")
				}
				if got, _ := m.Counter("cache.l1_hit"); got != 0 {
					t.Fatalf("SIE-only candidate counted as L1 hit: %d", got)
				}
			})
		})
	}
}

func TestCacheFilterL2FailureBackgroundPreservesSIE(t *testing.T) {
	for _, status := range []int{0, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f, cleanup := newTestFilterWithCleanup(t, time.Minute, time.Second, time.Second, 10*time.Minute)
				defer cleanup()
				client := newStubValkeyClient()
				f.storage = NewL2Storage(client, f.lruStorage, metrics.Void, time.Second, valkey.IsValkeyNil)
				req, err := http.NewRequest(http.MethodGet, "http://cache.test/resource", nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Accept-Language", "en")
				base := cacheKey("", req, f.keyHeaders)
				key := varyKey(base, req, []string{"Accept-Language"})
				entry := &Entry{
					StatusCode: http.StatusOK, Header: http.Header{"Vary": {"Accept-Language"}},
					Payload: []byte("last-good"), CreatedAt: time.Now(), TTL: time.Minute, StaleIfError: 10 * time.Minute,
				}
				if err := f.storeEntry(context.Background(), base, key, req, entry); err != nil {
					t.Fatal(err)
				}
				time.Sleep(2 * time.Minute)
				before, err := f.lruStorage.Get(context.Background(), key)
				if err != nil || before == nil {
					t.Fatalf("no local candidate: %v", err)
				}
				client.broken = true
				calls := 0
				f.fetch = func(*http.Request) (*http.Response, error) {
					calls++
					if status == 0 {
						return nil, errors.New("origin transport failure")
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("failed refresh"))}, nil
				}
				for range 2 {
					f.doRevalidateQueued(revalJob{
						key: key, baseKey: base, req: req.Clone(context.Background()),
						identity: snapshotCacheIdentity(req), enqueuedAt: time.Now(),
					})
					after, err := f.lruStorage.Get(context.Background(), key)
					if err != nil || !reflect.DeepEqual(before, after) {
						t.Fatalf("background failure changed last-good: before=%+v after=%+v err=%v", before, after, err)
					}
				}
				if calls != 2 {
					t.Fatalf("background failures reported as fresh hits: calls=%d", calls)
				}
			})
		})
	}
}

func TestCacheFilterL2FailureSIEVary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, cleanup := newTestFilterWithCleanup(t, time.Minute, time.Second, time.Second, 10*time.Minute)
		defer cleanup()
		client := newStubValkeyClient()
		f.storage = NewL2Storage(client, f.lruStorage, metrics.Void, time.Second, valkey.IsValkeyNil)
		for _, language := range []string{"en", "de"} {
			ctx := newCtx(http.MethodGet, "http://cache.test/resource", "")
			ctx.Request().Header.Set("Accept-Language", language)
			base := cacheKey(ctx.RouteId(), ctx.Request(), f.keyHeaders)
			entry := &Entry{
				StatusCode: http.StatusOK, Header: http.Header{"Vary": {"Accept-Language"}},
				Payload: []byte(language), CreatedAt: time.Now(), TTL: time.Minute, StaleIfError: 10 * time.Minute,
			}
			if err := f.storeEntry(context.Background(), base, base, ctx.Request(), entry); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(2 * time.Minute)
		client.broken = true
		f.fetch = func(*http.Request) (*http.Response, error) {
			return nil, errors.New("origin transport failure")
		}
		for _, language := range []string{"en", "de", "fr", "de", "en"} {
			ctx := newCtx(http.MethodGet, "http://cache.test/resource", "")
			ctx.Request().Header.Set("Accept-Language", language)
			f.Request(ctx)
			if language == "fr" {
				if ctx.FServed {
					t.Fatal("absent variant reused another representation")
				}
				continue
			}
			if !ctx.FServed || ctx.FResponse == nil {
				t.Fatalf("%s local variant not served after origin failure", language)
			}
			body, err := io.ReadAll(ctx.FResponse.Body)
			ctx.FResponse.Body.Close()
			if err != nil || string(body) != language || ctx.FResponse.Header.Get(cacheStatusHeader) != cacheStatusStale {
				t.Fatalf("%s fallback: body=%q header=%v err=%v", language, body, ctx.FResponse.Header, err)
			}
		}
	})
}
