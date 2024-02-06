package tls

import (
	"crypto/x509"
	"encoding/pem"
	"strings"

	"github.com/zalando/skipper/filters"
)

type tlsSpec struct{}
type tlsFilter struct {
	headerName string
}

// New
func New() filters.Spec {
	return &tlsSpec{}
}

func (*tlsSpec) Name() string {
	return filters.TlsName
}

func (c *tlsSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &tlsFilter{
		headerName: s,
	}, nil
}

const (
	certSeparator = ","
)

// sanitize As we pass the raw certificates, remove the useless data and make it http request compliant.
func sanitize(cert []byte) string {
	return strings.NewReplacer(
		"-----BEGIN CERTIFICATE-----", "",
		"-----END CERTIFICATE-----", "",
		"\n", "",
	).Replace(string(cert))
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
func (f *tlsFilter) Request(ctx filters.FilterContext) {
	if t := ctx.Request().TLS; t != nil {
		if len(t.PeerCertificates) > 0 {

		}
	}
}

func (f *tlsFilter) Response(ctx filters.FilterContext) {}
