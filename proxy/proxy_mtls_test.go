package proxy

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
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	filterstls "github.com/zalando/skipper/filters/tls"
)

// mtlsCertOptions configures certificate generation for mTLS tests.
type mtlsCertOptions struct {
	cn           string
	dns          string
	ip           string
	notBefore    time.Time
	notAfter     time.Time
	extKeyUsages []x509.ExtKeyUsage
	signerCert   *x509.Certificate
	signerKey    *ecdsa.PrivateKey
}

func generateMTLSCert(t *testing.T, opts mtlsCertOptions) (certPEM, keyPEM []byte, parsedCert *x509.Certificate, privateKey *ecdsa.PrivateKey) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: opts.cn},
		NotBefore:             opts.notBefore,
		NotAfter:              opts.notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           opts.extKeyUsages,
		BasicConstraintsValid: true,
	}

	if opts.dns != "" {
		template.DNSNames = []string{opts.dns}
	}
	if opts.ip != "" {
		if ip := net.ParseIP(opts.ip); ip != nil {
			template.IPAddresses = []net.IP{ip}
		}
	}

	signingTemplate := template
	signingKey := privateKey

	if opts.signerCert != nil && opts.signerKey != nil {
		signingTemplate = opts.signerCert
		signingKey = opts.signerKey
	} else {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, signingTemplate, &privateKey.PublicKey, signingKey)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	parsedCert, err = x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("failed to parse generated certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}

func writeMTLSTempCertFiles(t *testing.T, cert, key []byte) (string, string) {
	t.Helper()
	cFile, err := os.CreateTemp("", "proxy-mtls-cert-*.crt")
	if err != nil {
		t.Fatalf("failed to create temp cert file: %v", err)
	}
	kFile, err := os.CreateTemp("", "proxy-mtls-key-*.key")
	if err != nil {
		t.Fatalf("failed to create temp key file: %v", err)
	}
	if err := os.WriteFile(cFile.Name(), cert, 0644); err != nil {
		t.Fatalf("failed to write temp cert: %v", err)
	}
	if err := os.WriteFile(kFile.Name(), key, 0600); err != nil {
		t.Fatalf("failed to write temp key: %v", err)
	}
	t.Cleanup(func() {
		os.Remove(cFile.Name())
		os.Remove(kFile.Name())
	})
	return cFile.Name(), kFile.Name()
}

// newMTLSBackend starts a TLS server that requires client certs signed by clientCAs.
func newMTLSBackend(t *testing.T, serverCert tls.Certificate, clientCAs *x509.CertPool) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
		Certificates: []tls.Certificate{serverCert},
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// makeMTLSRegistry returns a filter registry that includes the mtlsCN filter.
func makeMTLSRegistry() filters.Registry {
	r := builtin.MakeRegistry()
	r.Register(filterstls.NewMtls())
	return r
}

// makeMTLSCA generates a CA cert/key for use in mTLS tests.
func makeMTLSCA(t *testing.T, cn string) (caPEM, caKeyPEM []byte, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) {
	t.Helper()
	now := time.Now()
	return generateMTLSCert(t, mtlsCertOptions{
		cn:        cn,
		notBefore: now.Add(-1 * time.Hour),
		notAfter:  now.Add(24 * time.Hour),
		dns:       "localhost",
		ip:        "127.0.0.1",
	})
}

// makeMTLSClientCert generates a client cert signed by the given CA.
func makeMTLSClientCert(t *testing.T, cn string, signerCert *x509.Certificate, signerKey *ecdsa.PrivateKey) (certPEM, keyPEM []byte) {
	t.Helper()
	now := time.Now()
	certPEM, keyPEM, _, _ = generateMTLSCert(t, mtlsCertOptions{
		cn:        cn,
		dns:       "localhost",
		ip:        "127.0.0.1",
		notBefore: now.Add(-1 * time.Hour),
		notAfter:  now.Add(24 * time.Hour),
		extKeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		signerCert: signerCert,
		signerKey:  signerKey,
	})
	return
}

// marshalECKey converts an ecdsa.PrivateKey to a PEM-encoded EC PRIVATE KEY block.
func marshalECKeyPEM(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal EC private key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

// TestProxyMTLS_OutboundSuccess verifies that the proxy successfully connects to
// an mTLS backend when configured with a valid client cert.
func TestProxyMTLS_OutboundSuccess(t *testing.T) {
	caPEM, _, caCert, caKey := makeMTLSCA(t, "Test CA")
	clientCertPEM, clientKeyPEM := makeMTLSClientCert(t, "test-client", caCert, caKey)
	certFile, keyFile := writeMTLSTempCertFiles(t, clientCertPEM, clientKeyPEM)

	clientCAs := x509.NewCertPool()
	clientCAs.AppendCertsFromPEM(caPEM)

	serverCert, err := tls.X509KeyPair(caPEM, marshalECKeyPEM(t, caKey))
	if err != nil {
		t.Fatalf("failed to create server cert: %v", err)
	}

	backend := newMTLSBackend(t, serverCert, clientCAs)

	clientRootCAs := x509.NewCertPool()
	clientRootCAs.AppendCertsFromPEM(caPEM)

	params := Params{
		EnableMTLS:                true,
		ClientCertFile:            certFile,
		ClientKeyFile:             keyFile,
		ClientCertRefreshInterval: 50 * time.Millisecond,
		ClientTLS:                 &tls.Config{RootCAs: clientRootCAs},
	}

	tp, err := newTestProxyWithParams(fmt.Sprintf(`* -> "%s"`, backend.URL), params)
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
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
}

// TestProxyMTLS_OutboundRejectedUntrustedClientCert verifies that a backend
// rejects the proxy when its client cert is signed by an untrusted (rogue) CA.
func TestProxyMTLS_OutboundRejectedUntrustedClientCert(t *testing.T) {
	// Trusted CA — only this is accepted by the backend's ClientCAs pool
	trustedCAPEM, _, _, _ := makeMTLSCA(t, "Trusted CA")

	// Rogue CA — signs the proxy's client cert but is not trusted by the backend
	_, _, rogueCACert, rogueCAKey := makeMTLSCA(t, "Rogue CA")
	rogueCertPEM, rogueKeyPEM := makeMTLSClientCert(t, "rogue-client", rogueCACert, rogueCAKey)
	certFile, keyFile := writeMTLSTempCertFiles(t, rogueCertPEM, rogueKeyPEM)

	// Backend server cert and its CA (separate from client-auth CA)
	backendCAPEM, _, _, backendCAKey := makeMTLSCA(t, "Backend Server CA")
	backendServerCert, err := tls.X509KeyPair(backendCAPEM, marshalECKeyPEM(t, backendCAKey))
	if err != nil {
		t.Fatalf("failed to create backend server cert: %v", err)
	}

	// Backend only trusts the Trusted CA for client auth
	backendClientCAs := x509.NewCertPool()
	backendClientCAs.AppendCertsFromPEM(trustedCAPEM)

	backend := newMTLSBackend(t, backendServerCert, backendClientCAs)

	// Proxy trusts the backend's server CA
	proxyRootCAs := x509.NewCertPool()
	proxyRootCAs.AppendCertsFromPEM(backendCAPEM)

	params := Params{
		EnableMTLS:                true,
		ClientCertFile:            certFile,
		ClientKeyFile:             keyFile,
		ClientCertRefreshInterval: 50 * time.Millisecond,
		ClientTLS:                 &tls.Config{RootCAs: proxyRootCAs},
	}

	tp, err := newTestProxyWithParams(fmt.Sprintf(`* -> "%s"`, backend.URL), params)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("proxy server request failed: %v", err)
	}
	defer rsp.Body.Close()

	// Backend rejects the rogue cert at the TLS layer; proxy propagates as 502/503.
	if rsp.StatusCode != http.StatusBadGateway && rsp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 502 or 503, got %d", rsp.StatusCode)
	}
}

// TestProxyMTLS_OutboundNoClientCert verifies that the backend rejects the proxy
// when mTLS is disabled and no client cert is presented.
func TestProxyMTLS_OutboundNoClientCert(t *testing.T) {
	caPEM, _, _, caKey := makeMTLSCA(t, "Backend CA")
	serverCert, err := tls.X509KeyPair(caPEM, marshalECKeyPEM(t, caKey))
	if err != nil {
		t.Fatalf("failed to create server cert: %v", err)
	}

	clientCAs := x509.NewCertPool()
	clientCAs.AppendCertsFromPEM(caPEM)

	backend := newMTLSBackend(t, serverCert, clientCAs)

	proxyRootCAs := x509.NewCertPool()
	proxyRootCAs.AppendCertsFromPEM(caPEM)

	// EnableMTLS=false: proxy does not present a client cert
	params := Params{
		EnableMTLS: false,
		ClientTLS:  &tls.Config{RootCAs: proxyRootCAs},
	}

	tp, err := newTestProxyWithParams(fmt.Sprintf(`* -> "%s"`, backend.URL), params)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("proxy server request failed: %v", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusBadGateway && rsp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 502 or 503 when no client cert presented, got %d", rsp.StatusCode)
	}
}

// TestProxyMTLS_OutboundExpiredClientCert verifies that a backend rejects a
// proxy presenting an expired client certificate.
func TestProxyMTLS_OutboundExpiredClientCert(t *testing.T) {
	caPEM, _, caCert, caKey := makeMTLSCA(t, "Test CA")

	now := time.Now()
	expiredCertPEM, expiredKeyPEM, _, _ := generateMTLSCert(t, mtlsCertOptions{
		cn:        "expired-client",
		dns:       "localhost",
		ip:        "127.0.0.1",
		notBefore: now.Add(-5 * time.Hour),
		notAfter:  now.Add(-1 * time.Hour), // already expired
		extKeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		signerCert: caCert,
		signerKey:  caKey,
	})

	certFile, keyFile := writeMTLSTempCertFiles(t, expiredCertPEM, expiredKeyPEM)

	clientCAs := x509.NewCertPool()
	clientCAs.AppendCertsFromPEM(caPEM)

	serverCert, err := tls.X509KeyPair(caPEM, marshalECKeyPEM(t, caKey))
	if err != nil {
		t.Fatalf("failed to create server cert: %v", err)
	}

	backend := newMTLSBackend(t, serverCert, clientCAs)

	clientRootCAs := x509.NewCertPool()
	clientRootCAs.AppendCertsFromPEM(caPEM)

	params := Params{
		EnableMTLS:                true,
		ClientCertFile:            certFile,
		ClientKeyFile:             keyFile,
		ClientCertRefreshInterval: 50 * time.Millisecond,
		ClientTLS:                 &tls.Config{RootCAs: clientRootCAs},
	}

	tp, err := newTestProxyWithParams(fmt.Sprintf(`* -> "%s"`, backend.URL), params)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("proxy server request failed: %v", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusBadGateway && rsp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 502 or 503 for expired cert, got %d", rsp.StatusCode)
	}
}

// TestProxyMTLS_CertRotation verifies that the proxy picks up a rotated client
// certificate from disk and continues to connect successfully to the backend.
func TestProxyMTLS_CertRotation(t *testing.T) {
	caPEM, _, caCert, caKey := makeMTLSCA(t, "Test CA")

	cert1PEM, key1PEM := makeMTLSClientCert(t, "client-v1", caCert, caKey)
	cert2PEM, key2PEM := makeMTLSClientCert(t, "client-v2", caCert, caKey)

	certFile, keyFile := writeMTLSTempCertFiles(t, cert1PEM, key1PEM)

	clientCAs := x509.NewCertPool()
	clientCAs.AppendCertsFromPEM(caPEM)

	serverCert, err := tls.X509KeyPair(caPEM, marshalECKeyPEM(t, caKey))
	if err != nil {
		t.Fatalf("failed to create server cert: %v", err)
	}

	backend := newMTLSBackend(t, serverCert, clientCAs)

	clientRootCAs := x509.NewCertPool()
	clientRootCAs.AppendCertsFromPEM(caPEM)

	params := Params{
		EnableMTLS:                true,
		ClientCertFile:            certFile,
		ClientKeyFile:             keyFile,
		ClientCertRefreshInterval: 20 * time.Millisecond,
		ClientTLS:                 &tls.Config{RootCAs: clientRootCAs},
	}

	tp, err := newTestProxyWithParams(fmt.Sprintf(`* -> "%s"`, backend.URL), params)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	// Baseline: first cert works
	rsp, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("initial request failed: %v", err)
	}
	rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		t.Fatalf("initial request: expected 200, got %d", rsp.StatusCode)
	}

	// Rotate cert files on disk
	if err := os.WriteFile(certFile, cert2PEM, 0644); err != nil {
		t.Fatalf("failed to write rotated cert: %v", err)
	}
	if err := os.WriteFile(keyFile, key2PEM, 0600); err != nil {
		t.Fatalf("failed to write rotated key: %v", err)
	}

	// Wait for the reloader to pick up the new cert (2+ intervals)
	time.Sleep(60 * time.Millisecond)

	rsp2, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("post-rotation request failed: %v", err)
	}
	rsp2.Body.Close()
	if rsp2.StatusCode != http.StatusOK {
		t.Errorf("post-rotation: expected 200, got %d", rsp2.StatusCode)
	}
}

// TestProxyMTLS_OutboundFutureClientCert verifies that a backend rejects a proxy
// presenting a client certificate whose validity period has not started yet.
func TestProxyMTLS_OutboundFutureClientCert(t *testing.T) {
	caPEM, _, caCert, caKey := makeMTLSCA(t, "Test CA")

	now := time.Now()
	futureCertPEM, futureKeyPEM, _, _ := generateMTLSCert(t, mtlsCertOptions{
		cn:        "future-client",
		dns:       "localhost",
		ip:        "127.0.0.1",
		notBefore: now.Add(1 * time.Hour), // not valid yet
		notAfter:  now.Add(5 * time.Hour),
		extKeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		signerCert: caCert,
		signerKey:  caKey,
	})

	certFile, keyFile := writeMTLSTempCertFiles(t, futureCertPEM, futureKeyPEM)

	clientCAs := x509.NewCertPool()
	clientCAs.AppendCertsFromPEM(caPEM)

	serverCert, err := tls.X509KeyPair(caPEM, marshalECKeyPEM(t, caKey))
	if err != nil {
		t.Fatalf("failed to create server cert: %v", err)
	}

	backend := newMTLSBackend(t, serverCert, clientCAs)

	clientRootCAs := x509.NewCertPool()
	clientRootCAs.AppendCertsFromPEM(caPEM)

	params := Params{
		EnableMTLS:                true,
		ClientCertFile:            certFile,
		ClientKeyFile:             keyFile,
		ClientCertRefreshInterval: 50 * time.Millisecond,
		ClientTLS:                 &tls.Config{RootCAs: clientRootCAs},
	}

	tp, err := newTestProxyWithParams(fmt.Sprintf(`* -> "%s"`, backend.URL), params)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("proxy server request failed: %v", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusBadGateway && rsp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 502 or 503 for not-yet-valid cert, got %d", rsp.StatusCode)
	}
}

// TestProxyMTLS_OutboundInsecureSkipVerify verifies that when InsecureSkipVerify
// is set, the proxy connects to a backend with a self-signed cert without needing
// to add the backend CA to RootCAs, while still presenting its own client cert.
func TestProxyMTLS_OutboundInsecureSkipVerify(t *testing.T) {
	caPEM, _, caCert, caKey := makeMTLSCA(t, "Test CA")
	clientCertPEM, clientKeyPEM := makeMTLSClientCert(t, "test-client", caCert, caKey)
	certFile, keyFile := writeMTLSTempCertFiles(t, clientCertPEM, clientKeyPEM)

	clientCAs := x509.NewCertPool()
	clientCAs.AppendCertsFromPEM(caPEM)

	serverCert, err := tls.X509KeyPair(caPEM, marshalECKeyPEM(t, caKey))
	if err != nil {
		t.Fatalf("failed to create server cert: %v", err)
	}

	backend := newMTLSBackend(t, serverCert, clientCAs)

	params := Params{
		EnableMTLS:                true,
		ClientCertFile:            certFile,
		ClientKeyFile:             keyFile,
		ClientCertRefreshInterval: 50 * time.Millisecond,
		// No RootCAs set — InsecureSkipVerify bypasses server cert verification.
		ClientTLS: &tls.Config{InsecureSkipVerify: true}, // #nosec G402
	}

	tp, err := newTestProxyWithParams(fmt.Sprintf(`* -> "%s"`, backend.URL), params)
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
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 with InsecureSkipVerify, got %d", rsp.StatusCode)
	}
}

// TestProxyMTLS_InboundNoTLS verifies the mtlsCN filter returns 401 when the
// downstream client connects over plain HTTP (req.TLS == nil).
func TestProxyMTLS_InboundNoTLS(t *testing.T) {
	fr := makeMTLSRegistry()
	doc := `* -> mtlsCN("any-cn") -> <shunt>`

	tp, err := newTestProxyWithFilters(fr, doc, FlagsNone)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	ps := httptest.NewServer(tp.proxy)
	defer ps.Close()

	rsp, err := ps.Client().Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for non-TLS connection, got %d", rsp.StatusCode)
	}
}

// TestProxyMTLS_InboundFilterCNCheck tests the mtlsCN filter allow-list on a
// TLS-terminating proxy that accepts client certificates.
func TestProxyMTLS_InboundFilterCNCheck(t *testing.T) {
	caPEM, _, caCert, caKey := makeMTLSCA(t, "trusted-issuer-ca")
	clientCertPEM, clientKeyPEM := makeMTLSClientCert(t, "test-client", caCert, caKey)

	for _, tt := range []struct {
		name           string
		filterCN       string
		expectedStatus int
	}{
		{
			name:           "matching issuer CN is allowed",
			filterCN:       "trusted-issuer-ca",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "wrong issuer CN is forbidden",
			filterCN:       "other-ca",
			expectedStatus: http.StatusForbidden,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxyCAs := x509.NewCertPool()
			proxyCAs.AppendCertsFromPEM(caPEM)

			proxyServerCertPEM, proxyServerKeyPEM, _, _ := generateMTLSCert(t, mtlsCertOptions{
				cn:           "proxy-server",
				dns:          "localhost",
				ip:           "127.0.0.1",
				notBefore:    time.Now().Add(-time.Hour),
				notAfter:     time.Now().Add(time.Hour),
				extKeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			})
			proxyCert, err := tls.X509KeyPair(proxyServerCertPEM, proxyServerKeyPEM)
			if err != nil {
				t.Fatalf("failed to create proxy server cert: %v", err)
			}

			fr := makeMTLSRegistry()
			doc := fmt.Sprintf(`* -> mtlsCN(%q) -> status(200) -> <shunt>`, tt.filterCN)

			tp, err := newTestProxyWithFilters(fr, doc, FlagsNone)
			if err != nil {
				t.Fatalf("failed to create test proxy: %v", err)
			}
			defer tp.close()

			// Wrap proxy in a TLS server that requests (but does not hard-require) client certs.
			// The mtlsCN filter enforces the authz check at the application layer.
			ps := httptest.NewUnstartedServer(tp.proxy)
			ps.TLS = &tls.Config{
				ClientAuth:   tls.RequireAnyClientCert,
				ClientCAs:    proxyCAs,
				Certificates: []tls.Certificate{proxyCert},
			}
			ps.StartTLS()
			defer ps.Close()

			clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
			if err != nil {
				t.Fatalf("failed to load client cert: %v", err)
			}

			proxyRootCAs := x509.NewCertPool()
			proxyRootCAs.AppendCertsFromPEM(proxyServerCertPEM)

			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						Certificates: []tls.Certificate{clientCert},
						RootCAs:      proxyRootCAs,
					},
				},
			}

			rsp, err := client.Get(ps.URL + "/")
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer rsp.Body.Close()

			if rsp.StatusCode != tt.expectedStatus {
				t.Errorf("expected %d, got %d", tt.expectedStatus, rsp.StatusCode)
			}
		})
	}
}

// TestProxyMTLS_InboundNoCertPresented verifies the mtlsCN filter returns 403
// when TLS is used but the client presents no certificate, so PeerCertificates
// is empty and no CN can match the allow-list.
func TestProxyMTLS_InboundNoCertPresented(t *testing.T) {
	proxyServerCertPEM, proxyServerKeyPEM, _, _ := generateMTLSCert(t, mtlsCertOptions{
		cn:           "proxy-server",
		dns:          "localhost",
		ip:           "127.0.0.1",
		notBefore:    time.Now().Add(-time.Hour),
		notAfter:     time.Now().Add(time.Hour),
		extKeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	proxyCert, err := tls.X509KeyPair(proxyServerCertPEM, proxyServerKeyPEM)
	if err != nil {
		t.Fatalf("failed to create proxy server cert: %v", err)
	}

	fr := makeMTLSRegistry()
	doc := `* -> mtlsCN("trusted-issuer") -> <shunt>`

	tp, err := newTestProxyWithFilters(fr, doc, FlagsNone)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	// Use tls.RequestClientCert so TLS handshake succeeds without a client cert;
	// the filter must then reject with 403 because PeerCertificates is empty.
	ps := httptest.NewUnstartedServer(tp.proxy)
	ps.TLS = &tls.Config{
		ClientAuth:   tls.RequestClientCert,
		Certificates: []tls.Certificate{proxyCert},
	}
	ps.StartTLS()
	defer ps.Close()

	proxyRootCAs := x509.NewCertPool()
	proxyRootCAs.AppendCertsFromPEM(proxyServerCertPEM)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: proxyRootCAs},
		},
	}

	rsp, err := client.Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer rsp.Body.Close()

	// PeerCertificates is empty → no CN match → 403 Forbidden
	if rsp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden when no client cert presented, got %d", rsp.StatusCode)
	}
}

// TestProxyMTLS_OutboundAndInboundCombined tests the full double-mTLS path:
// the proxy requires a client cert from downstream (inbound, enforced by filter)
// AND presents a client cert to the upstream backend (outbound).
func TestProxyMTLS_OutboundAndInboundCombined(t *testing.T) {
	// Inbound CA: signs the downstream client cert
	inboundCAPEM, _, inboundCACert, inboundCAKey := makeMTLSCA(t, "inbound-ca")
	downstreamCertPEM, downstreamKeyPEM := makeMTLSClientCert(t, "downstream-client", inboundCACert, inboundCAKey)

	// Outbound CA: signs the proxy-as-client cert
	outboundCAPEM, _, outboundCACert, outboundCAKey := makeMTLSCA(t, "outbound-ca")
	proxyCertPEM, proxyKeyPEM := makeMTLSClientCert(t, "proxy-client", outboundCACert, outboundCAKey)

	certFile, keyFile := writeMTLSTempCertFiles(t, proxyCertPEM, proxyKeyPEM)

	// Backend: requires client cert signed by outbound CA
	outboundClientCAs := x509.NewCertPool()
	outboundClientCAs.AppendCertsFromPEM(outboundCAPEM)
	backendServerCert, err := tls.X509KeyPair(outboundCAPEM, marshalECKeyPEM(t, outboundCAKey))
	if err != nil {
		t.Fatalf("failed to create backend server cert: %v", err)
	}
	backend := newMTLSBackend(t, backendServerCert, outboundClientCAs)

	proxyClientRootCAs := x509.NewCertPool()
	proxyClientRootCAs.AppendCertsFromPEM(outboundCAPEM)

	fr := makeMTLSRegistry()
	doc := fmt.Sprintf(`* -> mtlsCN("inbound-ca") -> "%s"`, backend.URL)

	params := Params{
		EnableMTLS:                true,
		ClientCertFile:            certFile,
		ClientKeyFile:             keyFile,
		ClientCertRefreshInterval: 50 * time.Millisecond,
		ClientTLS:                 &tls.Config{RootCAs: proxyClientRootCAs},
	}

	tp, err := newTestProxyWithFiltersAndParams(fr, doc, params, nil)
	if err != nil {
		t.Fatalf("failed to create test proxy: %v", err)
	}
	defer tp.close()

	// Proxy server-side TLS: accepts inbound client certs from inbound CA
	inboundClientCAs := x509.NewCertPool()
	inboundClientCAs.AppendCertsFromPEM(inboundCAPEM)

	proxyServerCertPEM, proxyServerKeyPEM, _, _ := generateMTLSCert(t, mtlsCertOptions{
		cn:           "proxy-server",
		dns:          "localhost",
		ip:           "127.0.0.1",
		notBefore:    time.Now().Add(-time.Hour),
		notAfter:     time.Now().Add(time.Hour),
		extKeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	proxyServerCert, err := tls.X509KeyPair(proxyServerCertPEM, proxyServerKeyPEM)
	if err != nil {
		t.Fatalf("failed to create proxy server cert: %v", err)
	}

	ps := httptest.NewUnstartedServer(tp.proxy)
	ps.TLS = &tls.Config{
		ClientAuth:   tls.RequireAnyClientCert,
		ClientCAs:    inboundClientCAs,
		Certificates: []tls.Certificate{proxyServerCert},
	}
	ps.StartTLS()
	defer ps.Close()

	downstreamCert, err := tls.X509KeyPair(downstreamCertPEM, downstreamKeyPEM)
	if err != nil {
		t.Fatalf("failed to load downstream client cert: %v", err)
	}

	downstreamRootCAs := x509.NewCertPool()
	downstreamRootCAs.AppendCertsFromPEM(proxyServerCertPEM)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{downstreamCert},
				RootCAs:      downstreamRootCAs,
			},
		},
	}

	rsp, err := client.Get(ps.URL + "/")
	if err != nil {
		t.Fatalf("combined mTLS request failed: %v", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rsp.StatusCode)
	}
}
