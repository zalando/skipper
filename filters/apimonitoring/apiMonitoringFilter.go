package apimonitoring

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"regexp"
	"strings"
	"time"
)

// Metric names
const (
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
	KeyPrefix = "filter.apimonitoring."
	KeyState  = KeyPrefix + "state"
)

type apiMonitoringFilter struct {
	paths   []*pathInfo
	verbose bool
}

var _ filters.Filter = new(apiMonitoringFilter)

type pathInfo struct {
	ApplicationId string
	ApiId         string
	PathTemplate  string
	Matcher       *regexp.Regexp
}

//
// IMPLEMENTS filters.Filter
//

// Request fulfills the Filter interface.
func (f *apiMonitoringFilter) Request(c filters.FilterContext) {
	log := log.WithField("op", "request")
	if f.verbose {
		log.Infof("Filter: %#v", f)
		log.Infof("FilterContext: %#v", c)
		jsStateBag, _ := json.MarshalIndent(c.StateBag(), "", "  ")
		log.Infof("StateBag:\n%s", jsStateBag)
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
		Filter:              f,
		FilterContext:       c,
		DimensionsPrefix:    dimensionsPrefix,
		Begin:               begin,
		OriginalRequestSize: originalRequestSize,
	}
	c.StateBag()[KeyState] = mfc
}

// Response fulfills the Filter interface.
func (f *apiMonitoringFilter) Response(c filters.FilterContext) {
	log := log.WithField("op", "response")
	if f.verbose {
		log.Infof("Filter: %#v", f)
		log.Infof("FilterContext: %#v", c)
		jsStateBag, _ := json.MarshalIndent(c.StateBag(), "", "  ")
		log.Infof("StateBag:\n%s", jsStateBag)
	}

	mfc, ok := c.StateBag()[KeyState].(*apiMonitoringFilterContext)
	if !ok {
		if f.verbose {
			log.Info("Call not tracked")
		}
		return
	}

	mfc.WriteMetricCount()
	mfc.WriteMetricLatency()
	mfc.WriteMetricSizeOfRequest()
	mfc.WriteMetricSizeOfResponse()
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
		path.ApplicationId,
		path.ApiId,
		req.Method,
		path.PathTemplate,
	}
	prefix := strings.Join(dimensions, ".") + "."
	return prefix, true
}
