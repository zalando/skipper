package net

import (
	"net"
	"net/http"
	"strings"
)

// HostPatch is used to modify host[:port] string
type HostPatch struct {
	// Remove port if present
	RemovePort bool

	// Remove trailing dot if present
	RemoveTrailingDot bool

	// Convert to lowercase
	ToLower bool
}

func (h *HostPatch) Apply(original string) string {
	host, port := original, ""

	// avoid net.SplitHostPort for value without port
	if strings.IndexByte(original, ':') != -1 {
		if sh, sp, err := net.SplitHostPort(original); err == nil {
			host, port = sh, sp
		}
	}

	if h.RemovePort {
		port = ""
	}

	if h.RemoveTrailingDot {
		last := len(host) - 1
		if last >= 0 && host[last] == '.' {
			host = host[:last]
		}
	}

	if h.ToLower {
		host = strings.ToLower(host)
	}

	if port != "" {
		return net.JoinHostPort(host, port)
	} else {
		return host
	}
}

type HostPatchHandler struct {
	Patch   HostPatch
	Handler http.Handler
}

func (h *HostPatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Host = h.Patch.Apply(r.Host)
	h.Handler.ServeHTTP(w, r)
}
