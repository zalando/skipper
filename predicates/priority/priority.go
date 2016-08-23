// Package priority implements a priority predicate specification.
//
// Priority predicates are used to indicate during route lookup that, in case
// of multiple routes matching a request, which matching route should be taken
// before the others.
//
// In the below example, the higher priority ensures that requests for '.html'
// paths will hit route1, even if they are '/directory/document.html'.
//
// Example:
//
// 	route1: Priority(2.72) && PathRegexp(/[.]html$/) -> "https://cache.example.org";
// 	route2: Path("/directory/**") -> "https://app.example.org";
//
package priority

import (
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

// Name of the priority predicate.
const Name = "Priority"

type priority struct{}

// New creates a predicate specification, that is used during route
// construction to create Priority marker predicates.
func New() routing.PredicateSpec { return &priority{} }

func (p *priority) Name() string { return Name }

func (p *priority) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	if v, ok := args[0].(float64); ok {
		return routing.Priority(v), nil
	}

	return nil, predicates.ErrInvalidPredicateParameters
}
