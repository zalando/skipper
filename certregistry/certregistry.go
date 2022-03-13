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

type tlsCertificate struct {
	hosts []string
	cert  *tls.Certificate
}

type CertRegistry struct {
	lookup         map[string]*tlsCertificate
	mx             *sync.Mutex
	defaultTLSCert *tls.Certificate
}

func NewCertRegistry() *CertRegistry {
	l := make(map[string]*tlsCertificate)

	return &CertRegistry{
		lookup:         l,
		mx:             &sync.Mutex{},
		defaultTLSCert: getFakeHostTLSCert(defaultHost),
	}
}

func (r *CertRegistry) getCertByKey(key string) (*tlsCertificate, error) {
	r.mx.Lock()
	defer r.mx.Unlock()

	cert, ok := r.lookup[key]
	if !ok || cert == nil {
		log.Debugf("certificate not found in registry - %s", key)
		return nil, errCertNotFound
	}
	
	return cert, nil
}

func (r *CertRegistry) addCertToRegistry(key string, cert *tlsCertificate) {
	r.mx.Lock()
	defer r.mx.Unlock()

	r.lookup[key] = cert
}

func (r *CertRegistry) SyncCert(key string, hosts []string, crt *tls.Certificate) {
	cert := &tlsCertificate{
		hosts: hosts, 
		cert: crt,
	}	

	_, err := r.getCertByKey(key)
	if err == nil {
		log.Debugf("updating certificate in registry - %s", key)
		r.addCertToRegistry(key, cert)
		return
	}

	log.Debugf("adding certificate to registry - %s", key)
	r.addCertToRegistry(key, cert)
}

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
