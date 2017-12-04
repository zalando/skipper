package metrics_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zalando/skipper/metrics"
)

func TestHandlerPrometheusBadRequests(t *testing.T) {
	o := metrics.Options{
		Format:               metrics.PrometheusKind,
		EnableRuntimeMetrics: true,
	}
	mh := metrics.NewDefaultHandler(o)

	r, _ := http.NewRequest("GET", "/", nil)
	rw := httptest.NewRecorder()

	mh.ServeHTTP(rw, r)
	if rw.Code != http.StatusBadRequest {
		t.Error("The root resource should not provide a valid response")
	}
}

func TestHandlerPrometheusMetricsRequest(t *testing.T) {
	o := metrics.Options{
		Format:               metrics.PrometheusKind,
		EnableRuntimeMetrics: true,
	}
	mh := metrics.NewDefaultHandler(o)

	r, _ := http.NewRequest("GET", "/metrics", nil)
	rw := httptest.NewRecorder()

	mh.ServeHTTP(rw, r)
	if rw.Code != http.StatusOK {
		t.Error("Metrics endpoint should provide a valid response")
	}
	b := rw.Body.Bytes()
	if len(b) == 0 {
		t.Error("Metrics endpoint should've returned some runtime metrics in it")
	}
}

func TestHandlerCodaHaleBadRequests(t *testing.T) {
	o := metrics.Options{
		Format:               metrics.CodaHaleKind,
		EnableRuntimeMetrics: true,
	}
	mh := metrics.NewDefaultHandler(o)

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

func TestHandlerCodaHaleAllMetricsRequest(t *testing.T) {
	o := metrics.Options{
		Format:               metrics.CodaHaleKind,
		EnableRuntimeMetrics: true,
	}
	m := metrics.NewCodaHale(o)
	mh := metrics.NewHandler(o, m)

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

func TestHandlerCodaHaleSingleMetricsRequest(t *testing.T) {
	o := metrics.Options{
		Format:               metrics.CodaHaleKind,
		EnableRuntimeMetrics: true,
	}
	m := metrics.NewCodaHale(o)
	mh := metrics.NewHandler(o, m)

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

func TestHandlerCodaHaleSingleMetricsRequestWhenUsingPrefix(t *testing.T) {
	o := metrics.Options{
		Format:               metrics.CodaHaleKind,
		Prefix:               "zmon.",
		EnableRuntimeMetrics: true,
	}
	m := metrics.NewCodaHale(o)
	mh := metrics.NewHandler(o, m)

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

func TestHandlerCodaHaleMetricsRequestWithPattern(t *testing.T) {
	o := metrics.Options{
		Format:               metrics.CodaHaleKind,
		EnableRuntimeMetrics: true,
	}
	m := metrics.NewCodaHale(o)
	mh := metrics.NewHandler(o, m)

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

func TestHandlerCodaHaleUnknownMetricRequest(t *testing.T) {
	o := metrics.Options{
		Format:               metrics.CodaHaleKind,
		EnableRuntimeMetrics: true,
	}
	m := metrics.NewCodaHale(o)
	mh := metrics.NewHandler(o, m)

	r, _ := http.NewRequest("GET", "/metrics/DOES-NOT-EXIST", nil)
	rw := httptest.NewRecorder()

	mh.ServeHTTP(rw, r)
	if rw.Code != http.StatusNotFound {
		t.Error("Request for unknown metrics should return a Not Found status")
	}
}
