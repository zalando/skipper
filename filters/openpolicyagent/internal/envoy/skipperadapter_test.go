package envoy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdaptToExtAuthRequest_BodyTruncation(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://example.com/test", nil)
	require.NoError(t, err)

	t.Run("truncated body sets x-envoy-auth-partial-body header", func(t *testing.T) {
		checkReq, err := AdaptToExtAuthRequest(req, nil, nil, []byte("partial"), true)
		require.NoError(t, err)
		assert.Equal(t, "true", checkReq.GetAttributes().GetRequest().GetHttp().GetHeaders()["x-envoy-auth-partial-body"])
	})

	t.Run("non-truncated body sets x-envoy-auth-partial-body to false", func(t *testing.T) {
		checkReq, err := AdaptToExtAuthRequest(req, nil, nil, []byte("full body"), false)
		require.NoError(t, err)
		assert.Equal(t, "false", checkReq.GetAttributes().GetRequest().GetHttp().GetHeaders()["x-envoy-auth-partial-body"])
	})
}
