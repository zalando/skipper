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
	MetricCountAll  = "Count"
	MetricCount500s = "Count500s"
	MetricCount400s = "Count400s"
	MetricCount300s = "Count300s"
	MetricCount200s = "Count200s"

	// Request & Response
	MetricRequestSize  = "ReqSize"
	MetricResponseSize = "ResSize"

	// Timings
	MetricLatency = "Latency"
)

// StateBag Keys
const (
	KeyPrefix = "filter.apimonitoring."
	KeyState  = KeyPrefix + "state"
)

type apiMonitoringFilter struct {
	verbose      bool
	apiId        string
	pathPatterns map[string]*regexp.Regexp
}

var _ filters.Filter = new(apiMonitoringFilter)

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
func (f *apiMonitoringFilter) getDimensionPrefix(c filters.FilterContext, log *logrus.Entry) (prefix string, track bool) {
	req := c.Request()

	//
	// PATH
	//
	path := ""
	for pathPat, regex := range f.pathPatterns {
		if regex.MatchString(req.URL.Path) {
			path = pathPat
			break
		}
	}
	if path == "" {
		// if no path pattern matches, do not track.
		if f.verbose {
			log.Info("Matching no path pattern. Not tracking this call.")
		}
		return
	}

	//
	// API ID
	//
	apiId := ""
	if f.apiId == "" {
		apiId = req.Host // no API ID set in the route. Using the host.
	} else {
		apiId = f.apiId // API ID configured in the route. Using it.
	}

	//
	// METHOD
	//
	method := req.Method

	//
	// FINAL PREFIX
	//
	dimensions := []string{apiId, method, path}
	prefix = strings.Join(dimensions, ".") + "."
	track = true
	return
}
