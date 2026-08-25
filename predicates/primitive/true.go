package primitive

import (
	"net/http"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	// Deprecated, use predicates.TrueName instead
	NameTrue = predicates.TrueName
)

type trueSpec struct{}

type truePredicate struct{}

// NewTrue provides a predicate spec to create a Predicate instance that evaluates to true
func NewTrue() routing.PredicateSpec { return &trueSpec{} }

func (*trueSpec) Name() string {
	return predicates.TrueName
}

// Create a predicate instance that always evaluates to true
func (*trueSpec) Create(args []any) (routing.Predicate, error) {
	return &truePredicate{}, nil
}

func (*truePredicate) Match(*http.Request) bool {
	return true
}
