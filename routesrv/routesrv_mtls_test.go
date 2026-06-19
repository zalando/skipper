package routesrv_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	skpnet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/routesrv"
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

// makeMTLSClientOrServerCert generates a client cert signed by the given CA.
func makeMTLSClientOrServerCert(t *testing.T, cn string, signerCert *x509.Certificate, signerKey *ecdsa.PrivateKey) (certPEM, keyPEM []byte) {
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

func TestMtlsRoutesrv(t *testing.T) {
	defer tl.Reset()

	caPEM, _, caCert, caKey := makeMTLSCA(t, "Test CA")
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)
	// clientRootCAs := x509.NewCertPool()
	// clientRootCAs.AppendCertsFromPEM(caPEM)

	trustedClientCN := "trusted-cn"
	clientCertPEM, clientKeyPEM := makeMTLSClientOrServerCert(t, trustedClientCN, caCert, caKey)
	clientCertFile, clientKeyFile := writeMTLSTempCertFiles(t, clientCertPEM, clientKeyPEM)

	serverCertPEM, serverKeyPEM := makeMTLSClientOrServerCert(t, "server-cn", caCert, caKey)
	serverCertFile, serverKeyFile := writeMTLSTempCertFiles(t, serverCertPEM, serverKeyPEM)

	ks, _ := newKubeServer(t, loadKubeYAML(t, "testdata/ing-v1-lb-target-multi.yaml"))
	ks.Start()
	defer ks.Close()
	addr := ":9090"
	rs := newRouteServerWithOptions(t, skipper.Options{
		Address:           addr,
		EnableMTLS:        true,
		MtlsAuthnCA:       caPool,
		CertPathTLS:       serverCertFile,
		KeyPathTLS:        serverKeyFile,
		SourcePollTimeout: pollInterval,
		Kubernetes:        true,
		KubernetesURL:     ks.URL,
		RouteServerFilters: []*eskip.Filter{
			{
				Name: filters.MtlsAuthn,
			},
			{
				Name: filters.MtlsCN,
				Args: []any{trustedClientCN},
			},
		},
	})

	rs.StartUpdates()
	defer rs.StopUpdates()

	if err := tl.WaitFor(routesrv.LogRoutesInitialized, waitTimeout); err != nil {
		t.Fatalf("routes not initialized: %v", err)
	}

	go rs.ListenAndServe()

	client := skpnet.NewClient(skpnet.Options{
		CertFile:            clientCertFile,
		KeyFile:             clientKeyFile,
		CertRefreshInterval: 50 * time.Millisecond,
		RootCAs:             caPool,
	})
	defer client.Close()

	// make sure routesrv started
	for range 10 {
		_, err := client.Get("https://localhost" + addr + "/health")
		if err != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	rsp, err := client.Get("https://localhost" + addr + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if rsp.StatusCode != http.StatusNoContent {
		t.Fatalf("request failed with status code: %d", rsp.StatusCode)
	}

	rsp, err = client.Get("https://localhost" + addr + "/routes")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected %d, got %d", http.StatusOK, rsp.StatusCode)
	}

	buf, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}

	want := parseEskipFixture(t, "testdata/ing-v1-lb-target-multi.eskip")
	got, err := eskip.Parse(string(buf))
	if err != nil {
		t.Fatalf("served routes are not valid eskip: %v", err)
	}
	if !eskip.EqLists(got, want) {
		t.Errorf("served routes do not reflect kubernetes resources: %s", cmp.Diff(got, want))
	}
}
