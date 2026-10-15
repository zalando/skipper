package cache

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestCoalesceCrossUserConfusion is the reporter's proof-of-concept for
// GHSA-9hwf-c47m-j4pg, kept as a regression test.
//
// Two concurrent requests to the same URL carry different Authorization
// credentials and land on the same cache key (Authorization not in
// keyHeaders). The upstream response is `Cache-Control: private, no-store`.
// Each requester must get its own fetch and its own body — never the other's.
func TestCoalesceCrossUserConfusion(t *testing.T) {
	// Pure RFC mode (0 args) => keyHeaders empty => Authorization NOT in the key,
	// and private/no-store are honoured on the STORE path.
	f := newTestFilterRFC(t, 0, 0, 0)

	mc := &missCounting{Storage: f.storage}
	mc.wg.Add(2)
	f.storage = mc

	var fetchCount atomic.Int64
	fetchStarted := make(chan struct{})
	releaseAll := make(chan struct{})
	var fetchStartedOnce sync.Once

	f.fetch = func(req *http.Request) (*http.Response, error) {
		fetchCount.Add(1)
		fetchStartedOnce.Do(func() { close(fetchStarted) })
		mc.wg.Wait() // both goroutines have observed a miss and will join this flight
		<-releaseAll // held open so the waiter is guaranteed to coalesce
		// Per-user, explicitly non-cacheable response echoing the CALLER'S credential.
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":  {"text/plain"},
				"Cache-Control": {"private, no-store"},
			},
			Body: io.NopCloser(strings.NewReader("PRIVATE_DATA_FOR=" + req.Header.Get("Authorization"))),
		}, nil
	}

	const url = "https://api.example.com/account/me"

	ctxA := newCtx("GET", url, "user-A-SECRET-TOKEN")
	ctxB := newCtx("GET", url, "user-B-attacker-token")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); f.Request(ctxA) }()
	go func() { defer wg.Done(); f.Request(ctxB) }()

	<-fetchStarted
	close(releaseAll)
	wg.Wait()

	if !ctxA.FServed || !ctxB.FServed {
		t.Fatalf("expected both to be served: A=%v B=%v", ctxA.FServed, ctxB.FServed)
	}

	bodyA, _ := io.ReadAll(ctxA.FResponse.Body)
	bodyB, _ := io.ReadAll(ctxB.FResponse.Body)

	t.Logf("upstream fetches = %d (2 == fixed: each user got its own fetch)", fetchCount.Load())
	t.Logf("user-A body = %q", string(bodyA))
	t.Logf("user-B body = %q", string(bodyB))

	if got := fetchCount.Load(); got != 2 {
		t.Fatalf("expected 2 independent upstream fetches (private/no-store must not be shared across different credentials), got %d", got)
	}
	if !strings.Contains(string(bodyA), "user-A-SECRET-TOKEN") {
		t.Fatalf("user-A must receive its own response, got %q", bodyA)
	}
	if !strings.Contains(string(bodyB), "user-B-attacker-token") {
		t.Fatalf("user-B must receive its own response, not user-A's — got %q", bodyB)
	}
	if strings.Contains(string(bodyB), "user-A-SECRET-TOKEN") {
		t.Fatalf("CROSS-USER CACHE CONFUSION: user-B received user-A's private (no-store) response: %q", bodyB)
	}
}
