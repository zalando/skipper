package proxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newH2cBackend starts an h2c (HTTP/2 cleartext) backend server.
// It records the incoming request protocol and serves a 200 response.
func newH2cBackend(t *testing.T, gotProto *string) *httptest.Server {
	t.Helper()
	return newH2cBackendWithTLSCheck(t, gotProto, nil)
}

// newH2cBackendWithTLSCheck starts an h2c backend that additionally records
// whether the request arrived over TLS. gotTLS is set to true if r.TLS != nil.
func newH2cBackendWithTLSCheck(t *testing.T, gotProto *string, gotTLS *bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotProto != nil {
			*gotProto = r.Proto
		}
		if gotTLS != nil {
			*gotTLS = r.TLS != nil
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

// TestH2CBackendDisabledByDefault verifies that without EnableH2CBackend, an h2c://
// route falls through to the default HTTP/1.1 transport.
func TestH2CBackendDisabledByDefault(t *testing.T) {
	var gotProto string
	backend := newH2cBackend(t, &gotProto)

	h2cURL := "h2c" + backend.URL[len("http"):]
	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, h2cURL),
		Params{},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	if tp.proxy.h2cRoundTripper != nil {
		t.Error("expected h2cRoundTripper to be nil when EnableH2CBackend is false")
	}

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
	if gotProto != "HTTP/1.1" {
		t.Errorf("expected HTTP/1.1 fallback, got %q", gotProto)
	}
}

// TestH2cProxy verifies that a route with an h2c:// backend URL reaches the
// backend over HTTP/2 cleartext when EnableH2CBackend is set, while http://
// backends use HTTP/1.x.
func TestH2cProxy(t *testing.T) {
	for _, tt := range []struct {
		name      string
		useH2cURL bool
		wantProto string
	}{
		{
			name:      "h2c scheme reaches backend via HTTP/2",
			useH2cURL: true,
			wantProto: "HTTP/2.0",
		},
		{
			name:      "http scheme uses HTTP/1",
			useH2cURL: false,
			wantProto: "HTTP/1.1",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var gotProto string
			backend := newH2cBackend(t, &gotProto)

			backendURL := backend.URL
			if tt.useH2cURL {
				backendURL = "h2c" + backendURL[len("http"):]
			}

			tp, err := newTestProxyWithParams(
				fmt.Sprintf(`* -> "%s"`, backendURL),
				Params{EnableH2CBackend: true},
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
			if gotProto != tt.wantProto {
				t.Errorf("expected backend to receive %q, got %q", tt.wantProto, gotProto)
			}
		})
	}
}

// TestH2cScheme_LBEndpointUsesH2cTransport verifies that LB endpoints with h2c://
// scheme reach the backend via HTTP/2 cleartext when EnableH2CBackend is set.
func TestH2cScheme_LBEndpointUsesH2cTransport(t *testing.T) {
	var gotProto string
	backend := newH2cBackend(t, &gotProto)

	h2cURL := "h2c" + backend.URL[len("http"):]

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> <"%s">`, h2cURL),
		Params{EnableH2CBackend: true},
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

// TestH2cServer_H2cTransportConfigured verifies that the proxy's h2c round-tripper
// has UnencryptedHTTP2 enabled when EnableH2CBackend is set.
func TestH2cServer_H2cTransportConfigured(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, backend.URL),
		Params{EnableH2CBackend: true},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ht, ok := tp.proxy.h2cRoundTripper.(*http.Transport)
	if !ok {
		t.Fatalf("h2cRoundTripper is not *http.Transport, got %T", tp.proxy.h2cRoundTripper)
	}
	if ht.Protocols == nil {
		t.Fatal("expected Protocols to be set on h2c transport, got nil")
	}
	if !ht.Protocols.UnencryptedHTTP2() {
		t.Error("expected UnencryptedHTTP2 to be true on h2c transport")
	}

	rt, ok := tp.proxy.roundTripper.(*http.Transport)
	if ok && rt.Protocols != nil && rt.Protocols.UnencryptedHTTP2() {
		t.Error("standard roundTripper must NOT have UnencryptedHTTP2 set")
	}
}

// TestH2CAndH2BothEnabled_SharedTLSConfigBug documents a bug where enabling both
// EnableH2CBackend and EnableH2Backend with a non-nil ClientTLS causes h2cTr and
// h2Tr to share the same *tls.Config pointer. When h2Tr initializes HTTP/2 on first
// use it mutates TLSClientConfig.NextProtos on the shared config, which also affects
// h2cTr. The fix is to Clone the TLS config for each transport independently.
//
// This test is expected to FAIL until the bug is fixed.
func TestH2CAndH2BothEnabled_SharedTLSConfigBug(t *testing.T) {
	var h2cProto string
	var h2cTLS bool
	h2cBackend := newH2cBackendWithTLSCheck(t, &h2cProto, &h2cTLS)
	h2cURL := "h2c" + h2cBackend.URL[len("http"):]

	var h2Proto string
	h2Backend := newH2Backend(t, &h2Proto)
	h2URL := "h2" + h2Backend.URL[len("https"):]

	doc := fmt.Sprintf(`
		r_h2c: Path("/h2c") -> "%s";
		r_h2:  Path("/h2")  -> "%s"
	`, h2cURL, h2URL)

	/* #nosec */
	tp, err := newTestProxyWithParams(doc, Params{
		EnableH2CBackend: true,
		EnableH2Backend:  true,
		ClientTLS:        &tls.Config{InsecureSkipVerify: true},
	})
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	h2cTr, ok := tp.proxy.h2cRoundTripper.(*http.Transport)
	if !ok {
		t.Fatalf("h2cRoundTripper is not *http.Transport, got %T", tp.proxy.h2cRoundTripper)
	}
	h2Tr, ok := tp.proxy.h2RoundTripper.(*http.Transport)
	if !ok {
		t.Fatalf("h2RoundTripper is not *http.Transport, got %T", tp.proxy.h2RoundTripper)
	}

	// BUG: both transports currently share the same *tls.Config pointer.
	// When h2Tr initialises HTTP/2 on first use it mutates NextProtos on the
	// shared config, which silently corrupts h2cTr's TLS config too.
	if h2cTr.TLSClientConfig == h2Tr.TLSClientConfig {
		t.Error("bug: h2cRoundTripper and h2RoundTripper share the same *tls.Config pointer; " +
			"mutations by one (e.g. NextProtos set by HTTP/2 init) affect the other")
	}

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	// Send the h2 request first to trigger h2Tr.onceSetNextProtoDefaults, which
	// mutates TLSClientConfig.NextProtos on the shared config.
	rsp, err := ps.Client().Get(ps.URL + "/h2")
	if err != nil {
		t.Fatalf("h2 proxy request failed: %v", err)
	}
	rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		t.Errorf("h2: expected 200, got %d", rsp.StatusCode)
	}
	if h2Proto != "HTTP/2.0" {
		t.Errorf("h2: expected HTTP/2.0, got %q", h2Proto)
	}

	// Send the h2c request after h2Tr has initialised. If the TLS configs are
	// shared, h2cTr's NextProtos may have been polluted with "h2", potentially
	// causing unexpected behaviour on cleartext connections.
	rsp, err = ps.Client().Get(ps.URL + "/h2c")
	if err != nil {
		t.Fatalf("h2c proxy request failed: %v", err)
	}
	rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		t.Errorf("h2c: expected 200, got %d", rsp.StatusCode)
	}
	if h2cProto != "HTTP/2.0" {
		t.Errorf("h2c: expected HTTP/2.0, got %q", h2cProto)
	}
	if h2cTLS {
		t.Error("h2c: backend received a TLS request — h2c must use cleartext (shared TLS config bug)")
	}
}
