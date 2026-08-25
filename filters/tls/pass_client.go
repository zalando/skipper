package tls

import (
	"crypto/x509"
	"encoding/pem"
	"strings"

	"github.com/zalando/skipper/filters"
)

type tlsSpec struct{}
type tlsFilter struct{}

func New() filters.Spec {
	return &tlsSpec{}
}

func (*tlsSpec) Name() string {
	return filters.TLSName
}

func (c *tlsSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &tlsFilter{}, nil
}

const (
	certSeparator  = ","
	certHeaderName = "X-Forwarded-Tls-Client-Cert"
)

var (
	replacer = strings.NewReplacer(
		"-----BEGIN CERTIFICATE-----", "",
		"-----END CERTIFICATE-----", "",
		"\n", "",
	)
)

// sanitize the raw certificates, remove the useless data and make it http request compliant.
func sanitize(cert []byte) string {
	return replacer.Replace(string(cert))
}

// getCertificates Build a string with the client certificates.
func getCertificates(certs []*x509.Certificate) string {
	var headerValues []string

	for _, peerCert := range certs {
		headerValues = append(headerValues, extractCertificate(peerCert))
	}

	return strings.Join(headerValues, certSeparator)
}

// extractCertificate extract the certificate from the request.
func extractCertificate(cert *x509.Certificate) string {
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if certPEM == nil {
		return ""
	}

	return sanitize(certPEM)
}

// Request passes cert information via X-Forwarded-Tls-Client-Cert header to the backend.
// Largely inspired by traefik, see also https://github.com/traefik/traefik/blob/6c19a9cb8fb9e41a274bf712580df3712b69dc3e/pkg/middlewares/passtlsclientcert/pass_tls_client_cert.go#L146
func (f *tlsFilter) Request(ctx filters.FilterContext) {
	if t := ctx.Request().TLS; t != nil {
		if len(t.PeerCertificates) > 0 {
			ctx.Request().Header.Set(certHeaderName, getCertificates(ctx.Request().TLS.PeerCertificates))
		}
	}
}

func (f *tlsFilter) Response(ctx filters.FilterContext) {}
