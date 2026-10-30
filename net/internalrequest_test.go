package net

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConsumeInternalRequest(t *testing.T) {
	for _, kind := range []string{"external", "forged", "internal"} {
		t.Run(kind, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if kind == "forged" {
				req.Header.Set(internalRequestHeader, "1")
			} else if kind == "internal" {
				MarkInternalRequest(req)
			}
			assert.Equal(t, kind == "internal", ConsumeInternalRequest(req))
			assert.Equal(t, kind == "internal", IsInternalRequest(req))
			assert.Empty(t, req.Header.Get(internalRequestHeader))
			assert.False(t, ConsumeInternalRequest(req))
		})
	}
}

func BenchmarkConsumeExternalRequest(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	b.ReportAllocs()
	for b.Loop() {
		ConsumeInternalRequest(req)
	}
}
