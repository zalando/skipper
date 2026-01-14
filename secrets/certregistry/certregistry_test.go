package certregistry

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"reflect"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const (
	tenYears = time.Hour * 24 * 365 * 10
)

type caInfra struct {
	sync.Once
	err       error
	chainKey  *rsa.PrivateKey
	chainCert *x509.Certificate
}

var ca = caInfra{}

func createDummyCertDetail(t *testing.T, arn string, altNames []string, notBefore, notAfter time.Time) *tls.Certificate {
	ca.Do(func() {
		caKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			ca.err = fmt.Errorf("unable to generate CA key: %w", err)
			return
		}

		caCert := x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject: pkix.Name{
				Organization: []string{"Testing CA"},
			},
			NotBefore: time.Time{},
			NotAfter:  time.Now().Add(tenYears),

			KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
			BasicConstraintsValid: true,

			IsCA: true,
		}
		caBody, err := x509.CreateCertificate(rand.Reader, &caCert, &caCert, caKey.Public(), caKey)
		if err != nil {
			ca.err = fmt.Errorf("unable to generate CA certificate: %w", err)
			return
		}
		caReparsed, err := x509.ParseCertificate(caBody)
		if err != nil {
			ca.err = fmt.Errorf("unable to parse CA certificate: %w", err)
			return
		}
		roots := x509.NewCertPool()
		roots.AddCert(caReparsed)

		chainKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			ca.err = fmt.Errorf("unable to generate sub-CA key: %w", err)
			return
		}
		chainCert := x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject: pkix.Name{
				Organization: []string{"Testing Sub-CA"},
			},
			NotBefore: time.Time{},
			NotAfter:  time.Now().Add(tenYears),

			KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
			BasicConstraintsValid: true,

			IsCA: true,
		}
		chainBody, err := x509.CreateCertificate(rand.Reader, &chainCert, caReparsed, chainKey.Public(), caKey)
		if err != nil {
			ca.err = fmt.Errorf("unable to generate sub-CA certificate: %w", err)
			return
		}
		chainReparsed, err := x509.ParseCertificate(chainBody)
		if err != nil {
			ca.err = fmt.Errorf("unable to parse sub-CA certificate: %w", err)
			return
		}

		ca.chainKey = chainKey
		ca.chainCert = chainReparsed
	})

	certKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		require.NoErrorf(t, err, "unable to generate certificate key")
	}
	cert := x509.Certificate{
		SerialNumber: big.NewInt(3),
		DNSNames:     altNames,
		NotBefore:    notBefore,
		NotAfter:     notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	body, err := x509.CreateCertificate(rand.Reader, &cert, ca.chainCert, certKey.Public(), ca.chainKey)
	if err != nil {
		require.NoErrorf(t, err, "unable to generate certificate")
	}

	crt := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: body})

	key := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(certKey)})

	certificate, err := tls.X509KeyPair([]byte(crt), []byte(key))
	if err != nil {
		log.Errorf("failed to generate fake serial number: %v", err)
	}

	return &certificate
}

func TestCertRegistry(t *testing.T) {

	domain := "example.org"
	validHostname := "foo." + domain

	now := time.Now().Truncate(time.Millisecond)

	old := now.Add(-time.Hour * 48 * 7)
	new := now.Add(-time.Hour * 24 * 7)
	after := now.Add(time.Hour*24*7 + 1*time.Second)
	dummyArn := "DUMMY"

	// simple cert
	validCert := createDummyCertDetail(t, dummyArn, []string{validHostname}, old, after)
	newValidCert := createDummyCertDetail(t, dummyArn, []string{validHostname}, new, after)

	validCert.Leaf, _ = x509.ParseCertificate(validCert.Certificate[0])
	newValidCert.Leaf, _ = x509.ParseCertificate(newValidCert.Certificate[0])

	hello := &tls.ClientHelloInfo{
		ServerName: "foo.example.org",
	}

	t.Run("sync new certificate", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.ConfigureCertificate(validHostname, validCert)
		cert, found := cr.lookup[validHostname]
		if !found {
			t.Error("failed to read certificate")
		}
		if cert.Leaf == nil {
			t.Error("synced cert should have a parsed leaf")
		}
	})

	t.Run("sync a nil certificate", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.ConfigureCertificate(validHostname, nil)
		_, found := cr.lookup[validHostname]
		if found {
			t.Error("nil certificate should not sync")
		}
	})

	t.Run("sync existing certificate", func(t *testing.T) {

		cr := NewCertRegistry()
		cr.ConfigureCertificate(validHostname, validCert)
		cert1 := cr.lookup[validHostname]
		cr.ConfigureCertificate(validHostname, newValidCert)
		cert2 := cr.lookup[validHostname]
		if equalCert(cert1, cert2) {
			t.Error("host cert was not updated")
		}

	})

	t.Run("get nonexistent cert", func(t *testing.T) {
		cr := NewCertRegistry()
		_, found := cr.lookup["foo"]
		if found {
			t.Error("nonexistent certificate was found")
		}
	})

	t.Run("get cert from hello", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.ConfigureCertificate(validHostname, validCert)
		crt, _ := cr.GetCertFromHello(hello)
		if crt == nil {
			t.Error("failed to read certificate from hello")
		} else {
			if !reflect.DeepEqual(crt.Certificate, validCert.Certificate) {
				t.Error("failed to read correct certificate from hello")
			}
		}
	})

	t.Run("get nil cert from unknown hello", func(t *testing.T) {
		cr := NewCertRegistry()
		cert, _ := cr.GetCertFromHello(hello)
		if cert != nil {
			t.Error("should return nil when cert not found")
		}
	})
}

func equalCert(l *tls.Certificate, r *tls.Certificate) bool {
	if !reflect.DeepEqual(l.Certificate, r.Certificate) {
		return false
	}

	if !reflect.DeepEqual(l.PrivateKey, r.PrivateKey) {
		return false
	}

	return true
}
