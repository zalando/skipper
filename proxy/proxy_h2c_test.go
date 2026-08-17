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

// TestH2cProxy verifies that a route with an h2c:// backend URL reaches the
// backend over HTTP/2 cleartext, while http:// backends use HTTP/1.x.
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
			if gotProto != tt.wantProto {
				t.Errorf("expected backend to receive %q, got %q", tt.wantProto, gotProto)
			}
		})
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

// TestH2cServer_H2cTransportConfigured verifies that the proxy's h2c round-tripper
// has UnencryptedHTTP2 enabled.
func TestH2cServer_H2cTransportConfigured(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, backend.URL),
		Params{},
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
