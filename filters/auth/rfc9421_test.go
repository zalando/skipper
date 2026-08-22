package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/rfc/rfc9421"
	"github.com/zalando/skipper/secrets"
)

type mockSecretsReader struct {
	data map[string][]byte
}

func newMockSecretsReader(data map[string][]byte) secrets.SecretsReader {
	return &mockSecretsReader{data: data}
}

func (m *mockSecretsReader) GetSecret(key string) ([]byte, bool) {
	val, ok := m.data[key]
	return val, ok
}

func (m *mockSecretsReader) Close() {}

func TestRFC9421Spec_CreateFilter(t *testing.T) {
	reader := newMockSecretsReader(map[string][]byte{
		"my-secret": []byte("test-key-bytes-12345"),
	})
	spec := NewRFC9421("my-secret", "key-1", reader)

	tests := []struct {
		name    string
		spec    filters.Spec
		args    []interface{}
		wantErr bool
	}{
		{
			name: "valid 2 args",
			spec: spec,
			args: []interface{}{
				rfc9421.AlgHmacSha256,
				"@method, @path, host",
			},
			wantErr: false,
		},
		{
			name: "valid 3 args with custom label",
			spec: spec,
			args: []interface{}{
				rfc9421.AlgHmacSha256,
				"@method, @path",
				"custom-sig",
			},
			wantErr: false,
		},
		{
			name:    "too few args",
			spec:    spec,
			args:    []interface{}{rfc9421.AlgHmacSha256},
			wantErr: true,
		},
		{
			name:    "too many args",
			spec:    spec,
			args:    []interface{}{rfc9421.AlgHmacSha256, "@method", "sig1", "extra"},
			wantErr: true,
		},
		{
			name: "missing secret in reader",
			spec: NewRFC9421("unknown-secret", "key-1", reader),
			args: []interface{}{
				rfc9421.AlgHmacSha256,
				"@method, @path",
			},
			wantErr: true,
		},
		{
			name: "missing flag configuration (empty key file)",
			spec: NewRFC9421("", "key-1", reader),
			args: []interface{}{
				rfc9421.AlgHmacSha256,
				"@method, @path",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.spec.CreateFilter(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateFilter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRFC9421Filter_Request_HMAC(t *testing.T) {
	secretBytes := []byte("secret-hmac-data")
	reader := newMockSecretsReader(map[string][]byte{
		"hmac-key": secretBytes,
	})
	spec := NewRFC9421("hmac-key", "key-1", reader)

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
			Path:   "/v1/orders",
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
		t.Fatal("expected Signature-Input and Signature headers to be set")
	}

	if !strings.HasPrefix(sigInput, "sig1=(") {
		t.Errorf("expected Signature-Input to start with sig1=(, got %s", sigInput)
	}
	if !strings.HasPrefix(sig, "sig1=:") {
		t.Errorf("expected Signature to start with sig1=:, got %s", sig)
	}
}

func TestRFC9421Filter_Request_Ed25519(t *testing.T) {
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

	reader := newMockSecretsReader(map[string][]byte{
		"ed25519-key": pemBytes,
	})
	spec := NewRFC9421("ed25519-key", "ed-key-id", reader)

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
			Path:   "/status",
		},
		Header: http.Header{
			"Host": []string{"api.partner.com"},
		},
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)

	if req.Header.Get("Signature-Input") == "" || req.Header.Get("Signature") == "" {
		t.Fatal("expected signature headers to be present")
	}
}
