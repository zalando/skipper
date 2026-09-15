package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

// makeSelfSignedCert creates an in-memory ECDSA P-256 self-signed certificate.
func makeSelfSignedCert(tb testing.TB, tmpl *x509.Certificate) *x509.Certificate {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatalf("generate key: %v", err)
	}
	now := time.Now()
	tmpl.SerialNumber = big.NewInt(now.UnixNano())
	tmpl.NotBefore = now.Add(-time.Hour)
	tmpl.NotAfter = now.Add(24 * time.Hour)
	tmpl.BasicConstraintsValid = true
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		tb.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		tb.Fatalf("parse certificate: %v", err)
	}
	return cert
}

// makeCA creates a self-signed CA certificate and returns it with its private key.
func makeCA(tb testing.TB, cn string) (*x509.Certificate, *ecdsa.PrivateKey) {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatalf("generate CA key: %v", err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		tb.Fatalf("create CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		tb.Fatalf("parse CA certificate: %v", err)
	}
	return cert, key
}

// makeSignedCert creates a leaf cert signed by the provided CA.
func makeSignedCert(tb testing.TB, tmpl *x509.Certificate, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) *x509.Certificate {
	tb.Helper()
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatalf("generate leaf key: %v", err)
	}
	now := time.Now()
	tmpl.SerialNumber = big.NewInt(now.UnixNano())
	tmpl.NotBefore = now.Add(-time.Hour)
	tmpl.NotAfter = now.Add(24 * time.Hour)
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		tb.Fatalf("create signed certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		tb.Fatalf("parse signed certificate: %v", err)
	}
	return cert
}

// reqWithCert wraps a certificate into an *http.Request with TLS populated.
func reqWithCert(cert *x509.Certificate) *http.Request {
	return &http.Request{
		TLS: &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		},
	}
}

var noTLSReq = &http.Request{}
var emptyTLSReq = &http.Request{TLS: &tls.ConnectionState{}}

// --- TestName ---

func TestName(t *testing.T) {
	for _, spec := range []routing.PredicateSpec{
		NewTLSClientCheckIssuerDNPredicate(),
		NewTLSClientCheckIssuerCNPredicate(),
		NewTLSClientCheckSanDNSPredicate(),
		NewTLSClientCheckSanCIDRPredicate(),
		NewTLSClientCheckSanIPPredicate(),
		NewTLSClientCheckSanURIPredicate(),
		NewTLSClientCheckCNPredicate(),
	} {
		if name := spec.Name(); name != predicates.TLSClientName {
			t.Errorf("expected name %q, got %q", predicates.TLSClientName, name)
		}
	}
}

// --- TestCreate ---

func TestCreateIssuerDN(t *testing.T) {
	spec := NewTLSClientCheckIssuerDNPredicate()
	for _, tc := range []struct {
		msg     string
		args    []any
		wantErr bool
	}{
		{"no args — empty allowlist is allowed", nil, false},
		{"non-string arg", []any{42}, true},
		{"empty string arg", []any{""}, true},
		{"one valid DN", []any{"CN=myca,O=Acme"}, false},
		{"multiple valid DNs", []any{"CN=myca,O=Acme", "CN=other,O=Corp"}, false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			_, err := spec.Create(tc.args)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCreateIssuerCN(t *testing.T) {
	spec := NewTLSClientCheckIssuerCNPredicate()
	for _, tc := range []struct {
		msg     string
		args    []any
		wantErr bool
	}{
		{"no args — empty allowlist is allowed", nil, false},
		{"non-string arg", []any{3.14}, true},
		{"empty string arg", []any{""}, true},
		{"one valid CN", []any{"My Issuer CA"}, false},
		{"multiple valid CNs", []any{"CA1", "CA2"}, false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			_, err := spec.Create(tc.args)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCreateCN(t *testing.T) {
	spec := NewTLSClientCheckCNPredicate()
	for _, tc := range []struct {
		msg     string
		args    []any
		wantErr bool
	}{
		{"no args — empty allowlist is allowed", nil, false},
		{"non-string arg", []any{true}, true},
		{"empty string arg", []any{""}, true},
		{"one valid CN", []any{"client.example.com"}, false},
		{"multiple valid CNs", []any{"client1", "client2"}, false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			_, err := spec.Create(tc.args)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCreateSanDNS(t *testing.T) {
	spec := NewTLSClientCheckSanDNSPredicate()
	for _, tc := range []struct {
		msg     string
		args    []any
		wantErr bool
	}{
		{"no args — empty allowlist is allowed", nil, false},
		{"non-string arg", []any{99}, true},
		{"empty string arg", []any{""}, true},
		{"IP address as hostname", []any{"192.168.1.1"}, true},
		{"bare wildcard * accepted as wildcard label", []any{"*"}, false},
		{"multilabel wildcard", []any{"*.*"}, true},
		{"underscore label", []any{"_foo.example.com"}, true},
		{"exact hostname", []any{"api.example.com"}, false},
		{"wildcard hostname", []any{"*.example.com"}, false},
		{"exact and wildcard mixed", []any{"api.example.com", "*.internal.example.com"}, false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			_, err := spec.Create(tc.args)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCreateSanCIDR(t *testing.T) {
	spec := NewTLSClientCheckSanCIDRPredicate()
	for _, tc := range []struct {
		msg     string
		args    []any
		wantErr bool
	}{
		{"no args", nil, true},
		{"non-string arg", []any{42}, true},
		{"invalid CIDR", []any{"not-a-cidr"}, true},
		{"bare IP without prefix length", []any{"10.0.0.1"}, true},
		{"single valid IPv4 CIDR", []any{"10.0.0.0/8"}, false},
		{"multiple CIDRs", []any{"10.0.0.0/8", "192.168.0.0/16"}, false},
		{"IPv6 CIDR", []any{"2001:db8::/32"}, false},
		{"mixed IPv4 and IPv6 CIDRs", []any{"10.0.0.0/8", "2001:db8::/32"}, false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			_, err := spec.Create(tc.args)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCreateSanIP(t *testing.T) {
	spec := NewTLSClientCheckSanIPPredicate()
	for _, tc := range []struct {
		msg     string
		args    []any
		wantErr bool
	}{
		{"no args", nil, true},
		{"non-string arg", []any{42}, true},
		{"invalid IP", []any{"999.0.0.1"}, true},
		{"CIDR notation not accepted", []any{"10.0.0.0/8"}, true},
		{"single valid IPv4", []any{"192.168.1.1"}, false},
		{"single valid IPv6", []any{"2001:db8::1"}, false},
		{"multiple IPs", []any{"192.168.1.1", "10.0.0.1"}, false},
		{"mixed IPv4 and IPv6", []any{"192.168.1.1", "2001:db8::1"}, false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			_, err := spec.Create(tc.args)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCreateSanURI(t *testing.T) {
	spec := NewTLSClientCheckSanURIPredicate()
	for _, tc := range []struct {
		msg     string
		args    []any
		wantErr bool
	}{
		{"no args — empty allowlist is allowed", nil, false},
		{"non-string arg", []any{42}, true},
		{"empty string", []any{""}, true},
		{"URI without scheme", []any{"example.com/path"}, true},
		{"invalid glob pattern", []any{"spiffe://[bad"}, true},
		{"exact URI with scheme", []any{"spiffe://cluster.local/ns/default/sa/myapp"}, false},
		{"glob URI", []any{"spiffe://*/ns/*/sa/mysa"}, false},
		{"exact and glob mixed", []any{"https://exact.example.com", "spiffe://*/ns/*/sa/*"}, false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			_, err := spec.Create(tc.args)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// --- TestMatch ---

func TestMatchIssuerDN(t *testing.T) {
	caCert, caKey := makeCA(t, "Test Issuer CA")
	issuerDN := caCert.Subject.String()
	leafCert := makeSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "client"},
	}, caCert, caKey)

	otherCA, otherKey := makeCA(t, "Other CA")
	otherLeaf := makeSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "client"},
	}, otherCA, otherKey)

	pred, err := NewTLSClientCheckIssuerDNPredicate().Create([]any{issuerDN})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, tc := range []struct {
		msg       string
		req       *http.Request
		wantMatch bool
	}{
		{"no TLS", noTLSReq, false},
		{"empty peer certs", emptyTLSReq, false},
		{"matching issuer DN", reqWithCert(leafCert), true},
		{"non-matching issuer DN", reqWithCert(otherLeaf), false},
{"self-signed CA: issuer == subject, so issuer DN matches predicate", reqWithCert(caCert), true},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			if got := pred.Match(tc.req); got != tc.wantMatch {
				t.Errorf("Match() = %v, want %v", got, tc.wantMatch)
			}
		})
	}
}

func TestMatchIssuerCN(t *testing.T) {
	caCert, caKey := makeCA(t, "Trusted Issuer")
	leafCert := makeSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "client"},
	}, caCert, caKey)

	otherCA, otherKey := makeCA(t, "Untrusted Issuer")
	otherLeaf := makeSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "client"},
	}, otherCA, otherKey)

	pred, err := NewTLSClientCheckIssuerCNPredicate().Create([]any{"Trusted Issuer"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, tc := range []struct {
		msg       string
		req       *http.Request
		wantMatch bool
	}{
		{"no TLS", noTLSReq, false},
		{"empty peer certs", emptyTLSReq, false},
		{"matching issuer CN", reqWithCert(leafCert), true},
		{"non-matching issuer CN", reqWithCert(otherLeaf), false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			if got := pred.Match(tc.req); got != tc.wantMatch {
				t.Errorf("Match() = %v, want %v", got, tc.wantMatch)
			}
		})
	}
}

func TestMatchCN(t *testing.T) {
	certMatch := makeSelfSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "allowed-client"},
	})
	certNoMatch := makeSelfSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "denied-client"},
	})
	certEmptyCN := makeSelfSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{},
	})

	pred, err := NewTLSClientCheckCNPredicate().Create([]any{"allowed-client"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, tc := range []struct {
		msg       string
		req       *http.Request
		wantMatch bool
	}{
		{"no TLS", noTLSReq, false},
		{"empty peer certs", emptyTLSReq, false},
		{"matching CN", reqWithCert(certMatch), true},
		{"non-matching CN", reqWithCert(certNoMatch), false},
		{"empty CN", reqWithCert(certEmptyCN), false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			if got := pred.Match(tc.req); got != tc.wantMatch {
				t.Errorf("Match() = %v, want %v", got, tc.wantMatch)
			}
		})
	}
}

func TestMatchSanDNS(t *testing.T) {
	certExact := makeSelfSignedCert(t, &x509.Certificate{
		Subject:  pkix.Name{CommonName: "c"},
		DNSNames: []string{"api.example.com"},
	})
	certUpper := makeSelfSignedCert(t, &x509.Certificate{
		Subject:  pkix.Name{CommonName: "c"},
		DNSNames: []string{"API.EXAMPLE.COM"},
	})
	certWildMatch := makeSelfSignedCert(t, &x509.Certificate{
		Subject:  pkix.Name{CommonName: "c"},
		DNSNames: []string{"sub.example.com"},
	})
	certTwoLabels := makeSelfSignedCert(t, &x509.Certificate{
		Subject:  pkix.Name{CommonName: "c"},
		DNSNames: []string{"sub.sub.example.com"},
	})
	certNoDNS := makeSelfSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "c"},
	})
	certMultiDNS := makeSelfSignedCert(t, &x509.Certificate{
		Subject:  pkix.Name{CommonName: "c"},
		DNSNames: []string{"other.example.com", "api.example.com"},
	})

	pred, err := NewTLSClientCheckSanDNSPredicate().Create([]any{"api.example.com", "*.example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, tc := range []struct {
		msg       string
		req       *http.Request
		wantMatch bool
	}{
		{"no TLS", noTLSReq, false},
		{"empty peer certs", emptyTLSReq, false},
		{"exact DNS match", reqWithCert(certExact), true},
		{"case-insensitive exact match", reqWithCert(certUpper), true},
		{"wildcard DNS match", reqWithCert(certWildMatch), true},
		{"two-label subdomain does not match wildcard", reqWithCert(certTwoLabels), false},
		{"no DNS SANs", reqWithCert(certNoDNS), false},
		{"second SAN matches", reqWithCert(certMultiDNS), true},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			if got := pred.Match(tc.req); got != tc.wantMatch {
				t.Errorf("Match() = %v, want %v", got, tc.wantMatch)
			}
		})
	}
}

func TestMatchSanCIDR(t *testing.T) {
	certIPv4 := makeSelfSignedCert(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: []net.IP{net.ParseIP("10.0.0.5")},
	})
	certIPv6 := makeSelfSignedCert(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: []net.IP{net.ParseIP("2001:db8::1")},
	})
	certMixed := makeSelfSignedCert(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: []net.IP{net.ParseIP("10.0.0.5"), net.ParseIP("2001:db8::1")},
	})
	certOutOfRange := makeSelfSignedCert(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: []net.IP{net.ParseIP("10.1.2.3")},
	})
	certNoIP := makeSelfSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "c"},
	})

	predIPv4, err := NewTLSClientCheckSanCIDRPredicate().Create([]any{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("Create IPv4 CIDR: %v", err)
	}
	predIPv6, err := NewTLSClientCheckSanCIDRPredicate().Create([]any{"2001:db8::/32"})
	if err != nil {
		t.Fatalf("Create IPv6 CIDR: %v", err)
	}
	predOther, err := NewTLSClientCheckSanCIDRPredicate().Create([]any{"192.168.0.0/16"})
	if err != nil {
		t.Fatalf("Create other CIDR: %v", err)
	}

	for _, tc := range []struct {
		msg       string
		pred      routing.Predicate
		req       *http.Request
		wantMatch bool
	}{
		{"no TLS", predIPv4, noTLSReq, false},
		{"empty peer certs", predIPv4, emptyTLSReq, false},
		{"IPv4 in CIDR", predIPv4, reqWithCert(certIPv4), true},
		{"IPv6 in CIDR", predIPv6, reqWithCert(certIPv6), true},
		{"mixed cert, IPv4 CIDR matches", predIPv4, reqWithCert(certMixed), true},
		{"mixed cert, IPv6 CIDR matches", predIPv6, reqWithCert(certMixed), true},
		{"mixed cert, neither CIDR matches", predOther, reqWithCert(certMixed), false},
		{"IPv4 out of CIDR range", predOther, reqWithCert(certOutOfRange), false},
		{"no IP SANs", predIPv4, reqWithCert(certNoIP), false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			if got := tc.pred.Match(tc.req); got != tc.wantMatch {
				t.Errorf("Match() = %v, want %v", got, tc.wantMatch)
			}
		})
	}
}

func TestMatchSanIP(t *testing.T) {
	certIPv4 := makeSelfSignedCert(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: []net.IP{net.ParseIP("192.168.1.1")},
	})
	certIPv6 := makeSelfSignedCert(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: []net.IP{net.ParseIP("2001:db8::1")},
	})
	// IPv4-mapped IPv6: net.ParseIP returns a 16-byte representation.
	certIPv4Mapped := makeSelfSignedCert(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: []net.IP{net.ParseIP("::ffff:192.168.1.1")},
	})
	certMixed := makeSelfSignedCert(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: []net.IP{net.ParseIP("192.168.1.1"), net.ParseIP("2001:db8::1")},
	})
	certOtherIP := makeSelfSignedCert(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: []net.IP{net.ParseIP("10.0.0.1")},
	})
	certNoIP := makeSelfSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "c"},
	})

	predIPv4, err := NewTLSClientCheckSanIPPredicate().Create([]any{"192.168.1.1"})
	if err != nil {
		t.Fatalf("Create IPv4: %v", err)
	}
	predIPv6, err := NewTLSClientCheckSanIPPredicate().Create([]any{"2001:db8::1"})
	if err != nil {
		t.Fatalf("Create IPv6: %v", err)
	}

	for _, tc := range []struct {
		msg       string
		pred      routing.Predicate
		req       *http.Request
		wantMatch bool
	}{
		{"no TLS", predIPv4, noTLSReq, false},
		{"empty peer certs", predIPv4, emptyTLSReq, false},
		{"IPv4 exact match", predIPv4, reqWithCert(certIPv4), true},
		{"IPv4-mapped IPv6 matches IPv4 predicate", predIPv4, reqWithCert(certIPv4Mapped), true},
		{"IPv6 exact match", predIPv6, reqWithCert(certIPv6), true},
		{"mixed cert, IPv4 predicate matches", predIPv4, reqWithCert(certMixed), true},
		{"mixed cert, IPv6 predicate matches", predIPv6, reqWithCert(certMixed), true},
		{"IP mismatch", predIPv4, reqWithCert(certOtherIP), false},
		{"no IP SANs", predIPv4, reqWithCert(certNoIP), false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			if got := tc.pred.Match(tc.req); got != tc.wantMatch {
				t.Errorf("Match() = %v, want %v", got, tc.wantMatch)
			}
		})
	}
}

func TestMatchSanURI(t *testing.T) {
	uriExact, _ := url.Parse("spiffe://cluster.local/ns/default/sa/myapp")
	uriSpiffe, _ := url.Parse("spiffe://cluster.local/ns/prod/sa/svc")
	uriHTTPS, _ := url.Parse("https://exact.example.com")
	uriOther, _ := url.Parse("https://other.com")

	certExact := makeSelfSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "c"},
		URIs:    []*url.URL{uriExact},
	})
	certGlob := makeSelfSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "c"},
		URIs:    []*url.URL{uriSpiffe},
	})
	// certBothURIs has an exact-matching HTTPS URI and a glob-matching spiffe URI.
	certBothURIs := makeSelfSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "c"},
		URIs:    []*url.URL{uriHTTPS, uriSpiffe},
	})
	certOther := makeSelfSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "c"},
		URIs:    []*url.URL{uriOther},
	})
	certNoURI := makeSelfSignedCert(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "c"},
	})

	predExact, err := NewTLSClientCheckSanURIPredicate().Create([]any{
		"spiffe://cluster.local/ns/default/sa/myapp",
	})
	if err != nil {
		t.Fatalf("Create exact: %v", err)
	}
	predGlob, err := NewTLSClientCheckSanURIPredicate().Create([]any{
		"spiffe://cluster.local/ns/*/sa/*",
	})
	if err != nil {
		t.Fatalf("Create glob: %v", err)
	}
	// predMixed has both an exact HTTPS URI and a spiffe glob.
	predMixed, err := NewTLSClientCheckSanURIPredicate().Create([]any{
		"https://exact.example.com",
		"spiffe://*/ns/*/sa/*",
	})
	if err != nil {
		t.Fatalf("Create mixed: %v", err)
	}

	for _, tc := range []struct {
		msg       string
		pred      routing.Predicate
		req       *http.Request
		wantMatch bool
	}{
		{"no TLS", predExact, noTLSReq, false},
		{"empty peer certs", predExact, emptyTLSReq, false},
		{"exact URI match", predExact, reqWithCert(certExact), true},
		{"glob URI match", predGlob, reqWithCert(certGlob), true},
		{"glob does not match different scheme", predGlob, reqWithCert(certOther), false},
		{"mixed predicate: cert has both URIs, HTTPS exact matches", predMixed, reqWithCert(certBothURIs), true},
		{"mixed predicate: cert has only glob-matching spiffe URI", predMixed, reqWithCert(certGlob), true},
		{"mixed predicate: cert has only exact-matched HTTPS URI", predMixed, reqWithCert(certBothURIs), true},
		{"cert URI matches neither exact nor glob", predMixed, reqWithCert(certOther), false},
		{"no URI SANs", predExact, reqWithCert(certNoURI), false},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			if got := tc.pred.Match(tc.req); got != tc.wantMatch {
				t.Errorf("Match() = %v, want %v", got, tc.wantMatch)
			}
		})
	}
}

// --- Benchmarks ---

func BenchmarkMatchIssuerDN(b *testing.B) {
	caCert, caKey := makeCA(b, "Bench Issuer")
	issuerDN := caCert.Subject.String()
	leafHit := makeSignedCert(b, &x509.Certificate{Subject: pkix.Name{CommonName: "client"}}, caCert, caKey)
	otherCA, otherKey := makeCA(b, "Other Issuer")
	leafMiss := makeSignedCert(b, &x509.Certificate{Subject: pkix.Name{CommonName: "client"}}, otherCA, otherKey)
	reqHit := reqWithCert(leafHit)
	reqMiss := reqWithCert(leafMiss)

	for _, n := range []int{1, 5, 10, 50, 100} {
		args := make([]any, n)
		for i := range n - 1 {
			args[i] = fmt.Sprintf("CN=ca%d,O=Bench", i)
		}
		args[n-1] = issuerDN
		pred, _ := NewTLSClientCheckIssuerDNPredicate().Create(args)
		b.Run(fmt.Sprintf("n=%d/hit", n), func(b *testing.B) {
			for b.Loop() {
				pred.Match(reqHit)
			}
		})
		b.Run(fmt.Sprintf("n=%d/miss", n), func(b *testing.B) {
			for b.Loop() {
				pred.Match(reqMiss)
			}
		})
	}
}

func BenchmarkMatchIssuerCN(b *testing.B) {
	caCert, caKey := makeCA(b, "Bench Issuer CN")
	leafHit := makeSignedCert(b, &x509.Certificate{Subject: pkix.Name{CommonName: "client"}}, caCert, caKey)
	otherCA, otherKey := makeCA(b, "Other Issuer CN")
	leafMiss := makeSignedCert(b, &x509.Certificate{Subject: pkix.Name{CommonName: "client"}}, otherCA, otherKey)
	reqHit := reqWithCert(leafHit)
	reqMiss := reqWithCert(leafMiss)

	for _, n := range []int{1, 5, 10, 50, 100} {
		args := make([]any, n)
		for i := range n - 1 {
			args[i] = fmt.Sprintf("other-ca-%d", i)
		}
		args[n-1] = "Bench Issuer CN"
		pred, _ := NewTLSClientCheckIssuerCNPredicate().Create(args)
		b.Run(fmt.Sprintf("n=%d/hit", n), func(b *testing.B) {
			for b.Loop() {
				pred.Match(reqHit)
			}
		})
		b.Run(fmt.Sprintf("n=%d/miss", n), func(b *testing.B) {
			for b.Loop() {
				pred.Match(reqMiss)
			}
		})
	}
}

func BenchmarkMatchCN(b *testing.B) {
	certHit := makeSelfSignedCert(b, &x509.Certificate{Subject: pkix.Name{CommonName: "allowed-client"}})
	certMiss := makeSelfSignedCert(b, &x509.Certificate{Subject: pkix.Name{CommonName: "denied-client"}})
	reqHit := reqWithCert(certHit)
	reqMiss := reqWithCert(certMiss)

	for _, n := range []int{1, 5, 10, 50, 100} {
		args := make([]any, n)
		for i := range n - 1 {
			args[i] = fmt.Sprintf("other-client-%d", i)
		}
		args[n-1] = "allowed-client"
		pred, _ := NewTLSClientCheckCNPredicate().Create(args)
		b.Run(fmt.Sprintf("n=%d/hit", n), func(b *testing.B) {
			for b.Loop() {
				pred.Match(reqHit)
			}
		})
		b.Run(fmt.Sprintf("n=%d/miss", n), func(b *testing.B) {
			for b.Loop() {
				pred.Match(reqMiss)
			}
		})
	}
}

func makeBenchDNSCert(b *testing.B, certSANs int) *x509.Certificate {
	b.Helper()
	dns := make([]string, certSANs)
	for i := range certSANs - 1 {
		dns[i] = fmt.Sprintf("other%d.example.com", i)
	}
	dns[certSANs-1] = "api.example.com"
	return makeSelfSignedCert(b, &x509.Certificate{
		Subject:  pkix.Name{CommonName: "c"},
		DNSNames: dns,
	})
}

func BenchmarkMatchSanDNS(b *testing.B) {
	certMiss := makeSelfSignedCert(b, &x509.Certificate{
		Subject:  pkix.Name{CommonName: "c"},
		DNSNames: []string{"nomatch.example.com"},
	})
	reqMiss := reqWithCert(certMiss)

	for _, certSANs := range []int{1, 5, 20} {
		certHit := makeBenchDNSCert(b, certSANs)
		reqHit := reqWithCert(certHit)
		for _, n := range []int{1, 5, 10, 50, 100} {
			args := make([]any, n)
			for i := range n - 1 {
				args[i] = fmt.Sprintf("host%d.example.com", i)
			}
			args[n-1] = "api.example.com"
			pred, _ := NewTLSClientCheckSanDNSPredicate().Create(args)
			b.Run(fmt.Sprintf("certSANs=%d/n=%d/hit", certSANs, n), func(b *testing.B) {
				for b.Loop() {
					pred.Match(reqHit)
				}
			})
			b.Run(fmt.Sprintf("certSANs=%d/n=%d/miss", certSANs, n), func(b *testing.B) {
				for b.Loop() {
					pred.Match(reqMiss)
				}
			})
		}
	}
}

func makeBenchIPCert(b *testing.B, certSANs int) *x509.Certificate {
	b.Helper()
	ips := make([]net.IP, certSANs)
	for i := range certSANs - 1 {
		ips[i] = net.ParseIP(fmt.Sprintf("192.168.%d.1", i%256))
	}
	ips[certSANs-1] = net.ParseIP("10.0.0.1")
	return makeSelfSignedCert(b, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: ips,
	})
}

func BenchmarkMatchSanCIDR(b *testing.B) {
	certMiss := makeSelfSignedCert(b, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: []net.IP{net.ParseIP("172.16.0.1")},
	})
	reqMiss := reqWithCert(certMiss)

	for _, certSANs := range []int{1, 5, 20} {
		certHit := makeBenchIPCert(b, certSANs)
		reqHit := reqWithCert(certHit)
		for _, n := range []int{1, 5, 10, 50, 100} {
			args := make([]any, n)
			for i := range n - 1 {
				args[i] = fmt.Sprintf("192.168.%d.0/24", i%256)
			}
			args[n-1] = "10.0.0.0/8"
			pred, _ := NewTLSClientCheckSanCIDRPredicate().Create(args)
			b.Run(fmt.Sprintf("certSANs=%d/n=%d/hit", certSANs, n), func(b *testing.B) {
				for b.Loop() {
					pred.Match(reqHit)
				}
			})
			b.Run(fmt.Sprintf("certSANs=%d/n=%d/miss", certSANs, n), func(b *testing.B) {
				for b.Loop() {
					pred.Match(reqMiss)
				}
			})
		}
	}
}

func BenchmarkMatchSanIP(b *testing.B) {
	certMiss := makeSelfSignedCert(b, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "c"},
		IPAddresses: []net.IP{net.ParseIP("172.16.0.1")},
	})
	reqMiss := reqWithCert(certMiss)

	for _, certSANs := range []int{1, 5, 20} {
		certHit := makeBenchIPCert(b, certSANs)
		reqHit := reqWithCert(certHit)
		for _, n := range []int{1, 5, 10, 50, 100} {
			args := make([]any, n)
			for i := range n - 1 {
				args[i] = fmt.Sprintf("192.168.%d.1", i%256)
			}
			args[n-1] = "10.0.0.1"
			pred, _ := NewTLSClientCheckSanIPPredicate().Create(args)
			b.Run(fmt.Sprintf("certSANs=%d/n=%d/hit", certSANs, n), func(b *testing.B) {
				for b.Loop() {
					pred.Match(reqHit)
				}
			})
			b.Run(fmt.Sprintf("certSANs=%d/n=%d/miss", certSANs, n), func(b *testing.B) {
				for b.Loop() {
					pred.Match(reqMiss)
				}
			})
		}
	}
}

func makeBenchURICert(b *testing.B, certSANs int) *x509.Certificate {
	b.Helper()
	uris := make([]*url.URL, certSANs)
	for i := range certSANs - 1 {
		u, _ := url.Parse(fmt.Sprintf("https://other%d.example.com/path", i))
		uris[i] = u
	}
	u, _ := url.Parse("spiffe://cluster.local/ns/prod/sa/svc")
	uris[certSANs-1] = u
	return makeSelfSignedCert(b, &x509.Certificate{
		Subject: pkix.Name{CommonName: "c"},
		URIs:    uris,
	})
}

func BenchmarkMatchSanURI(b *testing.B) {
	uMiss, _ := url.Parse("https://nomatch.example.com")
	certMiss := makeSelfSignedCert(b, &x509.Certificate{
		Subject: pkix.Name{CommonName: "c"},
		URIs:    []*url.URL{uMiss},
	})
	reqMiss := reqWithCert(certMiss)

	for _, certSANs := range []int{1, 5, 20} {
		certHit := makeBenchURICert(b, certSANs)
		reqHit := reqWithCert(certHit)
		for _, n := range []int{1, 5, 10, 50, 100} {
			args := make([]any, n)
			for i := range n - 1 {
				args[i] = fmt.Sprintf("https://other%d.example.com/path", i)
			}
			args[n-1] = "spiffe://*/ns/*/sa/*"
			pred, _ := NewTLSClientCheckSanURIPredicate().Create(args)
			b.Run(fmt.Sprintf("certSANs=%d/n=%d/hit", certSANs, n), func(b *testing.B) {
				for b.Loop() {
					pred.Match(reqHit)
				}
			})
			b.Run(fmt.Sprintf("certSANs=%d/n=%d/miss", certSANs, n), func(b *testing.B) {
				for b.Loop() {
					pred.Match(reqMiss)
				}
			})
		}
	}
}
