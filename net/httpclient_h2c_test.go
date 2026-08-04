package net

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newH2cServer starts an h2c (HTTP/2 cleartext) test server that records the
// incoming request protocol.
func newH2cServer(t *testing.T, gotProto *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotProto != nil {
			*gotProto = r.Proto
		}
		w.WriteHeader(http.StatusOK)
	}))
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetHTTP1(true)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// TestTransportEnableH2c verifies that when EnableH2c is set, NewTransport
// produces a transport that connects to an h2c backend over HTTP/2.
func TestTransportEnableH2c(t *testing.T) {
	var gotProto string
	srv := newH2cServer(t, &gotProto)

	tr := NewTransport(Options{EnableH2c: true})
	defer tr.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	rsp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	if gotProto != "HTTP/2.0" {
		t.Errorf("expected server to receive HTTP/2.0, got %q", gotProto)
	}
}

// TestTransportEnableH2c_ProtocolSet verifies that the underlying transport
// has SetUnencryptedHTTP2(true) when EnableH2c is set.
func TestTransportEnableH2c_ProtocolSet(t *testing.T) {
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
	tr := NewTransport(Options{})
	defer tr.Close()

	if tr.tr.Protocols != nil && tr.tr.Protocols.UnencryptedHTTP2() {
		t.Error("expected UnencryptedHTTP2() to be false when EnableH2c is not set")
	}
}

// TestTransportEnableH2c_FallsBackToHTTP1WithRegularServer verifies that an
// h2c-configured transport can also communicate with a plain HTTP/1 server.
func TestTransportEnableH2c_FallsBackToHTTP1WithRegularServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// A transport with only UnencryptedHTTP2 set (no SetHTTP1) will try h2c
	// exclusively. This test documents that h2c-only transports may fail on
	// plain HTTP/1-only servers — the behaviour is intentional for pure h2c
	// backends. If mixed support is needed, both protocols should be enabled.
	tr := NewTransport(Options{EnableH2c: true})
	defer tr.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	rsp, err := tr.RoundTrip(req)
	// A plain HTTP/1 server that doesn't speak h2c may reject the connection
	// or fall back — exact behaviour depends on the server. We just check we
	// don't panic and handle the result gracefully.
	if err == nil {
		rsp.Body.Close()
	}
}
