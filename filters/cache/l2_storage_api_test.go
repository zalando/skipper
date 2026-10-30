package cache

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valkey-io/valkey-go"
	"github.com/zalando/skipper/metrics"
	skpnet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/net/valkeytest"
)

func TestValkeyStorage_StorageAPIObservability(t *testing.T) {
	addr := os.Getenv("SKIPPER_TEST_OWNED_VALKEY_ADDR")
	if addr == "" {
		var done func()
		addr, done = valkeytest.NewTestValkey(t)
		defer done()
	}
	admin, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{addr}, DisableCache: true,
		Dialer: net.Dialer{Timeout: 2 * time.Second}, ConnWriteTimeout: 2 * time.Second,
	})
	require.NoError(t, err)
	defer admin.Close()
	ring, err := skpnet.NewValkeyRingClient(&skpnet.ValkeyOptions{
		Addrs: []string{addr}, DialTimeout: 2 * time.Second, ConnWriteTimeout: 2 * time.Second,
	})
	require.NoError(t, err)
	defer ring.Close()

	commandCounts := func() map[string]int64 {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		info, err := admin.Do(ctx, admin.B().Arbitrary("INFO").Args("commandstats").Build()).ToString()
		require.NoError(t, err)
		counts := make(map[string]int64)
		for line := range strings.SplitSeq(info, "\n") {
			name, fields, ok := strings.Cut(strings.TrimSpace(line), ":")
			if !ok || !strings.HasPrefix(name, "cmdstat_") {
				continue
			}
			for field := range strings.SplitSeq(fields, ",") {
				value, ok := strings.CutPrefix(field, "calls=")
				if ok {
					counts[strings.TrimPrefix(name, "cmdstat_")], err = strconv.ParseInt(value, 10, 64)
					require.NoError(t, err)
				}
			}
		}
		return counts
	}
	exposition := func(m *metrics.Prometheus) string {
		t.Helper()
		rec := httptest.NewRecorder()
		m.CreateHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		return rec.Body.String()
	}
	newStorage := func() (*L2Storage, *metrics.Prometheus) {
		m := metrics.NewPrometheus(metrics.Options{})
		return NewL2Storage(ring, NewLRUStorage(1<<20, nil, m), m, 0, valkey.IsValkeyNil), m
	}

	for _, purpose := range []struct {
		name string
		ctx  context.Context
	}{
		{"unknown", context.Background()},
		{"response", withL2Purpose(context.Background(), l2PurposeResponse)},
		{"metadata", withL2Purpose(context.Background(), l2PurposeMetadata)},
	} {
		t.Run("encode/"+purpose.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(purpose.ctx, 5*time.Second)
			defer cancel()
			storage, m := newStorage()
			key := fmt.Sprintf("storage-api-encode-%s-%d", purpose.name, time.Now().UnixNano())
			before := commandCounts()
			err := storage.Set(ctx, key, &Entry{
				StatusCode: http.StatusOK, TTL: time.Minute,
				CreatedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC),
			})
			require.ErrorContains(t, err, "encode L2 entry")
			after := commandCounts()
			for _, command := range []string{"set", "get", "del"} {
				assert.Equal(t, before[command], after[command], "encode failure must not call Valkey %s", command)
			}
			local, err := storage.l1.Get(ctx, key)
			require.NoError(t, err)
			assert.Nil(t, local, "encoding failure must not create an L1 fallback")
			out := exposition(m)
			assert.Contains(t, out, fmt.Sprintf(`skipper_cache_l2_processing_errors_total{operation="set",purpose="%s",stage="encode"} 1`, purpose.name))
			assert.Contains(t, out, `skipper_cache_storage_error_total{backend="l2",op="set"} 1`)
			assert.NotContains(t, out, "skipper_cache_l2_operations_total")
			assert.NotContains(t, out, `key="cache.l2_set_duration"`)
			assert.NotContains(t, out, `key="cache.l2_set_fallback"`)
			t.Logf("StorageAPI-only encode/%s: Valkey SET/GET/DEL delta=0, processing error=1, no operation/duration/fallback", purpose.name)
		})
	}

	for _, purpose := range []struct {
		name string
		ctx  context.Context
	}{
		{"absent", context.Background()},
		{"explicit unknown", withL2Purpose(context.Background(), l2PurposeUnknown)},
		{"invalid value", withL2Purpose(context.Background(), l2Purpose(255))},
		{"wrong type", context.WithValue(context.Background(), l2PurposeContextKey{}, "response")},
	} {
		t.Run("unknown/"+purpose.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(purpose.ctx, 5*time.Second)
			defer cancel()
			storage, m := newStorage()
			key := fmt.Sprintf("storage-api-unknown-%d", time.Now().UnixNano())
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				assert.NoError(t, admin.Do(ctx, admin.B().Del().Key(key).Build()).Error())
			})
			before := commandCounts()
			require.NoError(t, storage.Set(ctx, key, &Entry{
				StatusCode: http.StatusOK, Payload: []byte("stored"), TTL: time.Minute, CreatedAt: time.Now(),
			}))
			for _, read := range []func(context.Context, string) (*Entry, error){storage.Get, storage.GetSilent} {
				entry, err := read(ctx, key)
				require.NoError(t, err)
				require.NotNil(t, entry)
				assert.Equal(t, "stored", string(entry.Payload))
			}
			require.NoError(t, storage.Delete(ctx, key))
			entry, err := storage.Get(ctx, key)
			require.NoError(t, err)
			assert.Nil(t, entry)
			after := commandCounts()
			assert.Equal(t, int64(1), after["set"]-before["set"])
			assert.Equal(t, int64(3), after["get"]-before["get"])
			assert.Equal(t, int64(1), after["del"]-before["del"])
			out := exposition(m)
			for _, sample := range []string{
				`skipper_cache_l2_operations_total{operation="set",outcome="success",purpose="unknown"} 1`,
				`skipper_cache_l2_operations_total{operation="get",outcome="success",purpose="unknown"} 2`,
				`skipper_cache_l2_operations_total{operation="get",outcome="miss",purpose="unknown"} 1`,
				`skipper_cache_l2_operations_total{operation="delete",outcome="success",purpose="unknown"} 1`,
				`skipper_custom_duration_seconds_count{key="cache.l2_set_duration"} 1`,
				`skipper_custom_duration_seconds_count{key="cache.l2_get_duration"} 3`,
				`skipper_custom_duration_seconds_count{key="cache.l2_delete_duration"} 1`,
			} {
				assert.Contains(t, out, sample)
			}
			assert.NotContains(t, out, "skipper_cache_l2_processing_errors_total")
			t.Logf("StorageAPI-only unknown/%s: Valkey INFO deltas SET=1 GET=3 DEL=1; actual Prometheus operation/duration counts match", purpose.name)
		})
	}
}
