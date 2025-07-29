package openpolicyagent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
)

func TestOPABootstrapWithPreProcessor_ValidBundle(t *testing.T) {
	registry := setupRegistry(t)
	defer registry.Close()

	routes := eskip.MustParse(`r1: Path("/initial") -> opaAuthorizeRequest("test", "") -> status(204) -> <shunt>`)
	processed := registry.NewPreProcessor().Do(routes)

	require.Len(t, processed, 1)
	assert.True(t, hasOpaAuthorizeRequest(processed[0].Filters), "missing opaAuthorizeRequest")
	assertStatusFilter(t, processed[0].Filters, 204)

	inst, err := registry.GetOrStartInstance("test", "test")
	require.NoError(t, err)
	require.NotNil(t, inst)
	assert.Equal(t, "test", inst.bundleName)
}

func TestOPABootstrapWithPreProcessor_InvalidAndValidBundle(t *testing.T) {
	registry := setupRegistry(t)
	defer registry.Close()

	routes := eskip.MustParse(`
		r1: Path("/initial") -> opaAuthorizeRequest("test", "") -> status(204) -> <shunt>;
		r2: Path("/fail") -> opaAuthorizeRequest("invalid", "") -> status(403) -> <shunt>;
		r3: Path("/another") -> status(200) -> <shunt>;`)

	processed := registry.NewPreProcessor().Do(routes)
	require.Len(t, processed, 3)

	t.Run("should process route filters correctly", func(t *testing.T) {
		assert.True(t, hasOpaAuthorizeRequest(processed[0].Filters), "r1 should have opaAuthorizeRequest")
		assert.True(t, hasOpaAuthorizeRequest(processed[1].Filters), "r2 should have opaAuthorizeRequest")

		assertStatusFilter(t, processed[0].Filters, 204)
		assertStatusFilter(t, processed[1].Filters, 403)
		assertStatusFilter(t, processed[2].Filters, 200)
	})

	t.Run("valid bundle 'test' should succeed", func(t *testing.T) {
		inst, err := registry.GetOrStartInstance("test", "opaAuthorizeRequest")
		require.NoError(t, err)
		require.NotNil(t, inst)
		assert.Equal(t, "test", inst.bundleName)
	})

	t.Run("invalid bundle should fail with bootstrap timeout", func(t *testing.T) {
		inst, err := registry.GetOrStartInstance("invalid", "opaAuthorizeRequest") //bundle load is retried for 30s
		require.Error(t, err)
		assert.Nil(t, inst)
		assert.Contains(t, err.Error(), "open policy agent instance for bundle 'invalid' is not ready yet")
	})
}

// --- Helpers ---

func setupRegistry(t *testing.T) *OpenPolicyAgentRegistry {
	t.Helper()

	_, config := mockControlPlaneWithResourceBundle()
	registry, err := NewOpenPolicyAgentRegistry(
		WithReuseDuration(1*time.Second),
		WithCleanInterval(1*time.Second),
		WithPreloadingEnabled(true),
		WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)),
	)
	require.NoError(t, err)
	return registry
}

func hasOpaAuthorizeRequest(filters []*eskip.Filter) bool {
	for _, f := range filters {
		if f.Name == "opaAuthorizeRequest" {
			return true
		}
	}
	return false
}

func assertStatusFilter(t *testing.T, filters []*eskip.Filter, expectedCode int) {
	t.Helper()
	for _, f := range filters {
		if f.Name == "status" && len(f.Args) > 0 {
			if code, ok := f.Args[0].(float64); ok {
				assert.Equal(t, float64(expectedCode), code, "unexpected status code")
				return
			}
		}
	}
	t.Errorf("status(%d) filter not found", expectedCode)
}
