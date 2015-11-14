package metrics

import (
	"fmt"
	"github.com/rcrowley/go-metrics"
	"github.com/zalando/skipper/logging"
	"net/http"
	"time"
)

type metricsHandler int

type Options struct {
	Listener             string
	Prefix               string
	EnableDebugGcMetrics bool
	EnableRuntimeMetrics bool
}

var (
	reg metrics.Registry

	KeyRouting         = "skipper.routing"
	KeyFilterRequest   = "skipper.filter.%s.request"
	KeyFiltersRequest  = "skipper.filters.request.%s"
	KeyProxyBackend    = "skipper.backend.%s"
	KeyFilterResponse  = "skipper.filter.%s.response"
	KeyFiltersResponse = "skipper.filters.response.%s"
	KeyResponse        = "response.%d.%s.skipper.%s"
)

func Init(o Options) {
	if o.Listener == "" {
		logging.ApplicationLog().Infoln("Metrics are disabled")
		return
	}

	if o.Prefix != "" {
		KeyRouting = o.Prefix + KeyRouting
		KeyFilterRequest = o.Prefix + KeyFilterRequest
		KeyFiltersRequest = o.Prefix + KeyFiltersRequest
		KeyProxyBackend = o.Prefix + KeyProxyBackend
		KeyFilterResponse = o.Prefix + KeyFilterResponse
		KeyFiltersResponse = o.Prefix + KeyFiltersResponse
		KeyResponse = o.Prefix + KeyResponse
	}

	reg = metrics.NewRegistry()

	if o.EnableDebugGcMetrics {
		metrics.RegisterDebugGCStats(reg)
		go metrics.CaptureDebugGCStats(reg, 5e9)
	}

	if o.EnableRuntimeMetrics {
		metrics.RegisterRuntimeMemStats(reg)
		go metrics.CaptureRuntimeMemStats(reg, 5e9)
	}

	metrics.NewRegisteredTimer(KeyRouting, reg)

	handler := new(metricsHandler)
	logging.ApplicationLog().Infof("metrics listener on %s/metrics", o.Listener)
	go http.ListenAndServe(o.Listener, handler)
}

func getTimer(key string) metrics.Timer {
	if reg == nil {
		return nil
	}
	return reg.GetOrRegister(key, metrics.NewTimer()).(metrics.Timer)
}

func measureSince(key string, start time.Time) {
	if t := getTimer(key); t != nil {
		t.UpdateSince(start)
	}
}

func measure(key string, f func()) {
	if t := getTimer(key); t != nil {
		t.Time(f)
	} else {
		f()
	}
}

func MeasureRouting(start time.Time) {
	measureSince(KeyRouting, start)
}

func MeasureFilterRequest(filterName string, f func()) {
	measure(fmt.Sprintf(KeyFilterRequest, filterName), f)
}

func MeasureAllFiltersRequest(routeId string, f func()) {
	measure(fmt.Sprintf(KeyFiltersRequest, routeId), f)
}

func MeasureBackend(routeId string, start time.Time) {
	measureSince(fmt.Sprintf(KeyProxyBackend, routeId), start)
}

func MeasureFilterResponse(filterName string, f func()) {
	measure(fmt.Sprintf(KeyFilterResponse, filterName), f)
}

func MeasureAllFiltersResponse(routeId string, f func()) {
	measure(fmt.Sprintf(KeyFiltersResponse, routeId), f)
}

func MeasureResponse(code int, method string, routeId string, f func()) {
	measure(fmt.Sprintf(KeyResponse, code, method, routeId), f)
}

// This listener is ony used to expose the metrics
// Maybe it could be used to serve different groups of metrics or specific keys like a proper REST api
func (mh *metricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && r.URL.RequestURI() == "/metrics" {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		metrics.WriteJSONOnce(reg, w)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Header().Set("Content-Type", "application/problem+json")
	fmt.Fprint(w, `{"title":"Metrics Error", "detail": "Invalid request. Please send a GET request to /metrics"}`)
}
