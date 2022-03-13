package certregistry

import (
	"crypto/tls"
	"errors"
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	errCertNotFound = errors.New("certificate not found")
	defaultHost     = "ingress.local"
)

type CertRegistry struct {
	lookup map[string]*tls.Certificate
	mx     *sync.Mutex
}

func NewCertRegistry() *CertRegistry {
	cert := getFakeHostTLSCert(defaultHost)
	l := make(map[string]*tls.Certificate)
	
	l[defaultHost] = cert

	return &CertRegistry{
		lookup: l,
		mx:     &sync.Mutex{},
	}
}

func (r *CertRegistry) getCertByKey(key string) (*tls.Certificate, error) {
	r.mx.Lock()
	defer r.mx.Unlock()

	cert, ok := r.lookup[key]
	if !ok || cert == nil {
		log.Debugf("certificate not found in registry - %s", key)
		return nil, errCertNotFound
	}
	
	return cert, nil
}

func (r *CertRegistry) addCert(key string, cert *tls.Certificate) {
	r.mx.Lock()
	defer r.mx.Unlock()

	r.lookup[key] = cert
}

func (r *CertRegistry) SyncCert(key string, cert *tls.Certificate) {	
	log.Debugf("syncing certificate to registry - %s", key)
	_, err := r.getCertByKey(key)
	if err == nil {
		log.Debugf("updating certificate in registry - %s", key)
		r.addCert(key, cert)
		return
	}

	log.Debugf("adding certificate to registry - %s", key)
	r.addCert(key, cert)
}

func (r *CertRegistry) GetCertFromHello(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert, err := r.getCertByKey(hello.ServerName)
	if err != nil {
		return r.getCertByKey(defaultHost)
	}
	return cert, nil
}
