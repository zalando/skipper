package metrics

import (
	"encoding/json"
	"github.com/golang/glog"
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

type MetricsHandler struct {
	reg   metrics.Registry
	proxy http.Handler
}

func NewHandler(next http.Handler) http.Handler {
	return &MetricsHandler{reg: metrics.DefaultRegistry, proxy: next}
}

func (m *MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "/metrics" {
		b, err := json.Marshal(m.reg)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			// not sure if should return the errors, https.StatusText(int) should be enough for the public.
			w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	}

	now := time.Now()

	h := &metricsWrapper{writer: w}
	m.proxy.ServeHTTP(h, r)

	dur := time.Now().Sub(now)

	if h.code == 0 {
		h.code = 200
	}

	glog.Infof("dump access.log with duration", dur)
}
