// Package nitro provides TLS certificates backed by AWS ACM for Nitro Enclaves.
package nitro

import (
	"crypto/tls"
	"time"
)

// DefaultRefreshInterval is the default ACM certificate refresh interval.
const DefaultRefreshInterval = time.Hour

// Config contains the ACM and PKCS#11 settings for an enclave-backed certificate.
type Config struct {
	CertificateARN  string
	ModulePath      string
	TokenLabel      string
	PIN             string
	RefreshInterval time.Duration
}

// Provider supplies TLS certificates and releases its resources when closed.
type Provider interface {
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
	Close() error
}
