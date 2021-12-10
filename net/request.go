package net

import (
	"net/http"
	"net/url"
	"strings"
)

type RequestMatchHandler struct {
	Match   []string
	Handler http.Handler
}

func (h *RequestMatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.matches(r) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request\n"))
		return
	}
	h.Handler.ServeHTTP(w, r)
}

func (h *RequestMatchHandler) matches(r *http.Request) bool {
	unescapedURI, _ := url.QueryUnescape(r.RequestURI)
	for _, v := range h.Match {
		if strings.Contains(r.RequestURI, v) {
			return true
		}
		if strings.Contains(unescapedURI, v) {
			return true
		}
		for name, values := range r.Header {
			if strings.Contains(name, v) {
				return true
			}
			for _, value := range values {
				if strings.Contains(value, v) {
					return true
				}
			}
		}
	}
	return false
}
