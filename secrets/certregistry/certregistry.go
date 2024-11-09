package certregistry

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

// CertRegistryEntry holds a certificate and tls configuration.
type CertRegistryEntry struct {
	Certificate *tls.Certificate
	Config      *tls.Config
}

// CertRegistry object holds TLS certificates to be used to terminate TLS connections
// ensuring synchronized access to them.
type CertRegistry struct {
	mu     sync.Mutex
	lookup map[string]*CertRegistryEntry
}

// NewCertRegistry initializes the certificate registry.
func NewCertRegistry() *CertRegistry {
	l := make(map[string]*CertRegistryEntry)

	return &CertRegistry{
		lookup: l,
	}
}

// Configures certificate for the host if no configuration exists or
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
		if cert.Leaf.NotBefore.After(curr.Certificate.Leaf.NotBefore) {
			log.Infof("updating certificate in registry - %s", host)
			r.lookup[host].Certificate = cert
			return nil
		} else {
			return nil
		}
	} else {
		log.Infof("adding certificate to registry - %s", host)
		r.lookup[host].Certificate = cert
		return nil
	}
}

// ConfigureTLSConfig configures a tls config for the host.
func (r *CertRegistry) ConfigureTLSConfig(host string, config *tls.Config) error {
	if config == nil {
		return fmt.Errorf("cannot configure nil tls config")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	curr, found := r.lookup[host]

	if found && curr.Config != nil {
		log.Infof("updating tls config for host - %s", host)
		r.lookup[host].Config = config
		return nil
	} else {
		log.Infof("adding tls config for host - %s", host)
		r.lookup[host] = &CertRegistryEntry{
			Config: config,
		}
		return nil
	}
}

// GetCertFromHello reads the SNI from a TLS client and returns the appropriate certificate.
// If no certificate is found for the host it will return nil.
func (r *CertRegistry) GetCertFromHello(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	entry, found := r.lookup[hello.ServerName]
	r.mu.Unlock()
	if found {
		return entry.Certificate, nil
	}
	return nil, nil
}

// GetConfigFromHello reads the SNI from a TLS client and returns the appropriate config.
func (r *CertRegistry) GetConfigFromHello(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	r.mu.Lock()
	entry, found := r.lookup[hello.ServerName]
	r.mu.Unlock()
	if found {
		return entry.Config, nil
	}
	return entry.Config, nil
}
