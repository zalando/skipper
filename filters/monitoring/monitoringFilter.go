package monitoring

import (
	"fmt"
	"github.com/zalando/skipper/filters"
	"time"
)

// Metric names
const (
	// Status Code counting
	MetricStatus500s  = "Status500s"
	MetricStatus400s  = "Status400s"
	MetricStatus200s  = "Status200s"
	MetricStatusOther = "StatusOther"

	// Request & Response
	MetricRequestSize  = "ReqSize"
	MetricResponseSize = "ResSize"

	// Timings
	MetricLatency = "Latency"
)

type monitoringFilter struct {
	dimensionsPrefix string

	// Metrics gathering helper
	begin       time.Time // earliest point in time where the request is observed
	requestSize int64     // size of the initial request (before filters are applied)
}

func (f *monitoringFilter) Request(c filters.FilterContext) {
	log.Infof("Request! %+v", f)

	//
	// METRICS: Gathering from the initial request
	//

	f.begin = time.Now()

	req := c.Request()

	// Identify the dimensions "prefix" of the metrics.
	f.dimensionsPrefix = fmt.Sprintf(
		"%s.%s.",
		req.Host, // TODO: What could we consider the API ID...?
		req.Method,
	)

	// Retain the initial requests' size, before it is modified by other filters.
	f.requestSize = req.ContentLength
}

func (f *monitoringFilter) Response(c filters.FilterContext) {
	log.Infof("Response! %+v", f)

	f.writeMetricNumberOfCalls(c)
	f.writeMetricLatency(c)
	f.writeMetricSizeOfRequest(c)
	f.writeMetricSizeOfResponse(c)
}

func (f *monitoringFilter) writeMetricNumberOfCalls(c filters.FilterContext) {
	st := c.Response().StatusCode
	switch {
	case /* 100s */ st < 200:
		c.Metrics().IncCounter(f.dimensionsPrefix + MetricStatusOther)
	case /* 200s */ st < 300:
		c.Metrics().IncCounter(f.dimensionsPrefix + MetricStatus200s)
	case /* 300s */ st < 400:
		c.Metrics().IncCounter(f.dimensionsPrefix + MetricStatusOther)
	case /* 400s */ st < 500:
		c.Metrics().IncCounter(f.dimensionsPrefix + MetricStatus400s)
	case /* 500s */ st < 600:
		c.Metrics().IncCounter(f.dimensionsPrefix + MetricStatus500s)
	default:
		c.Metrics().IncCounter(f.dimensionsPrefix + MetricStatusOther)
	}
}

func (f *monitoringFilter) writeMetricLatency(c filters.FilterContext) {
	c.Metrics().MeasureSince(f.dimensionsPrefix+MetricLatency, f.begin)
}

func (f *monitoringFilter) writeMetricSizeOfRequest(c filters.FilterContext) {
	if f.requestSize < 0 {
		log.WithField("dimensions", f.dimensionsPrefix).
			Infof("unknown request content length: %d", f.requestSize)
	} else {
		c.Metrics().IncCounterBy(f.dimensionsPrefix+MetricRequestSize, f.requestSize)
	}
}

func (f *monitoringFilter) writeMetricSizeOfResponse(c filters.FilterContext) {
	responseSize := c.Response().ContentLength
	if responseSize < 0 {
		log.WithField("dimensions", f.dimensionsPrefix).
			Infof("unknown response content length: %d", responseSize)
	} else {
		c.Metrics().IncCounter(f.dimensionsPrefix + MetricResponseSize)
	}
}
