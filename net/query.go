package net

import (
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
)

type ValidateQueryHandler struct {
	LogsEnabled bool
	Handler     http.Handler
}

func (q *ValidateQueryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {

		if q.LogsEnabled {
			log.Info("Invalid query")
		}

		http.Error(w, "Invalid query", http.StatusBadRequest)
		return
	}
	q.Handler.ServeHTTP(w, r)
}
