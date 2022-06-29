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
	if err := validateQuery(r.URL.RawQuery); err != nil {
		http.Error(w, "Invalid query", http.StatusBadRequest)
		return
	}
	q.Handler.ServeHTTP(w, r)
}

type ValidateQueryLogHandler struct {
	Handler http.Handler
}

func (q *ValidateQueryLogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := validateQuery(r.URL.RawQuery); err != nil {
		log.Infof("Invalid query: %s -> %s %s %s", r.RemoteAddr, r.Host, r.URL.Path, r.Method)
	}
	q.Handler.ServeHTTP(w, r)
}

func validateQuery(s string) error {
	_, err := url.ParseQuery(s)
	return err
}
