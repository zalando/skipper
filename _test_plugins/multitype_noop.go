package main

import (
	"net/http"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

type multiSpec struct {
	Type string
}

func InitPlugin(opts []string) ([]filters.Spec, []routing.PredicateSpec, []routing.DataClient, error) {
	return []filters.Spec{multiSpec{"noop"}}, []routing.PredicateSpec{multiSpec{"None"}}, nil, nil
}

func (s multiSpec) Name() string {
	return s.Type
}

func (s multiSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	return noop{}, nil
}

func (s multiSpec) Create(config []interface{}) (routing.Predicate, error) {
	return noop{}, nil
}

type noop struct{}

func (p noop) Request(filters.FilterContext)  {}
func (p noop) Response(filters.FilterContext) {}

func (p noop) Match(*http.Request) bool {
	return false
}
