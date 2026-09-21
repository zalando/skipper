// Package tls is implementing predicates that can match based on the
// client peer certificate. There is no validation at this point,
// please use [github.com/zalando/skipper/filters/tls] in order to
// validate certificates for authentication and authorization
// purposes.
package tls

import (
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"

	snet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"

	"go4.org/netipx"
)

type certCheckType uint

const (
	checkIssuerDN certCheckType = iota + 1
	checkIssuerCN
	checkSanDNS
	checkSanCIDR
	checkSanIP
	checkSanURI
	checkCN
)

type tlsClientPredicateSpec struct {
	typ certCheckType
}

type tlsClientPredicate struct {
	typ             certCheckType
	allowedString   map[string]struct{}
	allowedSuffixes []string
	allowedGlobs    []string
	allowedIPs      *netipx.IPSet
}

func NewTLSClientCheckIssuerDN() routing.PredicateSpec {
	return &tlsClientPredicateSpec{
		typ: checkIssuerDN,
	}
}

func NewTLSClientCheckIssuerCN() routing.PredicateSpec {
	return &tlsClientPredicateSpec{
		typ: checkIssuerCN,
	}
}

func NewTLSClientCheckCN() routing.PredicateSpec {
	return &tlsClientPredicateSpec{
		typ: checkCN,
	}
}

func NewTLSClientCheckSanDNS() routing.PredicateSpec {
	return &tlsClientPredicateSpec{
		typ: checkSanDNS,
	}
}

func NewTLSClientCheckSanCIDR() routing.PredicateSpec {
	return &tlsClientPredicateSpec{
		typ: checkSanCIDR,
	}
}

func NewTLSClientCheckSanIP() routing.PredicateSpec {
	return &tlsClientPredicateSpec{
		typ: checkSanIP,
	}
}

func NewTLSClientCheckSanURI() routing.PredicateSpec {
	return &tlsClientPredicateSpec{
		typ: checkSanURI,
	}
}

func (spec *tlsClientPredicateSpec) Name() string {
	switch spec.typ {
	case checkIssuerCN:
		return predicates.TLSClientIssuerCNName
	case checkIssuerDN:
		return predicates.TLSClientIssuerDNName
	case checkCN:
		return predicates.TLSClientCNName
	case checkSanCIDR:
		return predicates.TLSClientSanCIDRName
	case checkSanDNS:
		return predicates.TLSClientSanDNSName
	case checkSanIP:
		return predicates.TLSClientSanIPName
	case checkSanURI:
		return predicates.TLSClientSanURIName
	}

	return predicates.TLSClientIssuerDNName
}

func allowedStrings(a []any) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	for _, arg := range a {
		s, ok := arg.(string)
		if !ok || s == "" {
			return nil, predicates.ErrInvalidPredicateParameters
		}
		result[s] = struct{}{}
	}
	return result, nil
}

func (spec *tlsClientPredicateSpec) Create(args []any) (routing.Predicate, error) {
	var err error
	pred := &tlsClientPredicate{typ: spec.typ}
	switch spec.typ {
	case checkCN:
		fallthrough
	case checkIssuerCN:
		fallthrough
	case checkIssuerDN:
		pred.allowedString, err = allowedStrings(args)
		if err != nil {
			return nil, err
		}

	case checkSanCIDR:
		var ipStrs []string
		for _, arg := range args {
			s, ok := arg.(string)
			if !ok {
				return nil, predicates.ErrInvalidPredicateParameters
			}
			if _, err := netip.ParsePrefix(s); err != nil {
				return nil, predicates.ErrInvalidPredicateParameters
			}
			ipStrs = append(ipStrs, s)
		}
		if len(ipStrs) > 0 {
			ipSet, err := snet.ParseIPCIDRs(ipStrs)
			if err != nil {
				return nil, predicates.ErrInvalidPredicateParameters
			}
			pred.allowedIPs = ipSet
		} else {
			return nil, predicates.ErrInvalidPredicateParameters
		}

	case checkSanIP:
		var ipStrs []string
		for _, arg := range args {
			s, ok := arg.(string)
			if !ok {
				return nil, predicates.ErrInvalidPredicateParameters
			}
			if _, err := netip.ParseAddr(s); err != nil {
				return nil, predicates.ErrInvalidPredicateParameters
			}
			ipStrs = append(ipStrs, s)
		}
		if len(ipStrs) > 0 {
			ipSet, err := snet.ParseIPCIDRs(ipStrs)
			if err != nil {
				return nil, predicates.ErrInvalidPredicateParameters
			}
			pred.allowedIPs = ipSet
		} else {
			return nil, predicates.ErrInvalidPredicateParameters
		}

	case checkSanDNS:
		allowedStrings, err := allowedStrings(args)
		if err != nil {
			return nil, err
		}
		pred.allowedString = make(map[string]struct{})
		pred.allowedSuffixes = make([]string, 0)
		for arg := range allowedStrings {
			if !isValidHostname(arg) {
				return nil, predicates.ErrInvalidPredicateParameters
			}
			lower := strings.ToLower(arg)
			if strings.HasPrefix(lower, "*.") {
				pred.allowedSuffixes = append(pred.allowedSuffixes, lower[2:])
			} else {
				pred.allowedString[lower] = struct{}{}
			}
		}

	case checkSanURI:
		allowedStrings, err := allowedStrings(args)
		if err != nil {
			return nil, err
		}
		pred.allowedString = make(map[string]struct{})
		pred.allowedGlobs = make([]string, 0)

		for arg := range allowedStrings {
			if u, err := url.Parse(arg); err != nil || u.Scheme == "" {
				return nil, predicates.ErrInvalidPredicateParameters
			}
			if strings.ContainsAny(arg, "*") {
				if _, err := path.Match(arg, ""); err != nil {
					return nil, predicates.ErrInvalidPredicateParameters
				}
				pred.allowedGlobs = append(pred.allowedGlobs, arg)
			} else {
				pred.allowedString[arg] = struct{}{}
			}
		}

	default:
		return nil, predicates.ErrInvalidPredicateParameters
	}

	return pred, nil
}

// matchesDNSWildcard reports whether the lowercased hostname name matches the
// wildcard pattern "*.suffix". It requires exactly one non-empty label before
// suffix, matching RFC 6125 single-label wildcard semantics.
func matchesDNSWildcard(name, suffix string) bool {
	if !strings.HasSuffix(name, "."+suffix) {
		return false
	}
	label := name[:len(name)-len(suffix)-1]
	return label != "" && !strings.Contains(label, ".")
}

// isValidHostname accepts dot-separated labels of [a-zA-Z0-9-], allowing a
// leading wildcard label ("*.example.com").
func isValidHostname(s string) bool {
	if s == "" {
		return false
	}
	// valid IP would be detected as valid hostname.
	if _, err := netip.ParseAddr(s); err == nil {
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

// Match implements [routing.Predicate].
func (p *tlsClientPredicate) Match(r *http.Request) bool {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return false
	}

	leaf := r.TLS.PeerCertificates[0]

	switch p.typ {
	case checkIssuerDN:
		_, ok := p.allowedString[leaf.Issuer.String()]
		return ok

	case checkIssuerCN:
		_, ok := p.allowedString[leaf.Issuer.CommonName]
		return ok

	case checkCN:
		_, ok := p.allowedString[leaf.Subject.CommonName]
		return ok

	case checkSanDNS:
		for _, dns := range leaf.DNSNames {
			lower := strings.ToLower(dns)
			if _, ok := p.allowedString[lower]; ok {
				return true
			}
			for _, suffix := range p.allowedSuffixes {
				if matchesDNSWildcard(lower, suffix) {
					return true
				}
			}
		}
		return false

	case checkSanCIDR, checkSanIP:
		for _, ip := range leaf.IPAddresses {
			addr, ok := netip.AddrFromSlice(ip)
			if ok && p.allowedIPs.Contains(addr.Unmap()) {
				return true
			}
		}

		return false

	case checkSanURI:
		for _, u := range leaf.URIs {
			uStr := u.String()
			if _, ok := p.allowedString[uStr]; ok {
				return true
			}
			for _, glob := range p.allowedGlobs {
				if ok, _ := path.Match(glob, uStr); ok {
					return true
				}
			}
		}

		return false
	}

	return false
}
