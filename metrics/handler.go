package metrics

import (
	"encoding/json"
	"github.com/rcrowley/go-metrics"
	"net/http"
	"path"
	"strings"
)

type metricsHandler struct {
	registry metrics.Registry
	options  Options
}

func filterMetrics(reg metrics.Registry, prefix, key string) skipperMetrics {
	metrics := make(skipperMetrics)

	canonicalKey := strings.TrimPrefix(key, prefix)
	m := reg.Get(canonicalKey)
	if m != nil {
		metrics[key] = m
	} else {
		reg.Each(func(name string, i interface{}) {
			if key == "" || (strings.HasPrefix(name, canonicalKey)) {
				metrics[prefix+name] = i
			}
		})
	}

	return metrics
}

func (mh *metricsHandler) sendMetrics(w http.ResponseWriter, p string) {
	_, k := path.Split(p)

	metrics := filterMetrics(mh.registry, mh.options.Prefix, k)

	if len(metrics) > 0 {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics)
	} else {
		http.NotFound(w, nil)
	}
}

// This listener is only used to expose the metrics
func (mh *metricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if r.Method == "GET" && (p == "/metrics" || strings.HasPrefix(p, "/metrics/")) {
		mh.sendMetrics(w, strings.TrimPrefix(p, "/metrics"))
	} else {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	}
}
