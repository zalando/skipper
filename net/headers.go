package net

import (
	"net"
	"net/http"
)

// ForwardedHeaders sets non-standard X-Forwarded-* Headers
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers#proxies and https://github.com/authelia/authelia
type ForwardedHeaders struct {
	// For sets or appends request remote IP to the X-Forwarded-For header
	For bool
	// PrependFor sets or prepends request remote IP to the X-Forwarded-For header, overrides For
	PrependFor bool
	// Host sets X-Forwarded-Host to the request host
	Host bool
	// Method sets the http method as X-Forwarded-Method to the request header
	Method bool
	// Uri sets the path and query as X-Forwarded-Uri header to the request header
	Uri bool
	// Sets X-Forwarded-Port value
	Port string
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

	if h.Method {
		req.Header.Set("X-Forwarded-Method", req.Method)
	}

	if h.Uri {
		req.Header.Set("X-Forwarded-Uri", req.RequestURI)
	}

	if h.Port != "" {
		req.Header.Set("X-Forwarded-Port", h.Port)
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

type ContentLengthHeadersHandler struct {
	Max     int64
	Handler http.Handler
}

func (h *ContentLengthHeadersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Max != 0 && r.ContentLength > h.Max {
		http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return
	}

	h.Handler.ServeHTTP(w, r)
}
