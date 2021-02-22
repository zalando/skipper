package main

import (
	"github.com/zalando/skipper/predicates"
	"net/http"

	"github.com/zalando/skipper/filters"
)

// this fails to load, because it implements multiple Init* functions

func InitFilter(opts []string) (filters.Spec, error) {
	return noopSpec{}, nil
}

func InitPredicate(opts []string) (predicates.PredicateSpec, error) {
	return noneSpec{}, nil
}

type noopSpec struct{}

func (s noopSpec) Name() string {
	return "noop"
}
func (s noopSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	return noopFilter{}, nil
}

type noopFilter struct{}

func (f noopFilter) Request(filters.FilterContext)  {}
func (f noopFilter) Response(filters.FilterContext) {}

type noneSpec struct{}

func (s noneSpec) Name() string {
	return "None"
}
func (s noneSpec) Create(config []interface{}) (predicates.Predicate, error) {
	return nonePredicate{}, nil
}

type nonePredicate struct{}

func (p nonePredicate) Match(*http.Request) bool {
	return false
}
