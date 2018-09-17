package apimonitoring

import (
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
	apiId        string
	includePath  bool
	pathPatterns map[string]*regexp.Regexp
}

var _ filters.Filter = new(apiMonitoringFilter)

//
// IMPLEMENTS filters.Filter
//

func (f *apiMonitoringFilter) Request(c filters.FilterContext) {
	log.WithField("op", "request").Infof("Filter: %p %+v", f, f)

	//
	// METRICS: Gathering from the initial request
	//

	// Identify the dimensions "prefix" of the metrics.
	dimensionsPrefix := f.getDimensionPrefix(c)

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

func (f *apiMonitoringFilter) getDimensionPrefix(c filters.FilterContext) (prefix string) {
	req := c.Request()
	parts := make([]string, 0, 3)

	//
	// API ID
	//
	apiId := ""
	if f.apiId == "" {
		apiId = req.Host // no API ID set in the route. Using the host.
	} else {
		apiId = f.apiId // API ID configured in the route. Using it.
	}
	parts = append(parts, apiId)

	//
	// PATH
	//
	if f.includePath {
		path := ""
		for pathPat, regex := range f.pathPatterns {
			if regex.MatchString(req.URL.Path) {
				path = pathPat
				break
			}
		}
		if path == "" {
			// if no path pattern matches, use the path as it is
			path = req.URL.Path
		}
		// Ensure head and tail `/`
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		if !strings.HasSuffix(path, "/") {
			path = path + "/"
		}
		parts = append(parts, path)
	}

	//
	// METHOD
	//
	method := req.Method
	parts = append(parts, method)

	//
	// FINAL PREFIX
	//
	prefix = strings.Join(parts, ".") + "."
	return
}

func (f *apiMonitoringFilter) Response(c filters.FilterContext) {
	log.WithField("op", "response").Infof("Filter: %+v", f)

	mfc, ok := c.StateBag()[KeyState].(*apiMonitoringFilterContext)
	if !ok {
		log.Errorf("monitoring filter state %q not found in FilterContext's StateBag or not of the expected type", KeyState)
		return
	}

	mfc.WriteMetricCount()
	mfc.WriteMetricLatency()
	mfc.WriteMetricSizeOfRequest()
	mfc.WriteMetricSizeOfResponse()
}
