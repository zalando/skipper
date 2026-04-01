package net

import (
	"crypto/tls"
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
	// Port sets X-Forwarded-Port value, use "auto" to detect from the listener address
	Port string
	// Proto sets X-Forwarded-Proto value, use "auto" to detect from TLS state
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

	if h.Port == "auto" {
		if addr, ok := req.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
			if _, port, err := net.SplitHostPort(addr.String()); err == nil {
				req.Header.Set("X-Forwarded-Port", port)
			}
		}
	} else if h.Port != "" {
		req.Header.Set("X-Forwarded-Port", h.Port)
	}

	if h.Proto == "auto" {
		if req.TLS != nil {
			req.Header.Set("X-Forwarded-Proto", "https")
		} else {
			req.Header.Set("X-Forwarded-Proto", "http")
		}
	} else if h.Proto != "" {
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

// ProxyProtoTLSHandler detects TLS from the destination port when PROXY protocol is used.
// Since the NLB sends different ports for HTTP (9998) and HTTPS (9999), we can infer
// the protocol from the local address port. This allows forwarded-headers with Proto=auto
// to correctly set X-Forwarded-Proto based on req.TLS.
type ProxyProtoTLSHandler struct {
	Handler http.Handler
}

func (h *ProxyProtoTLSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// If req.TLS is already set, don't override it
	if r.TLS == nil {
		// Infer TLS state from the local address port
		// Port 9999 is HTTPS, port 9998 is HTTP redirect
		if localAddr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
			if tcpAddr, ok := localAddr.(*net.TCPAddr); ok {
				// For NLB PROXY protocol:
				// 9999 = HTTPS connection
				// 9998 = HTTP redirect connection
				if tcpAddr.Port == 9999 {
					// Set a minimal TLS connection state to indicate HTTPS
					r.TLS = &tls.ConnectionState{}
				}
			}
		}
	}
	h.Handler.ServeHTTP(w, r)
}
