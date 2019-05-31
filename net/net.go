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
	ff := strings.Split(ffs, ",")[0]
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
