package certregistry

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"
)

func getFakeHostTLSCert(host string) *tls.Certificate {
	var priv interface{}
	var err error

	priv, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Errorf("failed to generate fake private key: %v", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Errorf("failed to generate fake serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
			CommonName:   "Kubernetes Ingress Controller Fake Certificate",
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{host},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.(*rsa.PrivateKey).PublicKey, priv)
	if err != nil {
		log.Fatalf("Failed to create fake certificate: %v", err)
	}

	crt := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	key := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv.(*rsa.PrivateKey))})

	cert, err := tls.X509KeyPair([]byte(crt), []byte(key))
	if err != nil {
		log.Errorf("failed to generate fake serial number: %v", err)
	}

	return &cert
}

func equalCert(l *tlsCertificate, r *tlsCertificate) bool {
	if !reflect.DeepEqual(l.hosts, r.hosts) {
		return false
	}

	if !reflect.DeepEqual(l.cert.Certificate, r.cert.Certificate) {
		return false
	}

	if !reflect.DeepEqual(l.cert.PrivateKey, r.cert.PrivateKey) {
		return false
	}

	return true
}