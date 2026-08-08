package net

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/AlexanderYastrebov/noleak"
)

// TestTransportEnableH2c verifies that when EnableH2c is set, NewTransport
// produces a transport that connects to an h2c backend over HTTP/2.
func TestTransportEnableH2c(t *testing.T) {
	noleak.Check(t)

	var mu sync.Mutex
	var proto string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		proto = r.Proto
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetHTTP1(true)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	srv.Start()
	t.Cleanup(srv.Close)

	tr := NewTransport(Options{EnableH2c: true})
	defer tr.Close()

	req, err := http.NewRequest("GET", srv.URL+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	rsp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	mu.Lock()
	gotProto := proto
	mu.Unlock()
	if gotProto != "HTTP/2.0" {
		t.Errorf("expected server to receive HTTP/2.0, got %q", gotProto)
	}
}

// TestTransportEnableH2c_ProtocolSet verifies that the underlying transport
// has SetUnencryptedHTTP2(true) when EnableH2c is set.
func TestTransportEnableH2c_ProtocolSet(t *testing.T) {
	noleak.Check(t)

	tr := NewTransport(Options{EnableH2c: true})
	defer tr.Close()

	if tr.tr.Protocols == nil {
		t.Fatal("expected Protocols to be set on transport, got nil")
	}
	if !tr.tr.Protocols.UnencryptedHTTP2() {
		t.Error("expected UnencryptedHTTP2() to be true")
	}
}

// TestTransportDisabledH2c_NoProtocolSet verifies that when EnableH2c is
// false (default) the transport does not set UnencryptedHTTP2.
func TestTransportDisabledH2c_NoProtocolSet(t *testing.T) {
	noleak.Check(t)

	tr := NewTransport(Options{})
	defer tr.Close()

	if tr.tr.Protocols != nil && tr.tr.Protocols.UnencryptedHTTP2() {
		t.Error("expected UnencryptedHTTP2() to be false when EnableH2c is not set")
	}
}

// TestTransportH2cOnly_FailsOnHTTP1Server documents that a transport with only
// SetUnencryptedHTTP2 enabled (no SetHTTP1) cannot fall back to HTTP/1. It will
// fail on a plain HTTP/1-only server. Callers that need mixed support must enable
// both protocols on the transport.
func TestTransportH2cOnly_FailsOnHTTP1Server(t *testing.T) {
	noleak.Check(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := NewTransport(Options{EnableH2c: true})
	defer tr.Close()

	req, err := http.NewRequest("GET", srv.URL+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	rsp, err := tr.RoundTrip(req)
	if err == nil {
		rsp.Body.Close()
		t.Error("expected h2c-only transport to fail against a plain HTTP/1 server")
	}
}
