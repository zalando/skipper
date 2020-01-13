package primitive

import (
	"net/http"

	"github.com/zalando/skipper/routing"
)

const (
	NameTrue = "True"
)

type trueSpec struct{}

type truePredicate struct{}

// NewTrue provides a predicate spec to create a Predicate instance that evaluates to true
func NewTrue() routing.PredicateSpec { return &trueSpec{} }

func (*trueSpec) Name() string {
	return NameTrue
}

// Create a predicate instance that always evaluates to true
func (*trueSpec) Create(args []interface{}) (routing.Predicate, error) {
	return &truePredicate{}, nil
}

func (*truePredicate) Match(*http.Request) bool {
	return true
}
