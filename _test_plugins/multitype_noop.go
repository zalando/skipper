package main

import (
	"github.com/zalando/skipper/predicates"
	"net/http"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

type multiSpec struct {
	Type string
}

func InitPlugin(opts []string) ([]filters.Spec, []predicates.PredicateSpec, []routing.DataClient, error) {
	return []filters.Spec{multiSpec{"noop"}}, []predicates.PredicateSpec{multiSpec{"None"}}, nil, nil
}

func (s multiSpec) Name() string {
	return s.Type
}

func (s multiSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	return noop{}, nil
}

func (s multiSpec) Create(config []interface{}) (predicates.Predicate, error) {
	return noop{}, nil
}

type noop struct{}

func (p noop) Request(filters.FilterContext)  {}
func (p noop) Response(filters.FilterContext) {}

func (p noop) Match(*http.Request) bool {
	return false
}
