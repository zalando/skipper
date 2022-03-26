package certregistry

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	defaultHost           = "ingress.local"
	errSyncNilCertificate = errors.New("empty certificate cannot sync")
)

// CertRegistry object holds TLS certificates to be used to terminate TLS connections
// We ensure ensure syncronized access to them and hold a default certificate.
type CertRegistry struct {
	lookup         map[string]*tls.Certificate
	mx             *sync.Mutex
	defaultTLSCert *tls.Certificate
}

// NewCertRegistry initializes the certificate registry with an empty map
// and a generated default certificate.
func NewCertRegistry() *CertRegistry {
	l := make(map[string]*tls.Certificate)

	return &CertRegistry{
		lookup:         l,
		mx:             &sync.Mutex{},
		defaultTLSCert: getFakeHostTLSCert(defaultHost),
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

	if cert == nil {
		log.Errorf("cannot sync nil certificate")
		return errSyncNilCertificate
	}

	r.lookup[key] = cert

	return nil
}

// SyncCert takes a TLS certificate and list of hosts and saves them to the registry with the
// provided key. If the object already exists it will be updated or added otherwise. Returns
// true if key was changed.
func (r *CertRegistry) SyncCert(host string, cert *tls.Certificate) {
	curr, found := r.getCertByKey(host)
	if found {
		log.Debugf("updating existing certificate in registry - %s", host)
		curr, err := chooseBestCertificate(curr, cert)
		if err != nil {
			log.Warnf("choosing best certificate for %s failed, keeping current", host)
			return
		}
		r.addCertToRegistry(host, curr)
	} else {
		log.Debugf("adding certificate to registry - %s", host)
		r.addCertToRegistry(host, cert)
	}

}

// GetCertFromHello reads the SNI from a TLS client and returns the appropriate certificate.
// If no certificate is found for the host it will return a default certificate.
func (r *CertRegistry) GetCertFromHello(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert, found := r.getCertByKey(hello.ServerName)
	if found {
		return cert, nil
	} else {
		return r.defaultTLSCert, nil
	}
}

// chooseBestCertificate compares two certificates and returns the newest certificate from
// NotBefore date.
func chooseBestCertificate(l *tls.Certificate, r *tls.Certificate) (*tls.Certificate, error) {
	lcert, err := x509.ParseCertificate(l.Certificate[0])
	if err != nil {
		return nil, err
	}

	rcert, err := x509.ParseCertificate(r.Certificate[0])
	if err != nil {
		return nil, err
	}

	lNotBefore := lcert.NotBefore
	rNotBefore := rcert.NotBefore

	if lNotBefore.After(rNotBefore) {
		return l, nil
	} else {
		return r, nil
	}
}
