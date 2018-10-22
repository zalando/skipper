package apiusagemonitoring

import (
	"github.com/zalando/skipper/filters"
)

type noopFilter struct {
	reason string
}

func (f *noopFilter) Request(filters.FilterContext) {
	log.Debugf("No API usage monitoring: %s", f.reason)
}

func (f *noopFilter) Response(filters.FilterContext) {}
