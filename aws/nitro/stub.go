//go:build !awsnitro

package nitro

import "github.com/zalando/skipper/secrets/certregistry"

// Provider loads certificates with private keys in an AWS Nitro Enclave.
type Provider struct{}

// NewProvider returns ErrNotSupported, because the binary was built
// without the "awsnitro" build tag.
func NewProvider(Options, *certregistry.CertRegistry) (*Provider, error) {
	return nil, ErrNotSupported
}

// Close is a no-op.
func (*Provider) Close() error { return nil }
