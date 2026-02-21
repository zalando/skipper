package apiusagemonitoring

import "github.com/zalando/skipper/filters"

type noopSpec struct {
	filter filters.Filter
}

func (*noopSpec) Name() string {
	return filters.ApiUsageMonitoringName
}

func (s *noopSpec) CreateFilter(config []any) (filters.Filter, error) {
	return s.filter, nil
}

type noopFilter struct{}

func (noopFilter) Request(filters.FilterContext)  {}
func (noopFilter) Response(filters.FilterContext) {}
