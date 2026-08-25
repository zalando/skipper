// Package content provides Content-Length related predicates.
package content

import (
	"net/http"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

type contentLengthBetweenSpec struct{}

type contentLengthBetweenPredicate struct {
	min int64
	max int64
}

// NewContentLengthBetween creates a predicate specification,
// whose instances match content length header value in range from min (inclusively) to max (exclusively).
// example: ContentLengthBetween(0, 5000)
func NewContentLengthBetween() routing.PredicateSpec { return &contentLengthBetweenSpec{} }

func (*contentLengthBetweenSpec) Name() string {
	return predicates.ContentLengthBetweenName
}

// Create a predicate instance that evaluates content length header value range
func (*contentLengthBetweenSpec) Create(args []any) (routing.Predicate, error) {
	if len(args) != 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	x, ok := args[0].(float64)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	minLength := int64(x)

	x, ok = args[1].(float64)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	maxLength := int64(x)

	if minLength < 0 || maxLength < 0 || minLength >= maxLength {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	return &contentLengthBetweenPredicate{
		min: minLength,
		max: maxLength,
	}, nil
}

func (p *contentLengthBetweenPredicate) Match(req *http.Request) bool {
	return req.ContentLength >= p.min && req.ContentLength < p.max
}
