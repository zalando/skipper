//go:build !awsnitro

package nitro

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewWithoutBuildTag(t *testing.T) {
	_, err := New(Config{})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "-tags awsnitro"))
}
