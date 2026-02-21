package primitive

import (
	"net/http"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	// Deprecated, use predicates.FalseName instead
	NameFalse = predicates.FalseName
)

type falseSpec struct{}

type falsePredicate struct{}

// NewFalse provides a predicate spec to create a Predicate instance that evaluates to false
func NewFalse() routing.PredicateSpec { return &falseSpec{} }

func (*falseSpec) Name() string {
	return predicates.FalseName
}

// Create a predicate instance that always evaluates to false
func (*falseSpec) Create(args []any) (routing.Predicate, error) {
	return &falsePredicate{}, nil
}

func (*falsePredicate) Match(*http.Request) bool {
	return false
}
