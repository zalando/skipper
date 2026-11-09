//go:build !awsnitro

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAWSNitroEnableRequiresBuildTag(t *testing.T) {
	cfg := NewConfig()
	err := cfg.ParseArgs("skipper", []string{
		"-enable-aws-nitro",
		"-aws-nitro-certificate-arn=arn:aws:acm:us-east-1:123456789012:certificate/example",
		"-aws-nitro-pkcs11-module=/usr/lib/p11-kit-client.so",
		"-aws-nitro-pkcs11-token-label=nitro-token",
	})
	require.ErrorContains(t, err, "-tags awsnitro")
}

func TestAWSNitroRejectsZeroRefreshInterval(t *testing.T) {
	cfg := NewConfig()
	err := cfg.ParseArgs("skipper", []string{
		"-enable-aws-nitro",
		"-aws-nitro-certificate-arn=arn:aws:acm:us-east-1:123456789012:certificate/example",
		"-aws-nitro-pkcs11-module=/usr/lib/p11-kit-client.so",
		"-aws-nitro-pkcs11-token-label=nitro-token",
		"-aws-nitro-certificate-refresh-interval=0",
	})
	require.ErrorContains(t, err, "must be greater than zero")
}
