package logging

import (
	"github.com/rcrowley/go-metrics"
	"net/http"
	"time"
)

// The logging handler wraps the proxy handler to produce an access log compatible to Apache's common log format
type LoggingHandler struct {
	registry metrics.Registry
	proxy    http.Handler
}

func NewHandler(next http.Handler, r metrics.Registry) http.Handler {
	return &LoggingHandler{registry: r, proxy: next}
}

func (lh *LoggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// how does this solution compare to the one where we
	// would open another listener for the metrics?
	if r.RequestURI == "/metrics" {
		w.WriteHeader(http.StatusOK)
		metrics.WriteJSONOnce(lh.registry, w)
		return
	}

	now := time.Now()

	h := &loggingWriter{writer: w}
	lh.proxy.ServeHTTP(h, r)

	dur := time.Now().Sub(now)

	if h.code == 0 {
		h.code = 200
	}

	entry := &AccessEntry{
		Request:      r,
		ResponseSize: h.bytes,
		StatusCode:   h.code,
		RequestTime:  now,
		Duration:     dur,
	}
	Access(entry)
}
