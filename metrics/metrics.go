package metrics

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/rcrowley/go-metrics"
	"net/http"
	"time"
)

type skipperMetrics map[string]interface{}

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
	// addition to the http traffic metrics.
	EnableRuntimeMetrics bool
}

const (
	KeyRouteLookup     = "routelookup"
	KeyFilterRequest   = "filter.%s.request"
	KeyFiltersRequest  = "allfilters.request.%s"
	KeyProxyBackend    = "backend.%s"
	KeyFilterResponse  = "filter.%s.response"
	KeyFiltersResponse = "allfilters.response.%s"
	KeyResponse        = "response.%d.%s.skipper.%s"

	statsRefreshDuration = time.Duration(5 * time.Second)
)

var reg metrics.Registry

// Initializes the collection of metrics.
func Init(o Options) {
	if o.Listener == "" {
		log.Infoln("Metrics are disabled")
		return
	}

	r := metrics.NewRegistry()
	if o.EnableDebugGcMetrics {
		metrics.RegisterDebugGCStats(r)
		go metrics.CaptureDebugGCStats(r, statsRefreshDuration)
	}

	if o.EnableRuntimeMetrics {
		metrics.RegisterRuntimeMemStats(r)
		go metrics.CaptureRuntimeMemStats(r, statsRefreshDuration)
	}

	handler := &metricsHandler{registry: r, options: o}
	log.Infof("metrics listener on %s/metrics", o.Listener)
	go http.ListenAndServe(o.Listener, handler)
	reg = r
}

func getTimer(key string) metrics.Timer {
	if reg == nil {
		return nil
	}
	return reg.GetOrRegister(key, metrics.NewTimer()).(metrics.Timer)
}

func measureSince(key string, start time.Time) {
	d := time.Since(start)
	go func() {
		if t := getTimer(key); t != nil {
			t.Update(d)
		}
	}()
}

func MeasureRouteLookup(start time.Time) {
	measureSince(KeyRouteLookup, start)
}

func MeasureFilterRequest(filterName string, start time.Time) {
	measureSince(fmt.Sprintf(KeyFilterRequest, filterName), start)
}

func MeasureAllFiltersRequest(routeId string, start time.Time) {
	measureSince(fmt.Sprintf(KeyFiltersRequest, routeId), start)
}

func MeasureBackend(routeId string, start time.Time) {
	measureSince(fmt.Sprintf(KeyProxyBackend, routeId), start)
}

func MeasureFilterResponse(filterName string, start time.Time) {
	measureSince(fmt.Sprintf(KeyFilterResponse, filterName), start)
}

func MeasureAllFiltersResponse(routeId string, start time.Time) {
	measureSince(fmt.Sprintf(KeyFiltersResponse, routeId), start)
}

func MeasureResponse(code int, method string, routeId string, start time.Time) {
	measureSince(fmt.Sprintf(KeyResponse, code, method, routeId), start)
}

// This listener is used to expose the collected metrics.
func (sm skipperMetrics) MarshalJSON() ([]byte, error) {
	data := make(map[string]map[string]interface{})
	for name, metric := range sm {
		values := make(map[string]interface{})
		switch m := metric.(type) {
		case metrics.Gauge:
			values["value"] = m.Value()
		case metrics.Histogram:
			h := m.Snapshot()
			ps := h.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
			values["count"] = h.Count()
			values["min"] = h.Min()
			values["max"] = h.Max()
			values["mean"] = h.Mean()
			values["stddev"] = h.StdDev()
			values["median"] = ps[0]
			values["75%"] = ps[1]
			values["95%"] = ps[2]
			values["99%"] = ps[3]
			values["99.9%"] = ps[4]
		case metrics.Timer:
			t := m.Snapshot()
			ps := t.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
			values["count"] = t.Count()
			values["min"] = t.Min()
			values["max"] = t.Max()
			values["mean"] = t.Mean()
			values["stddev"] = t.StdDev()
			values["median"] = ps[0]
			values["75%"] = ps[1]
			values["95%"] = ps[2]
			values["99%"] = ps[3]
			values["99.9%"] = ps[4]
			values["1m.rate"] = t.Rate1()
			values["5m.rate"] = t.Rate5()
			values["15m.rate"] = t.Rate15()
			values["mean.rate"] = t.RateMean()
		default:
			values["error"] = fmt.Sprintf("unknown metrics type %T", m)
		}

		data[name] = values
	}

	return json.Marshal(data)
}
