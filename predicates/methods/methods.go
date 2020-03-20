/*

Package methods implements a custom predicate to match routes
based on the http method in request

It supports multiple http methods, with case insensitive input

Examples:

    // matches GET request
    example1: Methods("GET") -> "http://example.org";

    // matches GET or POST request
    example1: Methods("GET", "post") -> "http://example.org";

*/
package methods

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/zalando/skipper/predicates"
)

const NameSingular = "Method"
const NamePlural = "Methods"

var ErrInvalidArgumentsCount = errors.New("at least one method should be specified")
var ErrInvalidArgumentType = errors.New("only string values are allowed")

type (
	spec struct {
		name           string
		allowedMethods map[string]bool
	}

	predicate struct {
		methods map[string]bool
	}
)

// NewSingular creates a new Method predicate specification
func NewSingular() predicates.PredicateSpec {
	return newSpec(NameSingular)
}

// NewPlural creates a new Methods predicate specification
func NewPlural() predicates.PredicateSpec {
	return newSpec(NamePlural)
}

func newSpec(name string) predicates.PredicateSpec {
	return &spec{
		name: name,
		allowedMethods: map[string]bool{
			http.MethodGet:     true,
			http.MethodHead:    true,
			http.MethodPost:    true,
			http.MethodPut:     true,
			http.MethodPatch:   true,
			http.MethodDelete:  true,
			http.MethodConnect: true,
			http.MethodOptions: true,
			http.MethodTrace:   true,
		},
	}
}

func (s *spec) Name() string { return s.name }

func (s *spec) Create(args []interface{}) (predicates.Predicate, error) {
	if len(args) == 0 {
		return nil, ErrInvalidArgumentsCount
	}

	if s.name == NameSingular && len(args) > 1 {
		return nil, ErrInvalidArgumentsCount
	}

	predicate := predicate{}
	predicate.methods = map[string]bool{}

	for _, arg := range args {
		method, isString := arg.(string)

		if !isString {
			return nil, ErrInvalidArgumentType
		}

		method = strings.ToUpper(method)

		if s.allowedMethods[method] {
			predicate.methods[method] = true
		} else {
			return nil, fmt.Errorf("method: %s is not allowed", method)
		}
	}

	return &predicate, nil
}

func (p *predicate) Match(r *http.Request) bool {
	return p.methods[strings.ToUpper(r.Method)]
}
