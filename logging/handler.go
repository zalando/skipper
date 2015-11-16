package logging

import (
	"net/http"
	"time"
)

// The logging handler wraps the proxy handler to produce an access log compatible to Apache's
type loggingHandler struct {
	proxy http.Handler
}

func NewHandler(next http.Handler) http.Handler {
	return &loggingHandler{proxy: next}
}

func (lh *loggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	lw := &loggingWriter{writer: w}
	lh.proxy.ServeHTTP(lw, r)

	dur := time.Now().Sub(now)

	if lw.code == 0 {
		lw.code = 200
	}

	entry := &AccessEntry{
		Request:      r,
		ResponseSize: lw.bytes,
		StatusCode:   lw.code,
		RequestTime:  now,
		Duration:     dur,
	}
	Access(entry)
}
