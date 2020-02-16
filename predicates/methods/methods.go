/*

Package source implements a custom predicate to match routes
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
	"github.com/zalando/skipper/routing"
	"net/http"
	"strings"
)

const Name = "Methods"

var ErrInvalidArgumentsCount = errors.New("at least one method should be specified")
var ErrInvalidArgumentType = errors.New("only string values are allowed")

type (
	spec struct {
		allowedMethods map[string]bool
	}

	predicate struct {
		methods map[string]bool
	}
)

// New creates a new Methods predicate specification
func New() routing.PredicateSpec {
	return &spec{allowedMethods: map[string]bool{
		http.MethodGet:     true,
		http.MethodHead:    true,
		http.MethodPost:    true,
		http.MethodPut:     true,
		http.MethodPatch:   true,
		http.MethodDelete:  true,
		http.MethodConnect: true,
		http.MethodOptions: true,
		http.MethodTrace:   true,
	}}
}

func (s *spec) Name() string { return Name }

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) == 0 {
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
			return nil, errors.New(fmt.Sprintf("method: %s is not allowed", method))
		}
	}

	return &predicate, nil
}

func (p *predicate) Match(r *http.Request) bool {
	return p.methods[strings.ToUpper(r.Method)]
}
