package skipper

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/dataclients/routestring"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/net/nettest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
)

// TestH2BackendEndToEnd verifies end-to-end HTTP/2: a TLS backend is reached
// over HTTP/2 when EnableH2Backend is set in proxy.Params.
func TestH2BackendEndToEnd(t *testing.T) {
	if !testListener() {
		t.Skip("skipping listener test; pass -args listener to enable")
	}

	var mu sync.Mutex
	var proto string
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		proto = r.Proto
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	backend.EnableHTTP2 = true
	backend.StartTLS()
	t.Cleanup(backend.Close)

	h2URL := "h2" + backend.URL[len("https"):]

	dc, err := routestring.New(fmt.Sprintf(`r0: * -> "%s"`, h2URL))
	if err != nil {
		t.Fatalf("failed to create data client: %v", err)
	}

	MuFindAddress.Lock()
	addr := FindAddress(t)
	MuFindAddress.Unlock()

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{dc},
	})
	defer rt.Close()

	/* #nosec */
	p := proxy.WithParams(proxy.Params{
		Routing:         rt,
		Flags:           proxy.Flags(proxy.OptionsNone),
		Metrics:         &metricstest.MockMetrics{},
		EnableH2Backend: true,
		ClientTLS:       &tls.Config{InsecureSkipVerify: true},
	})
	defer p.Close() //nolint:errcheck

	o := &Options{
		Address: addr,
	}

	sigs := make(chan os.Signal, 1)
	go func() {
		listenAndServeQuit(p, o, sigs, nil, nil, nil) //nolint:errcheck
	}()
	t.Cleanup(func() { sigs <- syscall.SIGTERM })

	client := &http.Client{Timeout: 5 * time.Second}

	var rsp *http.Response
	var reqErr error
	for range 20 {
		rsp, reqErr = client.Get("http://" + addr + "/")
		if reqErr == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if reqErr != nil {
		t.Fatalf("client request failed: %v", reqErr)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	mu.Lock()
	gotProto := proto
	mu.Unlock()
	if gotProto != "HTTP/2.0" {
		t.Errorf("expected proxy-to-backend proto HTTP/2.0, got %q", gotProto)
	}
}

// TestH2CBackendEndToEnd verifies end-to-end h2c: a cleartext HTTP/2 backend is
// reached via the h2c:// URL scheme when EnableH2CBackend is set in proxy.Params.
func TestH2CBackendEndToEnd(t *testing.T) {
	if !testListener() {
		t.Skip("skipping listener test; pass -args listener to enable")
	}

	var mu sync.Mutex
	var proto string
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		proto = r.Proto
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	backend.Config.Protocols = new(http.Protocols)
	backend.Config.Protocols.SetHTTP1(true)
	backend.Config.Protocols.SetUnencryptedHTTP2(true)
	backend.Start()
	t.Cleanup(backend.Close)

	h2cURL := "h2c" + backend.URL[len("http"):]

	dc, err := routestring.New(fmt.Sprintf(`r0: * -> "%s"`, h2cURL))
	if err != nil {
		t.Fatalf("failed to create data client: %v", err)
	}

	MuFindAddress.Lock()
	addr := FindAddress(t)
	MuFindAddress.Unlock()

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{dc},
	})
	defer rt.Close()

	p := proxy.WithParams(proxy.Params{
		Routing:          rt,
		Flags:            proxy.Flags(proxy.OptionsNone),
		Metrics:          &metricstest.MockMetrics{},
		EnableH2CBackend: true,
	})
	defer p.Close() //nolint:errcheck

	o := &Options{
		Address: addr,
	}

	sigs := make(chan os.Signal, 1)
	go func() {
		listenAndServeQuit(p, o, sigs, nil, nil, nil) //nolint:errcheck
	}()
	t.Cleanup(func() { sigs <- syscall.SIGTERM })

	client := &http.Client{Timeout: 5 * time.Second}

	var rsp *http.Response
	var reqErr error
	for range 20 {
		rsp, reqErr = client.Get("http://" + addr + "/")
		if reqErr == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if reqErr != nil {
		t.Fatalf("client request failed: %v", reqErr)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	mu.Lock()
	gotProto := proto
	mu.Unlock()
	if gotProto != "HTTP/2.0" {
		t.Errorf("expected proxy-to-backend h2c proto HTTP/2.0, got %q", gotProto)
	}
}

// TestH3BackendEndToEnd verifies end-to-end HTTP/3: a QUIC backend is reached
// via the h3:// URL scheme when EnableH3Backend is set in proxy.Params.
func TestH3BackendEndToEnd(t *testing.T) {
	if !testListener() {
		t.Skip("skipping listener test; pass -args listener to enable")
	}

	ca := nettest.NewCA(t)
	certPEM, keyPEM := ca.ServerCert(t, "127.0.0.1")

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed to create TLS certificate: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)

	tlsConf := http3.ConfigureTLSConfig(&tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen UDP: %v", err)
	}

	var mu sync.Mutex
	var proto string
	srv := &http3.Server{
		TLSConfig: tlsConf,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			proto = r.Proto
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}),
	}
	go srv.Serve(conn) //nolint:errcheck
	t.Cleanup(func() {
		srv.Close()
		conn.Close()
	})

	h3URL := "h3://" + conn.LocalAddr().String()

	dc, err := routestring.New(fmt.Sprintf(`r0: * -> "%s"`, h3URL))
	if err != nil {
		t.Fatalf("failed to create data client: %v", err)
	}

	MuFindAddress.Lock()
	addr := FindAddress(t)
	MuFindAddress.Unlock()

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{dc},
	})
	defer rt.Close()

	p := proxy.WithParams(proxy.Params{
		Routing:         rt,
		Flags:           proxy.Flags(proxy.OptionsNone),
		Metrics:         &metricstest.MockMetrics{},
		EnableH3Backend: true,
		ClientTLS:       &tls.Config{RootCAs: roots},
	})
	defer p.Close() //nolint:errcheck

	o := &Options{
		Address: addr,
	}

	sigs := make(chan os.Signal, 1)
	go func() {
		listenAndServeQuit(p, o, sigs, nil, nil, nil) //nolint:errcheck
	}()
	t.Cleanup(func() { sigs <- syscall.SIGTERM })

	client := &http.Client{Timeout: 5 * time.Second}

	var rsp *http.Response
	var reqErr error
	for range 20 {
		rsp, reqErr = client.Get("http://" + addr + "/")
		if reqErr == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if reqErr != nil {
		t.Fatalf("client request failed: %v", reqErr)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	mu.Lock()
	gotProto := proto
	mu.Unlock()
	if gotProto != "HTTP/3.0" {
		t.Errorf("expected proxy-to-backend h3 proto HTTP/3.0, got %q", gotProto)
	}
}

// TestH3ServerEndToEnd verifies that skipper accepts HTTP/3 (QUIC) connections
// when EnableH3Server is set and TLS is configured, and that the Alt-Svc header
// is present in the response.
func TestH3ServerEndToEnd(t *testing.T) {
	if !testListener() {
		t.Skip("skipping listener test; pass -args listener to enable")
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	dc, err := routestring.New(fmt.Sprintf(`r0: * -> "%s"`, backend.URL))
	if err != nil {
		t.Fatalf("failed to create data client: %v", err)
	}

	MuFindAddress.Lock()
	addr := FindAddress(t)
	MuFindAddress.Unlock()

	// Generate TLS cert for skipper's listener.
	ca := nettest.NewCA(t)
	host, _, _ := net.SplitHostPort(addr)
	if host == "" {
		host = "127.0.0.1"
	}
	certPEM, keyPEM := ca.ServerCert(t, host)
	certFile, keyFile := nettest.WriteCertKey(t, certPEM, keyPEM)

	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{dc},
	})
	defer rt.Close()

	p := proxy.WithParams(proxy.Params{
		Routing: rt,
		Flags:   proxy.Flags(proxy.OptionsNone),
		Metrics: &metricstest.MockMetrics{},
	})
	defer p.Close() //nolint:errcheck

	o := &Options{
		Address:        addr,
		CertPathTLS:    certFile,
		KeyPathTLS:     keyFile,
		EnableH3Server: true,
	}

	sigs := make(chan os.Signal, 1)
	go func() {
		listenAndServeQuit(p, o, sigs, nil, nil, nil) //nolint:errcheck
	}()
	t.Cleanup(func() { sigs <- syscall.SIGTERM })

	h3Transport := &http3.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots},
	}
	defer h3Transport.Close()
	h3Client := &http.Client{Transport: h3Transport, Timeout: 5 * time.Second}

	var rsp *http.Response
	var reqErr error
	for range 30 {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+"/", nil)
		rsp, reqErr = h3Client.Do(req)
		cancel()
		if reqErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if reqErr != nil {
		t.Fatalf("h3 client request failed: %v", reqErr)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
	if rsp.Header.Get("Alt-Svc") == "" {
		t.Error("expected Alt-Svc header to be set")
	}
}

func TestH3ServerGracefulShutdown(t *testing.T) {
	if !testListener() {
		t.Skip("skipping listener test; pass -args listener to enable")
	}

	const shutdownDelay = 1 * time.Second

	dc, err := routestring.New(`
		slow: Path("/slow") -> latency("3s") -> inlineContent("OK") -> status(200) -> <shunt>;
		fast: * -> status(200) -> <shunt>;
	`)
	require.NoError(t, err)

	MuFindAddress.Lock()
	addr := FindAddress(t)
	MuFindAddress.Unlock()

	ca := nettest.NewCA(t)
	host, _, _ := net.SplitHostPort(addr)
	if host == "" {
		host = "127.0.0.1"
	}
	certPEM, keyPEM := ca.ServerCert(t, host)
	certFile, keyFile := nettest.WriteCertKey(t, certPEM, keyPEM)

	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{dc},
	})
	defer rt.Close()

	p := proxy.WithParams(proxy.Params{
		Routing:              rt,
		Flags:                proxy.Flags(proxy.OptionsNone),
		CloseIdleConnsPeriod: -time.Second,
		Metrics:              &metricstest.MockMetrics{},
	})
	defer p.Close() //nolint:errcheck

	o := &Options{
		Address:                    addr,
		CertPathTLS:                certFile,
		KeyPathTLS:                 keyFile,
		EnableH3Server:             true,
		WaitForHealthcheckInterval: shutdownDelay,
	}

	sigs := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		assert.NoError(t, listenAndServeQuit(p, o, sigs, done, nil, nil))
	}()

	get := func(c *http.Client, path string, timeout time.Duration) (*http.Response, []byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+path, nil)
		if err != nil {
			return nil, nil, err
		}
		rsp, err := c.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer rsp.Body.Close()
		body, err := io.ReadAll(rsp.Body)
		return rsp, body, err
	}

	newClient := func(t *testing.T) *http.Client {
		tr := &http3.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}
		t.Cleanup(func() { tr.Close() })
		return &http.Client{Transport: tr}
	}

	probe := newClient(t)
	var probeErr error
	for range 30 {
		_, _, probeErr = get(probe, "/", 500*time.Millisecond)
		if probeErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, probeErr, "HTTP/3 listener did not become ready")
	probe.Transport.(*http3.Transport).Close()

	sigs <- syscall.SIGTERM

	time.Sleep(shutdownDelay / 2)
	t.Log("Make request in parallel before shutdown started")

	type result struct {
		rsp  *http.Response
		body []byte
		err  error
	}
	client := newClient(t)
	resCh := make(chan result, 1)
	go func() {
		rsp, body, err := get(client, "/slow", 10*time.Second)
		resCh <- result{rsp, body, err}
	}()

	time.Sleep(shutdownDelay)
	t.Log("We are 1.5x past the shutdown delay, so HTTP/3 shutdown should have been started")

	select {
	case <-resCh:
		t.Fatal("In-flight request must not be aborted or finished early by shutdown")
	default:
	}

	_, _, err = get(newClient(t), "/", 500*time.Millisecond)
	assert.Error(t, err, "New HTTP/3 connection should fail after shutdown started")

	res := <-resCh
	require.NoError(t, res.err, "In-flight request must succeed")
	assert.Equal(t, http.StatusOK, res.rsp.StatusCode)
	assert.Equal(t, "OK", string(res.body))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Shutdown takes too long after request is finished")
	}
}
