package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/rcrowley/go-metrics"
)

const (
	KeyRouteLookup                = "routelookup"
	KeyRouteFailure               = "routefailure"
	KeyFilterRequest              = "filter.%s.request"
	KeyFiltersRequest             = "allfilters.request.%s"
	KeyAllFiltersRequestCombined  = "allfilters.combined.request"
	KeyProxyBackend               = "backend.%s"
	KeyProxyBackendCombined       = "all.backend"
	KeyProxyBackendHost           = "backendhost.%s"
	KeyFilterResponse             = "filter.%s.response"
	KeyFiltersResponse            = "allfilters.response.%s"
	KeyAllFiltersResponseCombined = "allfilters.combined.response"
	KeyResponse                   = "response.%d.%s.skipper.%s"
	KeyResponseCombined           = "all.response.%d.%s.skipper"
	KeyServeRoute                 = "serveroute.%s.%s.%d"
	KeyServeHost                  = "servehost.%s.%s.%d"
	Key5xxsBackend                = "all.backend.5xx"

	KeyErrorsBackend   = "errors.backend.%s"
	KeyErrorsStreaming = "errors.streaming.%s"

	statsRefreshDuration = time.Duration(5 * time.Second)

	defaultUniformReservoirSize  = 1024
	defaultExpDecayReservoirSize = 1028
	defaultExpDecayAlpha         = 0.015
)

// CodaHale is the CodaHale format backend, implements Metrics interface in DropWizard's CodaHale metrics format.
type CodaHale struct {
	reg           metrics.Registry
	createTimer   func() metrics.Timer
	createCounter func() metrics.Counter
	createGauge   func() metrics.GaugeFloat64
	options       Options
	handler       http.Handler
}

// NewCodaHale returns a new CodaHale backend of metrics.
func NewCodaHale(o Options) *CodaHale {
	o = applyCompatibilityDefaults(o)

	c := &CodaHale{}
	c.reg = metrics.NewRegistry()

	var createSample func() metrics.Sample
	if o.UseExpDecaySample {
		createSample = newExpDecaySample
	} else {
		createSample = newUniformSample
	}
	c.createTimer = func() metrics.Timer { return createTimer(createSample()) }

	c.createCounter = metrics.NewCounter
	c.createGauge = metrics.NewGaugeFloat64
	c.options = o

	if o.EnableDebugGcMetrics {
		metrics.RegisterDebugGCStats(c.reg)
		go metrics.CaptureDebugGCStats(c.reg, statsRefreshDuration)
	}

	if o.EnableRuntimeMetrics {
		metrics.RegisterRuntimeMemStats(c.reg)
		go metrics.CaptureRuntimeMemStats(c.reg, statsRefreshDuration)
	}

	return c
}

func NewVoid() *CodaHale {
	c := &CodaHale{}
	c.reg = metrics.NewRegistry()
	c.createTimer = func() metrics.Timer { return metrics.NilTimer{} }
	c.createCounter = func() metrics.Counter { return metrics.NilCounter{} }
	c.createGauge = func() metrics.GaugeFloat64 { return metrics.NilGaugeFloat64{} }
	return c
}

func (c *CodaHale) getTimer(key string) metrics.Timer {
	return c.reg.GetOrRegister(key, c.createTimer).(metrics.Timer)
}

func (c *CodaHale) updateTimer(key string, d time.Duration) {
	if t := c.getTimer(key); t != nil {
		t.Update(d)
	}
}

func (c *CodaHale) MeasureSince(key string, start time.Time) {
	c.measureSince(key, start)
}

func (c *CodaHale) getGauge(key string) metrics.GaugeFloat64 {
	return c.reg.GetOrRegister(key, c.createGauge).(metrics.GaugeFloat64)
}

func (c *CodaHale) UpdateGauge(key string, v float64) {
	if t := c.getGauge(key); t != nil {
		t.Update(v)
	}
}

func (c *CodaHale) IncCounter(key string) {
	c.incCounter(key, 1)
}

func (c *CodaHale) IncCounterBy(key string, value int64) {
	c.incCounter(key, value)
}

func (c *CodaHale) IncFloatCounterBy(key string, value float64) {
	// Dropped. CodaHale does not support float counter.
}

func (c *CodaHale) measureSince(key string, start time.Time) {
	d := time.Since(start)
	go c.updateTimer(key, d)
}

func (c *CodaHale) MeasureRouteLookup(start time.Time) {
	c.measureSince(KeyRouteLookup, start)
}

func (c *CodaHale) MeasureFilterRequest(filterName string, start time.Time) {
	c.measureSince(fmt.Sprintf(KeyFilterRequest, filterName), start)
}

func (c *CodaHale) MeasureAllFiltersRequest(routeId string, start time.Time) {
	c.measureSince(KeyAllFiltersRequestCombined, start)
	if c.options.EnableAllFiltersMetrics {
		c.measureSince(fmt.Sprintf(KeyFiltersRequest, routeId), start)
	}
}

func (c *CodaHale) MeasureBackend(routeId string, start time.Time) {
	c.measureSince(KeyProxyBackendCombined, start)
	if c.options.EnableRouteBackendMetrics {
		c.measureSince(fmt.Sprintf(KeyProxyBackend, routeId), start)
	}
}

func (c *CodaHale) MeasureBackendHost(routeBackendHost string, start time.Time) {
	if c.options.EnableBackendHostMetrics {
		c.measureSince(fmt.Sprintf(KeyProxyBackendHost, hostForKey(routeBackendHost)), start)
	}
}

func (c *CodaHale) MeasureFilterResponse(filterName string, start time.Time) {
	c.measureSince(fmt.Sprintf(KeyFilterResponse, filterName), start)
}

func (c *CodaHale) MeasureAllFiltersResponse(routeId string, start time.Time) {
	c.measureSince(KeyAllFiltersResponseCombined, start)
	if c.options.EnableAllFiltersMetrics {
		c.measureSince(fmt.Sprintf(KeyFiltersResponse, routeId), start)
	}
}

func (c *CodaHale) MeasureResponse(code int, method string, routeId string, start time.Time) {
	method = measuredMethod(method)
	if c.options.EnableCombinedResponseMetrics {
		c.measureSince(fmt.Sprintf(KeyResponseCombined, code, method), start)
	}

	if c.options.EnableRouteResponseMetrics {
		c.measureSince(fmt.Sprintf(KeyResponse, code, method, routeId), start)
	}
}

func (c *CodaHale) MeasureServe(routeId, host, method string, code int, start time.Time) {
	method = measuredMethod(method)

	if c.options.EnableServeRouteMetrics {
		c.measureSince(fmt.Sprintf(KeyServeRoute, routeId, method, code), start)
	}

	if c.options.EnableServeHostMetrics {
		c.measureSince(fmt.Sprintf(KeyServeHost, hostForKey(host), method, code), start)
	}
}

func (c *CodaHale) getCounter(key string) metrics.Counter {
	return c.reg.GetOrRegister(key, c.createCounter).(metrics.Counter)
}

func (c *CodaHale) incCounter(key string, value int64) {
	go func() {
		if c := c.getCounter(key); c != nil {
			c.Inc(value)
		}
	}()
}

func (c *CodaHale) IncRoutingFailures() {
	c.incCounter(KeyRouteFailure, 1)
}

func (c *CodaHale) IncErrorsBackend(routeId string) {
	if c.options.EnableRouteBackendErrorsCounters {
		c.incCounter(fmt.Sprintf(KeyErrorsBackend, routeId), 1)
	}
}

func (c *CodaHale) MeasureBackend5xx(t time.Time) {
	c.measureSince(Key5xxsBackend, t)
}

func (c *CodaHale) IncErrorsStreaming(routeId string) {
	if c.options.EnableRouteStreamingErrorsCounters {
		c.incCounter(fmt.Sprintf(KeyErrorsStreaming, routeId), 1)
	}
}

func (c *CodaHale) RegisterHandler(path string, handler *http.ServeMux) {
	h := c.getHandler(path)
	handler.Handle(path, h)
}

func (c *CodaHale) CreateHandler(path string) http.Handler {
	return &codaHaleMetricsHandler{path: path, registry: c.reg, options: c.options}
}

func (c *CodaHale) getHandler(path string) http.Handler {
	if c.handler != nil {
		return c.handler
	}

	c.handler = c.CreateHandler(path)
	return c.handler
}

type codaHaleMetricsHandler struct {
	path     string
	registry metrics.Registry
	profile  http.Handler
	options  Options
}

func (c *codaHaleMetricsHandler) sendMetrics(w http.ResponseWriter, p string) {
	_, k := path.Split(p)

	metrics := filterMetrics(c.registry, c.options.Prefix, k)

	if len(metrics) > 0 {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics)
	} else {
		http.NotFound(w, nil)
	}
}

// This listener is only used to expose the metrics
func (c *codaHaleMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	p := r.URL.Path
	c.sendMetrics(w, strings.TrimPrefix(p, c.path))
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

type skipperMetrics map[string]interface{}

// This listener is used to expose the collected metrics.
func (sm skipperMetrics) MarshalJSON() ([]byte, error) {
	data := make(map[string]map[string]interface{})
	for name, metric := range sm {
		values := make(map[string]interface{})
		var metricsFamily string

		switch m := metric.(type) {
		case metrics.Gauge:
			metricsFamily = "gauges"
			values["value"] = m.Value()
		case metrics.GaugeFloat64:
			t := m.Snapshot()
			metricsFamily = "gauges"
			values["value"] = t.Value()
		case metrics.Histogram:
			metricsFamily = "histograms"
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
			metricsFamily = "timers"
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
		case metrics.Counter:
			metricsFamily = "counters"
			t := m.Snapshot()
			values["count"] = t.Count()
		default:
			metricsFamily = "unknown"
			values["error"] = fmt.Sprintf("unknown metrics type %T", m)
		}
		if data[metricsFamily] == nil {
			data[metricsFamily] = make(map[string]interface{})
		}
		data[metricsFamily][name] = values
	}

	return json.Marshal(data)
}
