package certregistry

import (
	"crypto/tls"
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	defaultHost = "ingress.local"
)

type tlsCertificate struct {
	hosts []string
	cert  *tls.Certificate
}

// CertRegistry object holds TLS certificates to be used to terminate TLS connections
// We ensure ensure syncronized access to them and hold a default certificate.
type CertRegistry struct {
	lookup         map[string]*tlsCertificate
	mx             *sync.Mutex
	defaultTLSCert *tls.Certificate
}

// NewCertRegistry initializes the certificate registry with an empty map
// and a generated default certificate.
func NewCertRegistry() *CertRegistry {
	l := make(map[string]*tlsCertificate)

	return &CertRegistry{
		lookup:         l,
		mx:             &sync.Mutex{},
		defaultTLSCert: getFakeHostTLSCert(defaultHost),
	}
}

func (r *CertRegistry) getCertByKey(key string) (*tlsCertificate, bool) {
	r.mx.Lock()
	defer r.mx.Unlock()

	cert, ok := r.lookup[key]
	if !ok || cert == nil {
		log.Debugf("certificate not found in registry - %s", key)
		return nil, false
	}

	return cert, true
}

func (r *CertRegistry) addCertToRegistry(key string, cert *tlsCertificate) {
	r.mx.Lock()
	defer r.mx.Unlock()

	r.lookup[key] = cert
}

// SyncCert takes a TLS certificate and list of hosts and saves them to the registry with the
// provided key. If the object already exists it will be updated or added otherwise. Returns
// true if key was changed.
func (r *CertRegistry) SyncCert(key string, hosts []string, crt *tls.Certificate) bool {
	cert := &tlsCertificate{
		hosts: hosts,
		cert:  crt,
	}

	curr, found := r.getCertByKey(key)
	if found {
		if !equalCert(curr, cert) {
			log.Debugf("updating certificate in registry - %s", key)
			r.addCertToRegistry(key, cert)
			return true
		} else {
			return false
		}
	} else {
		log.Debugf("adding certificate to registry - %s", key)
		r.addCertToRegistry(key, cert)
		return true
	}

}

// GetCertFromHello reads the SNI from a TLS client and returns the appropriate certificate.
// If no certificate is found for the host it will return a default certificate.
func (r *CertRegistry) GetCertFromHello(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	for _, cert := range r.lookup {
		for _, host := range cert.hosts {
			if hello.ServerName == host {
				return cert.cert, nil
			}
		}
	}
	return r.defaultTLSCert, nil
}
