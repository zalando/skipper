package apiusagemonitoring

import (
	"encoding/json"
	"fmt"
	"github.com/zalando/skipper/filters"
	"net/http"
	"time"
)

const (
	metricCountAll   = "http_count"
	metricCount500s  = "http5xx_count"
	metricCount400s  = "http4xx_count"
	metricCount300s  = "http3xx_count"
	metricCount200s  = "http2xx_count"
	metricCount100s  = "http1xx_count"
	metricLatency    = "latency"
	metricLatencySum = "latency_sum"
)

const (
	stateBagKeyPrefix = "filter." + Name + "."
	stateBagKeyBegin  = stateBagKeyPrefix + "begin"
)

// apiUsageMonitoringFilter implements filters.Filter interface and is the structure
// created for every route invocation of the `apiUsageMonitoring` filter.
type apiUsageMonitoringFilter struct {
	Spec  *apiUsageMonitoringSpec
	Paths []*pathInfo
}

func (f *apiUsageMonitoringFilter) Request(c filters.FilterContext) {
	// Gathering information from the initial request for further metrics calculation
	c.StateBag()[stateBagKeyBegin] = time.Now()
}

func (f *apiUsageMonitoringFilter) Response(c filters.FilterContext) {
	request, response, metrics := c.Request(), c.Response(), c.Metrics()
	path, metricsName := f.resolvePath(request)

	// METRIC: Count
	metrics.IncCounter(metricsName.CountAll)

	// METRIC: Response Status Range Count
	classMetricsIndex := (response.StatusCode / 100) - 1
	if classMetricsIndex < 0 || classMetricsIndex >= 5 {
		log.Errorf(
			"Response HTTP Status Code %d is invalid. Response status code metric not tracked for this call.",
			response.StatusCode)
	} else {
		metrics.IncCounter(metricsName.CountPerStatusCodeRange[classMetricsIndex])
	}

	// METRIC: Latency
	if begin, ok := c.StateBag()[stateBagKeyBegin].(time.Time); ok {
		metrics.MeasureSince(metricsName.Latency, begin)
	}

	f.trackClientMetrics(c, path)

	log.Debugf("Pushed metrics prefixed by %q", metricsName.GlobalPrefix)
}

func (f *apiUsageMonitoringFilter) trackClientMetrics(c filters.FilterContext, path *pathInfo) {
	//if path.ClientTracking == nil {
	//	log.Debug("No ClientTracking")
	//	return
	//}
	//
	//jwt := parseJwtBody(c.Request())
	//if jwt == nil {
	//	log.Debug("JWT body not parsed")
	//	return
	//}
	//
	//realm, ok := jwt[path.ClientTracking.RealmKey]
	//if !ok {
	//	log.Debugf("JWT does not has a %q value for realm", path.ClientTracking.RealmKey)
	//	return
	//}
	//
	//clientId, ok := jwt[path.ClientTracking.ClientIdKey]
	//if !ok {
	//	log.Debugf("JWT does not has a %q value for client ID", path.ClientTracking.ClientIdKey)
	//	return
	//}
	// TODO
}

// String returns a JSON representation of the filter prefixed by its type.
func (f *apiUsageMonitoringFilter) String() string {
	var js string
	if jsBytes, err := json.Marshal(f); err == nil {
		js = string(jsBytes)
	} else {
		js = fmt.Sprintf("<%v>", err)
	}
	return fmt.Sprintf("%T %s", f, js)
}

// resolvePath returns the structure with names of the metrics for this specific context.
// If it is not already cached, it is generated and cached to speed up next calls.
func (f *apiUsageMonitoringFilter) resolvePath(req *http.Request) (*pathInfo, *metricNames) {

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
	method := req.Method
	methodIndex, ok := methodToIndex[method]
	if !ok {
		methodIndex = MethodIndexUnknown
		method = unknownElementPlaceholder
	}

	prefixes := path.metricPrefixesPerMethod[methodIndex]
	if prefixes != nil {
		return path, prefixes
	}

	// Prefixes were not cached for this path and method. Generate and cache.
	prefix := path.ApplicationId + "." + path.ApiId + "." + method + "." + path.PathTemplate + "."
	prefixNoClient := prefix + "*.*."
	prefixes = &metricNames{
		GlobalPrefix: prefix,
		CountAll:     prefixNoClient + metricCountAll,
		CountPerStatusCodeRange: [5]string{
			prefixNoClient + metricCount100s,
			prefixNoClient + metricCount200s,
			prefixNoClient + metricCount300s,
			prefixNoClient + metricCount400s,
			prefixNoClient + metricCount500s,
		},
		Latency: prefixNoClient + metricLatency,
	}
	path.metricPrefixesPerMethod[methodIndex] = prefixes
	return path, prefixes
}
