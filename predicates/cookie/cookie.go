/*
Package cookie implements prediate to check parsed cookie headers by name and value.
*/
package cookie

import (
	"net/http"
	"regexp"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

// The predicate can be referenced in eskip by the name "Cookie".
const Name = "Cookie"

type (
	spec struct{}

	predicate struct {
		name     string
		valueExp *regexp.Regexp
	}
)

// New creates a predicate specification, whose instances can be used to match parsed request cookies.
//
// The cookie predicate accpets two arguments, the cookie name, with what a cookie must exist in the request,
// and an expression that the cookie value needs to match.
//
// Eskip example:
//
// 	Cookie("tcial", /^enabled$/) -> "https://www.example.org";
//
func New() routing.PredicateSpec { return &spec{} }

func (s *spec) Name() string { return Name }

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	name, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	value, ok := args[1].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	valueExp, err := regexp.Compile(value)
	if err != nil {
		return nil, err
	}

	return &predicate{name, valueExp}, nil
}

func (p *predicate) Match(r *http.Request) bool {
	c, err := r.Cookie(p.name)
	if err != nil {
		return false
	}

	return p.valueExp.MatchString(c.Value)
}
