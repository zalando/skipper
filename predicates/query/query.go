/*
Package query  implements a custom predicate to match routes
based on the Query Params in URL

It supports checking existence of query params and also checking whether
query params value match to a given regular exp

Examples:

	// Checking existence of a query param
	// matches http://example.org?bb=a&query=withvalue
	example1: QueryParam("query") -> "http://example.org";

	// Even a query param without a value
	// matches http://example.org?bb=a&query=
	example1: QueryParam("query") -> "http://example.org";

	// matches with regexp
	// matches http://example.org?bb=a&query=example
	example1: QueryParam("query", "^example$") -> "http://example.org";

	// matches with regexp and multiple values of query param
	// matches http://example.org?bb=a&query=testing&query=example
	example1: QueryParam("query", "^example$") -> "http://example.org";
*/
package query

import (
	"net/http"
	"regexp"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

type matchType int

const (
	exists matchType = iota + 1
	matches
)

type predicate struct {
	typ       matchType
	paramName string
	valueExp  *regexp.Regexp
}
type spec struct{}

// New creates a new QueryParam predicate specification.
func New() routing.PredicateSpec { return &spec{} }

func (s *spec) Name() string {
	return predicates.QueryParamName
}

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) == 0 || len(args) > 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	name, ok1 := args[0].(string)

	switch {
	case !ok1:
		return nil, predicates.ErrInvalidPredicateParameters
	case len(args) == 1:
		return &predicate{exists, name, nil}, nil
	case len(args) == 2:
		value, ok2 := args[1].(string)
		if !ok2 {
			return nil, predicates.ErrInvalidPredicateParameters
		}
		valueExp, err := regexp.Compile(value)
		if err != nil {
			return nil, err
		}
		return &predicate{matches, name, valueExp}, nil
	default:
		return nil, predicates.ErrInvalidPredicateParameters
	}

}

func (p *predicate) Match(r *http.Request) bool {
	queryMap := r.URL.Query()
	vals, ok := queryMap[p.paramName]

	switch p.typ {
	case exists:
		return ok
	case matches:
		if !ok {
			return false
		} else {
			for _, v := range vals {
				if p.valueExp.MatchString(v) {
					return true
				}
			}
			return false
		}

	}

	return false
}
