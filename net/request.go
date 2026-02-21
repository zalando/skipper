package net

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type RequestMatchHandler struct {
	Match   []string
	Handler http.Handler
}

func (h *RequestMatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.matchesRequest(r) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request\n"))
		return
	}
	h.Handler.ServeHTTP(w, r)
}

func (h *RequestMatchHandler) matchesRequest(r *http.Request) bool {
	if h.matches(r.RequestURI) {
		return true
	}
	unescapedURI, _ := url.QueryUnescape(r.RequestURI)
	if h.matches(unescapedURI) {
		return true
	}
	for name, values := range r.Header {
		if h.matches(name) {
			return true
		}
		if slices.ContainsFunc(values, h.matches) {
			return true
		}
	}
	return false
}

func (h *RequestMatchHandler) matches(value string) bool {
	for _, v := range h.Match {
		if strings.Contains(value, v) {
			return true
		}
	}
	return false
}
