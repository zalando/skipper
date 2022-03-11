package certregistry

import (
	"crypto/tls"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/cache"
)

type CertRegistry struct {
	cache.ThreadSafeStore
}

func NewCertRegistry() *CertRegistry {
	return &CertRegistry{
		cache.NewThreadSafeStore(cache.Indexers{}, cache.Indices{}),
	}
}

func (r CertRegistry) GetCertByKey(key string) (*tls.Certificate, error) {
	var err error
	cert, ok := r.Get(key)
	if !ok {
		log.Debugf("certificate not found in store - %s", key)
		return nil, err
	}
	return cert.(*tls.Certificate), nil
}

func (r *CertRegistry) SyncCert(key string, cert *tls.Certificate) {
	log.Debugf("syncing certificate to registry - %s", key)
	_, err := r.GetCertByKey(key)
	if err == nil {
		log.Debugf("updating certificate in registry - %s", key)
		r.Update(key, cert)
		return
	}
	log.Debugf("adding certificate to registry - %s", key)
	r.Add(key, cert)
}
