package net

import (
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
)

type ValidateQueryHandler struct {
	Handler http.Handler
}

func (q *ValidateQueryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
		log.Errorf("Invalid query: %s", r.URL.RawQuery)
		http.Error(w, "Invalid query", http.StatusBadRequest)
		return
	}
	q.Handler.ServeHTTP(w, r)
}
