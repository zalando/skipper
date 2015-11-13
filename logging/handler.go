package logging

import (
	log "github.com/Sirupsen/logrus"
	"github.com/rcrowley/go-metrics"
	"net/http"
	"time"
)

const (
	// DefaultLogFilename is the default log filename.
	DefaultLogFilename = "access.log"
	// CommonLogFormat is the common log format.
	CommonLogFormat = `{remote} ` + CommonLogEmptyValue + ` [{when}] "{method} {uri} {proto}" {status} {size}`
	// CommonLogEmptyValue is the common empty log value.
	CommonLogEmptyValue = "-"
	// CombinedLogFormat is the combined log format.
	CombinedLogFormat = CommonLogFormat + ` "{>Referer}" "{>User-Agent}"`
	// DefaultLogFormat is the default log format.
	DefaultLogFormat = CommonLogFormat
)

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

	log.Infof("dump access.log with duration %v", dur)
}
