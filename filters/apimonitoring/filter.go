package apimonitoring

import (
	"fmt"
	"github.com/zalando/skipper/filters"
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
	apiId string
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

	mfc := &monitoringFilterContext{
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

	apiId := ""
	if f.apiId == "" {
		apiId = req.Host // no API ID set in the route. Using the host.
	} else {
		apiId = f.apiId // API ID configured in the route. Using it.
	}

	method := req.Method

	prefix = fmt.Sprintf("%s.%s.", apiId, method)
	return
}

func (f *apiMonitoringFilter) Response(c filters.FilterContext) {
	log.WithField("op", "response").Infof("Filter: %+v", f)

	mfc, ok := c.StateBag()[KeyState].(*monitoringFilterContext)
	if !ok {
		log.Errorf("monitoring filter state %q not found in FilterContext's StateBag or not of the expected type", KeyState)
		return
	}

	mfc.WriteMetricCount()
	mfc.WriteMetricLatency()
	mfc.WriteMetricSizeOfRequest()
	mfc.WriteMetricSizeOfResponse()
}
