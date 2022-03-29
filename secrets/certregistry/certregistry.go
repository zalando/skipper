package certregistry

import (
	"crypto/tls"
	"crypto/x509"
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

func (r *CertRegistry) getCertByKey(key string) (*tls.Certificate, bool) {
	r.mx.Lock()
	defer r.mx.Unlock()

	cert, found := r.lookup[key]
	if !found {
		log.Debugf("certificate not found in registry - %s", key)
		return nil, false
	}

	return cert, true
}

func (r *CertRegistry) addCertToRegistry(key string, cert *tls.Certificate) error {
	r.mx.Lock()
	defer r.mx.Unlock()

	r.lookup[key] = cert

	return nil
}

// SyncCert takes a host and TLS certificate and saves them to the registry with the
// host as the key. If the cert already exists it will be updated or added otherwise.
func (r *CertRegistry) SyncCert(host string, cert *tls.Certificate) {
	if cert == nil {
		log.Errorf("cannot sync nil certificate")
		return
	}
	// loading parsed leaf certificate to certificate
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		log.Errorf("failed parsing leaf certificate for %s", host)
		return
	}
	cert.Leaf = leaf

	curr, found := r.getCertByKey(host)
	if found {
		if curr.Leaf.Equal(cert.Leaf) {
			return
		}
		log.Infof("updating existing certificate in registry - %s", host)
		curr, err := chooseBestCertificate(curr, cert)
		if err != nil {
			log.Warnf("choosing best certificate for %s failed, keeping current", host)
			return
		}
		r.addCertToRegistry(host, curr)
	} else {
		log.Infof("adding certificate to registry - %s", host)
		r.addCertToRegistry(host, cert)
	}

}

// GetCertFromHello reads the SNI from a TLS client and returns the appropriate certificate.
// If no certificate is found for the host it will return nil.
func (r *CertRegistry) GetCertFromHello(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert, found := r.getCertByKey(hello.ServerName)
	if found {
		return cert, nil
	}
	return nil, nil
}

// chooseBestCertificate compares two certificates and returns the newest certificate from
// NotBefore date.
func chooseBestCertificate(l *tls.Certificate, r *tls.Certificate) (*tls.Certificate, error) {
	if l.Leaf.NotBefore.After(r.Leaf.NotBefore) {
		return l, nil
	} else {
		return r, nil
	}
}
