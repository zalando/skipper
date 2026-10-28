package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/zalando/skipper/metrics"
)

type purposeTestContextKey struct{}

type purposeCall struct {
	method, purpose, key string
	marker               any
}

type purposeStorage struct {
	Storage
	mu    sync.Mutex
	calls []purposeCall
}

func (s *purposeStorage) record(ctx context.Context, method, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, purposeCall{method, l2PurposeFromContext(ctx), key, ctx.Value(purposeTestContextKey{})})
}

func (s *purposeStorage) Get(ctx context.Context, key string) (*Entry, error) {
	s.record(ctx, "get", key)
	return s.Storage.Get(ctx, key)
}

func (s *purposeStorage) GetSilent(ctx context.Context, key string) (*Entry, error) {
	s.record(ctx, "silent", key)
	return s.Storage.GetSilent(ctx, key)
}

func (s *purposeStorage) Set(ctx context.Context, key string, entry *Entry) error {
	s.record(ctx, "set", key)
	return s.Storage.Set(ctx, key, entry)
}

func (s *purposeStorage) Delete(ctx context.Context, key string) error {
	s.record(ctx, "delete", key)
	return s.Storage.Delete(ctx, key)
}

func (s *purposeStorage) take() []purposeCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	calls := s.calls
	s.calls = nil
	return calls
}

func assertPurposeCalls(t *testing.T, got, want []purposeCall) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("storage calls:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestCacheFilter_StoragePurposeLookupAndWrite(t *testing.T) {
	f := newTestFilter(t, time.Minute, time.Second, time.Minute)
	s := &purposeStorage{Storage: f.storage}
	f.storage = s
	ctx := newCtx(http.MethodGet, "http://example.test/resource", "")
	ctx.FRequest = ctx.FRequest.WithContext(context.WithValue(ctx.FRequest.Context(), purposeTestContextKey{}, "request"))
	ctx.FRequest.Header.Set("Accept-Language", "en")
	base := cacheKey(ctx.FRouteId, ctx.FRequest, f.keyHeaders)
	entry := &Entry{
		StatusCode: http.StatusOK, Header: http.Header{"Vary": {"Accept-Language"}},
		Payload: []byte("body"), CreatedAt: time.Now(), TTL: time.Minute,
	}
	variant := varyKey(base, ctx.Request(), []string{"Accept-Language"})
	if err := f.storeEntry(ctx.Request().Context(), base, base, ctx.Request(), entry); err != nil {
		t.Fatal(err)
	}
	assertPurposeCalls(t, s.take(), []purposeCall{
		{"set", "metadata", "vary:" + base, "request"},
		{"set", "response", variant, "request"},
	})
	f.Request(ctx)
	if !ctx.FServed || ctx.FResponse.Header.Get(cacheStatusHeader) != cacheStatusHit {
		t.Fatal("expected ordinary cache hit")
	}
	body, err := io.ReadAll(ctx.FResponse.Body)
	if err != nil || string(body) != "body" {
		t.Fatalf("body = %q, err = %v", body, err)
	}
	assertPurposeCalls(t, s.take(), []purposeCall{
		{"silent", "metadata", "vary:" + base, "request"},
		{"get", "response", variant, "request"},
	})

	cached := newCtx(http.MethodGet, "http://example.test/resource", "")
	cached.Request().Header.Set("Accept-Language", "en")
	cached.Request().Header.Set("Cache-Control", "only-if-cached")
	f.Request(cached)
	if !cached.FServed || cached.FResponse.Header.Get(cacheStatusHeader) != cacheStatusHit {
		t.Fatal("only-if-cached hit must serve the cached response")
	}
	assertPurposeCalls(t, s.take(), []purposeCall{
		{"silent", "metadata", "vary:" + base, nil},
		{"get", "response", variant, nil},
	})

	miss := newCtx(http.MethodGet, "http://example.test/missing", "")
	miss.FRequest.Header.Set("Cache-Control", "only-if-cached")
	missKey := cacheKey(miss.FRouteId, miss.Request(), f.keyHeaders)
	f.Request(miss)
	if !miss.FServed || miss.FResponse.StatusCode != http.StatusGatewayTimeout {
		t.Fatal("only-if-cached miss must return 504")
	}
	assertPurposeCalls(t, s.take(), []purposeCall{
		{"silent", "metadata", "vary:" + missKey, nil},
		{"get", "response", missKey, nil},
	})

	ordinary := newCtx(http.MethodGet, "http://example.test/missing", "")
	f.fetch = func(*http.Request) (*http.Response, error) {
		return upstreamResponse(http.StatusOK, "origin"), nil
	}
	f.Request(ordinary)
	if !ordinary.FServed || ordinary.FResponse.Header.Get(cacheStatusHeader) != cacheStatusMiss {
		t.Fatal("ordinary miss must serve the origin response")
	}
	body, err = io.ReadAll(ordinary.FResponse.Body)
	if err != nil || string(body) != "origin" {
		t.Fatalf("origin body = %q, err = %v", body, err)
	}
	assertPurposeCalls(t, s.take(), []purposeCall{
		{"silent", "metadata", "vary:" + missKey, nil},
		{"get", "response", missKey, nil},
		{"set", "response", missKey, nil},
	})
}

func TestCacheFilter_StoragePurposeHeadAndInvalidation(t *testing.T) {
	f := newTestFilter(t, time.Minute, time.Second, time.Minute)
	s := &purposeStorage{Storage: f.storage}
	f.storage = s
	head := newCtx(http.MethodHead, "http://example.test/resource", "")
	head.FRequest = head.Request().WithContext(context.WithValue(head.Request().Context(), purposeTestContextKey{}, "head"))
	key := cacheKey(head.FRouteId, head.Request(), f.keyHeaders)
	entry := &Entry{StatusCode: http.StatusOK, Header: make(http.Header), Payload: []byte("original"), CreatedAt: time.Now(), TTL: time.Minute}
	if err := s.Storage.Set(context.Background(), key, entry); err != nil {
		t.Fatal(err)
	}
	head.FStateBag[stateBagKey] = key
	head.FResponse = upstreamResponse(http.StatusOK, "")
	head.FResponse.Header.Set("ETag", `"updated"`)
	f.Response(head)
	assertPurposeCalls(t, s.take(), []purposeCall{
		{"silent", "response", key, "head"},
		{"set", "response", key, "head"},
	})
	stored, err := s.Storage.GetSilent(context.Background(), key)
	if err != nil || stored == nil || stored.ETag != `"updated"` || string(stored.Payload) != "original" {
		t.Fatalf("freshened entry = %#v, err = %v", stored, err)
	}

	post := newCtx(http.MethodPost, "http://example.test/resource", "")
	post.FRequest = post.Request().WithContext(context.WithValue(post.Request().Context(), purposeTestContextKey{}, "post"))
	post.FStateBag[stateBagKey] = key
	post.FResponse = upstreamResponse(http.StatusOK, "")
	post.FResponse.Header.Set("Location", "/other")
	post.FResponse.Header.Set("Content-Location", "/third")
	location := cacheKeyForURL(post.FRouteId, post.Request(), "/other", f.keyHeaders)
	contentLocation := cacheKeyForURL(post.FRouteId, post.Request(), "/third", f.keyHeaders)
	f.Response(post)
	assertPurposeCalls(t, s.take(), []purposeCall{
		{"delete", "response", key, "post"},
		{"delete", "metadata", "vary:" + key, "post"},
		{"delete", "response", location, "post"},
		{"delete", "metadata", "vary:" + location, "post"},
		{"delete", "response", contentLocation, "post"},
		{"delete", "metadata", "vary:" + contentLocation, "post"},
	})
}

func TestCacheFilter_StoragePurposeBackgroundReads(t *testing.T) {
	f := newTestFilter(t, time.Minute, time.Second, time.Minute, time.Minute)
	s := &purposeStorage{Storage: f.storage}
	f.storage = s
	entry := &Entry{StatusCode: http.StatusOK, Header: http.Header{"Etag": {`"v1"`}}, ETag: `"v1"`, Payload: []byte("body"), CreatedAt: time.Now(), TTL: time.Minute}
	if err := s.Storage.Set(context.Background(), "key", entry); err != nil {
		t.Fatal(err)
	}
	if f.staleIfErrorEntry("key") == nil {
		t.Fatal("expected stale-if-error candidate")
	}
	assertPurposeCalls(t, s.take(), []purposeCall{{"silent", "response", "key", nil}})

	req, err := http.NewRequest(http.MethodGet, "http://example.test/resource", nil)
	if err != nil {
		t.Fatal(err)
	}
	f.fetch = func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("If-None-Match"); got != `"v1"` {
			t.Errorf("If-None-Match = %q", got)
		}
		return &http.Response{StatusCode: http.StatusNotModified, Header: make(http.Header), Body: http.NoBody}, nil
	}
	f.revalidate("key", "key", req, "http://example.test", "", time.Now().Add(time.Second), "route")
	assertPurposeCalls(t, s.take(), []purposeCall{
		{"silent", "response", "key", nil},
		{"silent", "response", "key", nil},
		{"silent", "response", "key", nil},
		{"set", "response", "key", nil},
	})
	stored, err := s.Storage.GetSilent(context.Background(), "key")
	if err != nil || stored == nil || string(stored.Payload) != "body" {
		t.Fatalf("revalidated entry = %#v, err = %v", stored, err)
	}

	f.revalidate("key", "key", req, "http://example.test", "", time.Now().Add(-time.Second), "route")
	assertPurposeCalls(t, s.take(), []purposeCall{{"silent", "response", "key", nil}})
}

func TestCacheFilter_StoragePurposeConcurrentContexts(t *testing.T) {
	f := newTestFilter(t, time.Minute, time.Second, time.Minute)
	s := &purposeStorage{Storage: f.storage}
	f.storage = s
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i)
			ctx := context.WithValue(context.Background(), purposeTestContextKey{}, key)
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.test/"+key, nil)
			entry := &Entry{Header: http.Header{"Vary": {"Accept-Language"}}, CreatedAt: time.Now(), TTL: time.Minute}
			if err := f.storeEntry(ctx, key, key, req, entry); err != nil {
				t.Errorf("store %s: %v", key, err)
			}
			if l2PurposeFromContext(ctx) != "unknown" {
				t.Errorf("store mutated parent context for %s", key)
			}
		}(i)
	}
	wg.Wait()
	calls := s.take()
	if len(calls) != 64 {
		t.Fatalf("got %d calls, want 64", len(calls))
	}
	for _, call := range calls {
		marker, ok := call.marker.(string)
		req, _ := http.NewRequest(http.MethodGet, "http://example.test/"+marker, nil)
		variant := varyKey(marker, req, []string{"Accept-Language"})
		if !ok || (call.purpose == "metadata" && call.key != "vary:"+marker) ||
			(call.purpose == "response" && call.key != variant) ||
			(call.purpose != "metadata" && call.purpose != "response") {
			t.Errorf("wrong concurrent context: %#v", call)
		}
	}
}

func TestCacheFilter_AbsentVarySentinelHitAttribution(t *testing.T) {
	for _, tc := range []struct {
		name           string
		l1Hit          bool
		wantOperations []l2Observation
		wantL1, wantL2 int64
	}{
		{
			name: "l2 response hit",
			wantOperations: []l2Observation{
				{"get", "metadata", "miss"},
				{"get", "response", "success"},
			},
			wantL2: 1,
		},
		{
			name:           "fresh l1 response hit",
			l1Hit:          true,
			wantOperations: []l2Observation{{"get", "metadata", "miss"}},
			wantL1:         1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTestFilter(t, time.Minute, time.Second, time.Minute)
			ctx := newCtx(http.MethodGet, "http://example.test/attribution", "")
			key := cacheKey(ctx.FRouteId, ctx.Request(), f.keyHeaders)
			entry := &Entry{StatusCode: http.StatusOK, Header: make(http.Header), Payload: []byte("cached"), CreatedAt: time.Now(), TTL: time.Minute}
			client := newStubValkeyClient()
			l1 := NewLRUStorage(1<<20, nil, metrics.Default)
			m := &observableL2Metrics{}
			f.storage = NewL2Storage(client, l1, m, time.Minute, valkey.IsValkeyNil)
			if tc.l1Hit {
				if err := l1.Set(context.Background(), key, entry); err != nil {
					t.Fatal(err)
				}
			} else {
				encoded, err := json.Marshal(entry)
				if err != nil {
					t.Fatal(err)
				}
				if err := client.SetWithExpire(context.Background(), key, string(encoded), time.Minute); err != nil {
					t.Fatal(err)
				}
			}

			f.Request(ctx)
			if !ctx.FServed || ctx.FResponse.StatusCode != http.StatusOK || ctx.FResponse.Header.Get(cacheStatusHeader) != cacheStatusHit {
				t.Fatal("expected cached response")
			}
			body, err := io.ReadAll(ctx.FResponse.Body)
			if err != nil || string(body) != "cached" {
				t.Fatalf("cached body = %q, err = %v", body, err)
			}
			if !reflect.DeepEqual(m.operations, tc.wantOperations) || len(m.processing) != 0 {
				t.Fatalf("operations = %v, processing = %v; want %v", m.operations, m.processing, tc.wantOperations)
			}
			for _, counter := range []struct {
				name string
				want int64
			}{
				{"cache.l1_hit", tc.wantL1},
				{"cache.l2_hit", tc.wantL2},
				{"cache.l2_miss", 0},
				{"cache.l2_get_error", 0},
			} {
				if got, _ := m.Counter(counter.name); got != counter.want {
					t.Errorf("%s = %d, want %d", counter.name, got, counter.want)
				}
			}
			for _, result := range []struct {
				name, class string
				want        int64
			}{
				{"l1_hit", "2xx", tc.wantL1},
				{"l2_hit", "2xx", tc.wantL2},
				{"l2_miss", "n/a", 0},
				{"l2_get_error", "n/a", 0},
			} {
				if got, _ := m.CacheResultCount(result.name, result.class); got != result.want {
					t.Errorf("%s/%s = %d, want %d", result.name, result.class, got, result.want)
				}
			}
			m.WithMeasures(func(measures map[string][]time.Duration) {
				if got := len(measures["cache.l2_get_duration"]); got != len(tc.wantOperations) {
					t.Errorf("L2 get duration count = %d, want %d", got, len(tc.wantOperations))
				}
			})
		})
	}
}

func TestL2PurposeContextInheritanceAndConcurrency(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	parent, cancel := context.WithDeadline(context.WithValue(context.Background(), purposeTestContextKey{}, "shared"), deadline)
	defer cancel()

	const workers = 64
	children := make(chan context.Context, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			purpose, want := l2PurposeMetadata, "metadata"
			if i%2 == 0 {
				purpose, want = l2PurposeResponse, "response"
			}
			child := withL2Purpose(parent, purpose)
			if got := l2PurposeFromContext(child); got != want {
				t.Errorf("purpose = %q, want %q", got, want)
			}
			if got := child.Value(purposeTestContextKey{}); got != "shared" {
				t.Errorf("parent value = %v, want shared", got)
			}
			if got, ok := child.Deadline(); !ok || !got.Equal(deadline) {
				t.Errorf("deadline = %v, %v; want %v", got, ok, deadline)
			}
			if child.Err() != nil {
				t.Errorf("unexpected cancellation: %v", child.Err())
			}
			children <- child
		}(i)
	}
	wg.Wait()
	close(children)
	if l2PurposeFromContext(parent) != "unknown" {
		t.Error("purpose leaked into shared parent")
	}
	cancel()
	for child := range children {
		select {
		case <-child.Done():
			if child.Err() != context.Canceled {
				t.Errorf("cancellation error = %v, want context.Canceled", child.Err())
			}
		default:
			t.Error("child did not inherit cancellation")
		}
	}
	expired, expiredCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer expiredCancel()
	child := withL2Purpose(expired, l2PurposeResponse)
	if child.Err() != context.DeadlineExceeded {
		t.Errorf("expired deadline error = %v, want context.DeadlineExceeded", child.Err())
	}
}
