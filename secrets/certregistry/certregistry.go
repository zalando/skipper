// Package certregistry provides building blocks to have more than one
// certificate and use SNI to select the right certificate for the
// request.
package certregistry

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

// Priority of a certificate in the registry. A certificate with a
// higher priority always replaces a certificate with a lower priority
// for the same host and is never replaced by one with a lower priority.
type Priority int

const (
	// PriorityDefault is used by ConfigureCertificate.
	PriorityDefault Priority = 0
	// PriorityNitro is used for certificates with private keys in an
	// AWS Nitro Enclave.
	PriorityNitro Priority = 100
)

type entry struct {
	cert     *tls.Certificate
	priority Priority
}

// CertRegistry object holds TLS certificates to be used to terminate TLS connections
// ensuring synchronized access to them.
type CertRegistry struct {
	mu     sync.Mutex
	lookup map[string]*entry
}

// NewCertRegistry initializes the certificate registry.
func NewCertRegistry() *CertRegistry {
	return &CertRegistry{
		lookup: make(map[string]*entry),
	}
}

// ConfigureCertificate for the host with PriorityDefault if no configuration exists or
// if certificate is valid (`NotBefore` field) after previously configured certificate.
func (r *CertRegistry) ConfigureCertificate(host string, cert *tls.Certificate) error {
	return r.ConfigureCertificateWithPriority(host, cert, PriorityDefault)
}

// ConfigureCertificateWithPriority configures the certificate for the
// host. An existing certificate is replaced if the new one has a higher
// priority, or the same priority and a later `NotBefore`.
func (r *CertRegistry) ConfigureCertificateWithPriority(host string, cert *tls.Certificate, priority Priority) error {
	if cert == nil {
		return fmt.Errorf("cannot configure nil certificate")
	}
	if len(cert.Certificate) == 0 {
		return fmt.Errorf("cannot configure certificate without leaf")
	}
	// loading parsed leaf certificate to certificate
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("failed parsing leaf certificate: %w", err)
	}
	cert.Leaf = leaf
	host = normalizeHost(host)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.lookup == nil {
		r.lookup = make(map[string]*entry)
	}

	curr, found := r.lookup[host]
	if !found {
		log.Infof("adding certificate to registry - %s", host)
		r.lookup[host] = &entry{cert: cert, priority: priority}
		return nil
	}

	if priority > curr.priority ||
		priority == curr.priority && cert.Leaf.NotBefore.After(curr.cert.Leaf.NotBefore) {
		log.Infof("updating certificate in registry - %s", host)
		r.lookup[host] = &entry{cert: cert, priority: priority}
	}
	return nil
}

// GetCertFromHello reads the SNI from a TLS client and returns the appropriate certificate.
// An exact host match is preferred over a wildcard match of the parent domain,
// see RFC 6125 section 6.4.3.
// If no certificate is found for the host it will return nil.
func (r *CertRegistry) GetCertFromHello(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	host := normalizeHost(hello.ServerName)
	if host == "" {
		return nil, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if e, found := r.lookup[host]; found {
		return e.cert, nil
	}
	if _, parent, ok := strings.Cut(host, "."); ok && parent != "" {
		if e, found := r.lookup["*."+parent]; found {
			return e.cert, nil
		}
	}
	return nil, nil
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(host, "."))
}
