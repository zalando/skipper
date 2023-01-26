package net

import (
	"net"
	"net/http"
	"net/netip"
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

	addr, err := netip.ParseAddr(strings.TrimSpace(last))
	if err != nil {
		addr, _ := netip.ParseAddr(stripPort(r.RemoteAddr))
		return addr
	}
	return addr
}

// RemoteHostFromLast is *deprecated* use RemoteAddrFromLast instead
func RemoteHostFromLast(r *http.Request) net.IP {
	ffs := r.Header.Get("X-Forwarded-For")
	ffa := strings.Split(ffs, ",")
	ff := ffa[len(ffa)-1]
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

// ParseIPCIDRs returns a valid IPSet even in case there are parsing
// errors of some partial provided input cidrs. So recently added
// bogus values can be logged and ignored at runtime.
func ParseIPCIDRs(cidrs []string) (*netipx.IPSet, error) {
	var (
		b   netipx.IPSetBuilder
		err error
	)

	for _, w := range cidrs {
		if strings.Contains(w, "/") {
			if pref, e := netip.ParsePrefix(w); e != nil {
				err = e
			} else {
				b.AddPrefix(pref)
			}
		} else if addr, e := netip.ParseAddr(w); e != nil {
			err = e
		} else {
			b.Add(addr)
		}
	}

	ips, e := b.IPSet()
	if e != nil {
		return ips, e
	}

	return ips, err
}
