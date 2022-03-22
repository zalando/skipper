package certregistry

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	defaultHost = "ingress.local"
	currentTime = time.Now
	// ErrNoMatchingCertificateFound is used if there is no matching certificate found
	ErrNoMatchingCertificateFound = errors.New("no matching certificate found")
)

const (
	// used as wildcard char in Cert Hostname/AltName matches
	glob = "*"
	// minimal time period for the NotAfter attribute of a Cert to be in the future
	minimalCertValidityPeriod = 7 * 24 * time.Hour
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
	var certList = make([]*tlsCertificate, 0)
	for _, cert := range r.lookup {
		for _, host := range cert.hosts {
			if hello.ServerName == host {
				certList = append(certList, cert)
			}
		}
	}

	switch len(certList) {
	case 0:
		// no certs found so returning the default certificate
		return r.defaultTLSCert, nil
	case 1:
		// only one certificate found, retuning it
		return certList[0].cert, nil
	default:
		// more that one cert found, returning the best match
		cert, err := getBestMatchingCertificate(hello.ServerName, certList)
		if err != nil {
			return nil, err
		}
		return cert, nil
	}
}

// getBestMatchingCertificate uses a suffix search, best match operation, in order to find the best matching
// certificate for a given hostname.
func getBestMatchingCertificate(host string, certList []*tlsCertificate) (*tls.Certificate, error) {
	candidate := certList[0].cert
	longestMatch := -1
	now := currentTime()

	for _, cert := range certList {
		curr, err := x509.ParseCertificate(cert.cert.Certificate[0])
		if err != nil {
			return nil, err
		}

		notAfter := curr.NotAfter
		notBefore := curr.NotBefore

		for _, altName := range curr.DNSNames {
			if prefixGlob(altName, host) {
				nameLength := len(altName)

				switch {
				case longestMatch < 0:
					// first matching found
					longestMatch = nameLength
					candidate = cert.cert
				case longestMatch < nameLength:
					if notBefore.Before(now) && notAfter.Add(-minimalCertValidityPeriod).After(now) {
						// more specific valid cert found: *.example.org -> foo.example.org
						longestMatch = nameLength
						candidate = cert.cert
					}
				case longestMatch == nameLength:
					if notBefore.After(candidate.Leaf.NotBefore) &&
						!notAfter.Add(-minimalCertValidityPeriod).Before(now) {
						// cert is newer than curBestCert and is not invalid in 7 days
						longestMatch = nameLength
						candidate = cert.cert
					} else if notBefore.Equal(candidate.Leaf.NotBefore) && !candidate.Leaf.NotAfter.After(notAfter) {
						// cert has the same issue date, but is longer valid
						longestMatch = nameLength
						candidate = cert.cert
					} else if notBefore.Before(candidate.Leaf.NotBefore) &&
						candidate.Leaf.NotAfter.Add(-minimalCertValidityPeriod).Before(now) &&
						notAfter.After(candidate.Leaf.NotAfter) {
						// cert is older than curBestCert but curBestCert is invalid in 7 days and cert is longer valid
						longestMatch = nameLength
						candidate = cert.cert
					}
				case longestMatch > nameLength:
					if candidate.Leaf.NotAfter.Add(-minimalCertValidityPeriod).Before(now) &&
						now.Before(candidate.Leaf.NotBefore) &&
						notBefore.Before(now) &&
						now.Before(notAfter.Add(-minimalCertValidityPeriod)) {
						// foo.example.org -> *.example.org degradation when NotAfter requires a downgrade
						longestMatch = nameLength
						candidate = cert.cert
					}
				}
				candidate.Leaf, err = x509.ParseCertificate(candidate.Certificate[0])
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return candidate, nil
}

func prefixGlob(pattern, subj string) bool {
	// Empty pattern can only match empty subject
	if pattern == "" {
		return subj == pattern
	}

	// If the pattern _is_ a glob, it matches everything
	if pattern == glob {
		return true
	}

	leadingGlob := strings.HasPrefix(pattern, glob)

	if !leadingGlob {
		// No globs in pattern, so test for equality
		return subj == pattern
	}

	pat := string(pattern[1:])
	trimmedSubj := strings.TrimSuffix(subj, pat)
	return !strings.Contains(trimmedSubj, ".")
}
