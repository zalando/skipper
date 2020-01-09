package primitive

import (
	"net/http"

	"github.com/zalando/skipper/routing"
)

const (
	NameFalse = "False"
)

type falseSpec struct{}

type falsePredicate struct{}

// NewFalse provides a predicate spec to create a Predicate instance that evaluates to false
func NewFalse() routing.PredicateSpec { return &falseSpec{} }

func (*falseSpec) Name() string {
	return NameFalse
}

// Create a predicate instance that always evaluates to false
func (*falseSpec) Create(args []interface{}) (routing.Predicate, error) {
	return &falsePredicate{}, nil
}

func (*falsePredicate) Match(*http.Request) bool {
	return false
}
