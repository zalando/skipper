package valkeytest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"
)

// TestCerts holds PEM-encoded material for a valkey mutual TLS test.
// CAPEM verifies both sides, ServerCertPEM/ServerKeyPEM are presented by the
// valkey container, and ClientCertPEM/ClientKeyPEM are presented by Skipper.
type TestCerts struct {
	CAPEM         []byte
	ServerCertPEM []byte
	ServerKeyPEM  []byte
	ClientCertPEM []byte
	ClientKeyPEM  []byte
}

// NewTestCerts generates a CA, a server certificate (serverAuth, with the given
// hosts as SANs, defaulting to 127.0.0.1) and a client certificate (clientAuth),
// all PEM-encoded and signed by the CA. With a 127.0.0.1 SAN, a client dialing a
// host-mapped TLS port verifies the server without overriding ServerName.
func NewTestCerts(t testing.TB, hosts ...string) TestCerts {
	t.Helper()

	if len(hosts) == 0 {
		hosts = []string{"127.0.0.1"}
	}

	now := time.Now()
	caCert, caKey, _ := mkCert(t, &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "valkey-test-ca"},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}, nil, nil)

	serverTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano() + 1),
		Subject:               pkix.Name{CommonName: "valkey-server"},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			serverTmpl.IPAddresses = append(serverTmpl.IPAddresses, ip)
		} else {
			serverTmpl.DNSNames = append(serverTmpl.DNSNames, h)
		}
	}
	serverCert, _, serverKey := mkCert(t, serverTmpl, caCert, caKey)

	clientCert, _, clientKey := mkCert(t, &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano() + 2),
		Subject:               pkix.Name{CommonName: "skipper-client"},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}, caCert, caKey)

	return TestCerts{
		CAPEM:         caCert.certPEM,
		ServerCertPEM: serverCert.certPEM,
		ServerKeyPEM:  serverKey,
		ClientCertPEM: clientCert.certPEM,
		ClientKeyPEM:  clientKey,
	}
}

// parsedCert bundles a parsed certificate with its PEM encoding so leaves can be
// signed by it while callers still get the PEM bytes.
type parsedCert struct {
	cert    *x509.Certificate
	certPEM []byte
}

// mkCert creates a certificate from template. When signer is nil the certificate
// is self-signed; otherwise it is signed by signer/signerKey. It returns the
// parsed certificate (with PEM), its private key for signing further leaves, and
// the PEM-encoded private key.
func mkCert(t testing.TB, template *x509.Certificate, signer *parsedCert, signerKey *ecdsa.PrivateKey) (*parsedCert, *ecdsa.PrivateKey, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("valkeytest: failed to generate key: %v", err)
	}

	signingCert := template
	sKey := key
	if signer != nil {
		signingCert = signer.cert
		sKey = signerKey
	}

	der, err := x509.CreateCertificate(rand.Reader, template, signingCert, &key.PublicKey, sKey)
	if err != nil {
		t.Fatalf("valkeytest: failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("valkeytest: failed to parse certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("valkeytest: failed to marshal key: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &parsedCert{cert: cert, certPEM: certPEM}, key, keyPEM
}
