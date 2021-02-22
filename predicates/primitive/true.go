package primitive

import (
	"github.com/zalando/skipper/predicates"
	"net/http"
)

type trueSpec struct{}

type truePredicate struct{}

// NewTrue provides a predicate spec to create a Predicate instance that evaluates to true
func NewTrue() predicates.PredicateSpec { return &trueSpec{} }

func (*trueSpec) Name() string {
	return predicates.TrueName
}

// Create a predicate instance that always evaluates to true
func (*trueSpec) Create(args []interface{}) (predicates.Predicate, error) {
	return &truePredicate{}, nil
}

func (*truePredicate) Match(*http.Request) bool {
	return true
}
