package opatestutils

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"testing"
)

// ============================================================================
// TEST INFRASTRUCTURE VALIDATION
// ============================================================================
func TestControllableBundleServer(t *testing.T) {
	bundleName := "testbundle"
	cbs := StartControllableBundleServer(bundleName)
	defer cbs.Stop()

	url := cbs.URL() + "/bundles/" + bundleName

	// Initially unavailable → expect 429
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "temporarily unavailable")

	// Set available → expect 200 and bundle content
	cbs.SetAvailable(true)

	resp2, err := http.Get(url)
	require.NoError(t, err)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusOK, resp2.StatusCode)
	require.NoError(t, err)
	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	assert.NotEmpty(t, body2, "Expected non-empty bundle content")
}
