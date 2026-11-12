package proxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newH2Backend starts a TLS backend that accepts HTTP/2 and records the incoming protocol.
func newH2Backend(t *testing.T, gotProto *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotProto != nil {
			*gotProto = r.Proto
		}
		w.WriteHeader(http.StatusOK)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// TestH2BackendDisabledByDefault verifies that without EnableH2Backend, an h2://
// route falls through to the default transport, which cannot negotiate HTTP/2
// with a TLS backend that does not have an h2 ALPN fallback.
func TestH2BackendDisabledByDefault(t *testing.T) {
	backend := newH2Backend(t, nil)

	h2URL := "h2" + backend.URL[len("https"):]

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, h2URL),
		Params{},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	if tp.proxy.h2RoundTripper != nil {
		t.Error("expected h2RoundTripper to be nil when EnableH2Backend is false")
	}
}

// TestH2BackendEnabled verifies that with EnableH2Backend set, a route with an
// h2:// backend URL negotiates HTTP/2 with the TLS backend.
func TestH2BackendEnabled(t *testing.T) {
	var gotProto string
	backend := newH2Backend(t, &gotProto)

	h2URL := "h2" + backend.URL[len("https"):]

	/* #nosec */
	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, h2URL),
		Params{
			EnableH2Backend: true,
			ClientTLS:       &tls.Config{InsecureSkipVerify: true},
		},
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

// TestH2TransportConfigured verifies that the proxy's h2 round-tripper has
// ForceAttemptHTTP2 enabled when EnableH2Backend is set.
func TestH2TransportConfigured(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, backend.URL),
		Params{EnableH2Backend: true},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ht, ok := tp.proxy.h2RoundTripper.(*http.Transport)
	if !ok {
		t.Fatalf("h2RoundTripper is not *http.Transport, got %T", tp.proxy.h2RoundTripper)
	}
	if !ht.ForceAttemptHTTP2 {
		t.Error("expected ForceAttemptHTTP2 to be true on h2 transport")
	}

	rt, ok := tp.proxy.roundTripper.(*http.Transport)
	if ok && rt.ForceAttemptHTTP2 {
		t.Error("standard roundTripper must NOT have ForceAttemptHTTP2 set")
	}
}
