package nettest

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
)

func TestCAServerAndClientCerts(t *testing.T) {
	ca := NewCA(t)

	serverCertPEM, serverKeyPEM := ca.ServerCert(t, "127.0.0.1")
	clientCertPEM, clientKeyPEM := ca.ClientCert(t)

	// Keypairs must load.
	if _, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM); err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	if _, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM); err != nil {
		t.Fatalf("client keypair: %v", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca.CertPEM) {
		t.Fatal("failed to append CA PEM")
	}

	server := parseLeaf(t, serverCertPEM)
	if _, err := server.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSName:   "127.0.0.1",
	}); err != nil {
		t.Fatalf("server verify: %v", err)
	}

	client := parseLeaf(t, clientCertPEM)
	if _, err := client.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Fatalf("client verify: %v", err)
	}
}

func TestGenCertSelfSignedCA(t *testing.T) {
	certPEM, _, cert, key := MustGenCert(t, CertOptions{CN: "self-ca"})
	if key == nil {
		t.Fatal("nil key")
	}
	if !cert.IsCA {
		t.Error("self-signed cert should be a CA")
	}
	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("self-signed CA should have CertSign key usage")
	}
	if len(certPEM) == 0 {
		t.Error("empty cert PEM")
	}
}

func TestWriteCertKey(t *testing.T) {
	ca := NewCA(t)
	certPEM, keyPEM := ca.ClientCert(t)

	certFile, keyFile := WriteCertKey(t, certPEM, keyPEM)

	gotCert, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("read cert file: %v", err)
	}
	gotKey, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	if string(gotCert) != string(certPEM) {
		t.Error("cert file content mismatch")
	}
	if string(gotKey) != string(keyPEM) {
		t.Error("key file content mismatch")
	}
}

func parseLeaf(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}
