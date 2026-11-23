//go:build !awsnitro

package nitro

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zalando/skipper/secrets/certregistry"
)

func TestNewProviderNotSupported(t *testing.T) {
	p, err := NewProvider(Options{}, certregistry.NewCertRegistry())
	assert.ErrorIs(t, err, ErrNotSupported)
	assert.Nil(t, p)
	assert.NoError(t, p.Close())
}
