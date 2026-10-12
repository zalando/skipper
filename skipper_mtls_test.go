package skipper

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/net/nettest"
)

// startMTLSBackend starts a TLS listener that requires client certs signed by
// clientCAs. Returns the backend's https:// URL.
func startMTLSBackend(t *testing.T, serverCert tls.Certificate, clientCAs *x509.CertPool) string {
	t.Helper()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
		Certificates: []tls.Certificate{serverCert},
	})
	require.NoError(t, err)

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	go srv.Serve(ln) //nolint:errcheck
	t.Cleanup(func() { srv.Close() })

	return "https://" + ln.Addr().String()
}

// TestMTLSConfig_Validation tests the run() validation of mTLS options without
// starting a full listener.
func TestMTLSConfig_Validation(t *testing.T) {
	now := time.Now()

	// A self-signed CA cert/key pair used for valid configurations.
	validCertPEM, validKeyPEM, _, _ := nettest.MustGenCert(t, nettest.CertOptions{
		CN:        "Valid CA",
		Hosts:     []string{"localhost", "127.0.0.1"},
		NotBefore: now.Add(-time.Hour),
		NotAfter:  now.Add(24 * time.Hour),
	})
	validCertFile, validKeyFile := nettest.WriteCertKey(t, validCertPEM, validKeyPEM)

	// A second CA — its key does not match validCertPEM, so they form a mismatched pair.
	_, mismatchedKeyPEM, _, _ := nettest.MustGenCert(t, nettest.CertOptions{
		CN:        "Other CA",
		Hosts:     []string{"localhost", "127.0.0.1"},
		NotBefore: now.Add(-time.Hour),
		NotAfter:  now.Add(24 * time.Hour),
	})
	_, mismatchedKeyFile := nettest.WriteCertKey(t, validCertPEM, mismatchedKeyPEM)

	for _, tt := range []struct {
		name        string
		opts        Options
		errContains string // empty means "no mTLS error" (run may still fail at listener)
	}{
		{
			name: "valid cert and key passes validation",
			opts: Options{
				// Use an address that will fail to bind so run() exits quickly
				// after passing the mTLS validation step.
				Address:        "256.256.256.256:1",
				EnableMTLS:     true,
				ClientTLS:      &tls.Config{},
				ClientCertFile: validCertFile,
				ClientKeyFile:  validKeyFile,
			},
			// run() returns an error but NOT an mTLS one — the listener fails.
			errContains: "",
		},
		{
			name: "static ClientTLS.Certificates conflicts with EnableMTLS",
			opts: Options{
				Address:    "127.0.0.1:8080",
				EnableMTLS: true,
				ClientTLS: &tls.Config{
					Certificates: []tls.Certificate{{}},
				},
				ClientCertFile: validCertFile,
				ClientKeyFile:  validKeyFile,
			},
			errContains: "failed to enable MTLS, you have passed static certificates",
		},
		{
			name: "nonexistent cert file is rejected",
			opts: Options{
				Address:        "127.0.0.1:8080",
				EnableMTLS:     true,
				ClientTLS:      &tls.Config{},
				ClientCertFile: "/nonexistent/cert.crt",
				ClientKeyFile:  "/nonexistent/key.key",
			},
			errContains: "failed to enable MTLS, invalid key/cert pair",
		},
		{
			name: "mismatched cert and key is rejected",
			opts: Options{
				Address:        "127.0.0.1:8080",
				EnableMTLS:     true,
				ClientTLS:      &tls.Config{},
				ClientCertFile: validCertFile,
				ClientKeyFile:  mismatchedKeyFile,
			},
			errContains: "failed to enable MTLS, invalid key/cert pair",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.opts, nil, nil)
			require.Error(t, err, "run() must always return an error (listener fails or mTLS validation fails)")
			if tt.errContains != "" {
				assert.ErrorContains(t, err, tt.errContains)
			} else {
				assert.NotContains(t, err.Error(), "failed to enable MTLS")
			}
		})
	}
}

// TestMTLS_ProxyToBackend starts a full skipper instance and verifies end-to-end
// mTLS proxying: the positive case succeeds and the negative case (no client cert)
// returns a gateway error.
func TestMTLS_ProxyToBackend(t *testing.T) {
	now := time.Now()

	// CA that signs both the server cert and the proxy's client cert.
	caPEM, caKeyPEM, caCert, caKey := nettest.MustGenCert(t, nettest.CertOptions{
		CN:        "Test CA",
		Hosts:     []string{"localhost", "127.0.0.1"},
		NotBefore: now.Add(-time.Hour),
		NotAfter:  now.Add(24 * time.Hour),
	})
	_ = caKeyPEM

	// Client cert that skipper presents to the backend.
	clientCertPEM, clientKeyPEM, _, _ := nettest.MustGenCert(t, nettest.CertOptions{
		CN:           "proxy-client",
		Hosts:        []string{"localhost", "127.0.0.1"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		ExtKeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		SignerCert:   caCert,
		SignerKey:    caKey,
	})
	certFile, keyFile := nettest.WriteCertKey(t, clientCertPEM, clientKeyPEM)

	// Backend TLS: server cert = CA cert, client cert must be signed by CA.
	clientCAs := x509.NewCertPool()
	clientCAs.AppendCertsFromPEM(caPEM)

	caKeyDER, err := x509.MarshalECPrivateKey(caKey)
	require.NoError(t, err)
	serverTLSCert, err := tls.X509KeyPair(
		caPEM,
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: caKeyDER}),
	)
	require.NoError(t, err)

	backendURL := startMTLSBackend(t, serverTLSCert, clientCAs)

	// Proxy's outbound TLS config trusts the backend CA.
	proxyRootCAs := x509.NewCertPool()
	proxyRootCAs.AppendCertsFromPEM(caPEM)

	t.Run("positive: valid client cert reaches mTLS backend", func(t *testing.T) {
		MuFindAddress.Lock()
		addr := FindAddress(t)
		MuFindAddress.Unlock()

		o := Options{
			Address:                   addr,
			InlineRoutes:              fmt.Sprintf(`* -> "%s"`, backendURL),
			EnableMTLS:                true,
			ClientTLS:                 &tls.Config{RootCAs: proxyRootCAs},
			ClientCertFile:            certFile,
			ClientKeyFile:             keyFile,
			ClientCertRefreshInterval: 50 * time.Millisecond,
			WaitFirstRouteLoad:        true,
		}

		sigs := make(chan os.Signal, 1)
		go run(o, sigs, nil) //nolint:errcheck
		defer func() { sigs <- syscall.SIGTERM }()

		rsp, err := waitConnGet("http://" + addr + "/")
		require.NoError(t, err)
		defer rsp.Body.Close()

		assert.Equal(t, http.StatusOK, rsp.StatusCode)
	})

	t.Run("negative: no client cert causes backend to reject the connection", func(t *testing.T) {
		MuFindAddress.Lock()
		addr := FindAddress(t)
		MuFindAddress.Unlock()

		o := Options{
			Address:            addr,
			InlineRoutes:       fmt.Sprintf(`* -> "%s"`, backendURL),
			EnableMTLS:         false, // proxy presents no client cert
			ClientTLS:          &tls.Config{RootCAs: proxyRootCAs},
			WaitFirstRouteLoad: true,
		}

		sigs := make(chan os.Signal, 1)
		go run(o, sigs, nil) //nolint:errcheck
		defer func() { sigs <- syscall.SIGTERM }()

		rsp, err := waitConnGet("http://" + addr + "/")
		require.NoError(t, err)
		defer rsp.Body.Close()

		assert.True(t, rsp.StatusCode == http.StatusServiceUnavailable,
			"expected 503, got %d", rsp.StatusCode,
		)
	})
}
