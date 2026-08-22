package rfc9421

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSignRequest_HMAC(t *testing.T) {
	req := &http.Request{
		Method: "POST",
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.example.com",
			Path:   "/orders",
		},
		Header: http.Header{
			"Host":         []string{"api.example.com"},
			"Content-Type": []string{"application/json"},
		},
	}

	signer, err := NewSigner(
		"test-key-hmac",
		AlgHmacSha256,
		[]string{"@method", "@path", "@authority", "content-type"},
		"sig1",
		[]byte("secret-key-12345"),
	)
	if err != nil {
		t.Fatalf("unexpected error creating signer: %v", err)
	}

	err = signer.SignRequest(req)
	if err != nil {
		t.Fatalf("unexpected error signing request: %v", err)
	}

	sigInput := req.Header.Get("Signature-Input")
	sig := req.Header.Get("Signature")

	if sigInput == "" {
		t.Fatal("expected Signature-Input header to be set")
	}
	if sig == "" {
		t.Fatal("expected Signature header to be set")
	}
	if !strings.Contains(sigInput, `keyid="test-key-hmac"`) {
		t.Errorf("Signature-Input missing keyid: %s", sigInput)
	}
}

func TestSignRequest_Ed25519(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Scheme: "https",
			Host:   "example.org",
			Path:   "/test",
		},
		Header: http.Header{
			"Host": []string{"example.org"},
		},
	}

	signer, err := NewSigner("ed-key", AlgEd25519, []string{"@method", "@path"}, "sig-ed", pemBytes)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}

	if err := signer.SignRequest(req); err != nil {
		t.Fatalf("failed to sign request: %v", err)
	}

	if req.Header.Get("Signature-Input") == "" || req.Header.Get("Signature") == "" {
		t.Fatal("missing signature headers")
	}
}

func TestSignRequest_RSAPSS(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Scheme: "https",
			Host:   "example.org",
			Path:   "/rsa-test",
		},
		Header: http.Header{
			"Host": []string{"example.org"},
		},
	}

	signer, err := NewSigner("rsa-key", AlgRsaPssSha512, []string{"@method", "@path"}, "sig1", pemBytes)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}

	if err := signer.SignRequest(req); err != nil {
		t.Fatalf("failed to sign request: %v", err)
	}
}

func TestSignRequest_MissingComponentError(t *testing.T) {
	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Scheme: "https",
			Host:   "example.org",
			Path:   "/",
		},
		Header: http.Header{},
	}

	signer, err := NewSigner("k1", AlgHmacSha256, []string{"non-existent-header"}, "sig1", []byte("secret"))
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}

	if err := signer.SignRequest(req); err == nil {
		t.Fatal("expected error for missing required header component, got nil")
	}
}
