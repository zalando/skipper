package main

import (
	"net/http"

	"github.com/zalando/skipper/predicates"
)

type noneSpec struct{}

func InitPredicate(opts []string) (predicates.PredicateSpec, error) {
	return noneSpec{}, nil
}

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
