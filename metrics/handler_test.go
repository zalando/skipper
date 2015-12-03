package metrics

import (
	"encoding/json"
	"github.com/rcrowley/go-metrics"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBadRequests(t *testing.T) {
	o := Options{Listener: ":0", EnableRuntimeMetrics: true}
	r := metrics.NewRegistry()
	mh := &metricsHandler{r, o}

	r1, _ := http.NewRequest("GET", "/", nil)
	rw1 := httptest.NewRecorder()

	mh.ServeHTTP(rw1, r1)
	if rw1.Code != http.StatusBadRequest {
		t.Error("The root resource should not provide a valid response")
	}

	r2, _ := http.NewRequest("POST", "/metrics", nil)
	rw2 := httptest.NewRecorder()
	mh.ServeHTTP(rw2, r2)
	if rw2.Code != http.StatusBadRequest {
		t.Error("POST method should not provide a valid response")
	}
}

func TestAllMetricsRequest(t *testing.T) {
	o := Options{}
	reg := metrics.NewRegistry()
	metrics.RegisterRuntimeMemStats(reg)
	mh := &metricsHandler{reg, o}

	r, _ := http.NewRequest("GET", "/metrics", nil)
	rw := httptest.NewRecorder()
	mh.ServeHTTP(rw, r)
	if rw.Code != http.StatusOK {
		t.Error("Metrics endpoint should provide a valid response")
	}

	var data map[string]map[string]interface{}
	if err := json.Unmarshal(rw.Body.Bytes(), &data); err != nil {
		t.Error("Unable to unmarshal metrics response")
	}

	if _, ok := data["gauges"]["runtime.MemStats.NumGC"]; !ok {
		t.Error("Metrics endpoint should've returned some runtime metrics in it")
	}
}

func TestSingleMetricsRequest(t *testing.T) {
	o := Options{}
	reg := metrics.NewRegistry()
	metrics.RegisterRuntimeMemStats(reg)
	mh := &metricsHandler{reg, o}

	r, _ := http.NewRequest("GET", "/metrics/runtime.MemStats.NumGC", nil)
	rw := httptest.NewRecorder()
	mh.ServeHTTP(rw, r)
	if rw.Code != http.StatusOK {
		t.Error("Metrics endpoint should provide a valid response")
	}

	var data map[string]map[string]interface{}
	if err := json.Unmarshal(rw.Body.Bytes(), &data); err != nil {
		t.Error("Unable to unmarshal metrics response")
	}

	if len(data) != 1 {
		t.Error("Metrics endpoint for exact match should've returned exactly te requested item")
	}

	if _, ok := data["gauges"]["runtime.MemStats.NumGC"]; !ok {
		t.Error("Metrics endpoint should've returned some runtime metrics in it")
	}
}

func TestSingleMetricsRequestWhenUsingPrefix(t *testing.T) {
	o := Options{Prefix: "zmon."}
	reg := metrics.NewRegistry()
	metrics.RegisterRuntimeMemStats(reg)
	mh := &metricsHandler{reg, o}

	r, _ := http.NewRequest("GET", "/metrics/zmon.runtime.MemStats.NumGC", nil)
	rw := httptest.NewRecorder()
	mh.ServeHTTP(rw, r)
	if rw.Code != http.StatusOK {
		t.Error("Metrics endpoint should provide a valid response for exact match using prefix")
	}

	var data map[string]map[string]interface{}
	if err := json.Unmarshal(rw.Body.Bytes(), &data); err != nil {
		t.Error("Unable to unmarshal metrics response for exact match using prefix")
	}

	if len(data) != 1 {
		t.Error("Metrics endpoint for exact match using prefix should've returned exactly te requested item")
	}

	if _, ok := data["gauges"]["zmon.runtime.MemStats.NumGC"]; !ok {
		t.Error("Metrics endpoint for exact match using prefix should've returned some runtime metrics in it")
	}
}

func TestMetricsRequestWithPattern(t *testing.T) {
	o := Options{}
	reg := metrics.NewRegistry()
	metrics.RegisterRuntimeMemStats(reg)
	mh := &metricsHandler{reg, o}

	r, _ := http.NewRequest("GET", "/metrics/runtime.Num", nil)
	rw := httptest.NewRecorder()
	mh.ServeHTTP(rw, r)
	if rw.Code != http.StatusOK {
		t.Error("Metrics endpoint should provide a valid response")
	}

	var data map[string]map[string]interface{}
	if err := json.Unmarshal(rw.Body.Bytes(), &data); err != nil {
		t.Error("Unable to unmarshal metrics response")
	}

	if len(data) < 1 {
		t.Error("Metrics endpoint for prefix should've returned some runtime metrics in it")
	}

	for k, v := range data {
		if k != "gauges" {
			t.Error("Metrics should report `gauges` metrics")
		} else {
			for k2, _ := range v {
				if !strings.HasPrefix(k2, "runtime.Num") {
					t.Error("Metrics endpoint returned metrics with the wrong prefix")
				}
			}
		}
	}
}

func TestUnknownMetricRequest(t *testing.T) {
	o := Options{}
	reg := metrics.NewRegistry()
	metrics.RegisterRuntimeMemStats(reg)
	mh := &metricsHandler{reg, o}

	r, _ := http.NewRequest("GET", "/metrics/DOES-NOT-EXIST", nil)
	rw := httptest.NewRecorder()

	mh.ServeHTTP(rw, r)
	if rw.Code != http.StatusNotFound {
		t.Error("Request for unknown metrics should return a Not Found status")
	}
}
