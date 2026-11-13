//go:build !opa

package skipper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOpenPolicyAgentWithoutBuildTag(t *testing.T) {
	opa, err := newOpenPolicyAgent(&Options{}, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, opa.filters)
	assert.Empty(t, opa.preProcessors)
	assert.Empty(t, opa.postProcessors)

	_, err = newOpenPolicyAgent(&Options{EnableOpenPolicyAgent: true}, nil, nil)
	assert.ErrorContains(t, err, "-tags opa")
}
