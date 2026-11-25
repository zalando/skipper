package proxy

import (
	stdlibcontext "context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	snettest "github.com/zalando/skipper/net/nettest"
)

// newH3Backend starts an HTTP/3 (QUIC) backend server using a self-signed TLS
// certificate. It records the incoming request protocol. Returns the server's
// UDP address ("host:port"), the TLS root pool for the self-signed cert, and
// a cleanup func.
func newH3Backend(t *testing.T, gotProto *string) (addr string, roots *x509.CertPool, cleanup func()) {
	t.Helper()

	ca := snettest.NewCA(t)
	certPEM, keyPEM := ca.ServerCert(t, "127.0.0.1")

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed to create TLS certificate for h3 backend: %v", err)
	}

	roots = x509.NewCertPool()
	roots.AddCert(ca.Cert)

	tlsConf := http3.ConfigureTLSConfig(&tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen UDP for h3 backend: %v", err)
	}

	srv := &http3.Server{
		TLSConfig: tlsConf,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if gotProto != nil {
				*gotProto = r.Proto
			}
			w.WriteHeader(http.StatusOK)
		}),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(conn)
	}()

	addr = conn.LocalAddr().String()
	cleanup = func() {
		srv.Close()
		conn.Close()
	}
	return addr, roots, cleanup
}

// TestH3BackendDisabledByDefault verifies that without EnableH3Backend, the
// h3RoundTripper is nil.
func TestH3BackendDisabledByDefault(t *testing.T) {
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

	if tp.proxy.h3RoundTripper != nil {
		t.Error("expected h3RoundTripper to be nil when EnableH3Backend is false")
	}
}

// TestH3BackendEnabled verifies that with EnableH3Backend set, a route with an
// h3:// backend URL sends the request over HTTP/3 (QUIC).
func TestH3BackendEnabled(t *testing.T) {
	var gotProto string
	addr, roots, cleanup := newH3Backend(t, &gotProto)
	defer cleanup()

	h3URL := "h3://" + addr

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, h3URL),
		Params{
			EnableH3Backend: true,
			ClientTLS:       &tls.Config{RootCAs: roots},
		},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	ctx, cancel := stdlibcontext.WithTimeout(stdlibcontext.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ps.URL+"/", nil)
	rsp, err := ps.Client().Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	if gotProto != "HTTP/3.0" {
		t.Errorf("expected backend to receive HTTP/3.0, got %q", gotProto)
	}
}

// TestH3TransportConfigured verifies that the h3 round-tripper is a non-nil
// *http3.Transport when EnableH3Backend is set and no TLS config is provided.
func TestH3TransportConfigured(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, backend.URL),
		Params{EnableH3Backend: true},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ht, ok := tp.proxy.h3RoundTripper.(*http3.Transport)
	if !ok {
		t.Fatalf("h3RoundTripper is not *http3.Transport, got %T", tp.proxy.h3RoundTripper)
	}
	if ht.TLSClientConfig != nil {
		t.Error("expected TLSClientConfig to be nil when no TLS params are set")
	}
}

// TestH3TransportInsecure verifies that the Insecure flag propagates to the
// h3 transport's TLSClientConfig.
func TestH3TransportInsecure(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, backend.URL),
		Params{
			EnableH3Backend: true,
			Flags:           Insecure,
		},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ht, ok := tp.proxy.h3RoundTripper.(*http3.Transport)
	if !ok {
		t.Fatalf("h3RoundTripper is not *http3.Transport, got %T", tp.proxy.h3RoundTripper)
	}
	if ht.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig to be set when Insecure flag is used")
	}
	if !ht.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true on h3 transport when Insecure flag is set")
	}
}

// TestH3TransportMTLS verifies that mTLS configuration is propagated to the
// h3 transport's TLSClientConfig.
func TestH3TransportMTLS(t *testing.T) {
	_, _, caCert, caKey := makeMTLSCA(t, "h3-test-ca")
	certPEM, keyPEM := makeMTLSClientCert(t, "h3-test-client", caCert, caKey)
	certFile, keyFile := writeMTLSTempCertFiles(t, certPEM, keyPEM)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	tp, err := newTestProxyWithParams(
		fmt.Sprintf(`* -> "%s"`, backend.URL),
		Params{
			EnableH3Backend: true,
			EnableMTLS:      true,
			ClientCertFile:  certFile,
			ClientKeyFile:   keyFile,
		},
	)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ht, ok := tp.proxy.h3RoundTripper.(*http3.Transport)
	if !ok {
		t.Fatalf("h3RoundTripper is not *http3.Transport, got %T", tp.proxy.h3RoundTripper)
	}
	if ht.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig to be set when mTLS is enabled")
	}
	if ht.TLSClientConfig.GetClientCertificate == nil {
		t.Error("expected GetClientCertificate to be set on h3 transport when mTLS is enabled")
	}
}

func TestH3TransportTimeouts(t *testing.T) {
	tr := newH3Transport(Params{
		Timeout:              100 * time.Millisecond,
		TLSHandshakeTimeout:  2 * time.Second,
		CloseIdleConnsPeriod: 15 * time.Second,
		KeepAlive:            5 * time.Second,
	})
	if tr.Dial == nil {
		t.Fatal("expected Dial to be set on h3 transport")
	}
	if tr.QUICConfig == nil {
		t.Fatal("expected QUICConfig to be set on h3 transport")
	}
	if tr.QUICConfig.MaxIncomingStreams != -1 {
		t.Errorf("expected MaxIncomingStreams -1, got %d", tr.QUICConfig.MaxIncomingStreams)
	}
	if tr.QUICConfig.HandshakeIdleTimeout != 2*time.Second {
		t.Errorf("expected HandshakeIdleTimeout 2s, got %v", tr.QUICConfig.HandshakeIdleTimeout)
	}
	if tr.QUICConfig.MaxIdleTimeout != 15*time.Second {
		t.Errorf("expected MaxIdleTimeout 15s, got %v", tr.QUICConfig.MaxIdleTimeout)
	}
	if tr.QUICConfig.KeepAlivePeriod != 5*time.Second {
		t.Errorf("expected KeepAlivePeriod 5s, got %v", tr.QUICConfig.KeepAlivePeriod)
	}

	trNeg := newH3Transport(Params{
		CloseIdleConnsPeriod: -time.Second,
	})
	if trNeg.QUICConfig.MaxIdleTimeout != 0 {
		t.Errorf("expected MaxIdleTimeout 0 for negative CloseIdleConnsPeriod, got %v", trNeg.QUICConfig.MaxIdleTimeout)
	}
}

func TestH3DialTimeoutAndLBRetry(t *testing.T) {
	blackhole, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen UDP: %v", err)
	}
	defer blackhole.Close()

	var gotProto string
	healthyAddr, roots, cleanup := newH3Backend(t, &gotProto)
	defer cleanup()

	badURL := "h3://" + blackhole.LocalAddr().String()
	goodURL := "h3://" + healthyAddr

	doc := fmt.Sprintf(`
		bad: Path("/bad") -> "%s";
		lb:  Path("/lb")  -> <roundRobin, "%s", "%s">;
	`, badURL, badURL, goodURL)

	tp, err := newTestProxyWithParams(doc, Params{
		EnableH3Backend: true,
		Timeout:         50 * time.Millisecond,
		ClientTLS:       &tls.Config{RootCAs: roots},
	})
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL + "/bad")
	if err != nil {
		t.Fatalf("request to /bad failed: %v", err)
	}
	rsp.Body.Close()
	if rsp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502 for unreachable h3 backend, got %d", rsp.StatusCode)
	}

	rsp, err = ps.Client().Get(ps.URL + "/lb")
	if err != nil {
		t.Fatalf("request to /lb failed: %v", err)
	}
	rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after LB retry on h3 backend, got %d", rsp.StatusCode)
	}
	if gotProto != "HTTP/3.0" {
		t.Errorf("expected backend to receive HTTP/3.0, got %q", gotProto)
	}
}
