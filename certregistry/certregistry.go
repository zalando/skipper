package certregistry

import (
	"crypto/tls"
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
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

func (r *CertRegistry) GetCertByKey(key string) (*tls.Certificate, error) {
	var err error

	r.mx.Lock()
	defer r.mx.Unlock()

	cert, ok := r.lookup[key]
	if !ok || cert == nil {
		log.Debugf("certificate not found in store - %s", key)
		return nil, err
	}
	fmt.Println(cert)
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
	_, err := r.GetCertByKey(key)
	if err == nil {
		log.Debugf("updating certificate in registry - %s", key)
		r.AddCert(key, cert)
		return
	}
	
	r.AddCert(key, cert)
}
