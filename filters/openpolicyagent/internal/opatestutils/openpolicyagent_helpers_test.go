package opatestutils

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"testing"
	"time"
)

// ============================================================================
// TEST INFRASTRUCTURE VALIDATION
// ============================================================================
func TestControllableBundleServer(t *testing.T) {
	t.Run("Server availability can be toggled", func(t *testing.T) {
		cbs := StartControllableBundleServer(BundleServerConfig{BundleName: "testbundle", RespCode: http.StatusServiceUnavailable})
		defer cbs.Stop()

		assert.Equal(t, http.StatusServiceUnavailable, makeRequest(t, cbs.URL()+"/bundles/testbundle").StatusCode)

		cbs.SetRespCode(http.StatusOK)
		resp := makeRequest(t, cbs.URL()+"/bundles/testbundle")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.NotEmpty(t, readBody(t, resp))

		cbs.SetRespCode(http.StatusServiceUnavailable)
		assert.Equal(t, http.StatusServiceUnavailable, makeRequest(t, cbs.URL()+"/bundles/testbundle").StatusCode)
	})

	t.Run("Server delay functionality", func(t *testing.T) {
		cbs := StartControllableBundleServer(BundleServerConfig{BundleName: "testbundle", RespCode: http.StatusOK})
		defer cbs.Stop()

		cbs.SetDelay(100 * time.Millisecond)
		assert.GreaterOrEqual(t, measureRequestTime(t, cbs.URL()+"/bundles/testbundle"), 100*time.Millisecond)

		cbs.SetDelay(0)
		assert.Less(t, measureRequestTime(t, cbs.URL()+"/bundles/testbundle"), 100*time.Millisecond)
	})

	//t.Run("Multiple bundle servers", func(t *testing.T) {
	//	configs := []BundleServerConfig{
	//		{"bundle1", http.StatusOK, 0},
	//		{"bundle2", http.StatusServiceUnavailable, 0},
	//		{"bundle3", http.StatusOK, 25 * time.Millisecond},
	//	}
	//	servers := StartMultipleControllableBundleServers(configs)
	//	defer stopServers(servers)
	//
	//	assert.Equal(t, http.StatusOK, makeRequest(t, servers[0].URL()+"/bundles/bundle1").StatusCode)
	//	assert.Equal(t, http.StatusServiceUnavailable, makeRequest(t, servers[1].URL()+"/bundles/bundle2").StatusCode)
	//	assert.GreaterOrEqual(t, measureRequestTime(t, servers[2].URL()+"/bundles/bundle3"), 25*time.Millisecond)
	//})

	t.Run("Handles invalid bundle requests", func(t *testing.T) {
		cbs := StartControllableBundleServer(BundleServerConfig{BundleName: "testbundle", RespCode: http.StatusOK})
		defer cbs.Stop()

		assert.Equal(t, http.StatusNotFound, makeRequest(t, cbs.URL()+"/bundles/nonexistent").StatusCode)
	})
}

// Helper functions
func makeRequest(t *testing.T, url string) *http.Response {
	resp, err := http.Get(url)
	require.NoError(t, err)
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func measureRequestTime(t *testing.T, url string) time.Duration {
	start := time.Now()
	resp := makeRequest(t, url)
	defer resp.Body.Close()
	return time.Since(start)
}

func TestMultiBundleProxyServer_Control(t *testing.T) {
	configs := []BundleServerConfig{
		{BundleName: "bundle1", RespCode: http.StatusOK, Delay: 0},
		{BundleName: "bundle2", RespCode: http.StatusServiceUnavailable, Delay: 0},
	}
	proxy, controller := StartMultiBundleProxyServer(configs)
	defer proxy.Close()

	// bundle1 should be OK
	resp1 := makeRequest(t, proxy.URL+"/bundles/bundle1")
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	// bundle2 should be unavailable
	resp2 := makeRequest(t, proxy.URL+"/bundles/bundle2")
	assert.Equal(t, http.StatusServiceUnavailable, resp2.StatusCode)

	// Recover bundle2
	controller.SetStatus("bundle2", http.StatusOK)
	resp2ok := makeRequest(t, proxy.URL+"/bundles/bundle2")
	assert.Equal(t, http.StatusOK, resp2ok.StatusCode)

	// Set delay for bundle1
	controller.SetDelay("bundle1", 100*time.Millisecond)
	start := time.Now()
	resp1delayed := makeRequest(t, proxy.URL+"/bundles/bundle1")
	assert.Equal(t, http.StatusOK, resp1delayed.StatusCode)
	assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond)
}
