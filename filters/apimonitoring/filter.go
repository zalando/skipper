package apimonitoring

import (
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	metricCountAll  = "http_count"
	metricCount500s = "http5xx_count"
	metricCount400s = "http4xx_count"
	metricCount300s = "http3xx_count"
	metricCount200s = "http2xx_count"
	metricLatency   = "latency"
)

const (
	stateBagKeyPrefix = "filter.apimonitoring."
	stateBagKeyState  = stateBagKeyPrefix + "state"
)

// New creates a new instance of the API Monitoring filter
// specification (its factory).
//
// Parameter verbose indicates
// if all instances of the filter should be forced to activate
// their verbose mode, disregarding their JSON configuration.
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

type pathInfo struct {
	ApplicationId string
	PathTemplate  string
	Matcher       *regexp.Regexp
}

// apiMonitoringFilterContext contains information about the metrics tracking
// for one HTTP exchange (one routing). It serves to pass information from
// the `Request` call to the `Response` call (stored in the context's `StateBag`).
type apiMonitoringFilterContext struct {
	// DimensionPrefix is the prefix to all metrics tracked during this exchange (generated only once)
	DimensionsPrefix string
	// Begin is the earliest point in time where the request is observed
	Begin time.Time
	// OriginalRequestSize is the initial requests' size, before it is modified by other filters.
	OriginalRequestSize int64
}

func (f *apiMonitoringFilter) Request(c filters.FilterContext) {
	log := log.WithField("op", "request")

	// Identify the dimensions "prefix" of the metrics.
	dimensionsPrefix, track := f.getDimensionPrefix(c, log)
	if !track {
		return
	}

	// Gathering information from the initial request for further metrics calculation
	begin := time.Now()
	originalRequestSize := c.Request().ContentLength

	// Store that information in the FilterContext's state.
	mfc := &apiMonitoringFilterContext{
		DimensionsPrefix:    dimensionsPrefix,
		Begin:               begin,
		OriginalRequestSize: originalRequestSize,
	}
	c.StateBag()[stateBagKeyState] = mfc
}

func (f *apiMonitoringFilter) Response(c filters.FilterContext) {
	log := log.WithField("op", "response")
	if f.verbose {
		log.Info("RESPONSE CONTEXT: " + formatFilterContext(c))
	}

	mfc, ok := c.StateBag()[stateBagKeyState].(*apiMonitoringFilterContext)
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
}

// getDimensionPrefix generates the dimension part of the metrics key (before the name
// of the metric itself).
// Returns:
//   prefix:    the metric key prefix
//   track:     if this call should be tracked or not
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
			log.Info("Matching no path template. Not tracking this call.")
		}
		return "", false
	}
	dimensions := []string{
		path.ApplicationId,
		req.Method,
		path.PathTemplate,
	}
	prefix := strings.Join(dimensions, ".") + "."
	return prefix, true
}

func (f *apiMonitoringFilter) writeMetricCount(metrics filters.Metrics, mfc *apiMonitoringFilterContext) {
	key := mfc.DimensionsPrefix + metricCountAll
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
	if f.verbose {
		log.Infof("incrementing %q by 1", key)
	}
	metrics.IncCounter(key)
}

func (f *apiMonitoringFilter) writeMetricLatency(metrics filters.Metrics, mfc *apiMonitoringFilterContext) {
	key := mfc.DimensionsPrefix + metricLatency
	if f.verbose {
		log.Infof("measuring for %q since %v", key, mfc.Begin)
	}
	metrics.MeasureSince(key, mfc.Begin)
}
