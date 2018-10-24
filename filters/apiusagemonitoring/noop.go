package apiusagemonitoring

import (
	"github.com/zalando/skipper/filters"
)

type noopSpec struct {
	filter *noopFilter
}

func (*noopSpec) Name() string {
	return Name
}

func (s *noopSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	return s.filter, nil
}

type noopFilter struct {
	reason string
}

func (f *noopFilter) Request(filters.FilterContext) {
	log.Debugf("No API usage monitoring: %s", f.reason)
}

func (f *noopFilter) Response(filters.FilterContext) {}
