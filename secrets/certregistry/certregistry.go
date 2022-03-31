package certregistry

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

// CertRegistry object holds TLS certificates to be used to terminate TLS connections
// ensuring syncronized access to them.
type CertRegistry struct {
	lookup map[string]*tls.Certificate
	mx     *sync.Mutex
}

// NewCertRegistry initializes the certificate registry.
func NewCertRegistry() *CertRegistry {
	l := make(map[string]*tls.Certificate)

	return &CertRegistry{
		lookup: l,
		mx:     &sync.Mutex{},
	}
}

// Configures certificate for the host if no configuration exists or
// if certificate is valid (`NotBefore` field) after previously configured certificate.
func (r *CertRegistry) ConfigureCertificate(key string, cert *tls.Certificate) error {
	if cert == nil {
		return fmt.Errorf("cannot configure nil certificate")
	}
	// loading parsed leaf certificate to certificate
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("failed parsing leaf certificate")
	}
	cert.Leaf = leaf

	r.mx.Lock()
	defer r.mx.Unlock()

	curr, found := r.lookup[key]
	if found {
		if cert.Leaf.NotBefore.After(curr.Leaf.NotBefore) {
			log.Infof("updating certificate in registry - %s", key)
			r.lookup[key] = cert
			return nil
		} else {
			return nil
		}
	} else {
		log.Infof("adding certificate to registry - %s", key)
		r.lookup[key] = cert
		return nil
	}
}

// GetCertFromHello reads the SNI from a TLS client and returns the appropriate certificate.
// If no certificate is found for the host it will return nil.
func (r *CertRegistry) GetCertFromHello(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mx.Lock()
	defer r.mx.Unlock()

	cert, found := r.lookup[hello.ServerName]
	if found {
		return cert, nil
	}
	return nil, nil
}
