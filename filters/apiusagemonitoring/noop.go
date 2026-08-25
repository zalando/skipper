package apiusagemonitoring

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

type noopSpec struct {
	filter filters.Filter
}

func (*noopSpec) Name() string {
	return filters.ApiUsageMonitoringName
}

func (s *noopSpec) CreateFilter(config []any) (filters.Filter, error) {
	return s.filter, nil
}

// Do implements routing.PostProcessor as a noop so the disabled spec satisfies
// the same interface as the enabled one; the caller can then register it without
// a type-assertion guard. There is no filter cache to prune when disabled.
func (*noopSpec) Do(routes []*routing.Route) []*routing.Route {
	return routes
}

type noopFilter struct{}

func (noopFilter) Request(filters.FilterContext)  {}
func (noopFilter) Response(filters.FilterContext) {}
