//go:build !awsnitro

package nitro

import "crypto/tls"

// Provider supplies TLS certificates backed by an AWS Nitro Enclave.
type Provider struct{}

// New returns ErrNotSupported when built without the "awsnitro" build tag.
func New(Options) (*Provider, error) {
	return nil, ErrNotSupported
}

// GetCertificate returns ErrNotSupported when built without the "awsnitro" build tag.
func (*Provider) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return nil, ErrNotSupported
}

// Close is a no-op when built without the "awsnitro" build tag.
func (*Provider) Close() error {
	return nil
}
