package net

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"go4.org/netipx"
)

// strip port from addresses with hostname, ipv4 or ipv6
func stripPort(address string) string {
	if h, _, err := net.SplitHostPort(address); err == nil {
		return h
	}

	return address
}

func parse(addr string) net.IP {
	if addr != "" {
		res := net.ParseIP(stripPort(addr))
		return res
	}
	return nil
}

// RemoteAddr returns the remote address of the client. When the
// 'X-Forwarded-For' header is set, then it is used instead. This is
// how most often proxies behave. Wikipedia shows the format
// https://en.wikipedia.org/wiki/X-Forwarded-For#Format
//
// Example:
//
//	X-Forwarded-For: client, proxy1, proxy2
func RemoteAddr(r *http.Request) netip.Addr {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		s, _, _ := strings.Cut(xff, ",")
		if addr, err := netip.ParseAddr(stripPort(s)); err == nil {
			return addr
		}
	}
	addr, _ := netip.ParseAddr(stripPort(r.RemoteAddr))
	return addr
}

// RemoteHost is *deprecated* use RemoteAddr
func RemoteHost(r *http.Request) net.IP {
	ffs := r.Header.Get("X-Forwarded-For")
	ff, _, _ := strings.Cut(ffs, ",")
	if ffh := parse(ff); ffh != nil {
		return ffh
	}

	return parse(r.RemoteAddr)
}

// RemoteAddrFromLast returns the remote address of the client. When
// the 'X-Forwarded-For' header is set, then it is used instead. This
// is known to be true for AWS Application LoadBalancer. AWS docs
// https://docs.aws.amazon.com/elasticloadbalancing/latest/classic/x-forwarded-headers.html
//
// Example:
//
//	X-Forwarded-For: ip-address-1, ip-address-2, client-ip-address
func RemoteAddrFromLast(r *http.Request) netip.Addr {
	ffs := r.Header.Get("X-Forwarded-For")
	if ffs == "" {
		addr, _ := netip.ParseAddr(stripPort(r.RemoteAddr))
		return addr
	}

	last := ffs
	if i := strings.LastIndex(ffs, ","); i != -1 {
		last = ffs[i+1:]
	}

	addr, err := netip.ParseAddr(stripPort(strings.TrimSpace(last)))
	if err != nil {
		addr, _ := netip.ParseAddr(stripPort(r.RemoteAddr))
		return addr
	}
	return addr
}

// RemoteHostFromLast is *deprecated* use RemoteAddrFromLast instead
func RemoteHostFromLast(r *http.Request) net.IP {
	ffs := r.Header.Get("X-Forwarded-For")
	ff := ffs
	if i := strings.LastIndex(ffs, ","); i != -1 {
		ff = ffs[i+1:]
	}
	if ff != "" {
		if ip := parse(strings.TrimSpace(ff)); ip != nil {
			return ip
		}
	}

	return parse(r.RemoteAddr)
}

// IPNets is *deprecated* use netipx.IPSet instead
type IPNets []*net.IPNet

// Contain is *deprecated* use netipx.IPSet.Contains() instead
func (nets IPNets) Contain(ip net.IP) bool {
	for _, net := range nets {
		if net.Contains(ip) {
			return true
		}
	}
	return false
}

// ParseCIDRs is *deprecated* use ParseIPCIDRs.
func ParseCIDRs(cidrs []string) (nets IPNets, err error) {
	for _, cidr := range cidrs {
		if !strings.Contains(cidr, "/") {
			cidr += "/32"
		}
		_, net, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		nets = append(nets, net)
	}
	return nets, nil
}

// ParseIPCIDRs returns a valid IPSet when there is no parsing
// error.
func ParseIPCIDRs(cidrs []string) (*netipx.IPSet, error) {
	var b netipx.IPSetBuilder

	for _, w := range cidrs {
		if strings.Contains(w, "/") {
			pref, err := netip.ParsePrefix(w)
			if err != nil {
				return nil, err
			}
			b.AddPrefix(pref)

		} else if addr, err := netip.ParseAddr(w); err != nil {
			return nil, err
		} else if addr.IsUnspecified() {
			return nil, fmt.Errorf("failed to parse cidr: addr is unspecified: %s", w)
		} else {
			b.Add(addr)
		}
	}

	ips, err := b.IPSet()
	if err != nil {
		return nil, err
	}

	return ips, nil
}

// SchemeHost parses URI string (without #fragment part) and returns schema used in this URI as first return value and
// host[:port] part as second return value. Port is never omitted for HTTP(S): if no port is specified in URI, default port for given
// schema is used. If URI is invalid, error is returned.
func SchemeHost(input string) (string, string, error) {
	u, err := url.ParseRequestURI(input)
	if err != nil {
		return "", "", err
	}
	if u.Scheme == "" {
		return "", "", fmt.Errorf(`parse %q: missing scheme`, input)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf(`parse %q: missing host`, input)
	}

	// endpoint address cannot contain path, the rest is not case-sensitive
	s, h := strings.ToLower(u.Scheme), strings.ToLower(u.Host)

	hh, p, err := net.SplitHostPort(h)
	if err != nil {
		if strings.Contains(err.Error(), "missing port") {
			// Trim is needed to remove brackets from IPv6 addresses, JoinHostPort will add them in case of any IPv6 address,
			// so we need to remove them to avoid duplicate pairs of brackets.
			h = strings.Trim(h, "[]")
			switch s {
			case "http":
				p = "80"
			case "https":
				p = "443"
			default:
				p = ""
			}
		} else {
			return "", "", err
		}
	} else {
		h = hh
	}

	if p != "" {
		h = net.JoinHostPort(h, p)
	}
	return s, h, nil
}
