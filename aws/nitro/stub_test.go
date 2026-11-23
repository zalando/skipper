//go:build !awsnitro

package nitro

import (
	"crypto/tls"
	"errors"
	"testing"
)

func TestStubReturnsErrNotSupported(t *testing.T) {
	p, err := New(Options{Endpoint: "vsock://16:8000"})
	if !errors.Is(err, ErrNotSupported) {
		t.Fatalf("expected ErrNotSupported, got %v", err)
	}
	if p != nil {
		t.Fatalf("expected nil provider, got %v", p)
	}

	var stub Provider
	if _, err := stub.GetCertificate(&tls.ClientHelloInfo{}); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("expected ErrNotSupported from GetCertificate, got %v", err)
	}
	if err := stub.Close(); err != nil {
		t.Fatalf("unexpected error from Close: %v", err)
	}
}
