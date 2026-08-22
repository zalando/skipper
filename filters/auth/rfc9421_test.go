package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"strings"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/rfc/rfc9421"
)

func TestRFC9421Spec_CreateFilter(t *testing.T) {
	spec := NewRFC9421()

	if spec.Name() != RFC9421Name {
		t.Errorf("expected spec name %q, got %q", RFC9421Name, spec.Name())
	}

	tests := []struct {
		name    string
		args    []interface{}
		wantErr bool
	}{
		{
			name:    "valid 3 args",
			args:    []interface{}{"key-1", rfc9421.AlgorithmHmacSha256, "@method, @path, host"},
			wantErr: false,
		},
		{
			name:    "valid 4 args with custom label",
			args:    []interface{}{"key-1", rfc9421.AlgorithmHmacSha256, "@method, @path", "my-sig"},
			wantErr: false,
		},
		{
			name:    "too few args",
			args:    []interface{}{"key-1", rfc9421.AlgorithmHmacSha256},
			wantErr: true,
		},
		{
			name:    "too many args",
			args:    []interface{}{"key-1", rfc9421.AlgorithmHmacSha256, "@method", "sig1", "extra"},
			wantErr: true,
		},
		{
			name:    "invalid key id type",
			args:    []interface{}{123, rfc9421.AlgorithmHmacSha256, "@method"},
			wantErr: true,
		},
		{
			name:    "invalid algorithm type",
			args:    []interface{}{"key-1", 123, "@method"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := spec.CreateFilter(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateFilter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRFC9421Filter_Request_HMAC(t *testing.T) {
	spec := NewRFC9421()
	f, err := spec.CreateFilter([]interface{}{"test-key", rfc9421.AlgorithmHmacSha256, "@method, @path, host", "sig1"})
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://example.com/api/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("x-rfc9421-private-key", "secret-key-12345678901234567890")

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)

	// Ensure sensitive private key header was removed
	if req.Header.Get("x-rfc9421-private-key") != "" {
		t.Errorf("expected x-rfc9421-private-key header to be removed from request")
	}

	// Verify Signature-Input header
	sigInput := req.Header.Get("Signature-Input")
	if sigInput == "" {
		t.Fatal("expected non-empty Signature-Input header")
	}
	if !strings.Contains(sigInput, `sig1=("@method" "@path" "host")`) {
		t.Errorf("expected components in Signature-Input, got: %s", sigInput)
	}
	if !strings.Contains(sigInput, `keyid="test-key"`) {
		t.Errorf("expected keyid in Signature-Input, got: %s", sigInput)
	}

	// Verify Signature header
	sig := req.Header.Get("Signature")
	if sig == "" {
		t.Fatal("expected non-empty Signature header")
	}
	if !strings.HasPrefix(sig, "sig1=:") || !strings.HasSuffix(sig, ":") {
		t.Errorf("unexpected Signature header format: %s", sig)
	}
}

func TestRFC9421Filter_Request_MissingKey(t *testing.T) {
	spec := NewRFC9421()
	f, err := spec.CreateFilter([]interface{}{"test-key", rfc9421.AlgorithmHmacSha256, "@method, @path"})
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/api/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)

	if req.Header.Get("Signature") != "" || req.Header.Get("Signature-Input") != "" {
		t.Errorf("expected no signature headers when private key is missing")
	}
}

func TestRFC9421Filter_Request_Ed25519(t *testing.T) {
	spec := NewRFC9421()
	f, err := spec.CreateFilter([]interface{}{"ed-key", rfc9421.AlgorithmEd25519, "@method, @path"})
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/api/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("x-rfc9421-private-key", string(priv))

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)

	if req.Header.Get("x-rfc9421-private-key") != "" {
		t.Errorf("expected x-rfc9421-private-key header to be removed")
	}
	if req.Header.Get("Signature-Input") == "" || req.Header.Get("Signature") == "" {
		t.Errorf("expected signature headers to be populated")
	}
}
