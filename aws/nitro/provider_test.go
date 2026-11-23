//go:build awsnitro

package nitro

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlexanderYastrebov/noleak"
)

func TestProviderTLSHandshakes(t *testing.T) {
	defer noleak.Check(t)

	caCert, caKey, caDER := generateTestCA(t)
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	p256Key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate p256 key: %v", err)
	}
	p384Key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generate p384 key: %v", err)
	}

	now := time.Now().Add(-time.Minute)
	rsaDER := generateTestLeaf(t, caCert, caKey, rsaKey, []string{"rsa.example.com"}, now, 10)
	p256DER := generateTestLeaf(t, caCert, caKey, p256Key, []string{"ecdsa.example.com", "*.wild.example.com"}, now, 11)
	p384DER := generateTestLeaf(t, caCert, caKey, p384Key, []string{"p384.example.com"}, now, 12)

	srv := startFakeP11Server(t, "acm-enclave")
	srv.addCertAndKey([]byte("id-rsa"), rsaDER, rsaKey, caDER)
	srv.addCertAndKey([]byte("id-p256"), p256DER, p256Key)
	srv.addCertAndKey([]byte("id-p384"), p384DER, p384Key)

	provider, err := New(Options{
		Endpoint:        srv.endpoint,
		TokenLabel:      "acm-enclave",
		RefreshInterval: time.Hour,
		Timeout:         2 * time.Second,
		MaxSessions:     4,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer provider.Close()

	for _, tt := range []struct {
		name       string
		serverName string
		tlsVersion uint16
	}{
		{name: "RSA TLS 1.3 (RSA-PSS)", serverName: "rsa.example.com", tlsVersion: tls.VersionTLS13},
		{name: "RSA TLS 1.2 (PKCS1v15/PSS)", serverName: "rsa.example.com", tlsVersion: tls.VersionTLS12},
		{name: "ECDSA P-256 TLS 1.3", serverName: "ecdsa.example.com", tlsVersion: tls.VersionTLS13},
		{name: "ECDSA P-256 TLS 1.2", serverName: "ecdsa.example.com", tlsVersion: tls.VersionTLS12},
		{name: "ECDSA P-256 Wildcard SNI", serverName: "api.wild.example.com", tlsVersion: tls.VersionTLS13},
		{name: "ECDSA P-384 TLS 1.3", serverName: "p384.example.com", tlsVersion: tls.VersionTLS13},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("ok"))
			}))
			ts.TLS = &tls.Config{
				MinVersion:     tt.tlsVersion,
				MaxVersion:     tt.tlsVersion,
				GetCertificate: provider.GetCertificate,
			}
			ts.StartTLS()
			defer ts.Close()

			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						RootCAs:    roots,
						ServerName: tt.serverName,
						MinVersion: tt.tlsVersion,
						MaxVersion: tt.tlsVersion,
					},
				},
			}
			defer client.CloseIdleConnections()

			resp, err := client.Get(ts.URL)
			if err != nil {
				t.Fatalf("GET failed: %v", err)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if string(body) != "ok" {
				t.Fatalf("got body %q, want %q", string(body), "ok")
			}
		})
	}
}

func TestProviderSessionRecoveryOnDrop(t *testing.T) {
	defer noleak.Check(t)

	caCert, caKey, _ := generateTestCA(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	leafDER := generateTestLeaf(t, caCert, caKey, key, []string{"recover.example.com"}, time.Now().Add(-time.Minute), 20)

	srv := startFakeP11Server(t, "acm")
	srv.addCertAndKey([]byte("id-1"), leafDER, key)

	provider, err := New(Options{
		Endpoint:        srv.endpoint,
		RefreshInterval: time.Hour,
		Timeout:         2 * time.Second,
		MaxSessions:     2,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer provider.Close()

	cert, err := provider.GetCertificate(&tls.ClientHelloInfo{ServerName: "recover.example.com"})
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	signer := cert.PrivateKey.(crypto.Signer)

	// Force the next C_Sign RPC to drop the connection mid-flight.
	srv.dropNext.Store(true)

	digest := make([]byte, 32)
	sig, err := signer.Sign(rand.Reader, digest, crypto.SHA256)
	if err != nil {
		t.Fatalf("expected automatic reconnect on dropped sign, got error: %v", err)
	}
	if !ecdsa.VerifyASN1(&key.PublicKey, digest, sig) {
		t.Fatalf("signature failed verification")
	}
}

func TestProviderCertRotation(t *testing.T) {
	defer noleak.Check(t)

	caCert, caKey, _ := generateTestCA(t)
	key1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	cert1DER := generateTestLeaf(t, caCert, caKey, key1, []string{"rotate.example.com"}, time.Now().Add(-2*time.Hour), 100)
	cert2DER := generateTestLeaf(t, caCert, caKey, key2, []string{"rotate.example.com"}, time.Now().Add(-time.Minute), 200)

	srv := startFakeP11Server(t, "acm")
	srv.addCertAndKey([]byte("id-1"), cert1DER, key1)

	provider, err := New(Options{
		Endpoint:        srv.endpoint,
		RefreshInterval: 50 * time.Millisecond,
		Timeout:         2 * time.Second,
		MaxSessions:     2,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer provider.Close()

	c1, err := provider.GetCertificate(&tls.ClientHelloInfo{ServerName: "rotate.example.com"})
	if err != nil {
		t.Fatalf("GetCertificate initial: %v", err)
	}
	if c1.Leaf.SerialNumber.Int64() != 100 {
		t.Fatalf("got serial %d, want 100", c1.Leaf.SerialNumber.Int64())
	}

	srv.resetObjects()
	srv.addCertAndKey([]byte("id-2"), cert2DER, key2)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c2, err := provider.GetCertificate(&tls.ClientHelloInfo{ServerName: "rotate.example.com"})
		if err == nil && c2.Leaf.SerialNumber.Int64() == 200 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("certificate did not rotate to serial 200 before deadline")
}

func TestProviderErrors(t *testing.T) {
	defer noleak.Check(t)

	srv := startFakeP11Server(t, "acm-token")

	t.Run("no certificates in token", func(t *testing.T) {
		_, err := New(Options{
			Endpoint:        srv.endpoint,
			TokenLabel:      "acm-token",
			RefreshInterval: time.Hour,
		})
		if !errors.Is(err, ErrNoCertificates) {
			t.Fatalf("expected ErrNoCertificates, got %v", err)
		}
	})

	t.Run("wrong token label", func(t *testing.T) {
		_, err := New(Options{
			Endpoint:        srv.endpoint,
			TokenLabel:      "missing-token",
			RefreshInterval: time.Hour,
		})
		if err == nil {
			t.Fatalf("expected error for missing token label")
		}
	})
}
