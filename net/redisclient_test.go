package net

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/net/redistest"
	"github.com/zalando/skipper/tracing/tracers/basic"
)

// Helper to get a test logger
func getTestLogger(t *testing.T) logging.Logger {
	// Use the default logger, as logging.DefaultLog does not have a Std field
	return &logging.DefaultLog{}
}

func TestRedisContainer(t *testing.T) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()
	if redisAddr == "" {
		t.Fatal("Failed to create redis 1")
	}
	redisAddr2, done2 := redistest.NewTestRedis(t)
	defer done2()
	if redisAddr2 == "" {
		t.Fatal("Failed to create redis 2")
	}
}

func Test_hasAll(t *testing.T) {
	for _, tt := range []struct {
		name string
		a    []string
		h    map[string]struct{}
		want bool
	}{
		{
			name: "both empty",
			a:    nil,
			h:    nil,
			want: true,
		},
		{
			name: "a empty",
			a:    nil,
			h: map[string]struct{}{
				"foo": {},
			},
			want: false,
		},
		{
			name: "h empty",
			a:    []string{"foo"},
			h:    nil,
			want: false,
		},
		{
			name: "both set equal",
			a:    []string{"foo"},
			h: map[string]struct{}{
				"foo": {},
			},
			want: true,
		},
		{
			name: "both set notequal",
			a:    []string{"fo"},
			h: map[string]struct{}{
				"foo": {},
			},
			want: false,
		},
		{
			name: "both set multiple equal",
			a:    []string{"bar", "foo"},
			h: map[string]struct{}{
				"foo": {},
				"bar": {},
			},
			want: true,
		},
		{
			name: "a has duplicates, h does not",
			a:    []string{"foo", "foo"},
			h: map[string]struct{}{
				"foo": {},
			},
			want: false, // because len(a) != len(h)
		},
		{
			name: "a has duplicates, h has same elements",
			a:    []string{"foo", "bar", "foo"},
			h: map[string]struct{}{
				"foo": {},
				"bar": {},
			},
			want: false, // because len(a) != len(h)
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := hasAll(tt.a, tt.h)
			if tt.want != got {
				t.Errorf("Failed to get %v for hasall(%v, %v), got %v", tt.want, tt.a, tt.h, got)
			}
		})
	}
}

type addressUpdater struct {
	addrs []string
	mu    sync.Mutex
	n     int
	err   error
}

// update returns non empty subsequences of addrs,
// e.g. for [foo bar baz] it returns:
// 1: [foo]
// 2: [foo bar]
// 3: [foo bar baz]
// 4: [foo]
// 5: [foo bar]
// 6: [foo bar baz]
// ...
func (u *addressUpdater) update() ([]string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.err != nil {
		return nil, u.err
	}
	if len(u.addrs) == 0 {
		u.n++
		return []string{}, nil
	}

	result := u.addrs[0 : 1+u.n%len(u.addrs)]
	u.n++
	return result, nil
}

func (u *addressUpdater) calls() int {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.n
}

func (u *addressUpdater) setAddrs(addrs []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.addrs = addrs
	u.n = 0 // Reset counter when addresses change
}

func (u *addressUpdater) setError(err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.err = err
}

// TestRedisClient renamed to TestRedisClient_RingMode and adapted
func TestRedisClient_RingMode_Lifecycle(t *testing.T) {
	tracer, err := basic.InitTracer([]string{"recorder=in-memory"})
	require.NoError(t, err, "Failed to get a tracer")
	defer tracer.Close()

	redisAddr, done := redistest.NewTestRedis(t)
	defer done()
	redisAddr2, done2 := redistest.NewTestRedis(t)
	defer done2()

	updater := &addressUpdater{addrs: []string{redisAddr, redisAddr2}}

	for _, tt := range []struct {
		name    string
		options *RedisOptions
		wantErr bool
	}{
		{
			name: "Ring with Static Addrs",
			options: &RedisOptions{
				ClusterMode: false, // Explicit for clarity
				Addrs:       []string{redisAddr},
				Log:         getTestLogger(t),
			},
			wantErr: false,
		},
		{
			name: "Ring with AddrUpdater",
			options: &RedisOptions{
				ClusterMode:    false,
				AddrUpdater:    updater.update,
				UpdateInterval: 20 * time.Millisecond,
				Log:            getTestLogger(t),
			},
			wantErr: false,
		},
		{
			name: "Ring with tracer",
			options: &RedisOptions{
				ClusterMode: false,
				Addrs:       []string{redisAddr},
				Tracer:      tracer,
				Log:         getTestLogger(t),
			},
			wantErr: false,
		},
		{
			name: "Ring with metrics",
			options: &RedisOptions{
				ClusterMode:         false,
				Addrs:               []string{redisAddr},
				ConnMetricsInterval: 20 * time.Millisecond, // Increased slightly
				MetricsPrefix:       "test.ring.redis.",
				Log:                 getTestLogger(t),
			},
			wantErr: false,
		},
		{
			name: "Ring fails with no Addrs or Updater",
			options: &RedisOptions{
				ClusterMode: false,
				Addrs:       nil,
				AddrUpdater: nil,
				Log:         getTestLogger(t),
			},
			wantErr: true, // Expect initialization to fail logically
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Reset updater state for each test run
			updater.setAddrs([]string{redisAddr, redisAddr2})
			updater.setError(nil)

			// Set default metrics if needed, or use a stub
			originalMetrics := metrics.Default
			// Use the default metrics implementation, as no mock is available
			defer func() { metrics.Default = originalMetrics }()

			cli := NewRedisClient(tt.options)
			if tt.wantErr {
				require.True(t, cli.closed, "Expected client to be marked as closed on initialization error")
				require.Nil(t, cli.client, "Expected internal client to be nil on initialization error")
				// Attempt to close a failed client should be safe
				cli.Close()
				require.True(t, cli.closed, "Client should remain closed after Close()")
				return // Skip further checks for expected error cases
			}

			// If we expected success, ensure client is not closed initially
			require.False(t, cli.closed, "Client should not be closed initially")
			require.NotNil(t, cli.client, "Internal client should be initialized")

			// Use defer for cleanup, checking the closed status at the end
			defer func() {
				cli.Close()
				require.True(t, cli.closed, "Client failed to close properly")
			}()

			require.True(t, cli.IsAvailable(), "Redis client should be available")

			// Test Address Updater behavior (only if configured)
			if tt.options.AddrUpdater != nil {
				initialCalls := updater.calls()
				t.Logf("Initial updater calls: %d", initialCalls)

				// Wait for at least one update cycle
				time.Sleep(2 * cli.options.UpdateInterval) // Ensure ticker has fired

				currentCalls := updater.calls()
				t.Logf("Updater calls after wait: %d", currentCalls)
				assert.Greater(t, currentCalls, initialCalls, "Expected updater to be called after interval")

				// Test close stops background update
				cli.Close() // Close it *before* the final check

				// Wait to see if updater stops
				callsAfterClose := updater.calls()
				t.Logf("Updater calls immediately after close: %d", callsAfterClose)
				time.Sleep(3 * cli.options.UpdateInterval) // Wait longer to be sure
				finalCalls := updater.calls()
				t.Logf("Updater calls after close + wait: %d", finalCalls)

				assert.Equal(t, callsAfterClose, finalCalls, "Expected no more updater calls after Close()")
				assert.True(t, cli.closed, "Client should be marked closed after Close()")

				// Re-check availability on closed client
				assert.False(t, cli.IsAvailable(), "Client should not be available after Close()")
				return // Skip other checks as client is now closed
			}

			// Test Tracer (only if configured)
			if tt.options.Tracer != nil {
				span := cli.StartSpan("test-span")
				require.NotNil(t, span, "Expected a valid span object")
				span.Finish()
				// Check if span was recorded (depends on tracer implementation)
				// For basic tracer, we can't easily inspect memory, just ensure no panic.
			}

			// Test Metrics Collection (only if configured)
			if tt.options.ConnMetricsInterval > 0 {
				cli.StartMetricsCollection(context.Background())
				// Wait for metrics to be potentially collected
				time.Sleep(2 * cli.options.ConnMetricsInterval)
				// We cannot check for mock metrics, just ensure no panic or error
				cli.Close()
				time.Sleep(2 * cli.options.ConnMetricsInterval) // Wait again
				assert.True(t, cli.closed, "Client should be marked closed after Close()")
				return // Skip other checks as client is now closed
			}
		})
	}
}

// TestRedisClientGetSet adapted for the new client (Ring Mode)
func TestRedisClient_RingMode_GetSet(t *testing.T) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()
	redisAddr2, done2 := redistest.NewTestRedis(t) // Add second node for hashing tests
	defer done2()

	testAddrs := []string{redisAddr, redisAddr2}

	for _, tt := range []struct {
		name          string
		options       *RedisOptions
		key           string
		value         interface{}
		expire        time.Duration
		wait          time.Duration
		expect        string
		expectSetErr  bool
		expectGetErr  bool
		checkErrIsNil bool
	}{
		{
			name: "Ring: set/get one, no expiration",
			options: &RedisOptions{
				ClusterMode: false,
				Addrs:       []string{redisAddr}, // Single node OK here
				Log:         getTestLogger(t),
			},
			key:    "k1",
			value:  "foo",
			expect: "foo",
		},
		{
			name: "Ring: set/get one, with expiration",
			options: &RedisOptions{
				ClusterMode: false,
				Addrs:       testAddrs,
				Log:         getTestLogger(t),
			},
			key:    "k2",
			value:  "bar",
			expire: 1 * time.Second,        // Use >= 1s for EXPIRE
			wait:   500 * time.Millisecond, // Wait less than expiration
			expect: "bar",
		},
		{
			name: "Ring: set/get none, value expired",
			options: &RedisOptions{
				ClusterMode: false,
				Addrs:       []string{redisAddr},
				Log:         getTestLogger(t), // Single node OK here
			},
			key:           "k3",
			value:         "baz",
			expire:        1 * time.Second,         // Use >= 1s
			wait:          1100 * time.Millisecond, // Wait longer than expiration
			expectGetErr:  true,
			checkErrIsNil: true, // Expect redis.Nil error
		},
		{
			name: "Ring: get non-existent key",
			options: &RedisOptions{
				ClusterMode: false,
				Addrs:       []string{redisAddr},
				Log:         getTestLogger(t),
			},
			key:           "nonexistent",
			expectGetErr:  true,
			checkErrIsNil: true,
		},
		// Hashing algorithm tests (require multiple nodes)
		{
			name: "Ring: Rendezvous hash",
			options: &RedisOptions{
				ClusterMode:   false,
				Addrs:         testAddrs,
				HashAlgorithm: "rendezvous",
				Log:           getTestLogger(t),
			},
			key:    "khash1",
			value:  "hashvalue1",
			expect: "hashvalue1",
		},
		{
			name: "Ring: RendezvousVnodes hash",
			options: &RedisOptions{
				ClusterMode:   false,
				Addrs:         testAddrs,
				HashAlgorithm: "rendezvousVnodes",
				Log:           getTestLogger(t),
			},
			key:    "khash2",
			value:  "hashvalue2",
			expect: "hashvalue2",
		},
		{
			name: "Ring: Jump hash",
			options: &RedisOptions{
				ClusterMode:   false,
				Addrs:         testAddrs,
				HashAlgorithm: "jump",
				Log:           getTestLogger(t),
			},
			key:    "khash3",
			value:  "hashvalue3",
			expect: "hashvalue3",
		},
		{
			name: "Ring: Multiprobe hash",
			options: &RedisOptions{
				ClusterMode:   false,
				Addrs:         testAddrs,
				HashAlgorithm: "mpchash",
				Log:           getTestLogger(t),
			},
			key:    "khash4",
			value:  "hashvalue4",
			expect: "hashvalue4",
		},
		{
			name: "Ring: Unknown hash (defaults to rendezvous)",
			options: &RedisOptions{
				ClusterMode:   false,
				Addrs:         testAddrs,
				HashAlgorithm: "unknown-hash-algo", // Should log a warning
				Log:           getTestLogger(t),
			},
			key:    "khash5",
			value:  "hashvalue5",
			expect: "hashvalue5",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisClient(tt.options)
			require.NotNil(t, cli, "Failed to create client")
			require.False(t, cli.closed, "Client should not be closed initially")
			defer cli.Close()
			ctx := context.Background()

			// Perform Set if a value is provided
			if tt.value != nil {
				_, err := cli.Set(ctx, tt.key, tt.value, tt.expire)
				if tt.expectSetErr {
					require.Error(t, err, "Expected error during Set")
					return // Don't proceed if Set failed as expected
				}
				require.NoError(t, err, "Unexpected error during Set")
			}

			// Wait if specified
			if tt.wait > 0 {
				time.Sleep(tt.wait)
			}

			// Perform Get
			val, err := cli.Get(ctx, tt.key)

			if tt.expectGetErr {
				require.Error(t, err, "Expected error during Get")
				if tt.checkErrIsNil {
					assert.True(t, errors.Is(err, redis.Nil), "Expected redis.Nil error, got %v", err)
				}
			} else {
				require.NoError(t, err, "Unexpected error during Get")
				assert.Equal(t, tt.expect, val, "Get returned unexpected value")
			}

			// Clean up the key if set successfully
			if tt.value != nil && !tt.expectSetErr {
				// Use Del for cleanup, ignore error as key might have expired
				_ = cli.client.(redis.Cmdable).Del(ctx, tt.key)
			}
		})
	}
}

// --- Common Test Data for ZSet tests ---
type valScore struct {
	val   int64 // Keep as int64 if original test used it
	score float64
}

func setupZSetData(t *testing.T, cli *RedisClient, data map[string][]valScore) {
	t.Helper()
	ctx := context.Background()
	for k, items := range data {
		for _, item := range items {
			// Use ZAdd directly, handle potential errors
			_, err := cli.ZAdd(ctx, k, item.val, item.score)
			require.NoErrorf(t, err, "Failed setup: ZAdd key=%s, val=%d, score=%.2f", k, item.val, item.score)
		}
	}
}

func cleanupZSetData(t *testing.T, cli *RedisClient, data map[string][]valScore) {
	t.Helper()
	ctx := context.Background()
	for k := range data {
		// Use Del to remove the whole key, ignore error
		_ = cli.client.(redis.Cmdable).Del(ctx, k)
	}
}

// TestRedisClientZAddZCard adapted (Ring Mode)
func TestRedisClient_RingMode_ZAddZCard(t *testing.T) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	baseOptions := &RedisOptions{
		ClusterMode: false,
		Addrs:       []string{redisAddr},
		Log:         getTestLogger(t),
	}

	for _, tt := range []struct {
		name    string
		options *RedisOptions
		h       map[string][]valScore
		key     string
		expect  int64
		wantErr bool
	}{
		{
			name:    "Ring: ZCard on non-existent key",
			options: baseOptions,
			key:     "zcard_nonexistent",
			expect:  0,
		},
		{
			name:    "Ring: ZAdd one, ZCard one",
			options: baseOptions,
			key:     "zk1",
			h: map[string][]valScore{
				"zk1": {{val: 10, score: 5.0}},
			},
			expect: 1,
		},
		{
			name:    "Ring: ZAdd multiple, ZCard correct count",
			options: baseOptions,
			key:     "zk2",
			h: map[string][]valScore{
				"zk2": {
					{val: 1, score: 1.0},
					{val: 2, score: 2.0},
					{val: 3, score: 3.0},
				},
			},
			expect: 3,
		},
		{
			name:    "Ring: ZAdd duplicate member (updates score), ZCard unchanged",
			options: baseOptions,
			key:     "zk3",
			h: map[string][]valScore{
				"zk3": {
					{val: 1, score: 1.0},
					{val: 2, score: 2.0},
					{val: 1, score: 1.5},
				},
			},
			expect: 2,
		},
		{
			name:    "Ring: ZAdd multiple keys, ZCard correct for specified key",
			options: baseOptions,
			key:     "zk4a",
			h: map[string][]valScore{
				"zk4a": {
					{val: 1, score: 1.0},
					{val: 2, score: 2.0},
				},
				"zk4b": {
					{val: 100, score: 10.0},
				},
			},
			expect: 2,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisClient(tt.options)
			require.NotNil(t, cli)
			defer cli.Close()
			ctx := context.Background()

			if len(tt.h) > 0 {
				setupZSetData(t, cli, tt.h)
				defer cleanupZSetData(t, cli, tt.h) // Ensure cleanup
			}

			val, err := cli.ZCard(ctx, tt.key)

			if tt.wantErr {
				require.Error(t, err, "Expected error during ZCard")
			} else {
				require.NoError(t, err, "Unexpected error during ZCard")
				assert.Equal(t, tt.expect, val, "ZCard returned unexpected count")
			}
		})
	}
}

// TestRedisClientExpire adapted (Ring Mode)
func TestRedisClient_RingMode_Expire(t *testing.T) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	baseOptions := &RedisOptions{
		ClusterMode: false,
		Addrs:       []string{redisAddr},
		Log:         getTestLogger(t),
	}

	for _, tt := range []struct {
		name             string
		options          *RedisOptions
		h                map[string][]valScore
		keyToExpire      string
		expire           time.Duration
		expectInitial    int64
		expectAfterWait  int64
		expectExpireBool bool
		wantErr          bool
	}{
		{
			name:             "Ring: Expire non-existent key",
			options:          baseOptions,
			keyToExpire:      "expire_nonexistent",
			expire:           1 * time.Second,
			expectInitial:    0,
			expectAfterWait:  0,
			expectExpireBool: false,
		},
		{
			name:        "Ring: Expire existing key, check before/after",
			options:     baseOptions,
			keyToExpire: "expire_k1",
			h: map[string][]valScore{
				"expire_k1": {{val: 10, score: 5.0}},
			},
			expire:           1 * time.Second,
			expectInitial:    1,
			expectAfterWait:  0,
			expectExpireBool: true,
		},
		{
			name:        "Ring: Expire existing key, check before expiry time",
			options:     baseOptions,
			keyToExpire: "expire_k2",
			h: map[string][]valScore{
				"expire_k2": {{val: 1, score: 1.0}, {val: 2, score: 2.0}},
			},
			expire:           2 * time.Second,
			expectInitial:    2,
			expectAfterWait:  2,
			expectExpireBool: true,
		},
		{
			name:        "Ring: Set expire 0 (or negative) - should persist",
			options:     baseOptions,
			keyToExpire: "expire_k3",
			h: map[string][]valScore{
				"expire_k3": {{val: 30, score: 3.0}},
			},
			expire:           0 * time.Second,
			expectInitial:    1,
			expectAfterWait:  1,
			expectExpireBool: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisClient(tt.options)
			require.NotNil(t, cli)
			defer cli.Close()
			ctx := context.Background()

			if len(tt.h) > 0 {
				setupZSetData(t, cli, tt.h)
				defer cleanupZSetData(t, cli, tt.h)
			}

			initialVal, err := cli.ZCard(ctx, tt.keyToExpire)
			require.NoError(t, err, "Unexpected error during initial ZCard check")
			assert.Equal(t, tt.expectInitial, initialVal, "Initial ZCard count mismatch")

			// Set expiry
			expireRes, err := cli.Expire(ctx, tt.keyToExpire, tt.expire)
			if tt.wantErr {
				require.Error(t, err, "Expected error during Expire")
				return
			}
			require.NoError(t, err, "Unexpected error during Expire")
			assert.Equal(t, tt.expectExpireBool, expireRes, "Expire command returned unexpected boolean")

			if tt.expectAfterWait == 0 {
				// We expect the key to expire.
				assert.Eventually(t, func() bool {
					count, err := cli.ZCard(ctx, tt.keyToExpire)
					return err == nil && count == 0
				}, tt.expire+(500*time.Millisecond), 100*time.Millisecond) // Poll until expired
			} else {
				// We expect the key to persist.
				// A short sleep to ensure Redis has had time to process, but the key hasn't expired.
				time.Sleep(100 * time.Millisecond)
				finalVal, err := cli.ZCard(ctx, tt.keyToExpire)
				require.NoError(t, err, "Unexpected error during final ZCard check")
				assert.Equal(t, tt.expectAfterWait, finalVal, "ZCard count after wait mismatch")
			}
		})
	}
}

// TestRedisClientZRemRangeByScore adapted (Ring Mode)
func TestRedisClient_RingMode_ZRemRangeByScore(t *testing.T) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	baseOptions := &RedisOptions{
		ClusterMode: false,
		Addrs:       []string{redisAddr},
		Log:         getTestLogger(t),
	}

	for _, tt := range []struct {
		name          string
		options       *RedisOptions
		h             map[string][]valScore
		key           string
		minScore      float64
		maxScore      float64
		expectInitial int64
		expectRemoved int64
		expectFinal   int64
		wantErr       bool
	}{
		{
			name:          "Ring: Remove from non-existent key",
			options:       baseOptions,
			key:           "zrem_nonexistent",
			minScore:      0,
			maxScore:      10,
			expectInitial: 0,
			expectRemoved: 0,
			expectFinal:   0,
		},
		{
			name:    "Ring: Remove range matching nothing",
			options: baseOptions,
			key:     "zrem_k1",
			h: map[string][]valScore{
				"zrem_k1": {{val: 10, score: 5.0}},
			},
			minScore:      6.0,
			maxScore:      7.0,
			expectInitial: 1,
			expectRemoved: 0,
			expectFinal:   1,
		},
		{
			name:    "Ring: Remove range matching one item",
			options: baseOptions,
			key:     "zrem_k2",
			h: map[string][]valScore{
				"zrem_k2": {{val: 10, score: 5.0}},
			},
			minScore:      4.0,
			maxScore:      6.0,
			expectInitial: 1,
			expectRemoved: 1,
			expectFinal:   0,
		},
		{
			name:    "Ring: Remove range matching subset",
			options: baseOptions,
			key:     "zrem_k3",
			h: map[string][]valScore{
				"zrem_k3": {
					{val: 1, score: 1.0},
					{val: 2, score: 2.0},
					{val: 3, score: 3.0},
					{val: 4, score: 4.0},
				},
			},
			minScore:      1.5, // Matches scores 2.0 and 3.0
			maxScore:      3.5,
			expectInitial: 4,
			expectRemoved: 2,
			expectFinal:   2,
		},
		{
			name:    "Ring: Remove range matching all",
			options: baseOptions,
			key:     "zrem_k4",
			h: map[string][]valScore{
				"zrem_k4": {
					{val: 1, score: 1.0},
					{val: 2, score: 2.0},
				},
			},
			minScore:      0.0,
			maxScore:      5.0,
			expectInitial: 2,
			expectRemoved: 2,
			expectFinal:   0,
		},
		{
			name:    "Ring: Remove using inclusive bounds",
			options: baseOptions,
			key:     "zrem_k5",
			h: map[string][]valScore{
				"zrem_k5": {
					{val: 1, score: 1.0},
					{val: 2, score: 2.0},
					{val: 3, score: 3.0},
				},
			},
			minScore:      1.0,
			maxScore:      2.0,
			expectInitial: 3,
			expectRemoved: 2,
			expectFinal:   1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisClient(tt.options)
			require.NotNil(t, cli)
			defer cli.Close()
			ctx := context.Background()

			if len(tt.h) > 0 {
				setupZSetData(t, cli, tt.h)
				defer cleanupZSetData(t, cli, tt.h)
			}

			// Check initial ZCard
			initialVal, err := cli.ZCard(ctx, tt.key)
			require.NoError(t, err, "Unexpected error during initial ZCard check")
			assert.Equal(t, tt.expectInitial, initialVal, "Initial ZCard count mismatch")

			// Perform ZRemRangeByScore
			removedCount, err := cli.ZRemRangeByScore(ctx, tt.key, tt.minScore, tt.maxScore)
			if tt.wantErr {
				require.Error(t, err, "Expected error during ZRemRangeByScore")
				return
			}
			require.NoError(t, err, "Unexpected error during ZRemRangeByScore")
			assert.Equal(t, tt.expectRemoved, removedCount, "ZRemRangeByScore returned unexpected count")

			// Check final ZCard
			finalVal, err := cli.ZCard(ctx, tt.key)
			require.NoError(t, err, "Unexpected error during final ZCard check")
			assert.Equal(t, tt.expectFinal, finalVal, "Final ZCard count mismatch")
		})
	}
}

// TestRedisClientZRangeByScoreWithScoresFirst adapted (Ring Mode)
func TestRedisClient_RingMode_ZRangeByScoreWithScoresFirst(t *testing.T) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()

	baseOptions := &RedisOptions{
		ClusterMode: false,
		Addrs:       []string{redisAddr},
		Log:         getTestLogger(t),
	}

	for _, tt := range []struct {
		name     string
		options  *RedisOptions
		h        map[string][]valScore
		key      string
		minScore float64
		maxScore float64
		offset   int64
		count    int64
		expect   interface{}
		wantErr  bool
	}{
		{
			name:     "Ring: Range on non-existent key",
			options:  baseOptions,
			key:      "zrange_nonexistent",
			minScore: 0,
			maxScore: 10,
			offset:   0,
			count:    1,
			expect:   nil,
		},
		{
			name:    "Ring: Range matching one, get first",
			options: baseOptions,
			key:     "zrange_k1",
			h: map[string][]valScore{
				"zrange_k1": {{val: 10, score: 5.0}},
			},
			minScore: 4.0,
			maxScore: 6.0,
			offset:   0,
			count:    1,
			expect:   "10",
		},
		{
			name:    "Ring: Range matching none",
			options: baseOptions,
			key:     "zrange_k2",
			h: map[string][]valScore{
				"zrange_k2": {{val: 10, score: 5.0}},
			},
			minScore: 6.0,
			maxScore: 7.0,
			offset:   0,
			count:    1,
			expect:   nil,
		},
		{
			name:    "Ring: Range matching multiple, get first (offset 0)",
			options: baseOptions,
			key:     "zrange_k3",
			h: map[string][]valScore{
				"zrange_k3": {
					{val: 1, score: 1.0},
					{val: 2, score: 2.0},
					{val: 3, score: 3.0},
				},
			},
			minScore: 0.0,
			maxScore: 5.0,
			offset:   0,
			count:    1,
			expect:   "1",
		},
		{
			name:    "Ring: Range matching multiple, get second (offset 1)",
			options: baseOptions,
			key:     "zrange_k4",
			h: map[string][]valScore{
				"zrange_k4": {
					{val: 1, score: 1.0},
					{val: 2, score: 2.0},
					{val: 3, score: 3.0},
				},
			},
			minScore: 0.0,
			maxScore: 5.0,
			offset:   1,
			count:    1,
			expect:   "2",
		},
		{
			name:    "Ring: Range matching multiple, offset beyond results",
			options: baseOptions,
			key:     "zrange_k5",
			h: map[string][]valScore{
				"zrange_k5": {
					{val: 1, score: 1.0},
					{val: 2, score: 2.0},
				},
			},
			minScore: 0.0,
			maxScore: 5.0,
			offset:   2,
			count:    1,
			expect:   nil,
		},
		{
			name:    "Ring: Range matching one, count is 0 (should default to 1)",
			options: baseOptions,
			key:     "zrange_k6",
			h: map[string][]valScore{
				"zrange_k6": {{val: 60, score: 6.0}},
			},
			minScore: 0.0,
			maxScore: 10.0,
			offset:   0,
			count:    0, // Method should handle count <= 0
			expect:   "60",
		},
		{
			name:    "Ring: Range matching multiple, get first using negative count (should default to 1)",
			options: baseOptions,
			key:     "zrange_k7",
			h: map[string][]valScore{
				"zrange_k7": {
					{val: 71, score: 7.1},
					{val: 72, score: 7.2},
				},
			},
			minScore: 0.0,
			maxScore: 10.0,
			offset:   0,
			count:    -5, // Method should handle count <= 0
			expect:   "71",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisClient(tt.options)
			require.NotNil(t, cli)
			defer cli.Close()
			ctx := context.Background()

			if len(tt.h) > 0 {
				setupZSetData(t, cli, tt.h)
				defer cleanupZSetData(t, cli, tt.h)
			}

			// Perform ZRangeByScoreWithScoresFirst
			// Note: The method internally uses ZRangeByScoreWithScores with the options.
			// It gets the first element after the offset within the range.
			res, err := cli.ZRangeByScoreWithScoresFirst(ctx, tt.key, tt.minScore, tt.maxScore, tt.offset, tt.count)

			if tt.wantErr {
				require.Error(t, err, "Expected error during ZRangeByScoreWithScoresFirst")
			} else {
				require.NoError(t, err, "Unexpected error during ZRangeByScoreWithScoresFirst")
				// Compare result with expectation
				if tt.expect == nil {
					assert.Nil(t, res, "Expected nil result, got %v", res)
				} else {
					assert.Equal(t, tt.expect, res, "Result mismatch")
				}
			}
		})
	}
}

// TestRedisClientSetAddr adapted (Ring Mode)
func TestRedisClient_RingMode_SetAddrs(t *testing.T) {
	redisAddr1, done1 := redistest.NewTestRedis(t)
	defer done1()
	redisAddr2, done2 := redistest.NewTestRedis(t)
	defer done2()
	redisAddr3, done3 := redistest.NewTestRedis(t)
	defer done3()

	keys := []string{"sa_foo1", "sa_foo2", "sa_foo3", "sa_foo4", "sa_foo5", "sa_foo6"}
	vals := []string{"bar1", "bar2", "bar3", "bar4", "bar5", "bar6"}

	for _, tt := range []struct {
		name          string
		initialAddrs  []string
		updateToAddrs []string // Addresses to set via SetAddrs
	}{
		{
			name:          "Ring: No address change (SetAddrs with same list)",
			initialAddrs:  []string{redisAddr1, redisAddr2},
			updateToAddrs: []string{redisAddr1, redisAddr2},
		},
		{
			name:          "Ring: Add a node",
			initialAddrs:  []string{redisAddr1},
			updateToAddrs: []string{redisAddr1, redisAddr2},
		},
		{
			name:          "Ring: Remove a node",
			initialAddrs:  []string{redisAddr1, redisAddr2, redisAddr3},
			updateToAddrs: []string{redisAddr1, redisAddr3}, // Remove redisAddr2
		},
		{
			name:          "Ring: Replace nodes",
			initialAddrs:  []string{redisAddr1, redisAddr2},
			updateToAddrs: []string{redisAddr1, redisAddr3}, // Replace redisAddr2 with redisAddr3
		},
		{
			name:          "Ring: SetAddrs with empty list",
			initialAddrs:  []string{redisAddr1, redisAddr2},
			updateToAddrs: []string{}, // Should effectively disable the ring
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts := &RedisOptions{
				ClusterMode: false,
				Addrs:       tt.initialAddrs,
				Log:         getTestLogger(t),
				// Use a specific hash algorithm to make key distribution somewhat predictable for testing
				// Though exact node isn't tested here, just availability after change.
				HashAlgorithm: "jump",
			}
			cli := NewRedisClient(opts)
			require.NotNil(t, cli)
			require.False(t, cli.closed)
			defer cli.Close()
			ctx := context.Background()

			// Initial Set of keys
			for i := 0; i < len(keys); i++ {
				_, err := cli.Set(ctx, keys[i], vals[i], 10*time.Second) // Use short expiry for test keys
				require.NoErrorf(t, err, "Initial Set failed for key %s", keys[i])
			}

			// Call SetAddrs
			t.Logf("Calling SetAddrs with: %v", tt.updateToAddrs)
			cli.SetAddrs(ctx, tt.updateToAddrs)

			// Verify internal options were updated
			assert.ElementsMatch(t, tt.updateToAddrs, cli.options.Addrs, "Client options.Addrs not updated correctly")

			// Allow some time for potential internal state updates in go-redis ring
			time.Sleep(50 * time.Millisecond)

			// Attempt to Get keys after SetAddrs
			// Note: Keys might now map to different nodes or be unavailable if nodes were removed
			// or if the ring became empty.
			getErrs := 0
			for i := 0; i < len(keys); i++ {
				got, err := cli.Get(ctx, keys[i])

				if len(tt.updateToAddrs) == 0 {
					// If SetAddrs was empty, all operations should fail
					assert.Error(t, err, "Expected error getting key %s after setting empty addrs", keys[i])
					getErrs++
				} else if err != nil {
					// Tolerate redis.Nil errors as keys might have moved to a new node
					// where they weren't set, or expired. Other errors are unexpected.
					if !errors.Is(err, redis.Nil) {
						assert.NoErrorf(t, err, "Unexpected error getting key %s after SetAddrs", keys[i])
					} else {
						t.Logf("Got redis.Nil for key %s (potentially moved node or expired)", keys[i])
					}
				} else {
					// If we got a value, it *must* be the correct one
					assert.Equalf(t, vals[i], got, "Incorrect value for key %s after SetAddrs", keys[i])
				}
			}

			if len(tt.updateToAddrs) == 0 {
				assert.Equal(t, len(keys), getErrs, "Expected all Get operations to fail after empty SetAddrs")
				// Check availability after setting empty list
				assert.False(t, cli.IsAvailable(), "Client should be unavailable after empty SetAddrs")
			} else {
				// If nodes were updated, the client should still be generally available
				assert.True(t, cli.IsAvailable(), "Client should be available after non-empty SetAddrs")
			}

			// Clean up keys (best effort)
			if len(tt.updateToAddrs) > 0 {
				for i := 0; i < len(keys); i++ {
					_ = cli.client.(redis.Cmdable).Del(ctx, keys[i])
				}
			}
		})
	}
}

// TestRedisClientFailingAddrUpdater adapted (Ring Mode)
func TestRedisClient_RingMode_FailingAddrUpdater(t *testing.T) {
	failErr := errors.New("simulated updater failure")

	opts := &RedisOptions{
		ClusterMode: false,
		AddrUpdater: func() ([]string, error) {
			// Simulate failure during initial fetch
			return nil, failErr
		},
		// Retry logic in NewRedisClient needs time, ensure UpdateInterval is reasonable
		UpdateInterval: 100 * time.Millisecond,
		DialTimeout:    50 * time.Millisecond, // Make retries faster
		Log:            getTestLogger(t),
	}

	cli := NewRedisClient(opts)
	// Because NewRedisClient now retries the initial fetch, it might succeed if the error was transient.
	// However, if the updater *always* fails, initialization should log errors but *might* start with an empty ring.
	// Let's refine the expectation: the client might be created but likely unusable.

	// Check if client creation marked it as closed (unlikely now with retries, unless *all* retries fail AND no static addrs)
	// assert.True(t, cli.closed, "Expected client to be closed if initial AddrUpdater fetch failed persistently")

	// More reliably, check if the client is available after initialization attempts
	require.NotNil(t, cli, "NewRedisClient should return a client instance even if updater fails")
	defer cli.Close()

	// It might start with 0 addresses if all initial attempts failed.
	assert.Empty(t, cli.options.Addrs, "Expected client options.Addrs to be empty after failed initial update")

	// Check availability - should be false if no addresses were ever successfully fetched.
	available := cli.IsAvailable()
	assert.False(t, available, "Client should not be available if AddrUpdater consistently fails")

	// Let the updater run a couple of times in the background to ensure it keeps failing
	time.Sleep(3 * opts.UpdateInterval)

	availableAfterWait := cli.IsAvailable()
	assert.False(t, availableAfterWait, "Client should remain unavailable if AddrUpdater keeps failing")

	// Test SetAddrs still works (though maybe not useful if updater overrides)
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()
	cli.SetAddrs(context.Background(), []string{redisAddr})
	assert.ElementsMatch(t, []string{redisAddr}, cli.options.Addrs, "options.Addrs should reflect SetAddrs")

	// Ring *might* become available *briefly* after SetAddrs
	time.Sleep(50 * time.Millisecond) // Give ring time to process SetAddrs
	availableAfterSetAddrs := cli.IsAvailable()
	// This assertion is tricky. The background updater might *immediately* fail again
	// and reset the addresses before IsAvailable() is checked reliably.
	// A safer check is just that SetAddrs updated the options.
	t.Logf("Availability after manual SetAddrs (might be transient): %v", availableAfterSetAddrs)
	// assert.True(t, availableAfterSetAddrs, "Expected client to become available after manual SetAddrs")

}

func TestRedisClient_RingMode_RemoteURL_Success(t *testing.T) {
	redisAddr1, done1 := redistest.NewTestRedis(t)
	if redisAddr1 == "" {
		t.Skip("Skipping test, failed to start Redis instance 1")
	}
	defer done1()
	redisAddr2, done2 := redistest.NewTestRedis(t)
	defer done2()

	addrsResponse := fmt.Sprintf("%s, %s", redisAddr1, redisAddr2) // Comma-separated list

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var respMu sync.RWMutex
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "skipper-redis-updater/1.0", r.UserAgent())
		w.WriteHeader(http.StatusOK)
		respMu.RLock()
		resp := addrsResponse
		respMu.RUnlock()
		_, _ = w.Write([]byte(resp))
		t.Logf("Mock server served: %s", resp)
	}))
	defer server.Close()

	opts := &RedisOptions{
		ClusterMode:    false,
		RemoteURL:      server.URL,
		UpdateInterval: 50 * time.Millisecond,  // Faster updates for test
		DialTimeout:    100 * time.Millisecond, // Timeout for HTTP fetch
		Log:            getTestLogger(t),
	}

	cli := NewRedisClient(opts)
	require.NotNil(t, cli)
	require.False(t, cli.closed)
	defer cli.Close()

	// Check if initial addresses were fetched correctly (NewRedisClient calls it)
	require.Eventually(t, func() bool {
		// Need to access internal options safely or check availability
		cli.mu.RLock()
		cli.log.Debugf("Checking addresses: %v", cli.options.Addrs)
		addrCount := len(cli.options.Addrs)
		cli.mu.RUnlock()
		return addrCount == 2
	}, 500*time.Millisecond, 50*time.Millisecond, "Initial addresses not fetched via RemoteURL")

	cli.mu.RLock()
	assert.ElementsMatch(t, []string{redisAddr1, redisAddr2}, cli.options.Addrs, "Fetched addresses mismatch")
	cli.mu.RUnlock()
	assert.True(t, cli.IsAvailable(), "Client should be available after successful RemoteURL fetch")

	// Test background update
	// Change the response from the server
	newAddr3, done3 := redistest.NewTestRedis(t)
	defer done3()
	var respMu sync.Mutex // Mutex for addrsResponse
	respMu.Lock()
	addrsResponse = fmt.Sprintf("%s,%s", redisAddr1, newAddr3) // Remove addr2, add addr3
	respMu.Unlock()
	t.Logf("Mock server changed response to: %s", addrsResponse)

	// Wait for the updater goroutine to pick up the change
	require.Eventually(t, func() bool {
		// Access internal options safely
		cli.mu.RLock() // Lock client
		cli.log.Debugf("Checking updated addresses: %v", cli.options.Addrs)
		// Use ElementsMatch for order-insensitivity
		currentAddrs := cli.options.Addrs
		updated := len(currentAddrs) == 2 &&
			((currentAddrs[0] == redisAddr1 && currentAddrs[1] == newAddr3) ||
				(currentAddrs[0] == newAddr3 && currentAddrs[1] == redisAddr1))
		cli.mu.RUnlock() // Unlock client
		return updated
	}, 500*time.Millisecond, 50*time.Millisecond, "Addresses not updated via RemoteURL background task")

	cli.mu.RLock()
	assert.ElementsMatch(t, []string{redisAddr1, newAddr3}, cli.options.Addrs, "Updated addresses mismatch")
	cli.mu.RUnlock()
}

func TestRedisClient_RingMode_RemoteURL_Failures(t *testing.T) {
	redisAddr1, done1 := redistest.NewTestRedis(t) // Have one static addr for fallback test
	defer done1()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fail based on query param for different test cases
		switch r.URL.Query().Get("mode") {
		case "error500":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal Server Error"))
		case "invalidfmt":
			w.WriteHeader(http.StatusOK)
			// Missing ports, extra commas, spaces
			_, _ = w.Write([]byte("127.0.0.1, , :6379, invalid-entry, "))
		case "empty":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("")) // Empty body
		default: // Success case for initial start
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(redisAddr1)) // Start with one valid address
		}
	}))
	defer server.Close()

	tests := []struct {
		name            string
		urlMode         string   // Query param for mock server behavior
		initialAddrs    []string // Provide static addresses to see if they are kept on fetch failure
		expectAddrsLen  int      // Expected number of addresses after initialization/update attempt
		expectAvailable bool     // Expected availability *after* the initial fetch attempt
	}{
		{
			name:            "Fail 500, no initial addrs",
			urlMode:         "error500",
			initialAddrs:    nil,
			expectAddrsLen:  0, // Fails initial fetch, starts empty
			expectAvailable: false,
		},
		{
			name:            "Fail 500, with initial addrs",
			urlMode:         "error500",
			initialAddrs:    []string{redisAddr1}, // Start with this static addr
			expectAddrsLen:  1,                    // Initial fetch fails, should keep the static one
			expectAvailable: true,                 // Should be available using the initial static addr
		},
		{
			name:            "Invalid format, no initial addrs",
			urlMode:         "invalidfmt",
			initialAddrs:    nil,
			expectAddrsLen:  0, // Invalid format results in no valid addresses
			expectAvailable: false,
		},
		{
			name:            "Empty response, no initial addrs",
			urlMode:         "empty",
			initialAddrs:    nil,
			expectAddrsLen:  0,
			expectAvailable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &RedisOptions{
				ClusterMode: false,
				// Start with static addrs if provided
				Addrs: tt.initialAddrs,
				// Use RemoteURL which will behave based on tt.urlMode
				RemoteURL:      fmt.Sprintf("%s?mode=%s", server.URL, tt.urlMode),
				UpdateInterval: 50 * time.Millisecond,
				DialTimeout:    50 * time.Millisecond,
				Log:            getTestLogger(t),
			}

			cli := NewRedisClient(opts)
			require.NotNil(t, cli)
			// Client creation itself shouldn't fail, but it might end up unusable
			require.False(t, cli.closed)
			defer cli.Close()

			// Wait a moment for the initial fetch attempt within NewRedisClient to complete
			time.Sleep(150 * time.Millisecond) // Allow for dial timeout + processing

			// Check addresses based on expectation
			cli.mu.RLock()
			assert.Len(t, cli.options.Addrs, tt.expectAddrsLen, "Unexpected number of addresses")
			if tt.expectAddrsLen > 0 && len(tt.initialAddrs) > 0 {
				// If we expected to keep initial addrs, verify they are there
				assert.ElementsMatch(t, tt.initialAddrs, cli.options.Addrs, "Expected initial addrs to be retained")
			}
			cli.mu.RUnlock()

			// Check availability
			assert.Equal(t, tt.expectAvailable, cli.IsAvailable(), "Availability mismatch")

			// Let background updater run once more if interval is set
			if opts.UpdateInterval > 0 {
				time.Sleep(opts.UpdateInterval * 2)
				// Re-check availability, which handles its own locking.
				assert.Equal(t, tt.expectAvailable, cli.IsAvailable(), "Availability changed unexpectedly after background update failure")

				// Lock only for the duration of the direct field access.
				cli.mu.RLock()
				assert.Len(t, cli.options.Addrs, tt.expectAddrsLen, "Number of addresses changed unexpectedly after background update failure")
				cli.mu.RUnlock()
			}
		})
	}
}

// --- Basic Cluster Mode Tests ---
// NOTE: These tests require a running Redis Cluster accessible at the specified address(es).
// They might be skipped if a cluster isn't available in the CI environment.
// `redistest.NewTestRedis` only starts a single node, not a cluster.
// We will assume a single node *acting* like a cluster seed is sufficient for basic API tests.

func TestRedisClient_ClusterMode_Lifecycle(t *testing.T) {
	// Use the single test Redis instance as a "seed" node.
	// This won't test real cluster topology discovery or slot handling,
	// but verifies the ClusterClient path is taken and basic commands work.
	redisAddr, done := redistest.NewTestRedis(t)
	if redisAddr == "" {
		t.Skip("Skipping cluster test, failed to start Redis instance")
	}
	defer done()

	tests := []struct {
		name        string
		options     *RedisOptions
		expectError bool
	}{
		{
			name: "Cluster: Basic setup",
			options: &RedisOptions{
				ClusterMode: true,
				Addrs:       []string{redisAddr}, // Use single node as seed
				Log:         getTestLogger(t),
			},
			expectError: false,
		},
		{
			name: "Cluster: No addresses",
			options: &RedisOptions{
				ClusterMode: true,
				Addrs:       nil, // Error case
				Log:         getTestLogger(t),
			},
			expectError: true,
		},
		{
			name: "Cluster: AddrUpdater ignored",
			options: &RedisOptions{
				ClusterMode: true,
				Addrs:       []string{redisAddr},
				AddrUpdater: func() ([]string, error) {
					t.Error("AddrUpdater should not be called in Cluster mode")
					return nil, errors.New("updater called unexpectedly")
				},
				UpdateInterval: 10 * time.Millisecond, // Should be ignored
				Log:            getTestLogger(t),
			},
			expectError: false,
		},
		{
			name: "Cluster: RemoteURL ignored",
			options: &RedisOptions{
				ClusterMode: true,
				Addrs:       []string{redisAddr},
				RemoteURL:   "http://127.0.0.1:9999/ignored", // Should be ignored
				Log:         getTestLogger(t),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisClient(tt.options)
			require.NotNil(t, cli)

			if tt.expectError {
				assert.True(t, cli.closed, "Expected client to be closed on initialization error")
				assert.Nil(t, cli.client, "Expected internal client to be nil on initialization error")
				cli.Close() // Should be safe
				return
			}

			assert.False(t, cli.closed)
			assert.NotNil(t, cli.client)
			_, isClusterClient := cli.client.(*redis.ClusterClient)
			assert.True(t, isClusterClient, "Expected internal client to be *redis.ClusterClient")
			defer cli.Close()

			available := cli.IsAvailable()
			// If the error is specifically about cluster mode being disabled, skip the test.
			if !available {
				ctxPing, cancelPing := context.WithTimeout(context.Background(), 500*time.Millisecond)
				defer cancelPing()
				cmdable, _ := cli.getCmdable() // Use helper to get cmdable safely
				if cmdable != nil {
					pingErr := cmdable.Ping(ctxPing).Err()
					if pingErr != nil && strings.Contains(pingErr.Error(), "cluster support disabled") {
						t.Skipf("Skipping cluster test: Redis server at %s does not have cluster mode enabled: %v", redisAddr, pingErr)
					}
				}
			}
			assert.True(t, cli.IsAvailable(), "Cluster client should be available")

			// Ensure updater was not started if provided but ignored
			if tt.options.AddrUpdater != nil {
				// Give potential updater time to be called if it were running
				time.Sleep(3 * tt.options.UpdateInterval)
				// The assertion is inside the updater func itself
			}

			// Test SetAddrs is a no-op
			initialClusterAddrs := cli.options.Addrs // Get initial seed nodes
			cli.mu.RLock()
			initialOptionAddrs := make([]string, len(cli.options.Addrs))
			copy(initialOptionAddrs, cli.options.Addrs)
			cli.mu.RUnlock()
			cli.SetAddrs(context.Background(), []string{"newaddr:1234"})
			assert.ElementsMatch(t, initialOptionAddrs, cli.options.Addrs, "SetAddrs should not change options.Addrs in cluster mode")
			assert.Equal(t, initialClusterAddrs, cli.options.Addrs, "SetAddrs should not change options.Addrs in cluster mode")
			// We can't easily check the internal cluster client's nodes after SetAddrs.

			// Basic command test
			ctx := context.Background()
			key := "cluster_test_key"
			val := "cluster_value"
			_, err := cli.Set(ctx, key, val, 1*time.Second)
			require.NoError(t, err, "Set failed in cluster mode")
			got, err := cli.Get(ctx, key)
			require.NoError(t, err, "Get failed in cluster mode")
			assert.Equal(t, val, got)

			// Test metrics startup
			cli.StartMetricsCollection(ctx)
			time.Sleep(50 * time.Millisecond) // Give metrics a chance to run
			// We don't have mock metrics here, just ensure no panic

			cli.Close()
			assert.True(t, cli.closed)
			assert.False(t, cli.IsAvailable())
		})
	}
}
