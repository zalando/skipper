package net

import (
	"net"
	"net/http"
)

// Sets non-standard X-Forwarded-* Headers
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers#proxies
type ForwardedHeaders struct {
	// Sets or appends request remote IP to the X-Forwarded-For header
	For bool
	// Sets or prepends request remote IP to the X-Forwarded-For header, overrides For
	PrependFor bool
	// Sets X-Forwarded-Host to the request host
	Host bool
	// Sets X-Forwarded-Proto value
	Proto string
}

func (h *ForwardedHeaders) Set(req *http.Request) {
	if (h.For || h.PrependFor) && req.RemoteAddr != "" {
		addr := req.RemoteAddr
		if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			addr = host
		}

		v := req.Header.Get("X-Forwarded-For")
		if v == "" {
			v = addr
		} else if h.PrependFor {
			v = addr + ", " + v
		} else {
			v = v + ", " + addr
		}
		req.Header.Set("X-Forwarded-For", v)
	}

	if h.Host {
		req.Header.Set("X-Forwarded-Host", req.Host)
	}

	if h.Proto != "" {
		req.Header.Set("X-Forwarded-Proto", h.Proto)
	}
}

type ForwardedHeadersHandler struct {
	Headers ForwardedHeaders
	Exclude IPNets
	Handler http.Handler
}

func (h *ForwardedHeadersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && !h.Exclude.Contain(net.ParseIP(host)) {
		h.Headers.Set(r)
	}
	h.Handler.ServeHTTP(w, r)
}
