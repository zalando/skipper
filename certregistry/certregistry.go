package certregistry

import (
	"crypto/tls"
	"errors"
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	errCertNotFound = errors.New("certificate not found")
)

type CertRegistry struct {
	lookup map[string]*tls.Certificate
	mx     *sync.Mutex
}

func NewCertRegistry() *CertRegistry {
	return &CertRegistry{
		lookup: make(map[string]*tls.Certificate),
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

func (r *CertRegistry) AddCert(key string, cert *tls.Certificate) {
	r.mx.Lock()
	defer r.mx.Unlock()

	log.Debugf("adding certificate to registry - %s", key)

	r.lookup[key] = cert
}

func (r *CertRegistry) SyncCert(key string, cert *tls.Certificate) {	
	log.Debugf("syncing certificate to registry - %s", key)
	_, err := r.getCertByKey(key)
	if err == nil {
		log.Debugf("updating certificate in registry - %s", key)
		r.AddCert(key, cert)
		return
	}
	
	r.AddCert(key, cert)
}

func (r *CertRegistry) GetCertFromHello(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	key := hello.ServerName
	cert, err := r.getCertByKey(key)
	if err != nil {
		return nil, nil
	}
	return cert, nil
}