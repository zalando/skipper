package apiusagemonitoring

import (
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	metricsSeparator = "."
	metricCountAll   = "http_count"
	metricCount500s  = "http5xx_count"
	metricCount400s  = "http4xx_count"
	metricCount300s  = "http3xx_count"
	metricCount200s  = "http2xx_count"
	metricLatency    = "latency"
)

const (
	stateBagKeyPrefix = "filter.apiUsageMonitoring."
	stateBagKeyState  = stateBagKeyPrefix + "state"
)

var (
	unknownPath = &pathInfo{
		ApplicationId: unknownElementPlaceholder,
		ApiId:         unknownElementPlaceholder,
		PathTemplate:  unknownElementPlaceholder,
	}
)

type apiUsageMonitoringFilter struct {
	paths []*pathInfo
}

type pathInfo struct {
	ApplicationId string
	ApiId         string
	PathTemplate  string
	Matcher       *regexp.Regexp
}

// apiUsageMonitoringFilterContext contains information about the metrics tracking
// for one HTTP exchange (one routing). It serves to pass information from
// the `Request` call to the `Response` call (stored in the context's `StateBag`).
type apiUsageMonitoringFilterContext struct {
	// DimensionPrefix is the prefix to all metrics tracked during this exchange (generated only once)
	DimensionsPrefix string
	// Begin is the earliest point in time where the request is observed
	Begin time.Time
	// OriginalRequestSize is the initial requests' size, before it is modified by other filters.
	OriginalRequestSize int64
}

func (f *apiUsageMonitoringFilter) String() string {
	return toJsonStringOrError(mapApiUsageMonitoringFilter(f))
}

func (f *apiUsageMonitoringFilter) Request(c filters.FilterContext) {
	log := log.WithField("op", "request")

	// Identify the dimensions "prefix" of the metrics.
	dimensionsPrefix := f.getDimensionPrefix(c, log)

	// Gathering information from the initial request for further metrics calculation
	begin := time.Now()
	originalRequestSize := c.Request().ContentLength

	// Store that information in the FilterContext's state.
	mfc := &apiUsageMonitoringFilterContext{
		DimensionsPrefix:    dimensionsPrefix,
		Begin:               begin,
		OriginalRequestSize: originalRequestSize,
	}
	c.StateBag()[stateBagKeyState] = mfc
}

func (f *apiUsageMonitoringFilter) Response(c filters.FilterContext) {
	log := log.WithField("op", "response")
	if log.Logger.Level >= logrus.DebugLevel {
		contextAsJson := toJsonStringOrError(mapFilterContext(c))
		log.Debugf("RESPONSE CONTEXT: %s", contextAsJson)
	}

	mfc, ok := c.StateBag()[stateBagKeyState].(*apiUsageMonitoringFilterContext)
	if !ok {
		log.Debugf("Call not tracked (key %q not found in StateBag)", stateBagKeyState)
		return
	}

	metrics := c.Metrics()
	response := c.Response()

	f.writeMetricCount(metrics, mfc)
	f.writeMetricResponseStatusClassCount(metrics, mfc, response)
	f.writeMetricLatency(metrics, mfc)
}

// getDimensionPrefix generates the dimension part of the metrics key (before the name
// of the metric itself).
func (f *apiUsageMonitoringFilter) getDimensionPrefix(c filters.FilterContext, log *logrus.Entry) string {
	req := c.Request()
	var path *pathInfo = nil
	for _, p := range f.paths {
		if p.Matcher.MatchString(req.URL.Path) {
			path = p
			break
		}
	}
	if path == nil {
		log.Debugf("Matching no path template. Tracking as unknown.")
		path = unknownPath
	}
	dimensions := []string{
		path.ApplicationId,
		path.ApiId,
		req.Method,
		path.PathTemplate,
		"",
	}
	prefix := strings.Join(dimensions, metricsSeparator)
	return prefix
}

func (f *apiUsageMonitoringFilter) writeMetricCount(metrics filters.Metrics, mfc *apiUsageMonitoringFilterContext) {
	key := mfc.DimensionsPrefix + metricCountAll
	log.Debugf("incrementing %q by 1", key)
	metrics.IncCounter(key)
}

func (f *apiUsageMonitoringFilter) writeMetricResponseStatusClassCount(metrics filters.Metrics, mfc *apiUsageMonitoringFilterContext, response *http.Response) {
	var classMetricName string
	st := response.StatusCode
	switch {
	case st < 200:
		return
	case st < 300:
		classMetricName = metricCount200s
	case st < 400:
		classMetricName = metricCount300s
	case st < 500:
		classMetricName = metricCount400s
	case st < 600:
		classMetricName = metricCount500s
	default:
		return
	}

	key := mfc.DimensionsPrefix + classMetricName
	log.Debugf("incrementing %q by 1", key)
	metrics.IncCounter(key)
}

func (f *apiUsageMonitoringFilter) writeMetricLatency(metrics filters.Metrics, mfc *apiUsageMonitoringFilterContext) {
	key := mfc.DimensionsPrefix + metricLatency
	log.Debugf("measuring for %q since %v", key, mfc.Begin)
	metrics.MeasureSince(key, mfc.Begin)
}
