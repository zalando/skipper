package skipper

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mtlsGenCert generates an ECDSA P-256 certificate. When signerCert/signerKey
// are nil the cert is self-signed and marked as a CA.
func mtlsGenCert(t *testing.T, cn, dns, ip string, notBefore, notAfter time.Time, extKeyUsages []x509.ExtKeyUsage, signerCert *x509.Certificate, signerKey *ecdsa.PrivateKey) (certPEM, keyPEM []byte, cert *x509.Certificate, key *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           extKeyUsages,
		BasicConstraintsValid: true,
	}
	if dns != "" {
		tmpl.DNSNames = []string{dns}
	}
	if ip != "" {
		if parsed := net.ParseIP(ip); parsed != nil {
			tmpl.IPAddresses = []net.IP{parsed}
		}
	}

	parent, parentKey := tmpl, key
	if signerCert != nil && signerKey != nil {
		parent, parentKey = signerCert, signerKey
	} else {
		tmpl.IsCA = true
		tmpl.KeyUsage |= x509.KeyUsageCertSign
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	require.NoError(t, err)
	cert, err = x509.ParseCertificate(der)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}

func mtlsWriteTempFiles(t *testing.T, certPEM, keyPEM []byte) (certFile, keyFile string) {
	t.Helper()
	cf, err := os.CreateTemp("", "skipper-mtls-cert-*.crt")
	require.NoError(t, err)
	kf, err := os.CreateTemp("", "skipper-mtls-key-*.key")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cf.Name(), certPEM, 0644))
	require.NoError(t, os.WriteFile(kf.Name(), keyPEM, 0600))
	t.Cleanup(func() { os.Remove(cf.Name()); os.Remove(kf.Name()) })
	return cf.Name(), kf.Name()
}

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
	validCertPEM, validKeyPEM, _, _ := mtlsGenCert(t, "Valid CA", "localhost", "127.0.0.1",
		now.Add(-time.Hour), now.Add(24*time.Hour), nil, nil, nil)
	validCertFile, validKeyFile := mtlsWriteTempFiles(t, validCertPEM, validKeyPEM)

	// A second CA — its key does not match validCertPEM, so they form a mismatched pair.
	_, mismatchedKeyPEM, _, _ := mtlsGenCert(t, "Other CA", "localhost", "127.0.0.1",
		now.Add(-time.Hour), now.Add(24*time.Hour), nil, nil, nil)
	_, mismatchedKeyFile := mtlsWriteTempFiles(t, validCertPEM, mismatchedKeyPEM)

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
	caPEM, caKeyPEM, caCert, caKey := mtlsGenCert(t, "Test CA", "localhost", "127.0.0.1",
		now.Add(-time.Hour), now.Add(24*time.Hour), nil, nil, nil)
	_ = caKeyPEM

	// Client cert that skipper presents to the backend.
	clientCertPEM, clientKeyPEM, _, _ := mtlsGenCert(t,
		"proxy-client", "localhost", "127.0.0.1",
		now.Add(-time.Hour), now.Add(24*time.Hour),
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		caCert, caKey,
	)
	certFile, keyFile := mtlsWriteTempFiles(t, clientCertPEM, clientKeyPEM)

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
