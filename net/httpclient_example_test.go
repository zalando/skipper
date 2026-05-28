package net_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	stdlibnet "net"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/lightstep/lightstep-tracer-go"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/secrets"
	"github.com/zalando/skipper/tracing/tracingtest"
)

func ExampleTransport() {
	tracer := lightstep.NewTracer(lightstep.Options{})

	cli := net.NewTransport(net.Options{
		Tracer: tracer,
	})
	defer cli.Close()
	cli = net.WithSpanName(cli, "myspan")
	cli = net.WithBearerToken(cli, "mytoken")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	rsp, err := cli.RoundTrip(req)
	if err != nil {
		log.Fatalf("Failed to do request: %v", err)
	}
	log.Printf("rsp code: %v", rsp.StatusCode)
}

func ExampleClient() {
	tracer := lightstep.NewTracer(lightstep.Options{})

	cli := net.NewClient(net.Options{
		Tracer:                     tracer,
		OpentracingComponentTag:    "testclient",
		OpentracingSpanName:        "clientSpan",
		BearerTokenRefreshInterval: 10 * time.Second,
		BearerTokenFile:            "/tmp/foo.token",
		IdleConnTimeout:            2 * time.Second,
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for i := 0; i < 15; i++ {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

func ExampleClient_withTransport() {
	tracer := lightstep.NewTracer(lightstep.Options{})

	d := stdlibnet.Dialer{
		Timeout:   3 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
	f := d.DialContext

	cli := net.NewClient(net.Options{
		Transport: &http.Transport{
			IdleConnTimeout: 10 * time.Second,
			DialContext:     f,
		},
		Tracer:                     tracer,
		OpentracingComponentTag:    "testclient",
		OpentracingSpanName:        "clientSpan",
		BearerTokenRefreshInterval: 10 * time.Second,
		BearerTokenFile:            "/tmp/foo.token",
	})

	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for i := 0; i < 15; i++ {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

func ExampleClient_fileSecretsReader() {
	tracer := lightstep.NewTracer(lightstep.Options{})

	sp := secrets.NewSecretPaths(10 * time.Second)
	if err := sp.Add("/tmp/bar.token"); err != nil {
		log.Fatalf("failed to read secret: %v", err)
	}

	cli := net.NewClient(net.Options{
		Tracer:                  tracer,
		OpentracingComponentTag: "testclient",
		OpentracingSpanName:     "clientSpan",
		SecretsReader:           sp,
		IdleConnTimeout:         2 * time.Second,
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for i := 0; i < 15; i++ {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

func ExampleClient_staticSecret() {
	tracer := lightstep.NewTracer(lightstep.Options{})
	sec := []byte("mysecret")
	cli := net.NewClient(net.Options{
		Tracer:                  tracer,
		OpentracingComponentTag: "testclient",
		OpentracingSpanName:     "clientSpan",
		SecretsReader:           secrets.StaticSecret(sec),
		IdleConnTimeout:         2 * time.Second,
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for i := 0; i < 15; i++ {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

type customTracer struct {
	opentracing.Tracer
}

func (t *customTracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	span := t.Tracer.StartSpan(operationName, opts...)
	span.SetTag("customtag", "test")
	return span
}

func ExampleClient_customTracer() {
	mockTracer := tracingtest.NewTracer()
	cli := net.NewClient(net.Options{
		Tracer:              &customTracer{mockTracer},
		OpentracingSpanName: "clientSpan",
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	cli.Get("http://" + srv.Listener.Addr().String() + "/")
	fmt.Printf("customtag: %s", mockTracer.FinishedSpans()[0].Tags()["customtag"])

	// Output:
	// customtag: test
}

type testSecretsReader struct {
	h map[string][]byte
}

func newTestSecretsReader(m map[string][]byte) *testSecretsReader {
	return &testSecretsReader{
		h: m,
	}
}

func (*testSecretsReader) Close() {}
func (tsr *testSecretsReader) GetSecret(k string) ([]byte, bool) {
	b, ok := tsr.h[k]
	return b, ok
}

func ExampleClient_staticDelegateSecret() {
	tracer := lightstep.NewTracer(lightstep.Options{})
	sec := []byte("mysecret")

	cli := net.NewClient(net.Options{
		Tracer:                  tracer,
		OpentracingComponentTag: "testclient",
		OpentracingSpanName:     "clientSpan",
		SecretsReader: secrets.NewStaticDelegateSecret(
			newTestSecretsReader(
				map[string][]byte{
					"key": sec,
				},
			),
			"key",
		),
		IdleConnTimeout: 2 * time.Second,
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for i := 0; i < 15; i++ {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

func ExampleClient_hostSecret() {
	tracer := lightstep.NewTracer(lightstep.Options{})
	sec := []byte("mysecret")

	cli := net.NewClient(net.Options{
		Tracer:                  tracer,
		OpentracingComponentTag: "testclient",
		OpentracingSpanName:     "clientSpan",
		SecretsReader: secrets.NewHostSecret(
			newTestSecretsReader(
				map[string][]byte{
					"key": sec,
				},
			),
			map[string]string{
				"127.0.0.1": "key",
			},
		),
		IdleConnTimeout: 2 * time.Second,
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for i := 0; i < 15; i++ {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

func ExampleClient_withBeforeSendHook() {
	mockTracer := tracingtest.NewTracer()
	peerService := "my-peer-service"
	cli := net.NewClient(net.Options{
		Tracer:                  &customTracer{mockTracer},
		OpentracingComponentTag: "testclient",
		OpentracingSpanName:     "clientSpan",
		IdleConnTimeout:         2 * time.Second,
		BeforeSend: func(req *http.Request) {
			req.Header.Set("X-Foo", "qux")
			if span := opentracing.SpanFromContext(req.Context()); span != nil {
				logrus.Println("BeforeSend: found span")
				span.SetTag(string(ext.PeerService), peerService)
			} else {
				logrus.Println("BeforeSend: no span found")
			}
		},
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("X-Foo: %s\n", r.Header.Get("X-Foo"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cli.Get("http://" + srv.Listener.Addr().String() + "/")
	fmt.Printf("request tag %q set to %q", string(ext.PeerService), mockTracer.FinishedSpans()[0].Tags()[string(ext.PeerService)])

	// Output:
	// X-Foo: qux
	// request tag "peer.service" set to "my-peer-service"
}

func ExampleClient_withAfterResponseHook() {
	mockTracer := tracingtest.NewTracer()
	cli := net.NewClient(net.Options{
		Tracer:                     &customTracer{mockTracer},
		OpentracingComponentTag:    "testclient",
		OpentracingSpanName:        "clientSpan",
		BearerTokenRefreshInterval: 10 * time.Second,
		BearerTokenFile:            "/tmp/foo.token",
		IdleConnTimeout:            2 * time.Second,
		AfterResponse: func(rsp *http.Response, err error) {
			if span := opentracing.SpanFromContext(rsp.Request.Context()); span != nil {
				span.SetTag("status.code", rsp.StatusCode)
				if err != nil {
					span.SetTag("error", err.Error())
				}
			}
			rsp.StatusCode = 255
		},
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rsp, err := cli.Get("http://" + srv.Listener.Addr().String() + "/")
	if err != nil {
		log.Fatalf("Failed to get: %v", err)
	}

	fmt.Printf("response code: %d\n", rsp.StatusCode)
	fmt.Printf("span status.code: %d", mockTracer.FinishedSpans()[0].Tags()["status.code"])

	// Output:
	// response code: 255
	// span status.code: 200
}

func ExampleClient_withMtlsCertRotation() {
	now := time.Now()
	caPEM, caKeyPEM, caCert, caKey := generateCert(certOptions{
		cn:        "Test Master Root CA and server cert",
		notBefore: now.Add(-1 * time.Hour),
		notAfter:  now.Add(24 * time.Hour),
		dns:       "localhost",
		ip:        "127.0.0.1",
	})

	opts := certOptions{
		cn:        "localhost",
		dns:       "localhost",
		ip:        "127.0.0.1",
		notBefore: now.Add(-2 * time.Hour),
		notAfter:  now.Add(1 * time.Hour),
		extKeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		signerCert: caCert,
		signerKey:  caKey,
	}
	clientCertPEM, clientKeyPEM, _, _ := generateCert(opts)

	// Setup trust pools
	serverClientCAs := x509.NewCertPool()
	serverClientCAs.AppendCertsFromPEM(caPEM) // Server only trusts master CA

	clientRootCAs := x509.NewCertPool()
	clientRootCAs.AppendCertsFromPEM(caPEM)

	// Setup Test Server
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	serverCert, err := tls.X509KeyPair(caPEM, caKeyPEM)
	if err != nil {
		log.Fatalf("failed to prepare server cert: %v", err)
	}

	srv.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS13,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    serverClientCAs,
		Certificates: []tls.Certificate{serverCert},

		// Explicitly enforce client SAN validation at the TLS layer
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if len(verifiedChains) > 0 && len(verifiedChains[0]) > 0 {
				clientCert := verifiedChains[0][0]
				// Reject the client if it has not the expected IP SAN
				allowedIP := stdlibnet.ParseIP("127.0.0.1")
				fail := true
				for _, ipadrr := range clientCert.IPAddresses {
					if ipadrr.Equal(allowedIP) {
						fail = false
						break
					}
				}
				if fail {
					return fmt.Errorf("mTLS authentication failed: client certificate SAN extension should contain expected IP 127.0.0.1")
				}
			}
			return nil
		},
	}
	srv.StartTLS()
	defer srv.Close()

	certFile, keyFile, cleanUp := writeTempCertFiles(clientCertPEM, clientKeyPEM)
	defer cleanUp()

	//
	// the Client will create a CertReloader and TLSClientConfig
	//
	cli := net.NewClient(net.Options{
		CertFile:            certFile,
		KeyFile:             keyFile,
		CertRefreshInterval: 10 * time.Millisecond,
		RootCAs:             clientRootCAs,
	})
	defer cli.Close() // do not leak goroutines

	rsp, err := cli.Get(srv.URL)
	if err != nil {
		log.Fatalf("mTLS request failed unexpectedly: %v", err)
	}
	rsp.Body.Close()

	fmt.Printf("response code: %d\n", rsp.StatusCode)

	// renew expired cert
	certPEM1, keyPEM1, _, _ := generateCert(certOptions{
		cn:        "localhost",
		notBefore: now.Add(-2 * time.Hour),
		notAfter:  now.Add(-1 * time.Hour), // expired
		dns:       "localhost",
		ip:        "127.0.0.1",
		extKeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		signerCert: caCert,
		signerKey:  caKey,
	})
	certFile1, keyFile1, cleanUp1 := writeTempCertFiles(certPEM1, keyPEM1)
	defer cleanUp1()
	os.Rename(certFile1, certFile)
	os.Rename(keyFile1, keyFile)
	cli.CloseIdleConnections()

	time.Sleep(50 * time.Millisecond)

	rsp, err = cli.Get(srv.URL)
	if err == nil {
		log.Fatal("mTLS request should be rejected")
	} else {
		fmt.Printf("Failed as expected\n")
	}

	// renew cert not matching SAN IP
	certPEM2, keyPEM2, _, _ := generateCert(certOptions{
		cn:        "localhost",
		notBefore: now.Add(-1 * time.Hour),
		notAfter:  now.Add(1 * time.Hour),
		dns:       "localhost",
		ip:        "10.0.0.1", // unknown IP
		extKeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		signerCert: caCert,
		signerKey:  caKey,
	})
	certFile2, keyFile2, cleanUp2 := writeTempCertFiles(certPEM2, keyPEM2)
	defer cleanUp2()
	os.Rename(certFile2, certFile)
	os.Rename(keyFile2, keyFile)
	cli.CloseIdleConnections()

	time.Sleep(50 * time.Millisecond)

	rsp, err = cli.Get(srv.URL)
	if err == nil {
		log.Fatal("mTLS request should be rejected")
	} else {
		fmt.Printf("Failed as expected\n")
	}

	// renew cert good cert
	certPEM3, keyPEM3, _, _ := generateCert(certOptions{
		cn:        "localhost",
		notBefore: now.Add(-1 * time.Hour),
		notAfter:  now.Add(1 * time.Hour),
		dns:       "localhost",
		ip:        "127.0.0.1",
		extKeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		signerCert: caCert,
		signerKey:  caKey,
	})
	certFile3, keyFile3, cleanUp3 := writeTempCertFiles(certPEM3, keyPEM3)
	defer cleanUp3()
	os.Rename(certFile3, certFile)
	os.Rename(keyFile3, keyFile)
	cli.CloseIdleConnections()

	time.Sleep(50 * time.Millisecond)

	rsp, err = cli.Get(srv.URL)
	if err != nil {
		log.Fatalf("mTLS request failed unexpectedly: %v", err)
	}
	rsp.Body.Close()

	fmt.Printf("response code: %d\n", rsp.StatusCode)

	// Output:
	// response code: 200
	// Failed as expected
	// Failed as expected
	// response code: 200

}

type certOptions struct {
	cn           string
	dns          string
	ip           string
	notBefore    time.Time
	notAfter     time.Time
	extKeyUsages []x509.ExtKeyUsage
	signerCert   *x509.Certificate // If nil, certificate will be self-signed
	signerKey    *ecdsa.PrivateKey
}

func generateCert(opts certOptions) (certPEM, keyPEM []byte, parsedCert *x509.Certificate, privateKey *ecdsa.PrivateKey) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: opts.cn},
		NotBefore:    opts.notBefore,
		NotAfter:     opts.notAfter,

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           opts.extKeyUsages,
		BasicConstraintsValid: true,
	}

	// SAN DNS
	if opts.dns != "" {
		template.DNSNames = []string{opts.dns}
	}
	// SAN IP
	if opts.ip != "" {
		if ip := stdlibnet.ParseIP(opts.ip); ip != nil {
			template.IPAddresses = []stdlibnet.IP{ip}
		}
	}

	signingTemplate := template
	signingKey := privateKey

	// If a separate signer CA is provided, use it instead of self-signing
	if opts.signerCert != nil && opts.signerKey != nil {
		signingTemplate = opts.signerCert
		signingKey = opts.signerKey
	} else {
		// If self-signed, act as a CA so it can be added to RootCAs pools
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, signingTemplate, &privateKey.PublicKey, signingKey)
	if err != nil {
		log.Fatalf("failed to create certificate: %v", err)
	}

	parsedCert, err = x509.ParseCertificate(certDER)
	if err != nil {
		log.Fatalf("failed to parse generated certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		log.Fatalf("failed to marshal key: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}

func writeTempCertFiles(cert, key []byte) (string, string, func()) {
	cFile, err := os.CreateTemp("", "client-cert-*.crt")
	if err != nil {
		log.Fatalf("failed to create temp cert file: %v", err)
	}
	kFile, err := os.CreateTemp("", "client-key-*.key")
	if err != nil {
		log.Fatalf("failed to create temp key file: %v", err)
	}

	if err := os.WriteFile(cFile.Name(), cert, 0644); err != nil {
		log.Fatalf("failed to write temp cert: %v", err)
	}
	if err := os.WriteFile(kFile.Name(), key, 0600); err != nil {
		log.Fatalf("failed to write temp key: %v", err)
	}

	return cFile.Name(), kFile.Name(), func() {
		_ = os.Remove(cFile.Name())
		_ = os.Remove(kFile.Name())
	}
}
