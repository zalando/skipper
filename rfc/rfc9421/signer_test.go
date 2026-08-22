package rfc9421

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSignRequest_HMAC(t *testing.T) {
	fixedTime := time.Unix(1618884473, 0)
	secret := []byte("secret-key-12345678901234567890")

	signer, err := NewSigner(SignerConfig{
		KeyID:      "test-key-hmac",
		Algorithm:  AlgorithmHmacSha256,
		PrivateKey: secret,
		Components: []string{"@method", "@path", "@authority", "content-type"},
		SignLabel:  "sig1",
		Created:    &fixedTime,
	})
	if err != nil {
		t.Fatalf("unexpected error creating signer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://example.com/api/v1/resource?q=test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	sigInput, sig, err := signer.SignRequest(req)
	if err != nil {
		t.Fatalf("failed to sign request: %v", err)
	}

	expectedInput := `sig1=("@method" "@path" "@authority" "content-type");created=1618884473;keyid="test-key-hmac";alg="hmac-sha256"`
	if sigInput != expectedInput {
		t.Errorf("expected signature input %q, got %q", expectedInput, sigInput)
	}

	if !strings.HasPrefix(sig, "sig1=:") || !strings.HasSuffix(sig, ":") {
		t.Errorf("unexpected signature header format: %q", sig)
	}
}

func TestSignRequest_Ed25519(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	fixedTime := time.Unix(1618884473, 0)
	signer, err := NewSigner(SignerConfig{
		KeyID:      "test-ed25519",
		Algorithm:  AlgorithmEd25519,
		PrivateKey: priv,
		Components: []string{"@method", "@path"},
		SignLabel:  "sig1",
		Created:    &fixedTime,
	})
	if err != nil {
		t.Fatalf("unexpected error creating signer: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "http://localhost/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	sigInput, sig, err := signer.SignRequest(req)
	if err != nil {
		t.Fatalf("failed to sign request: %v", err)
	}

	if sigInput == "" || sig == "" {
		t.Errorf("expected non-empty headers, got input: %q, sig: %q", sigInput, sig)
	}
}

func TestSignRequest_RSAPSS(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	fixedTime := time.Unix(1618884473, 0)
	signer, err := NewSigner(SignerConfig{
		KeyID:      "test-rsa",
		Algorithm:  AlgorithmRsaPssSha512,
		PrivateKey: privKey,
		Components: []string{"@method", "@path"},
		Created:    &fixedTime,
	})
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "http://localhost/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	sigInput, sig, err := signer.SignRequest(req)
	if err != nil {
		t.Fatalf("failed to sign request: %v", err)
	}

	if !strings.Contains(sigInput, `alg="rsa-pss-sha512"`) {
		t.Errorf("expected alg in signature input, got: %s", sigInput)
	}
	if !strings.HasPrefix(sig, "sig1=:") {
		t.Errorf("unexpected signature header format: %s", sig)
	}
}

func TestSignRequest_MissingComponentError(t *testing.T) {
	secret := []byte("secret")
	signer, err := NewSigner(SignerConfig{
		KeyID:      "test",
		Algorithm:  AlgorithmHmacSha256,
		PrivateKey: secret,
		Components: []string{"x-missing-header"},
	})
	if err != nil {
		t.Fatalf("unexpected error creating signer: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "http://localhost/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	_, _, err = signer.SignRequest(req)
	if err == nil {
		t.Fatal("expected error for missing header component, got nil")
	}
}
