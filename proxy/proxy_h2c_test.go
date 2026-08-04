package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newH2cBackend starts an h2c (HTTP/2 cleartext) backend server.
// It records the incoming request protocol and serves a 200 response.
func newH2cBackend(t *testing.T, gotProto *string) *httptest.Server {
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

// TestH2cBackend_ProxyConnectsUsingH2c verifies that when EnableH2cBackends is
// true the proxy forwards to the backend over HTTP/2 cleartext.
func TestH2cBackend_ProxyConnectsUsingH2c(t *testing.T) {
	var gotProto string
	backend := newH2cBackend(t, &gotProto)

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, backend.URL),
		Params{EnableH2cBackends: true},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	if gotProto != "HTTP/2.0" {
		t.Errorf("expected backend to receive HTTP/2.0, got %q", gotProto)
	}
}

// TestH2cBackend_DisabledFallsBackToHTTP1 verifies that when EnableH2cBackends
// is false the proxy uses HTTP/1.x to reach the backend.
func TestH2cBackend_DisabledFallsBackToHTTP1(t *testing.T) {
	var gotProto string
	backend := newH2cBackend(t, &gotProto)

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, backend.URL),
		Params{EnableH2cBackends: false},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	if gotProto == "HTTP/2.0" {
		t.Errorf("expected HTTP/1.x on backend, got %q", gotProto)
	}
}

// TestH2cScheme_RouteUsesH2cTransport verifies that a route with an h2c:// backend
// URL reaches the backend via HTTP/2 cleartext without any flag set.
func TestH2cScheme_RouteUsesH2cTransport(t *testing.T) {
	var gotProto string
	backend := newH2cBackend(t, &gotProto)

	// Replace http:// with h2c:// in the backend URL.
	h2cURL := "h2c" + backend.URL[len("http"):]

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, h2cURL),
		Params{},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	if gotProto != "HTTP/2.0" {
		t.Errorf("expected backend to receive HTTP/2.0, got %q", gotProto)
	}
}

// TestH2cScheme_LBEndpointUsesH2cTransport verifies that LB endpoints with h2c://
// scheme reach the backend via HTTP/2 cleartext without any flag set.
func TestH2cScheme_LBEndpointUsesH2cTransport(t *testing.T) {
	var gotProto string
	backend := newH2cBackend(t, &gotProto)

	h2cURL := "h2c" + backend.URL[len("http"):]

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> <"%s">`, h2cURL),
		Params{},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	if gotProto != "HTTP/2.0" {
		t.Errorf("expected backend to receive HTTP/2.0, got %q", gotProto)
	}
}

// TestH2cScheme_HttpBackendStillUsesHTTP1 verifies that plain http:// backends
// are unaffected by h2c:// scheme support (no flag, no upgrade).
func TestH2cScheme_HttpBackendStillUsesHTTP1(t *testing.T) {
	var gotProto string
	backend := newH2cBackend(t, &gotProto)

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, backend.URL),
		Params{},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	if gotProto == "HTTP/2.0" {
		t.Errorf("expected HTTP/1.x for http:// backend, got %q", gotProto)
	}
}

// TestH2cServer_AcceptsH2cClient verifies that when EnableH2cBackends is set,
// the proxy transport is configured to use h2c (SetUnencryptedHTTP2 is set).
func TestH2cServer_TransportProtocolSet(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, backend.URL),
		Params{EnableH2cBackends: true},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	// Access the underlying transport via the proxy's roundTripper.
	// The proxy stores it as an http.RoundTripper; we can type-assert to
	// *http.Transport to inspect the Protocols field.
	ht, ok := tp.proxy.roundTripper.(*http.Transport)
	if !ok {
		t.Skip("roundTripper is not *http.Transport, skipping protocol check")
	}
	if ht.Protocols == nil {
		t.Fatal("expected Protocols to be set on the transport, got nil")
	}
	if !ht.Protocols.UnencryptedHTTP2() {
		t.Error("expected UnencryptedHTTP2 to be true on the transport")
	}
}
