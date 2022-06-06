package net

import (
	"net"
	"net/http"
	"strings"
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

// RemoteHost returns the remote address of the client. When the
// 'X-Forwarded-For' header is set, then it is used instead. This is
// how most often proxies behave. Wikipedia shows the format
// https://en.wikipedia.org/wiki/X-Forwarded-For#Format
//
// Example:
//
//     X-Forwarded-For: client, proxy1, proxy2
func RemoteHost(r *http.Request) net.IP {
	ffs := r.Header.Get("X-Forwarded-For")
	ff, _, _ := strings.Cut(ffs, ",")
	if ffh := parse(ff); ffh != nil {
		return ffh
	}

	return parse(r.RemoteAddr)
}

// RemoteHostFromLast returns the remote address of the client. When
// the 'X-Forwarded-For' header is set, then it is used instead. This
// is known to be true for AWS Application LoadBalancer. AWS docs
// https://docs.aws.amazon.com/elasticloadbalancing/latest/classic/x-forwarded-headers.html
//
// Example:
//
//     X-Forwarded-For: ip-address-1, ip-address-2, client-ip-address
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

// A list of IPNets
type IPNets []*net.IPNet

// Check if any of IPNets contains the IP
func (nets IPNets) Contain(ip net.IP) bool {
	for _, net := range nets {
		if net.Contains(ip) {
			return true
		}
	}
	return false
}

// Parses list of CIDR addresses into a list of IPNets
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
