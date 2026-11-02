package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/AlexanderYastrebov/noleak"
)

func newTestOpenIDConfigCache() *openIDConfigCache {
	return &openIDConfigCache{configs: make(map[string]*openIDConfig)}
}

func TestOpenIDConfigCacheOptIn(t *testing.T) {
	stop := StartOpenIDConfigCacheCleaner()
	defer stop()
	var requests atomic.Int32
	var unavailable atomic.Bool
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, `{"issuer":"discovery-%d"}`, n)
	}))
	defer issuer.Close()

	for i := 1; i <= 2; i++ {
		cfg, err := getOpenIDConfig(issuer.URL, TokenintrospectionOptions{})
		if err != nil || cfg.Issuer != fmt.Sprintf("discovery-%d", i) {
			t.Fatalf("default discovery did not fetch each time: %v, %v", cfg, err)
		}
	}
	openIDConfigs.mu.RLock()
	_, cached := openIDConfigs.configs[issuer.URL]
	openIDConfigs.mu.RUnlock()
	if cached {
		t.Fatal("default discovery populated the cache")
	}

	enabled := TokenintrospectionOptions{EnableOpenIDConfigCache: true}
	want, err := getOpenIDConfig(issuer.URL, enabled)
	if err != nil || want.Issuer != "discovery-3" {
		t.Fatalf("enabled discovery did not fetch: %v, %v", want, err)
	}
	var disabled *openIDConfig
	var disabledErr error
	done := make(chan struct{})
	openIDConfigs.mu.Lock()
	go func() {
		disabled, disabledErr = getOpenIDConfig(issuer.URL, TokenintrospectionOptions{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		openIDConfigs.mu.Unlock()
		<-done
		t.Fatal("disabled discovery waited for the global cache lock")
	}
	retained := openIDConfigs.configs[issuer.URL]
	openIDConfigs.mu.Unlock()
	if disabledErr != nil || disabled == want || disabled.Issuer != "discovery-4" {
		t.Fatalf("disabled discovery reused a cached entry: %v, %v", disabled, disabledErr)
	}
	if retained != want {
		t.Fatal("disabled discovery changed the enabled caller's cached entry")
	}

	unavailable.Store(true)
	if _, err := getOpenIDConfig(issuer.URL, TokenintrospectionOptions{}); err == nil {
		t.Fatal("disabled discovery used a cached entry during an outage")
	}
	if cfg, err := getOpenIDConfig(issuer.URL, enabled); err != nil || cfg != want {
		t.Fatalf("enabled discovery lost its cached entry: %v, %v", cfg, err)
	}
	if got := requests.Load(); got != 5 {
		t.Fatalf("got %d discovery requests, want 5", got)
	}
}

func TestOpenIDConfigCacheHitAndClear(t *testing.T) {
	c := newTestOpenIDConfigCache()
	var requests atomic.Int32
	var version atomic.Int32
	version.Store(1)
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprintf(w, `{"issuer":%q,"jwks_uri":"/keys/%d"}`, r.URL.Path, version.Load())
	}))
	defer issuer.Close()

	// Issuer paths and trailing slashes are distinct, unnormalized cache keys.
	issuers := []string{issuer.URL, issuer.URL + "/tenant", issuer.URL + "/tenant/"}
	for _, u := range issuers {
		cfg, err := c.get(u, TokenintrospectionOptions{})
		if err != nil || cfg.JwksURI != "/keys/1" || cfg.Issuer != u[len(issuer.URL):]+TokenIntrospectionConfigPath {
			t.Fatalf("discovery for %q: %v, %v", u, cfg, err)
		}
	}
	version.Store(2)
	for _, u := range issuers {
		cfg, err := c.get(u, TokenintrospectionOptions{})
		if err != nil || cfg.JwksURI != "/keys/1" {
			t.Fatalf("cached discovery: %v, %v", cfg, err)
		}
	}
	if got := requests.Load(); got != int32(len(issuers)) {
		t.Fatalf("cache hits made HTTP requests: %d", got)
	}
	c.clear()
	if len(c.configs) != 0 {
		t.Fatal("whole-map clear retained entries")
	}
	for _, u := range issuers {
		cfg, err := c.get(u, TokenintrospectionOptions{})
		if err != nil || cfg.JwksURI != "/keys/2" {
			t.Fatalf("rediscovery: %v, %v", cfg, err)
		}
	}
	issuer.Close()
	if _, err := c.get(issuers[0], TokenintrospectionOptions{}); err != nil {
		t.Fatalf("warm discovery contacted unavailable issuer: %v", err)
	}
	c.clear()
	if _, err := c.get(issuers[0], TokenintrospectionOptions{}); err == nil {
		t.Fatal("cold discovery used a stale backup after clear")
	}
}

func TestOpenIDConfigCacheFailures(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
		stall  bool
	}{
		{name: "unauthorized", status: 401},
		{name: "not found", status: 404},
		{name: "internal error", status: 500},
		{name: "unavailable", status: 503},
		{name: "malformed", status: 200, body: `not json`},
		{name: "truncated", status: 200, body: `{"issuer":`},
		{name: "header timeout", stall: true},
		{name: "body timeout", status: 200, body: `{"issuer":`, stall: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestOpenIDConfigCache()
			var requests atomic.Int32
			var recovered atomic.Bool
			issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if recovered.Load() {
					fmt.Fprint(w, `{"issuer":"recovered"}`)
					return
				}
				if tt.status != 0 {
					w.WriteHeader(tt.status)
					fmt.Fprint(w, tt.body)
					w.(http.Flusher).Flush()
				}
				if tt.stall {
					<-r.Context().Done()
				}
			}))
			defer issuer.Close()
			for range 2 {
				if _, err := c.get(issuer.URL, TokenintrospectionOptions{Timeout: 50 * time.Millisecond}); err == nil {
					t.Fatal("expected discovery error")
				}
				if len(c.configs) != 0 {
					t.Fatal("failed discovery was cached")
				}
			}
			recovered.Store(true)
			cfg, err := c.get(issuer.URL, TokenintrospectionOptions{Timeout: time.Second})
			if err != nil || cfg.Issuer != "recovered" || requests.Load() != 3 {
				t.Fatalf("discovery did not recover: %v, %v, requests=%d", cfg, err, requests.Load())
			}
		})
	}
	t.Run("invalid URL", func(t *testing.T) {
		c := newTestOpenIDConfigCache()
		if _, err := c.get("://invalid", TokenintrospectionOptions{}); err == nil || len(c.configs) != 0 {
			t.Fatal("invalid URL must fail without caching")
		}
	})
}

func TestOpenIDConfigCacheConcurrentFill(t *testing.T) {
	c := newTestOpenIDConfigCache()
	var requests atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		fmt.Fprint(w, `{"issuer":"shared"}`)
	}))
	defer issuer.Close()

	const callers = 32
	start := make(chan struct{})
	results := make(chan *openIDConfig, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			<-start
			cfg, err := c.get(issuer.URL, TokenintrospectionOptions{Timeout: 5 * time.Second})
			if err != nil {
				t.Error(err)
			}
			results <- cfg
		})
	}
	close(start)
	<-entered
	if c.mu.TryRLock() {
		c.mu.RUnlock()
		t.Error("discovery request did not hold the global write lock")
	}
	close(release)
	wg.Wait()
	close(results)
	var first *openIDConfig
	for cfg := range results {
		if first == nil {
			first = cfg
		}
		if cfg != first || cfg == nil {
			t.Fatal("concurrent callers did not share the successful result")
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("concurrent first fill made %d requests", requests.Load())
	}
}

func TestOpenIDConfigCacheRecheckAfterMiss(t *testing.T) {
	c := newTestOpenIDConfigCache()
	var requests atomic.Int32
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{"issuer":"unexpected fetch"}`)
	}))
	defer issuer.Close()

	// Model the initial read-locked miss before entering get's miss helper.
	c.mu.RLock()
	_, hit := c.configs[issuer.URL]
	c.mu.RUnlock()
	if hit {
		t.Fatal("expected an initial discovery miss")
	}

	var cfg *openIDConfig
	var err error
	started := make(chan struct{})
	done := make(chan struct{})
	c.mu.Lock()
	go func() {
		close(started)
		cfg, err = c.load(issuer.URL, TokenintrospectionOptions{Timeout: time.Second})
		close(done)
	}()
	<-started
	// Insert before load can acquire the write lock, regardless of scheduling.
	want := &openIDConfig{Issuer: "intervening fill"}
	c.configs[issuer.URL] = want
	c.mu.Unlock()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("discovery miss helper did not finish")
	}
	if err != nil {
		t.Fatalf("discovery miss helper failed: %v", err)
	}
	if cfg != want {
		t.Error("write-locked recheck did not reuse the intervening discovery result")
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("write-locked recheck made %d HTTP requests, want zero", got)
	}
}

func TestOpenIDConfigCacheClearDuringDecode(t *testing.T) {
	c := newTestOpenIDConfigCache()
	stop := c.startCleaner()
	defer stop()
	entered := make(chan struct{})
	release := make(chan struct{})
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"issuer":`)
		w.(http.Flusher).Flush()
		close(entered)
		<-release
		fmt.Fprint(w, `"decoded"}`)
	}))
	defer issuer.Close()
	fetched := make(chan struct{})
	go func() {
		defer close(fetched)
		if _, err := c.get(issuer.URL, TokenintrospectionOptions{Timeout: 5 * time.Second}); err != nil {
			t.Error(err)
		}
	}()
	<-entered
	if c.mu.TryRLock() {
		c.mu.RUnlock()
		t.Error("decode did not hold the global write lock")
	}
	cleared := make(chan struct{})
	go func() {
		c.clear()
		close(cleared)
	}()
	// Cleaner teardown must not need the discovery lock when no tick is due.
	stop()
	close(release)
	<-fetched
	<-cleared
	if len(c.configs) != 0 {
		t.Fatal("clear did not remove the in-flight discovery result")
	}
}

func TestOpenIDConfigCacheCleanerShutdownDuringMiss(t *testing.T) {
	c := newTestOpenIDConfigCache()
	entered := make(chan struct{})
	release := make(chan struct{})
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		fmt.Fprint(w, `{}`)
	}))
	defer issuer.Close()
	fetched := make(chan struct{})
	go func() {
		defer close(fetched)
		if _, err := c.get(issuer.URL, TokenintrospectionOptions{Timeout: 5 * time.Second}); err != nil {
			t.Error(err)
		}
	}()
	<-entered

	// Keep real network I/O outside the fake clock's bubble.
	synctest.Test(t, func(t *testing.T) {
		stop := c.startCleaner()
		defer stop()
		synctest.Wait()
		time.Sleep(time.Hour)
		stopped := make(chan struct{})
		go func() {
			stop()
			close(stopped)
		}()
		close(release)
		<-fetched
		<-stopped
		select {
		case <-c.done:
		default:
			t.Fatal("shutdown did not join the cleaner after an in-flight miss")
		}
	})
}

func TestOpenIDConfigCacheCleaner(t *testing.T) {
	noleak.Check(t)
	synctest.Test(t, func(t *testing.T) {
		c := newTestOpenIDConfigCache()
		put := func(key string) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.configs[key] = &openIDConfig{}
		}
		size := func() int {
			c.mu.RLock()
			defer c.mu.RUnlock()
			return len(c.configs)
		}
		stopFirst := c.startCleaner()
		defer stopFirst()
		done := c.done
		stopSecond := c.startCleaner()
		defer stopSecond()
		if c.done != done || c.owners != 2 {
			t.Fatal("overlapping owners started multiple cleaners")
		}
		synctest.Wait()
		put("first")
		time.Sleep(time.Hour - time.Second)
		synctest.Wait()
		if size() != 1 {
			t.Fatal("cache cleared before the hourly tick")
		}
		put("just inserted")
		time.Sleep(time.Second)
		synctest.Wait()
		if size() != 0 {
			t.Fatal("hourly tick did not clear the whole map")
		}
		stopFirst()
		stopFirst()
		if c.owners != 1 {
			t.Fatal("stop was not idempotent")
		}
		put("remaining owner")
		time.Sleep(time.Hour)
		synctest.Wait()
		if size() != 0 {
			t.Fatal("first owner stopped the shared cleaner")
		}
		put("retained after stop")
		stopSecond()
		select {
		case <-done:
		default:
			t.Fatal("last owner did not join the cleaner")
		}
		time.Sleep(2 * time.Hour)
		if size() != 1 {
			t.Fatal("stop cleared the cache or left the cleaner running")
		}
		stopRestarted := c.startCleaner()
		defer stopRestarted()
		if c.done == done {
			t.Fatal("cleaner did not restart")
		}
		synctest.Wait()
		time.Sleep(time.Hour)
		synctest.Wait()
		if size() != 0 {
			t.Fatal("restarted cleaner did not clear the map")
		}
		stopRestarted()
	})
}
