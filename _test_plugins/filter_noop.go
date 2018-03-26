package main

import (
	"github.com/zalando/skipper/filters"
)

type noopSpec struct{}

func InitFilter(opts []string) (filters.Spec, error) {
	return noopSpec{}, nil
}

func (s noopSpec) Name() string {
	return "noop"
}
func (s noopSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	return noopFilter{}, nil
}

type noopFilter struct{}

func (f noopFilter) Request(filters.FilterContext)  {}
func (f noopFilter) Response(filters.FilterContext) {}
