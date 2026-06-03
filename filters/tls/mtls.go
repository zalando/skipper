package tls

import (
	"crypto/x509"
	"net/http"
	"net/netip"
	"strings"
	"sync"

	"go4.org/netipx"

	"github.com/sirupsen/logrus"
	snet "github.com/zalando/skipper/net"

	"github.com/zalando/skipper/filters"
)

type mtlsType uint

const (
	mtlsIssuerDN mtlsType = iota + 1
	mtlsSAN
	mtlsCN
	mtlsAuthn
)

func (mt mtlsType) String() string {
	switch mt {
	case mtlsIssuerDN:
		return filters.MtlsIssuerDN
	case mtlsSAN:
		return filters.MtlsSAN
	case mtlsCN:
		return filters.MtlsCN
	case mtlsAuthn:
		return filters.MtlsAuthn
	default:
		return "unknown"
	}
}

func (mt mtlsType) rejectReason() rejectReason {
	switch mt {
	case mtlsIssuerDN:
		return wrongDN
	case mtlsSAN:
		return wrongSAN
	case mtlsCN:
		return wrongCN
	case mtlsAuthn:
		return wrongAuth
	default:
		return unknown
	}
}

type auditBuffer interface {
	WriteString(string) (int, error)
	String() string
}

type voidAuditBuffer struct{}

func (voidAuditBuffer) WriteString(string) (int, error) { return 0, nil }
func (voidAuditBuffer) String() string                  { return "" }

type mtlsSpec struct {
	typ            mtlsType
	enableAuditLog bool
	caPool         *x509.CertPool
}

type mtlsFilter struct {
	typ mtlsType

	enableAuditLog bool

	// mtlsCN allow-list
	allowedCN sync.Map // string -> struct{}

	// mtlsIssuerDN allow-list (RFC 2253 DN strings)
	allowedDN sync.Map // string -> struct{}

	// mtlsSAN allow-lists: hostnames and IP/CIDR ranges are stored separately
	// so the hot path can use a single IPSet.Contains call instead of
	// re-parsing every pattern on each request.
	allowedHostnames sync.Map // string -> struct{}
	allowedIPs       *netipx.IPSet

	// mtlsAuthn: verfiy options created at filter-creation time.
	verifyOpt x509.VerifyOptions
}

func NewMtlsCN() filters.Spec {
	return &mtlsSpec{
		typ: mtlsCN,
	}
}

// NewMtlsAuthn returns a filter spec for the mtlsAuthn filter. It
// takes no argument and loads the system CA certificates used to
// verify the client certificate chain.
func NewMtlsAuthn() filters.Spec {
	pool, _ := x509.SystemCertPool()
	return &mtlsSpec{
		typ:    mtlsAuthn,
		caPool: pool,
	}
}

// NewMtlsIssuerDN returns a filter spec for the mtlsIssuerDN filter. Each
// argument must be a non-empty RFC 2253 Distinguished Name string. The filter
// allows a request when the certificate's full issuer DN matches any of the
// configured values exactly (e.g. "CN=My CA,O=My Org,C=DE").
func NewMtlsIssuerDN() filters.Spec {
	return &mtlsSpec{typ: mtlsIssuerDN}
}

// NewMtlsSAN returns a filter spec for the mtlsSAN filter. Each argument must
// be a valid IP address, CIDR block, or hostname. IP addresses and CIDRs are
// combined into a single netipx.IPSet at filter-creation time so that matching
// in the hot path is O(log n) rather than O(n).
func NewMtlsSAN() filters.Spec {
	return &mtlsSpec{typ: mtlsSAN}
}

// isValidSAN returns true if s is a valid CIDR, IPv4/IPv6 address, or hostname.
func isValidSAN(s string) bool {
	if s == "" {
		return false
	}
	if _, err := netip.ParsePrefix(s); err == nil {
		return true
	}
	if _, err := netip.ParseAddr(s); err == nil {
		return true
	}
	return isValidHostname(s)
}

// isValidHostname accepts dot-separated labels of [a-zA-Z0-9-], allowing a
// leading wildcard label ("*.example.com").
func isValidHostname(s string) bool {
	if s == "" {
		return false
	}
	for i, label := range strings.Split(s, ".") {
		if label == "" {
			return false
		}
		if i == 0 && label == "*" {
			continue
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}

func (ms *mtlsSpec) Name() string {
	return ms.typ.String()
}

func (ms *mtlsSpec) CreateFilter(args []any) (filters.Filter, error) {
	mf := &mtlsFilter{
		typ:            ms.typ,
		enableAuditLog: ms.enableAuditLog,
	}

	switch ms.typ {
	case mtlsAuthn:
		if len(args) != 0 {
			return nil, filters.ErrInvalidFilterParameters
		}

		mf.verifyOpt = x509.VerifyOptions{
			Roots:     ms.caPool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}

	default:
		if len(args) == 0 {
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	switch ms.typ {
	case mtlsIssuerDN:
		for _, arg := range args {
			s, ok := arg.(string)
			if !ok || s == "" {
				return nil, filters.ErrInvalidFilterParameters
			}
			mf.allowedDN.Store(s, struct{}{})
		}
	case mtlsCN:
		for _, arg := range args {
			if s, ok := arg.(string); !ok {
				return nil, filters.ErrInvalidFilterParameters
			} else {
				mf.allowedCN.Store(s, struct{}{})
			}
		}
	case mtlsSAN:
		var ipStrs []string
		for _, arg := range args {
			s, ok := arg.(string)
			if !ok {
				return nil, filters.ErrInvalidFilterParameters
			}
			if !isValidSAN(s) {
				return nil, filters.ErrInvalidFilterParameters
			}
			// Route to the appropriate bucket: hostnames go into the slice,
			// IP addresses and CIDRs are collected for the IPSet.
			if _, err := netip.ParsePrefix(s); err == nil {
				ipStrs = append(ipStrs, s)
			} else if _, err := netip.ParseAddr(s); err == nil {
				ipStrs = append(ipStrs, s)
			} else {
				mf.allowedHostnames.Store(strings.ToLower(s), struct{}{})
			}
		}
		if len(ipStrs) > 0 {
			ipSet, err := snet.ParseIPCIDRs(ipStrs)
			if err != nil {
				return nil, filters.ErrInvalidFilterParameters
			}
			mf.allowedIPs = ipSet
		}
	}

	return mf, nil
}

type rejectReason string

const (
	missingTLS rejectReason = "missing-tls"
	wrongCN    rejectReason = "wrong-cn"
	wrongSAN   rejectReason = "wrong-san"
	wrongDN    rejectReason = "wrong-dn"
	wrongAuth  rejectReason = "wrong-auth"
	unknown    rejectReason = "unknown"
)

func reject(
	ctx filters.FilterContext,
	status int,
	certInput string,
	reason rejectReason,
	hostname string,
) {
	ctx.Logger().Debugf(
		"Rejected: status: %d, checked cert data: %s, reason: %s.",
		status, certInput, reason,
	)

	rsp := &http.Response{
		StatusCode: status,
		Header:     make(map[string][]string),
	}

	if hostname != "" {
		// https://www.w3.org/Protocols/rfc2616/rfc2616-sec10.html#sec10.4.2
		rsp.Header.Add("WWW-Authenticate", hostname)
	}

	ctx.Serve(rsp)
}

func (mf *mtlsFilter) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	if req.TLS == nil {
		reject(ctx, http.StatusUnauthorized, "no tls", missingTLS, req.Host)
		return
	}

	var auditCertData auditBuffer
	if mf.enableAuditLog {
		auditCertData = &strings.Builder{}
	} else {
		auditCertData = voidAuditBuffer{}
	}

	peerCerts := req.TLS.PeerCertificates
	allowed := false
	for _, cert := range peerCerts {
		switch mf.typ {
		case mtlsIssuerDN:
			issuerDN := cert.Issuer.String()
			if _, ok := mf.allowedDN.Load(issuerDN); ok {
				allowed = true
				auditCertData.WriteString("DN: ")
				auditCertData.WriteString(issuerDN)
			}

		case mtlsSAN:
			// Check IP/CIDR SANs via the pre-built IPSet.
			if mf.allowedIPs != nil {
				for _, ip := range cert.IPAddresses {
					addr, ok := netip.AddrFromSlice(ip)
					if ok && mf.allowedIPs.Contains(addr.Unmap()) {
						allowed = true
						auditCertData.WriteString("SAN IP: ")
						auditCertData.WriteString(ip.String())
					}
				}
			}
			// Check hostname SANs against the allowlist.
			for _, dns := range cert.DNSNames {
				if _, ok := mf.allowedHostnames.Load(strings.ToLower(dns)); ok {
					allowed = true
					auditCertData.WriteString("SAN DNS: ")
					auditCertData.WriteString(dns)
				}
			}

		case mtlsCN:
			if _, ok := mf.allowedCN.Load(cert.Issuer.CommonName); ok {
				allowed = true
				auditCertData.WriteString("CN: ")
				auditCertData.WriteString(cert.Issuer.CommonName)
			}

		case mtlsAuthn:
			if _, err := cert.Verify(mf.verifyOpt); err != nil {
				break
			}
			allowed = true
			auditCertData.WriteString("authn: ")
			auditCertData.WriteString(cert.Subject.String())
		}
	}

	if !allowed {
		reject(ctx, http.StatusForbidden, "", mf.typ.rejectReason(), req.Host)
	}
	if mf.enableAuditLog {
		logrus.Infof("access allowed by %s: %s", mf.typ.String(), auditCertData.String())
	}
}

func (*mtlsFilter) Response(filters.FilterContext) {}
