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

// The remote address of the client. When the 'X-Forwarded-For'
// header is set, then it is used instead.
func RemoteHost(r *http.Request) net.IP {
	ffs := r.Header.Get("X-Forwarded-For")
	ff := strings.Split(ffs, ",")[0]
	if ffh := parse(ff); ffh != nil {
		return ffh
	}

	return parse(r.RemoteAddr)
}

// The remote address of the client. When the 'X-Forwarded-For'
// header is set, then it is used instead.
func RemoteHostFromLast(r *http.Request) net.IP {
	ffs := r.Header.Get("X-Forwarded-For")
	a := strings.Split(ffs, ",")
	ff := strings.TrimLeft(a[len(a)-1], " ")
	if ffh := parse(ff); ffh != nil {
		return ffh
	}
	return parse(r.RemoteAddr)
}
