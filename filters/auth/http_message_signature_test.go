package auth

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/rfc/rfc9421"
)

func TestHTTPMessageSignatureSpec_CreateFilter(t *testing.T) {
	reader := &testSecretsReader{
		name:   "my-secret",
		secret: "test-key-bytes",
	}

	for _, tc := range []struct {
		name    string
		spec    filters.Spec
		args    []interface{}
		wantErr bool
	}{
		{
			name: "valid 2 args",
			spec: NewHTTPMessageSignature("my-secret", "key-1", reader),
			args: []interface{}{
				rfc9421.AlgHmacSha256,
				"@method, @path",
			},
			wantErr: false,
		},
		{
			name: "valid 3 args with custom label",
			spec: NewHTTPMessageSignature("my-secret", "key-1", reader),
			args: []interface{}{
				rfc9421.AlgHmacSha256,
				"@method, @path",
				"custom-sig",
			},
			wantErr: false,
		},
		{
			name: "too few args",
			spec: NewHTTPMessageSignature("my-secret", "key-1", reader),
			args: []interface{}{
				rfc9421.AlgHmacSha256,
			},
			wantErr: true,
		},
		{
			name: "too many args",
			spec: NewHTTPMessageSignature("my-secret", "key-1", reader),
			args: []interface{}{
				rfc9421.AlgHmacSha256,
				"@method, @path",
				"sig1",
				"extra",
			},
			wantErr: true,
		},
		{
			name: "missing flag configuration (empty key file)",
			spec: NewHTTPMessageSignature("", "key-1", reader),
			args: []interface{}{
				rfc9421.AlgHmacSha256,
				"@method, @path",
			},
			wantErr: true,
		},
		{
			name: "missing flag configuration (empty key id)",
			spec: NewHTTPMessageSignature("my-secret", "", reader),
			args: []interface{}{
				rfc9421.AlgHmacSha256,
				"@method, @path",
			},
			wantErr: true,
		},
		{
			name: "missing secrets reader",
			spec: NewHTTPMessageSignature("my-secret", "key-1", nil),
			args: []interface{}{
				rfc9421.AlgHmacSha256,
				"@method, @path",
			},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.spec.CreateFilter(tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("CreateFilter() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && f == nil {
				t.Fatal("expected filter instance, got nil")
			}
		})
	}
}

func TestHTTPMessageSignatureFilter_Request_HMAC(t *testing.T) {
	reader := &testSecretsReader{
		name:   "hmac-key",
		secret: "super-secret-hmac-key-12345",
	}
	spec := NewHTTPMessageSignature("hmac-key", "test-key-id", reader)

	f, err := spec.CreateFilter([]interface{}{
		rfc9421.AlgHmacSha256,
		"@method, @path, @authority, content-type",
		"sig1",
	})
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}

	req := &http.Request{
		Method: "POST",
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.partner.com",
			Path:   "/orders",
		},
		Header: http.Header{
			"Host":         []string{"api.partner.com"},
			"Content-Type": []string{"application/json"},
		},
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)

	sigInput := req.Header.Get("Signature-Input")
	sig := req.Header.Get("Signature")

	if sigInput == "" || sig == "" {
		t.Fatal("expected signature headers to be present")
	}
	if !strings.HasPrefix(sigInput, `sig1=`) {
		t.Errorf("expected Signature-Input to start with sig1=, got %s", sigInput)
	}
}

func TestHTTPMessageSignatureFilter_Request_KeyRotation(t *testing.T) {
	reader := &testSecretsReader{
		name:   "hmac-key",
		secret: "initial-secret-key-12345",
	}
	spec := NewHTTPMessageSignature("hmac-key", "key-1", reader)

	f, err := spec.CreateFilter([]interface{}{
		rfc9421.AlgHmacSha256,
		"@method, @path",
		"sig1",
	})
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}

	newReq := func() *http.Request {
		return &http.Request{
			Method: "GET",
			URL: &url.URL{
				Scheme: "https",
				Host:   "api.partner.com",
				Path:   "/orders",
			},
			Header: http.Header{
				"Host": []string{"api.partner.com"},
			},
		}
	}

	req1 := newReq()
	f.Request(&filtertest.Context{FRequest: req1})
	sig1 := req1.Header.Get("Signature")
	if sig1 == "" {
		t.Fatal("expected signature header to be set")
	}

	// Rotate secret in reader
	reader.secret = "rotated-secret-key-67890"

	req2 := newReq()
	f.Request(&filtertest.Context{FRequest: req2})
	sig2 := req2.Header.Get("Signature")
	if sig2 == "" {
		t.Fatal("expected signature header to be set after rotation")
	}

	if sig1 == sig2 {
		t.Errorf("expected signature to change after secret rotation, but both were %s", sig1)
	}
}

func TestHTTPMessageSignatureFilter_Request_Ed25519(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal pkcs8: %v", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})

	reader := &testSecretsReader{
		name:   "ed-key",
		secret: string(pemBytes),
	}
	spec := NewHTTPMessageSignature("ed-key", "ed-key-id", reader)

	f, err := spec.CreateFilter([]interface{}{
		rfc9421.AlgEd25519,
		"@method, @path",
		"sig-ed",
	})
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}

	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.partner.com",
			Path:   "/items",
		},
		Header: http.Header{
			"Host": []string{"api.partner.com"},
		},
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)

	if req.Header.Get("Signature") == "" || req.Header.Get("Signature-Input") == "" {
		t.Fatal("expected signature headers to be present")
	}
}

func TestHTTPMessageSignatureFilter_Request_ECDSA_P256(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ecdsa key: %v", err)
	}

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal ecdsa key: %v", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privBytes,
	})

	reader := &testSecretsReader{
		name:   "ecdsa-key",
		secret: string(pemBytes),
	}
	spec := NewHTTPMessageSignature("ecdsa-key", "ec-key-id", reader)

	f, err := spec.CreateFilter([]interface{}{
		rfc9421.AlgEcdsaP256,
		"@method, @path",
		"sig-ec",
	})
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}

	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Scheme: "https",
			Host:   "api.partner.com",
			Path:   "/status",
		},
		Header: http.Header{
			"Host": []string{"api.partner.com"},
		},
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)

	sigHeader := req.Header.Get("Signature")
	if sigHeader == "" {
		t.Fatal("expected signature header to be present")
	}

	parts := strings.Split(sigHeader, ":")
	if len(parts) < 2 {
		t.Fatalf("invalid signature format: %s", sigHeader)
	}
	rawSig, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("failed to decode base64 signature: %v", err)
	}

	if len(rawSig) != 64 {
		t.Errorf("expected ECDSA P-256 signature length to be 64 bytes, got %d", len(rawSig))
	}
}
