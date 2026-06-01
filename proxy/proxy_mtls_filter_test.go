package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	filterstls "github.com/zalando/skipper/filters/tls"
)

// makeMTLSRegistry returns a filter registry that includes the mtlsCN filter.
func makeMTLSRegistry() filters.Registry {
	r := builtin.MakeRegistry()
	r.Register(filterstls.NewMtls())
	return r
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
