//go:build !awsnitro || !linux

package nitro

import (
	"errors"
	"testing"

	"github.com/zalando/skipper/secrets/certregistry"
)

func TestNewProviderStub(t *testing.T) {
	p, err := NewProvider(Options{}, certregistry.NewCertRegistry())
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got: %v", err)
	}
	if p != nil {
		t.Fatalf("expected nil provider, got: %v", p)
	}
}
