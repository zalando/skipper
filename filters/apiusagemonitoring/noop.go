package apiusagemonitoring

import "github.com/zalando/skipper/filters"

type noopSpec struct{
	filter filters.Filter
}

func (*noopSpec) Name() string {
	return Name
}

func (s *noopSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	return s.filter, nil
}

type noopFilter struct{}

func (*noopFilter) Request(filters.FilterContext)  {}
func (*noopFilter) Response(filters.FilterContext) {}
