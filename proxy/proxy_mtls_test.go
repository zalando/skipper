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

	// Backend rejects the rogue cert at the TLS layer; proxy propagates as 503.
	if rsp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rsp.StatusCode)
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

	if rsp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when no client cert presented, got %d", rsp.StatusCode)
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

	if rsp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for expired cert, got %d", rsp.StatusCode)
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

	if rsp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for not-yet-valid cert, got %d", rsp.StatusCode)
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
