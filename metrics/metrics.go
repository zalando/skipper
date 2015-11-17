/*
Package metrics implements collection of common performance metrics.

It uses the Go implementation of the Coda Hale metrics library:

https://github.com/dropwizard/metrics

The collected metrics include the total request processing time, the
time of looking up routes, the time spent with processing all filters
and every single filter, the time waiting for the response from the
backend services, and the time spent with forwarding the response to the
client.

For the keys used for the different metrics, please, see the Key*
variables.

To enable metrics, it needs to be initialized with a Listener address.
In this case, Skipper will start an additional http listener, where the
current metrics values can be downloaded.
*/
package metrics

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/rcrowley/go-metrics"
	"net/http"
	"time"
)

type metricsHandler int

// Options for initializing metrics collection.
type Options struct {
	// Network address where the current metrics values
	// can be pulled from. If not set, the collection of
	// the metrics is disabled.
	Listener string

	// Common prefix for the keys of the different
	// collected metrics.
	Prefix string

	// If set, garbage collector metrics are collected
	// in addition to the http traffic metrics.
	EnableDebugGcMetrics bool

	// If set, Go runtime metrics are collected in
	// addition to the http http traffic metrics.
	EnableRuntimeMetrics bool
}

var (
	reg metrics.Registry

	KeyRouteLookup     = "skipper.route.lookup"
	KeyFilterRequest   = "skipper.filter.%s.request"
	KeyFiltersRequest  = "skipper.filters.request.%s"
	KeyProxyBackend    = "skipper.backend.%s"
	KeyFilterResponse  = "skipper.filter.%s.response"
	KeyFiltersResponse = "skipper.filters.response.%s"
	KeyResponse        = "response.%d.%s.skipper.%s"
)

// Initializes the collection of metrics.
func Init(o Options) {
	if o.Listener == "" {
		log.Infoln("Metrics are disabled")
		return
	}

	if o.Prefix != "" {
		KeyRouteLookup = o.Prefix + KeyRouteLookup
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

	metrics.NewRegisteredTimer(KeyRouteLookup, reg)

	handler := new(metricsHandler)
	log.Infof("metrics listener on %s/metrics", o.Listener)
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

func MeasureRouteLookup(start time.Time) {
	measureSince(KeyRouteLookup, start)
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

// This listener is used to expose the collected metrics.
func (mh *metricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO:
	// maybe it could be used to serve different groups of metrics or specific keys like a proper REST api
	// (+1)
	if r.Method == "GET" && r.URL.RequestURI() == "/metrics" {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		metrics.WriteJSONOnce(reg, w)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Header().Set("Content-Type", "application/problem+json")
	fmt.Fprint(w,
		`{"title":"Metrics Error", "detail": "Invalid request. Please send a GET request to /metrics"}`)
}
