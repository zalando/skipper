package valkeytest

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestNewTestCerts(t *testing.T) {
	c := NewTestCerts(t, "127.0.0.1")

	// Server and client keypairs must load.
	if _, err := tls.X509KeyPair(c.ServerCertPEM, c.ServerKeyPEM); err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	if _, err := tls.X509KeyPair(c.ClientCertPEM, c.ClientKeyPEM); err != nil {
		t.Fatalf("client keypair: %v", err)
	}

	// Both leaves must verify against the CA.
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(c.CAPEM) {
		t.Fatal("failed to append CA PEM to pool")
	}

	serverLeaf := parseLeaf(t, c.ServerCertPEM)
	if _, err := serverLeaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSName:   "127.0.0.1",
	}); err != nil {
		t.Fatalf("server leaf verify: %v", err)
	}

	clientLeaf := parseLeaf(t, c.ClientCertPEM)
	if _, err := clientLeaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Fatalf("client leaf verify: %v", err)
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
