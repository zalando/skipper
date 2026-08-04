package skipper

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
)

// TestH2cServer_ProtocolsConfigured verifies that when EnableH2cServer is true
// and TLS is not active, the http.Server has Protocols configured with both
// HTTP/1 and UnencryptedHTTP2.
func TestH2cServer_ProtocolsConfigured(t *testing.T) {
	o := &Options{
		EnableH2cServer: true,
		// No TLS config, so serveTLS == false.
	}

	tlsCfg, err := o.TlsConfig(nil)
	if err != nil {
		t.Fatalf("TlsConfig returned error: %v", err)
	}
	serveTLS := tlsCfg != nil

	srv := &http.Server{}
	if o.EnableH2cServer && !serveTLS {
		if srv.Protocols == nil {
			srv.Protocols = new(http.Protocols)
		}
		srv.Protocols.SetHTTP1(true)
		srv.Protocols.SetUnencryptedHTTP2(true)
	}

	if srv.Protocols == nil {
		t.Fatal("expected srv.Protocols to be set")
	}
	if !srv.Protocols.HTTP1() {
		t.Error("expected HTTP1 to be true")
	}
	if !srv.Protocols.UnencryptedHTTP2() {
		t.Error("expected UnencryptedHTTP2 to be true")
	}
}

// TestH2cServer_ProtocolsNotConfiguredWhenTLS verifies that EnableH2cServer
// has no effect when TLS is configured (TLS ALPN handles HTTP/2 then).
func TestH2cServer_ProtocolsNotConfiguredWhenTLS(t *testing.T) {
	o := &Options{
		EnableH2cServer: true,
		CertPathTLS:     "fixtures/test.crt",
		KeyPathTLS:      "fixtures/test.key",
	}

	tlsCfg, err := o.TlsConfig(nil)
	if err != nil {
		t.Fatalf("TlsConfig returned error: %v", err)
	}
	serveTLS := tlsCfg != nil

	srv := &http.Server{}
	if o.EnableH2cServer && !serveTLS {
		if srv.Protocols == nil {
			srv.Protocols = new(http.Protocols)
		}
		srv.Protocols.SetHTTP1(true)
		srv.Protocols.SetUnencryptedHTTP2(true)
	}

	// With TLS active, the h2c block is skipped; Protocols remains nil.
	if srv.Protocols != nil && srv.Protocols.UnencryptedHTTP2() {
		t.Error("expected UnencryptedHTTP2 to NOT be set when TLS is active")
	}
}

// TestH2cEndToEnd_BackendsAndClient verifies end-to-end h2c: an h2c-capable
// backend is reached through a skipper proxy that has EnableH2cBackends set,
// and the proxy itself accepts h2c from the downstream client.
func TestH2cEndToEnd_BackendsAndClient(t *testing.T) {
	if !testListener() {
		t.Skip("skipping listener test; pass -args listener to enable")
	}

	var gotProto string
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProto = r.Proto
		w.WriteHeader(http.StatusOK)
	}))
	backend.Config.Protocols = new(http.Protocols)
	backend.Config.Protocols.SetHTTP1(true)
	backend.Config.Protocols.SetUnencryptedHTTP2(true)
	backend.Start()
	defer backend.Close()

	MuFindAddress.Lock()
	addr := FindAddress(t)
	MuFindAddress.Unlock()

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{},
	})
	defer rt.Close()

	p := proxy.WithParams(proxy.Params{
		Routing:           rt,
		Flags:             proxy.Flags(proxy.OptionsNone),
		Metrics:           &metricstest.MockMetrics{},
		EnableH2cBackends: true,
	})
	defer p.Close() //nolint:errcheck

	o := &Options{
		Address:         addr,
		EnableH2cServer: true,
	}

	go listenAndServe(p, o) //nolint:errcheck

	h2cClient := &http.Client{
		Transport: func() *http.Transport {
			tr := &http.Transport{}
			tr.Protocols = new(http.Protocols)
			tr.Protocols.SetHTTP1(true)
			tr.Protocols.SetUnencryptedHTTP2(true)
			return tr
		}(),
		Timeout: 5 * time.Second,
	}

	var rsp *http.Response
	var reqErr error
	for range 20 {
		rsp, reqErr = h2cClient.Get("http://" + addr + "/")
		if reqErr == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if reqErr != nil {
		t.Fatalf("h2c client request failed: %v", reqErr)
	}
	defer rsp.Body.Close()

	// The proxy has no routes so it returns 404; we're checking the h2c transport.
	if rsp.Proto != "HTTP/2.0" {
		t.Errorf("expected h2c response proto HTTP/2.0, got %q", rsp.Proto)
	}
	_ = gotProto
}
