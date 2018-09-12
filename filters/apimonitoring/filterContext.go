package apimonitoring

import (
	"github.com/zalando/skipper/filters"
	"time"
)

// monitoringFilterContext holds the information relevant for ONE round-trip
// (one combination of "apiMonitoringFilter" and "filters.FilterContext")
type monitoringFilterContext struct {
	Filter        *apiMonitoringFilter
	FilterContext filters.FilterContext

	DimensionsPrefix    string

	// Metrics gathering helper
	Begin               time.Time // earliest point in time where the request is observed
	OriginalRequestSize int64     // initial requests' size, before it is modified by other filters.
}

func (c *monitoringFilterContext) WriteMetricCount() {
	// Count all calls
	c.incCounter(MetricCountAll)
	// Count by status class
	st := c.FilterContext.Response().StatusCode
	switch {
	case st < 200:
		// NOOP
	case st < 300:
		c.incCounter(MetricCount200s)
	case st < 400:
		c.incCounter(MetricCount300s)
	case st < 500:
		c.incCounter(MetricCount400s)
	case st < 600:
		c.incCounter(MetricCount500s)
	}
}

func (c *monitoringFilterContext) WriteMetricLatency() {
	c.measureSince(MetricLatency, c.Begin)
}

func (c *monitoringFilterContext) WriteMetricSizeOfRequest() {
	requestSize := c.OriginalRequestSize
	if requestSize < 0 {
		log.WithField("dimensions", c.DimensionsPrefix).
			Infof("unknown request content length: %d", requestSize)
	} else {
		c.incCounterBy(MetricRequestSize, requestSize)
	}
}

func (c *monitoringFilterContext) WriteMetricSizeOfResponse() {
	response := c.FilterContext.Response()
	if response == nil {
		return
	}
	responseSize := response.ContentLength
	if responseSize < 0 {
		log.WithField("dimensions", c.DimensionsPrefix).
			Infof("unknown response content length: %d", responseSize)
	} else {
		c.incCounterBy(MetricResponseSize, responseSize)
	}
}

//
// METRICS HELPERS
//

func (c *monitoringFilterContext) incCounter(key string) {
	k := c.DimensionsPrefix + key
	log.Infof("incrementing %q by 1", k)
	c.FilterContext.Metrics().IncCounter(k)
}

func (c *monitoringFilterContext) incCounterBy(key string, value int64) {
	k := c.DimensionsPrefix + key
	log.Infof("incrementing %q by %d", k, value)
	c.FilterContext.Metrics().IncCounterBy(k, value)
}

func (c *monitoringFilterContext) measureSince(key string, start time.Time) {
	k := c.DimensionsPrefix + key
	log.Infof("measuring for %q since %v", k, start)
	c.FilterContext.Metrics().MeasureSince(k, start)
}
