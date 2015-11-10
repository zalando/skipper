package log

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"net"
	"net/http"
	"strings"
)

type accessLogFormatter int

type AccessEntry struct {
	Request    *http.Request
	Response   *http.Response
	StatusCode int
}

func remoteAddr(r *http.Request) string {
	// in case a proxy on the remote end sets the x-forwarded-for header,
	// then we may get meaningless ip addresses here. Input needed. On,
	// the other hand, Apache documentation,
	// https://httpd.apache.org/docs/1.3/logs.html#common, says that it
	// 'will be' the address of the proxy. If the proxy is the elb, or
	// something on our end, then that's not very interesting. The go
	// RemoteAddr field will be the proxy.

	// can this contain multiple values? what's the meaning of it then?
	// each proxy appends its own? no way to decide on the order.
	ff := r.Header.Get("X-Forwarded-For")
	if ff != "" {
		return ff
	}

	return r.RemoteAddr
}

// strip port from addresses with hostname, ipv4 or ipv6
func stripPort(address string) string {
	ip := net.ParseIP(address)
	if ip != nil {
		// ipv4 or ipv6 without a port
		return address
	}

	lastColon := strings.LastIndex(address, ":")
	if lastColon < 0 {
		// hostname without port
		return address
	}

	return address[:lastColon]
}

func remoteHost(r *http.Request) string {
	a := remoteAddr(r)
	h := stripPort(a)
	if h != "" {
		return h
	}

	return "-"
}

func requestUser(r *http.Request) string {
	u, _, _ := r.BasicAuth()
	if u != "" {
		return u
	}

	return "-"
}

func (f *accessLogFormatter) Format(e *logrus.Entry) ([]byte, error) {
	host := "-"
	user := "-"

	req, _ := e.Data["request"].(*http.Request)

	if req != nil {
		host = remoteHost(req)
		user = requestUser(req)
	}

	return []byte(fmt.Sprintf("%s %s", host, user)), nil
}
