package net_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/metrics/metricstest"
	snet "github.com/zalando/skipper/net"
)

func TestConnManager(t *testing.T) {
	const (
		keepaliveRequests = 3
		keepalive         = 100 * time.Millisecond

		testRequests = keepaliveRequests * 5
	)
	t.Run("does not close connection without limits", func(t *testing.T) {
		ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		m := &metricstest.MockMetrics{}
		cm := &snet.ConnManager{
			Metrics: m,
		}
		cm.Configure(ts.Config)

		ts.Start()
		defer ts.Close()

		for range testRequests {
			resp, err := ts.Client().Get(ts.URL)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.False(t, resp.Close)
		}

		time.Sleep(100 * time.Millisecond) // wait for connection state update

		m.WithCounters(func(counters map[string]int64) {
			assert.Equal(t, int64(1), counters["lb-conn-new"])
			assert.Equal(t, int64(testRequests), counters["lb-conn-active"])
			assert.Equal(t, int64(testRequests), counters["lb-conn-idle"])
			assert.Equal(t, int64(0), counters["lb-conn-closed"])
		})
	})
	t.Run("closes connection after keepalive requests", func(t *testing.T) {
		const keepaliveRequests = 3

		ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		m := &metricstest.MockMetrics{}
		cm := &snet.ConnManager{
			Metrics:           m,
			KeepaliveRequests: keepaliveRequests,
		}
		cm.Configure(ts.Config)

		ts.Start()
		defer ts.Close()

		for i := 1; i < testRequests; i++ {
			resp, err := ts.Client().Get(ts.URL)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			if i%keepaliveRequests == 0 {
				assert.True(t, resp.Close)
			} else {
				assert.False(t, resp.Close)
			}
		}

		time.Sleep(100 * time.Millisecond) // wait for connection state update

		m.WithCounters(func(counters map[string]int64) {
			rounds := int64(testRequests / keepaliveRequests)

			assert.Equal(t, rounds, counters["lb-conn-new"])
			assert.Equal(t, rounds-1, counters["lb-conn-closed"])
			assert.Equal(t, rounds-1, counters["lb-conn-closed.keepalive-requests"])
		})
	})

	t.Run("closes connection after keepalive timeout", func(t *testing.T) {
		const keepalive = 100 * time.Millisecond

		ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		m := &metricstest.MockMetrics{}
		cm := &snet.ConnManager{
			Metrics:   m,
			Keepalive: keepalive,
		}
		cm.Configure(ts.Config)

		ts.Start()
		defer ts.Close()

		for range testRequests {
			resp, err := ts.Client().Get(ts.URL)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.False(t, resp.Close)
		}

		time.Sleep(2 * keepalive)

		resp, err := ts.Client().Get(ts.URL)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, resp.Close)

		time.Sleep(100 * time.Millisecond) // wait for connection state update

		m.WithCounters(func(counters map[string]int64) {
			assert.Equal(t, int64(1), counters["lb-conn-new"])
			assert.Equal(t, int64(1), counters["lb-conn-closed"])
			assert.Equal(t, int64(1), counters["lb-conn-closed.keepalive"])
		})
	})
}
