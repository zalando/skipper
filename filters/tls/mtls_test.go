package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

// buildConnStateWithSANs generates an ECDSA P-256 certificate with the given
// DNS names, IP addresses, and URI SANs and returns a *tls.ConnectionState
// with it as the sole peer certificate.
func buildConnStateWithSANs(t *testing.T, dnsNames []string, ips []net.IP, uris []*url.URL) *tls.ConnectionState {
	if t != nil {
		t.Helper()
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:              dnsNames,
		IPAddresses:           ips,
		URIs:                  uris,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic(err)
	}

	return &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
}

func mustParseIP(s string) net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		panic("invalid IP: " + s)
	}
	return ip
}

// TestNewMtlsSAN_CreateFilter verifies argument validation in CreateFilter.
func TestNewMtlsSAN_CreateFilter(t *testing.T) {
	spec := NewMtlsSAN()
	assert.Equal(t, "mtlsSAN", spec.Name())

	for _, tt := range []struct {
		name    string
		args    []any
		wantErr bool
	}{
		{
			name:    "no args",
			args:    []any{},
			wantErr: true,
		},
		{
			name:    "non-string arg",
			args:    []any{42},
			wantErr: true,
		},
		{
			name:    "empty string arg",
			args:    []any{""},
			wantErr: true,
		},
		{
			name:    "invalid arg with special chars",
			args:    []any{"not valid!!"},
			wantErr: true,
		},
		{
			name:    "invalid arg with space",
			args:    []any{"foo bar"},
			wantErr: true,
		},
		{
			name:    "valid IP",
			args:    []any{"192.168.1.1"},
			wantErr: false,
		},
		{
			name:    "valid IPv6",
			args:    []any{"::1"},
			wantErr: false,
		},
		{
			name:    "valid CIDR",
			args:    []any{"10.0.0.0/8"},
			wantErr: false,
		},
		{
			name:    "valid IPv6 CIDR",
			args:    []any{"2001:db8::/32"},
			wantErr: false,
		},
		{
			name:    "valid hostname",
			args:    []any{"example.com"},
			wantErr: false,
		},
		{
			name:    "valid wildcard hostname",
			args:    []any{"*.example.com"},
			wantErr: false,
		},
		{
			name:    "multiple valid mixed args",
			args:    []any{"10.0.0.0/8", "example.com", "1.2.3.4"},
			wantErr: false,
		},
		{
			name:    "valid then invalid arg",
			args:    []any{"example.com", "not!!valid"},
			wantErr: true,
		},
		{
			name:    "valid URI (spiffe scheme)",
			args:    []any{"spiffe://example.org/service"},
			wantErr: false,
		},
		{
			name:    "valid URI (https scheme)",
			args:    []any{"https://example.com/path"},
			wantErr: false,
		},
		{
			name:    "multiple valid mixed args including URI",
			args:    []any{"10.0.0.0/8", "example.com", "spiffe://trust-domain/svc"},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, err := spec.CreateFilter(tt.args)
			if tt.wantErr {
				assert.ErrorIs(t, err, filters.ErrInvalidFilterParameters)
				assert.Nil(t, f)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, f)
			}
		})
	}
}

// buildConnStateWithIssuer generates a leaf certificate whose Issuer is set to
// issuerName by signing it with a CA that has issuerName as its Subject.
// Returns a *tls.ConnectionState containing the leaf cert as the sole peer
// certificate, plus the leaf cert itself so callers can read Issuer.String().
func buildConnStateWithIssuer(t *testing.T, issuerName pkix.Name) (*tls.ConnectionState, *x509.Certificate) {
	t.Helper()
	now := time.Now()

	// Generate CA key and self-signed CA cert with the desired Subject.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               issuerName,
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	// Generate leaf key and cert signed by the CA.
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "leaf"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	require.NoError(t, err)
	leafCert, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)

	return &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leafCert}}, leafCert
}

// TestNewMtlsIssuerDN_CreateFilter verifies argument validation in CreateFilter.
func TestNewMtlsIssuerDN_CreateFilter(t *testing.T) {
	spec := NewMtlsIssuerDN()
	assert.Equal(t, "mtlsIssuerDN", spec.Name())

	for _, tt := range []struct {
		name    string
		args    []any
		wantErr bool
	}{
		{
			name:    "no args",
			args:    []any{},
			wantErr: true,
		},
		{
			name:    "non-string arg",
			args:    []any{42},
			wantErr: true,
		},
		{
			name:    "empty string",
			args:    []any{""},
			wantErr: true,
		},
		{
			name:    "single valid DN",
			args:    []any{"CN=My CA,O=Org,C=DE"},
			wantErr: false,
		},
		{
			name:    "multiple valid DNs",
			args:    []any{"CN=CA1,O=Org", "CN=CA2,O=Org"},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, err := spec.CreateFilter(tt.args)
			if tt.wantErr {
				assert.ErrorIs(t, err, filters.ErrInvalidFilterParameters)
				assert.Nil(t, f)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, f)
			}
		})
	}
}

// TestNewMtlsIssuerCN_CreateFilter verifies argument validation in CreateFilter.
func TestNewMtlsIssuerCN_CreateFilter(t *testing.T) {
	spec := NewMtlsCN()
	assert.Equal(t, "mtlsCN", spec.Name())

	for _, tt := range []struct {
		name    string
		args    []any
		wantErr bool
	}{
		{
			name:    "no args",
			args:    []any{},
			wantErr: true,
		},
		{
			name:    "non-string arg",
			args:    []any{42},
			wantErr: true,
		},
		{
			name:    "empty string",
			args:    []any{""},
			wantErr: false,
		},
		{
			name:    "single valid CN",
			args:    []any{"My CA"},
			wantErr: false,
		},
		{
			name:    "multiple valid CNs",
			args:    []any{"CA1", "CA2"},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, err := spec.CreateFilter(tt.args)
			if tt.wantErr {
				assert.ErrorIs(t, err, filters.ErrInvalidFilterParameters)
				assert.Nil(t, f)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, f)
			}
		})
	}
}

// TestMtlsIssuerDN_Request verifies the filter's Request method covers all
// matching and rejection paths.
func TestMtlsIssuerDN_Request(t *testing.T) {
	spec := NewMtlsIssuerDN()

	// Build connection states once; derive the expected DN from the parsed cert.
	tlsExactMatch, certExact := buildConnStateWithIssuer(t, pkix.Name{
		CommonName:   "My CA",
		Organization: []string{"Org"},
		Country:      []string{"DE"},
	})
	exactDN := certExact.Issuer.String()

	tlsWrongCN, certWrongCN := buildConnStateWithIssuer(t, pkix.Name{
		CommonName:   "Other CA",
		Organization: []string{"Org"},
		Country:      []string{"DE"},
	})
	wrongCNDN := certWrongCN.Issuer.String()
	_ = wrongCNDN

	tlsWrongOrg, certWrongOrg := buildConnStateWithIssuer(t, pkix.Name{
		CommonName:   "My CA",
		Organization: []string{"Evil"},
		Country:      []string{"DE"},
	})
	_ = certWrongOrg

	tlsCA1, certCA1 := buildConnStateWithIssuer(t, pkix.Name{
		CommonName:   "CA1",
		Organization: []string{"Org"},
	})
	dn1 := certCA1.Issuer.String()

	tlsCA2, certCA2 := buildConnStateWithIssuer(t, pkix.Name{
		CommonName:   "CA2",
		Organization: []string{"Org"},
	})
	dn2 := certCA2.Issuer.String()

	tlsCA3, _ := buildConnStateWithIssuer(t, pkix.Name{
		CommonName:   "CA3",
		Organization: []string{"Org"},
	})

	for _, tt := range []struct {
		name           string
		tlsState       *tls.ConnectionState
		filterArgs     []any
		expectedStatus int
		expectServed   bool
	}{
		{
			name:           "no TLS — req.TLS is nil — 401 Unauthorized",
			tlsState:       nil,
			filterArgs:     []any{"CN=My CA"},
			expectedStatus: http.StatusUnauthorized,
			expectServed:   true,
		},
		{
			name:           "TLS but no peer certificates — 403 Forbidden",
			tlsState:       &tls.ConnectionState{},
			filterArgs:     []any{"CN=My CA"},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "exact full DN match is allowed",
			tlsState:       tlsExactMatch,
			filterArgs:     []any{exactDN},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "wrong CN is rejected — 403 Forbidden",
			tlsState:       tlsWrongCN,
			filterArgs:     []any{exactDN},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "wrong Organisation is rejected — 403 Forbidden",
			tlsState:       tlsWrongOrg,
			filterArgs:     []any{exactDN},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "partial DN (CN only) does not match full issuer DN — 403 Forbidden",
			tlsState:       tlsExactMatch,
			filterArgs:     []any{"CN=My CA"},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "multiple DNs — first matches",
			tlsState:       tlsCA1,
			filterArgs:     []any{dn1, dn2},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "multiple DNs — second matches",
			tlsState:       tlsCA2,
			filterArgs:     []any{dn1, dn2},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "multiple DNs — none match — 403 Forbidden",
			tlsState:       tlsCA3,
			filterArgs:     []any{dn1, dn2},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := spec.CreateFilter(tt.filterArgs)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
			require.NoError(t, err)
			req.TLS = tt.tlsState

			ctx := &filtertest.Context{FRequest: req}
			f.Request(ctx)

			assert.Equal(t, tt.expectServed, ctx.FServed)
			if tt.expectServed {
				require.NotNil(t, ctx.FResponse)
				assert.Equal(t, tt.expectedStatus, ctx.FResponse.StatusCode)
			}
		})
	}
}

// TestMtlsIssuerCN_Request verifies the filter's Request method covers all
// matching and rejection paths.
func TestMtlsIssuerCN_Request(t *testing.T) {
	spec := NewMtlsCN()

	// Build connection states once; derive the expected CN from the parsed cert.
	tlsExactMatch, certExact := buildConnStateWithIssuer(t, pkix.Name{
		CommonName:   "CA",
		Organization: []string{"Org"},
		Country:      []string{"DE"},
	})
	exactCN := certExact.Issuer.CommonName

	tlsWrongCN, _ := buildConnStateWithIssuer(t, pkix.Name{
		CommonName:   "Other CA",
		Organization: []string{"Org"},
		Country:      []string{"DE"},
	})

	tlsCA1, certCA1 := buildConnStateWithIssuer(t, pkix.Name{
		CommonName:   "cn1",
		Organization: []string{"Org"},
	})
	cn1 := certCA1.Issuer.CommonName

	tlsCA2, certCA2 := buildConnStateWithIssuer(t, pkix.Name{
		CommonName:   "cn2",
		Organization: []string{"Org"},
	})
	cn2 := certCA2.Issuer.CommonName

	tlsCA3, _ := buildConnStateWithIssuer(t, pkix.Name{
		CommonName:   "cn3",
		Organization: []string{"Org"},
	})

	for _, tt := range []struct {
		name           string
		tlsState       *tls.ConnectionState
		filterArgs     []any
		expectedStatus int
		expectServed   bool
	}{
		{
			name:           "no TLS — req.TLS is nil — 401 Unauthorized",
			tlsState:       nil,
			filterArgs:     []any{exactCN},
			expectedStatus: http.StatusUnauthorized,
			expectServed:   true,
		},
		{
			name:           "TLS but no peer certificates — 403 Forbidden",
			tlsState:       &tls.ConnectionState{},
			filterArgs:     []any{exactCN},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "exact full CN match is allowed",
			tlsState:       tlsExactMatch,
			filterArgs:     []any{exactCN},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "wrong CN is rejected — 403 Forbidden",
			tlsState:       tlsWrongCN,
			filterArgs:     []any{exactCN},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "multiple CNs — first matches",
			tlsState:       tlsCA1,
			filterArgs:     []any{cn1, cn2},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "multiple CNs — second matches",
			tlsState:       tlsCA2,
			filterArgs:     []any{cn1, cn2},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "multiple CN — none match — 403 Forbidden",
			tlsState:       tlsCA3,
			filterArgs:     []any{cn1, cn2},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := spec.CreateFilter(tt.filterArgs)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
			require.NoError(t, err)
			req.TLS = tt.tlsState

			ctx := &filtertest.Context{FRequest: req}
			f.Request(ctx)

			assert.Equal(t, tt.expectServed, ctx.FServed)
			if tt.expectServed {
				require.NotNil(t, ctx.FResponse)
				assert.Equal(t, tt.expectedStatus, ctx.FResponse.StatusCode)
			}
		})
	}
}

// TestMtlsSAN_Request verifies the filter's Request method covers all matching
// and rejection paths.
func TestMtlsSAN_Request(t *testing.T) {
	spec := NewMtlsSAN()

	for _, tt := range []struct {
		name           string
		tlsState       *tls.ConnectionState // nil → req.TLS == nil (plain HTTP)
		filterArgs     []any
		expectedStatus int  // 0 = allowed (no rejection)
		expectServed   bool // true when filter short-circuits via Serve()
	}{
		{
			name:           "no TLS — req.TLS is nil — 401 Unauthorized",
			tlsState:       nil,
			filterArgs:     []any{"example.com"},
			expectedStatus: http.StatusUnauthorized,
			expectServed:   true,
		},
		{
			name:           "TLS but no peer certificates — 403 Forbidden",
			tlsState:       &tls.ConnectionState{},
			filterArgs:     []any{"example.com"},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "DNS name matches exactly",
			tlsState:       buildConnStateWithSANs(nil, []string{"example.com"}, nil, nil),
			filterArgs:     []any{"example.com"},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "DNS name match is case-insensitive",
			tlsState:       buildConnStateWithSANs(nil, []string{"Example.COM"}, nil, nil),
			filterArgs:     []any{"example.com"},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "DNS name does not match — 403 Forbidden",
			tlsState:       buildConnStateWithSANs(nil, []string{"other.com"}, nil, nil),
			filterArgs:     []any{"example.com"},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "IP exact match",
			tlsState:       buildConnStateWithSANs(nil, nil, []net.IP{mustParseIP("1.2.3.4")}, nil),
			filterArgs:     []any{"1.2.3.4"},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "IP does not match — 403 Forbidden",
			tlsState:       buildConnStateWithSANs(nil, nil, []net.IP{mustParseIP("1.2.3.4")}, nil),
			filterArgs:     []any{"5.6.7.8"},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "IP inside CIDR /8 matches",
			tlsState:       buildConnStateWithSANs(nil, nil, []net.IP{mustParseIP("10.1.2.3")}, nil),
			filterArgs:     []any{"10.0.0.0/8"},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "IP outside CIDR /8 — 403 Forbidden",
			tlsState:       buildConnStateWithSANs(nil, nil, []net.IP{mustParseIP("192.168.1.1")}, nil),
			filterArgs:     []any{"10.0.0.0/8"},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "IP inside /24 matches",
			tlsState:       buildConnStateWithSANs(nil, nil, []net.IP{mustParseIP("192.168.1.100")}, nil),
			filterArgs:     []any{"192.168.1.0/24"},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "IP outside /24 — 403 Forbidden",
			tlsState:       buildConnStateWithSANs(nil, nil, []net.IP{mustParseIP("192.168.2.1")}, nil),
			filterArgs:     []any{"192.168.1.0/24"},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "multiple patterns — first matches",
			tlsState:       buildConnStateWithSANs(nil, []string{"a.com"}, nil, nil),
			filterArgs:     []any{"a.com", "b.com"},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "multiple patterns — second matches",
			tlsState:       buildConnStateWithSANs(nil, []string{"b.com"}, nil, nil),
			filterArgs:     []any{"a.com", "b.com"},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name:           "multiple patterns — none match — 403 Forbidden",
			tlsState:       buildConnStateWithSANs(nil, []string{"c.com"}, nil, nil),
			filterArgs:     []any{"a.com", "b.com"},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "multiple patterns including CIDR — IP matches second CIDR",
			tlsState:       buildConnStateWithSANs(nil, nil, []net.IP{mustParseIP("172.16.0.5")}, nil),
			filterArgs:     []any{"10.0.0.0/8", "172.16.0.0/12"},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name: "URI exact match — allowed",
			tlsState: buildConnStateWithSANs(nil, nil, nil,
				[]*url.URL{{Scheme: "spiffe", Host: "example.org", Path: "/svc"}}),
			filterArgs:     []any{"spiffe://example.org/svc"},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name: "URI no match — 403 Forbidden",
			tlsState: buildConnStateWithSANs(nil, nil, nil,
				[]*url.URL{{Scheme: "spiffe", Host: "example.org", Path: "/svc"}}),
			filterArgs:     []any{"spiffe://other.org/svc"},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name: "URI match among multiple patterns — second matches",
			tlsState: buildConnStateWithSANs(nil, nil, nil,
				[]*url.URL{{Scheme: "spiffe", Host: "b.org", Path: "/svc"}}),
			filterArgs:     []any{"spiffe://a.org/svc", "spiffe://b.org/svc"},
			expectedStatus: 0,
			expectServed:   false,
		},
		{
			name: "URI — multiple patterns, none match — 403 Forbidden",
			tlsState: buildConnStateWithSANs(nil, nil, nil,
				[]*url.URL{{Scheme: "spiffe", Host: "c.org", Path: "/svc"}}),
			filterArgs:     []any{"spiffe://a.org/svc", "spiffe://b.org/svc"},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := spec.CreateFilter(tt.filterArgs)
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
			require.NoError(t, err)
			req.TLS = tt.tlsState

			ctx := &filtertest.Context{FRequest: req}
			f.Request(ctx)

			assert.Equal(t, tt.expectServed, ctx.FServed)
			if tt.expectServed {
				require.NotNil(t, ctx.FResponse)
				assert.Equal(t, tt.expectedStatus, ctx.FResponse.StatusCode)
			}
		})
	}
}

// caBundle holds a CA certificate and its private key so callers can sign
// additional leaf certificates with different validity windows.
type caBundle struct {
	pem  []byte
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

// newCABundle generates a self-signed ECDSA CA cert and returns a caBundle.
func newCABundle(t *testing.T, cn string) *caBundle {
	t.Helper()
	now := time.Now()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return &caBundle{pem: pemBytes, cert: cert, key: key}
}

// signLeaf creates a leaf certificate signed by the given CA bundle.
func signLeaf(t *testing.T, ca *caBundle, leafCN string, notBefore, notAfter time.Time) (*tls.ConnectionState, *x509.Certificate) {
	t.Helper()
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: leafCN},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &leafKey.PublicKey, ca.key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}, cert
}

// TestNewMtlsAuthn_CreateFilter verifies argument validation in CreateFilter.
func TestNewMtlsAuthn_CreateFilter(t *testing.T) {
	ca, _ := x509.SystemCertPool()
	spec := NewMtlsAuthn(ca)
	assert.Equal(t, "mtlsAuthn", spec.Name())

	for _, tt := range []struct {
		name    string
		args    []any
		wantErr bool
	}{
		{
			name:    "no args",
			args:    []any{},
			wantErr: false,
		},
		{
			name:    "too many args",
			args:    []any{"a"},
			wantErr: true,
		},
		{
			name:    "non-string arg",
			args:    []any{42},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, err := spec.CreateFilter(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, f)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, f)
			}
		})
	}
}

// TestMtlsAuthn_Request verifies the filter's Request method.
func TestMtlsAuthn_Request(t *testing.T) {
	now := time.Now()

	// Trusted CA: signs the "positive" leaf and the expired/future leaves.
	trustedCA := newCABundle(t, "Trusted CA")

	// A separate CA — its leaves must be rejected by a filter loaded with trustedCA.
	untrustedCA := newCABundle(t, "Untrusted CA")

	validLeafTLS, _ := signLeaf(t, trustedCA, "my-service",
		now.Add(-time.Hour), now.Add(time.Hour))

	untrustedLeafTLS, _ := signLeaf(t, untrustedCA, "attacker",
		now.Add(-time.Hour), now.Add(time.Hour))

	// Expired leaf signed by the trusted CA.
	expiredLeafTLS, _ := signLeaf(t, trustedCA, "expired-service",
		now.Add(-2*time.Hour), now.Add(-time.Hour))

	// Future leaf signed by the trusted CA.
	futureLeafTLS, _ := signLeaf(t, trustedCA, "future-service",
		now.Add(time.Hour), now.Add(2*time.Hour))

	pool, err := x509.SystemCertPool()
	require.NoError(t, err)
	pool.AddCert(trustedCA.cert)
	spec := NewMtlsAuthn(pool)

	for _, tt := range []struct {
		name           string
		tlsState       *tls.ConnectionState
		expectedStatus int
		expectServed   bool
	}{
		{
			name:           "no TLS — req.TLS is nil — 401 Unauthorized",
			tlsState:       nil,
			expectedStatus: http.StatusUnauthorized,
			expectServed:   true,
		},
		{
			name:           "TLS but no peer certificates — 403 Forbidden",
			tlsState:       &tls.ConnectionState{},
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:         "valid cert signed by trusted CA — allowed, stateBag populated",
			tlsState:     validLeafTLS,
			expectServed: false,
		},
		{
			name:           "cert signed by untrusted CA — 403 Forbidden",
			tlsState:       untrustedLeafTLS,
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "expired cert — 403 Forbidden",
			tlsState:       expiredLeafTLS,
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
		{
			name:           "future cert (NotBefore in the future) — 403 Forbidden",
			tlsState:       futureLeafTLS,
			expectedStatus: http.StatusForbidden,
			expectServed:   true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := spec.CreateFilter([]any{})
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
			require.NoError(t, err)
			req.TLS = tt.tlsState

			ctx := &filtertest.Context{
				FRequest:  req,
				FStateBag: map[string]any{},
			}
			f.Request(ctx)

			assert.Equal(t, tt.expectServed, ctx.FServed)
			if tt.expectServed {
				require.NotNil(t, ctx.FResponse)
				assert.Equal(t, tt.expectedStatus, ctx.FResponse.StatusCode)
			}
		})
	}
}

func buildConnStateForBenchmark(issuerName pkix.Name) (*tls.ConnectionState, *x509.Certificate) {
	now := time.Now()

	// Generate CA key and self-signed CA cert with the desired Subject.
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               issuerName,
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caDER)

	// Generate leaf key and cert signed by the CA.
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "leaf"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	leafDER, _ := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	leafCert, _ := x509.ParseCertificate(leafDER)

	return &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leafCert}}, leafCert
}

func newCABundleForBench(cn string) *caBundle {
	now := time.Now()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(der)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return &caBundle{pem: pemBytes, cert: cert, key: key}
}

func signLeafForBench(ca *caBundle, leafCN string, notBefore, notAfter time.Time) (*tls.ConnectionState, *x509.Certificate) {
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: leafCN},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &leafKey.PublicKey, ca.key)
	cert, _ := x509.ParseCertificate(der)
	return &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}, cert
}

// BenchmarkMtlsIssuerDN             421309              2691 ns/op             624 B/op         21 allocs/op
func BenchmarkMtlsIssuerDN(b *testing.B) {
	spec := NewMtlsIssuerDN()

	tlsExactMatch, certExact := buildConnStateForBenchmark(pkix.Name{
		CommonName:   "My CA",
		Organization: []string{"Org"},
		Country:      []string{"DE"},
	})
	exactDN := certExact.Issuer.String()

	f, err := spec.CreateFilter([]any{exactDN})
	if err != nil {
		b.Fatalf("Failed to create filter: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err != nil {
		b.Fatalf("Failed to create request: %v", err)
	}

	req.TLS = tlsExactMatch
	ctx := &filtertest.Context{FRequest: req}

	for b.Loop() {
		f.Request(ctx)
		if ctx.FServed {
			b.Fatal("served but should not")
		}
	}
}

// BenchmarkMtlsIssuerCN           26978078                44.30 ns/op            0 B/op          0 allocs/op
func BenchmarkMtlsIssuerCN(b *testing.B) {
	spec := NewMtlsCN()

	tlsExactMatch, certExact := buildConnStateForBenchmark(pkix.Name{
		CommonName:   "CA",
		Organization: []string{"Org"},
		Country:      []string{"DE"},
	})
	exactCN := certExact.Issuer.CommonName

	f, err := spec.CreateFilter([]any{exactCN})
	if err != nil {
		b.Fatalf("Failed to create filter: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err != nil {
		b.Fatalf("Failed to create request: %v", err)
	}

	req.TLS = tlsExactMatch
	ctx := &filtertest.Context{FRequest: req}

	for b.Loop() {
		f.Request(ctx)
		if ctx.FServed {
			b.Fatal("served but should not")
		}
	}
}

// BenchmarkMtlsSAN/DNS_name_matches_exactly               16645082                71.22 ns/op            0 B/op          0 allocs/op
// BenchmarkMtlsSAN/IP_exact_match                         13228872                83.84 ns/op            8 B/op          1 allocs/op
// BenchmarkMtlsSAN/IP_inside_CIDR_/8_matches              13206384                84.08 ns/op            8 B/op          1 allocs/op
// BenchmarkMtlsSAN/multiple_patterns_—_first_matches      29038336                39.24 ns/op            0 B/op          0 allocs/op
// BenchmarkMtlsSAN/URI_match                               5628175                209.8 ns/op           64 B/op          2 allocs/op
func BenchmarkMtlsSAN(b *testing.B) {
	spec := NewMtlsSAN()

	for _, bb := range []struct {
		name       string
		tlsState   *tls.ConnectionState // nil → req.TLS == nil (plain HTTP)
		filterArgs []any
	}{
		{
			name:       "DNS name matches exactly",
			tlsState:   buildConnStateWithSANs(nil, []string{"example.com"}, nil, nil),
			filterArgs: []any{"example.com"},
		},
		{
			name:       "IP exact match",
			tlsState:   buildConnStateWithSANs(nil, nil, []net.IP{mustParseIP("1.2.3.4")}, nil),
			filterArgs: []any{"1.2.3.4"},
		},
		{
			name:       "IP inside CIDR /8 matches",
			tlsState:   buildConnStateWithSANs(nil, nil, []net.IP{mustParseIP("10.1.2.3")}, nil),
			filterArgs: []any{"10.0.0.0/8"},
		},
		{
			name:       "multiple patterns — first matches",
			tlsState:   buildConnStateWithSANs(nil, []string{"a.com"}, nil, nil),
			filterArgs: []any{"a.com", "b.com"},
		},
		{
			name:       "URI match",
			tlsState:   buildConnStateWithSANs(nil, nil, nil, []*url.URL{{Scheme: "spiffe", Host: "b.org", Path: "/svc"}}),
			filterArgs: []any{"spiffe://a.org/svc", "spiffe://b.org/svc"},
		},
	} {
		b.Run(bb.name, func(b *testing.B) {
			f, err := spec.CreateFilter(bb.filterArgs)
			if err != nil {
				b.Fatalf("Failed to create filter: %v", err)
			}

			req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
			if err != nil {
				b.Fatalf("Failed to create request: %v", err)
			}

			req.TLS = bb.tlsState

			ctx := &filtertest.Context{FRequest: req}

			for b.Loop() {
				f.Request(ctx)
				if ctx.FServed {
					b.Fatal("served but should not")
				}
			}
		})
	}
}

// BenchmarkMtlsAuthn                 10000            114586 ns/op            1778 B/op         39 allocs/op
func BenchmarkMtlsAuthn(b *testing.B) {
	now := time.Now()

	trustedCA := newCABundleForBench("Trusted CA")
	validLeafTLS, _ := signLeafForBench(trustedCA, "my-service", now.Add(-time.Hour), now.Add(time.Hour))

	pool, err := x509.SystemCertPool()
	if err != nil {
		b.Fatalf("Failed to get system cert pool: %v", err)
	}
	pool.AddCert(trustedCA.cert)
	spec := NewMtlsAuthn(pool)

	f, err := spec.CreateFilter([]any{})
	if err != nil {
		b.Fatalf("Failed to create filter: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err != nil {
		b.Fatalf("Failed to create request: %v", err)
	}

	req.TLS = validLeafTLS
	ctx := &filtertest.Context{
		FRequest: req,
	}

	for b.Loop() {
		f.Request(ctx)
		if ctx.FServed {
			b.Fatal("served but should not")
		}
	}
}
