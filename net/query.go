package net

import (
	"net/http"
	"net/url"
)

type ValidateQueryHandler struct {
	Handler http.Handler
}

func (q *ValidateQueryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
		http.Error(w, "Invalid query", http.StatusBadRequest)
		return
	}
	q.Handler.ServeHTTP(w, r)
}
