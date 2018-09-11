package main

import (
	"net/http"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

type noopSpec struct {
	Type string
}

func InitPlugin(opts []string) ([]filters.Spec, []routing.PredicateSpec, []routing.DataClient, error) {
	return []filters.Spec{noopSpec{"noop"}}, []routing.PredicateSpec{noopSpec{"None"}}, nil, nil
}

func (s noopSpec) Name() string {
	return s.Type
}

func (s noopSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	return noop{}, nil
}

func (s noopSpec) Create(config []interface{}) (routing.Predicate, error) {
	return noop{}, nil
}

type noop struct{}

func (p noop) Request(filters.FilterContext)  {}
func (p noop) Response(filters.FilterContext) {}

func (p noop) Match(*http.Request) bool {
	return false
}
