package apimonitoring

import (
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Metric names
const (
	// Prefix
	MetricPrefix = "api-mon"

	// Status Code counting
	MetricCountAll  = "http_count"
	MetricCount500s = "http500_count"
	MetricCount400s = "http400_count"
	MetricCount300s = "http300_count"
	MetricCount200s = "http200_count"

	// Request & Response
	MetricRequestSize  = "req_size_sum"
	MetricResponseSize = "resp_size_sum"

	// Timings
	MetricLatency = "latency"
)

// StateBag Keys
const (
	StateBagKeyPrefix = "filter.apimonitoring."
	StateBagKeyState  = StateBagKeyPrefix + "state"
)

func New(verbose bool) filters.Spec {
	spec := &apiMonitoringFilterSpec{
		verbose: verbose,
	}
	if verbose {
		log.Infof("Created filter spec: %+v", spec)
	}
	return spec
}

type apiMonitoringFilter struct {
	paths   []*pathInfo
	verbose bool
}

var _ filters.Filter = new(apiMonitoringFilter)

type pathInfo struct {
	ApplicationId string
	PathTemplate  string
	Matcher       *regexp.Regexp
}

type apiMonitoringFilterContext struct {
	DimensionsPrefix string

	// Information that is read at `request` time and needed at `response` time
	Begin               time.Time // earliest point in time where the request is observed
	OriginalRequestSize int64     // initial requests' size, before it is modified by other filters.
}

// Request fulfills the Filter interface.
func (f *apiMonitoringFilter) Request(c filters.FilterContext) {
	log := log.WithField("op", "request")
	if f.verbose {
		log.Info("REQUEST CONTEXT: " + formatFilterContext(c))
	}

	//
	// METRICS: Gathering from the initial request
	//

	// Identify the dimensions "prefix" of the metrics.
	dimensionsPrefix, track := f.getDimensionPrefix(c, log)
	if !track {
		return
	}

	begin := time.Now()
	originalRequestSize := c.Request().ContentLength

	mfc := &apiMonitoringFilterContext{
		DimensionsPrefix:    dimensionsPrefix,
		Begin:               begin,
		OriginalRequestSize: originalRequestSize,
	}
	c.StateBag()[StateBagKeyState] = mfc
}

// Response fulfills the Filter interface.
func (f *apiMonitoringFilter) Response(c filters.FilterContext) {
	log := log.WithField("op", "response")
	if f.verbose {
		log.Info("RESPONSE CONTEXT: " + formatFilterContext(c))
	}

	mfc, ok := c.StateBag()[StateBagKeyState].(*apiMonitoringFilterContext)
	if !ok {
		if f.verbose {
			log.Info("Call not tracked")
		}
		return
	}

	metrics := c.Metrics()
	response := c.Response()

	f.writeMetricCount(metrics, mfc)
	f.writeMetricResponseStatusClassCount(metrics, mfc, response)
	f.writeMetricLatency(metrics, mfc)
	f.writeMetricSizeOfRequest(metrics, mfc)
	f.writeMetricSizeOfResponse(metrics, mfc, response)
}

// getDimensionPrefix generates the dimension part of the metrics key (before the name
// of the metric itself).
// Returns:
//   prefix:	the metric key prefix
//   track:		if this call should be tracked or not
func (f *apiMonitoringFilter) getDimensionPrefix(c filters.FilterContext, log *logrus.Entry) (string, bool) {
	req := c.Request()
	var path *pathInfo = nil
	for _, p := range f.paths {
		if p.Matcher.MatchString(req.URL.Path) {
			path = p
			break
		}
	}
	if path == nil {
		if f.verbose {
			log.Info("Matching no path pattern. Not tracking this call.")
		}
		return "", false
	}
	dimensions := []string{
		MetricPrefix,
		path.ApplicationId,
		req.Method,
		path.PathTemplate,
	}
	prefix := strings.Join(dimensions, ".") + "."
	return prefix, true
}

func (f *apiMonitoringFilter) writeMetricCount(metrics filters.Metrics, mfc *apiMonitoringFilterContext) {
	key := mfc.DimensionsPrefix + MetricCountAll
	if f.verbose {
		log.Infof("incrementing %q by 1", key)
	}
	metrics.IncCounter(key)
}

func (f *apiMonitoringFilter) writeMetricResponseStatusClassCount(metrics filters.Metrics, mfc *apiMonitoringFilterContext, response *http.Response) {
	var classMetricName string
	st := response.StatusCode
	switch {
	case st < 200:
		return
	case st < 300:
		classMetricName = MetricCount200s
	case st < 400:
		classMetricName = MetricCount300s
	case st < 500:
		classMetricName = MetricCount400s
	case st < 600:
		classMetricName = MetricCount500s
	default:
		return
	}

	key := mfc.DimensionsPrefix + classMetricName
	if f.verbose {
		log.Infof("incrementing %q by 1", key)
	}
	metrics.IncCounter(key)
}

func (f *apiMonitoringFilter) writeMetricLatency(metrics filters.Metrics, mfc *apiMonitoringFilterContext) {
	key := mfc.DimensionsPrefix + MetricLatency
	if f.verbose {
		log.Infof("measuring for %q since %v", key, mfc.Begin)
	}
	metrics.MeasureSince(key, mfc.Begin)
}

func (f *apiMonitoringFilter) writeMetricSizeOfRequest(metrics filters.Metrics, mfc *apiMonitoringFilterContext) {
	requestSize := mfc.OriginalRequestSize
	if requestSize < 0 {
		log.WithField("dimensions", mfc.DimensionsPrefix).
			Infof("unknown request content length: %d", requestSize)
		return
	}

	key := mfc.DimensionsPrefix + MetricRequestSize
	if f.verbose {
		log.Infof("incrementing %q by %d", key, requestSize)
	}
	metrics.IncCounterBy(key, requestSize)
}

func (f *apiMonitoringFilter) writeMetricSizeOfResponse(metrics filters.Metrics, mfc *apiMonitoringFilterContext, response *http.Response) {
	if response == nil {
		return
	}
	responseSize := response.ContentLength // todo: this always return 0, investigate why
	if responseSize < 0 {
		log.WithField("dimensions", mfc.DimensionsPrefix).Infof("unknown response content length: %d", responseSize)
		return
	}

	key := mfc.DimensionsPrefix + MetricResponseSize
	if f.verbose {
		log.Infof("incrementing %q by %d", key, responseSize)
	}
	metrics.IncCounterBy(key, responseSize)
}
