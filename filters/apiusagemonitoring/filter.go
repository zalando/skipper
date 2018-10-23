package apiusagemonitoring

import (
	"github.com/zalando/skipper/filters"
	"net/http"
	"time"
)

const (
	metricCountAll  = "http_count"
	metricCount500s = "http5xx_count"
	metricCount400s = "http4xx_count"
	metricCount300s = "http3xx_count"
	metricCount200s = "http2xx_count"
	metricCount100s = "http1xx_count"
	metricLatency   = "latency"
)

const (
	stateBagKeyPrefix = "filter." + Name + "."
	stateBagKeyState  = stateBagKeyPrefix + "state"
)

// apiUsageMonitoringFilter implements filters.Filter interface and is the structure
// created for every route invocation of the `apiUsageMonitoring` filter.
type apiUsageMonitoringFilter struct {
	Paths []*pathInfo
}

// apiUsageMonitoringFilterContext contains information about the metrics tracking
// for one HTTP exchange (one routing). It serves to pass information from
// the `Request` call to the `Response` call (stored in the context's `StateBag`).
type apiUsageMonitoringFilterContext struct {
	// Begin is the earliest point in time where the request is observed
	Begin time.Time
}

func (f *apiUsageMonitoringFilter) Request(c filters.FilterContext) {
	// Gathering information from the initial request for further metrics calculation
	c.StateBag()[stateBagKeyState] = &apiUsageMonitoringFilterContext{
		Begin: time.Now(),
	}
}

func (f *apiUsageMonitoringFilter) Response(c filters.FilterContext) {
	mfc, ok := c.StateBag()[stateBagKeyState].(*apiUsageMonitoringFilterContext)
	if !ok {
		log.Debugf("Call not tracked (key %q not found in StateBag)", stateBagKeyState)
		return
	}

	request, response, metrics := c.Request(), c.Response(), c.Metrics()
	metricsName := f.getMetricsName(request)

	// METRIC: Count
	metrics.IncCounter(metricsName.CountAll)

	// METRIC: Response Status Range Count
	classMetricsIndex := (response.StatusCode / 100) - 1
	if classMetricsIndex < 0 || classMetricsIndex >= 5 {
		log.Warnf(
			"Response HTTP Status Code %d is invalid. Response status code metric not tracked for this call.",
			response.StatusCode)
	} else {
		metrics.IncCounter(metricsName.CountPerStatusCodeRange[classMetricsIndex])
	}

	// METRIC: Latency
	metrics.MeasureSince(metricsName.Latency, mfc.Begin)

	log.Debugf("Pushed metrics prefixed by %q", metricsName.GlobalPrefix)
}

func (f *apiUsageMonitoringFilter) String() string {
	return toTypedJsonOrErr(f)
}

// getMetricsName returns the structure with names of the metrics for this specific context.
// If it is not already cached, it is generated and cached to speed up next calls.
func (f *apiUsageMonitoringFilter) getMetricsName(req *http.Request) *specificMetricsName {

	// Match the path to a known template
	var path *pathInfo = nil
	for _, p := range f.Paths {
		if p.Matcher.MatchString(req.URL.Path) {
			path = p
			break
		}
	}
	if path == nil {
		path = unknownPath
	}

	// Get the cached prefixes for this path and verb
	prefixes, ok := path.metricPrefixesPerMethod[req.Method]
	if ok {
		return prefixes
	}

	// Prefixes were not cached for this path and verb
	// Generate and cache prefixes
	globalPrefix := path.ApplicationId + "." + path.ApiId + "." + req.Method + "." + path.PathTemplate + "."
	prefixes = &specificMetricsName{
		GlobalPrefix: globalPrefix,
		CountAll:     globalPrefix + metricCountAll,
		CountPerStatusCodeRange: [5]string{
			globalPrefix + metricCount100s,
			globalPrefix + metricCount200s,
			globalPrefix + metricCount300s,
			globalPrefix + metricCount400s,
			globalPrefix + metricCount500s,
		},
		Latency: globalPrefix + metricLatency,
	}
	path.metricPrefixesPerMethod[req.Method] = prefixes
	return prefixes
}
