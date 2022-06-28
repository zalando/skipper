package net

import (
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
)

type ValidateQueryHandler struct {
	LogsEnabled bool
	LogLevel    string
	Handler     http.Handler
}

func (q *ValidateQueryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {

		if q.LogsEnabled {
			logLevel, _ := log.ParseLevel(q.LogLevel)
			log.SetLevel(logLevel)
			log.Errorf("Invalid query: %s", err)
		}

		http.Error(w, "Invalid query", http.StatusBadRequest)
		return
	}
	q.Handler.ServeHTTP(w, r)
}
