//go:build !awsnitro || !linux

package nitro

import "github.com/zalando/skipper/secrets/certregistry"

// Provider manages certificates and TLS signing via an AWS Nitro Enclave.
type Provider struct{}

// NewProvider returns ErrUnsupported when built without the awsnitro build tag.
func NewProvider(_ Options, _ *certregistry.CertRegistry) (*Provider, error) {
	return nil, ErrUnsupported
}

// Close is a no-op when built without the awsnitro build tag.
func (p *Provider) Close() error {
	return nil
}
