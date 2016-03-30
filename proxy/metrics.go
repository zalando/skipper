package proxy

import (
	"github.com/zalando/skipper/metrics"
	"time"
)

type (
	meter interface {
		incRoutingFailures()
		measureRouteLookup(time.Time)
		measureFilterRequest(string, time.Time)
		measureAllFiltersRequest(string, time.Time)
		incErrorsBackend(string)
		measureBackend(string, time.Time)
		measureFilterResponse(string, time.Time)
		measureAllFiltersResponse(string, time.Time)
		incErrorsStreaming(string)
		measureResponse(int, string, string, time.Time)
	}

	voidMeter    struct{}
	defaultMeter struct{}
)

func newMeter(o Options) meter {
	if o.Debug() {
		return voidMeter{}
	}

	return defaultMeter{}
}

func (m voidMeter) incRoutingFailures()                            {}
func (m voidMeter) measureRouteLookup(time.Time)                   {}
func (m voidMeter) measureFilterRequest(string, time.Time)         {}
func (m voidMeter) measureAllFiltersRequest(string, time.Time)     {}
func (m voidMeter) incErrorsBackend(string)                        {}
func (m voidMeter) measureBackend(string, time.Time)               {}
func (m voidMeter) measureFilterResponse(string, time.Time)        {}
func (m voidMeter) measureAllFiltersResponse(string, time.Time)    {}
func (m voidMeter) incErrorsStreaming(string)                      {}
func (m voidMeter) measureResponse(int, string, string, time.Time) {}

func (m defaultMeter) incRoutingFailures()                    { metrics.IncRoutingFailures() }
func (m defaultMeter) measureRouteLookup(t time.Time)         { metrics.MeasureRouteLookup(t) }
func (m defaultMeter) incErrorsBackend(rid string)            { metrics.IncErrorsBackend(rid) }
func (m defaultMeter) measureBackend(rid string, t time.Time) { metrics.MeasureBackend(rid, t) }
func (m defaultMeter) incErrorsStreaming(rid string)          { metrics.IncErrorsStreaming(rid) }

func (m defaultMeter) measureFilterRequest(rid string, t time.Time) {
	metrics.MeasureFilterRequest(rid, t)
}

func (m defaultMeter) measureAllFiltersRequest(rid string, t time.Time) {
	metrics.MeasureAllFiltersRequest(rid, t)
}

func (m defaultMeter) measureFilterResponse(rid string, t time.Time) {
	metrics.MeasureFilterResponse(rid, t)
}

func (m defaultMeter) measureAllFiltersResponse(rid string, t time.Time) {
	metrics.MeasureAllFiltersResponse(rid, t)
}

func (m defaultMeter) measureResponse(status int, method string, rid string, t time.Time) {
	metrics.MeasureResponse(status, method, rid, t)
}
