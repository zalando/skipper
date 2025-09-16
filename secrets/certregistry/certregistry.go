// Package certregistry provides building blocks to have more than one
// certificate and use SNI to select the right certificate for the
// request.
package certregistry

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

// CertRegistry object holds TLS certificates to be used to terminate TLS connections
// ensuring synchronized access to them.
type CertRegistry struct {
	mu     sync.Mutex
	lookup map[string]*tls.Certificate
}

// NewCertRegistry initializes the certificate registry.
func NewCertRegistry() *CertRegistry {
	l := make(map[string]*tls.Certificate)

	return &CertRegistry{
		lookup: l,
	}
}

// ConfigureCertificate for the host if no configuration exists or
// if certificate is valid (`NotBefore` field) after previously configured certificate.
func (r *CertRegistry) ConfigureCertificate(host string, cert *tls.Certificate) error {
	if cert == nil {
		return fmt.Errorf("cannot configure nil certificate")
	}
	// loading parsed leaf certificate to certificate
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("failed parsing leaf certificate: %w", err)
	}
	cert.Leaf = leaf

	r.mu.Lock()
	defer r.mu.Unlock()

	curr, found := r.lookup[host]
	if found {
		if cert.Leaf.NotBefore.After(curr.Leaf.NotBefore) {
			log.Infof("updating certificate in registry - %s", host)
			r.lookup[host] = cert
			return nil
		} else {
			return nil
		}
	} else {
		log.Infof("adding certificate to registry - %s", host)
		r.lookup[host] = cert
		return nil
	}
}

// GetCertFromHello reads the SNI from a TLS client and returns the appropriate certificate.
// If no certificate is found for the host it will return nil.
func (r *CertRegistry) GetCertFromHello(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	cert, found := r.lookup[hello.ServerName]
	r.mu.Unlock()
	if found {
		return cert, nil
	}
	return nil, nil
}
