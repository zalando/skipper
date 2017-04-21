/*
Package source implements a custom predicate to match routes
based on the non-existence of query parameters in URL.

This predicate returns true, if the given query parameter
does not exist in the URL, or it exists but all values are
empty.

Examples:

    // Checking non-existence of a query param
    // matches http://example.org?bb=a&query=withvalue
    example: QueryParamMissing("param1") -> "http://example.org";

    // Checking empty query params
    // matches http://example.org?p=&x=something&p=
    example: QueryParamMissing("p") -> "http://example.org";

*/
package query_missing

import (
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
	"net/http"
)

type predicate struct {
	paramName string
}

type spec struct{}

const name = "QueryParamMissing"

// New creates a new QueryParamMissing predicate specification.
func New() routing.PredicateSpec { return &spec{} }

func (s *spec) Name() string {
	return name
}

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) == 0 || len(args) > 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	name, ok := args[0].(string)

	switch {
	case !ok:
		return nil, predicates.ErrInvalidPredicateParameters
	case len(args) == 1:
		return &predicate{name}, nil
	default:
		return nil, predicates.ErrInvalidPredicateParameters
	}
}

func (p *predicate) Match(r *http.Request) bool {
	queryMap := r.URL.Query()
	values, ok := queryMap[p.paramName]

	if !ok {
		return true
	}

	for _, v := range values {
		if v != "" {
			return false
		}
	}

	return true
}
