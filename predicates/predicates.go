package predicates

import (
	"errors"
	"net/http"
)

// Error used in case of invalid predicate parameters.
var ErrInvalidPredicateParameters = errors.New("invalid predicate parameters")

// Predicate instances are used as custom user defined route
// matching predicates.
type Predicate interface {

	// Returns true if the request matches the predicate.
	Match(*http.Request) bool
}

// PredicateSpec instances are used to create custom predicates
// (of type Predicate) with concrete arguments during the
// construction of the routing tree.
type PredicateSpec interface {

	// Name of the predicate as used in the route definitions.
	Name() string

	// Creates a predicate instance with concrete arguments.
	Create([]interface{}) (Predicate, error)
}
