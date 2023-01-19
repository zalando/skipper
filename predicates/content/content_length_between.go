package content

import (
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
	"net/http"
)

type contentLengthBetweenSpec struct{}

type contentLengthBetweenPredicate struct {
	min int64
	max int64
}

// NewContentLengthBetween creates a predicate specification,
// whose instances match content length header value in range from min to max inclusively.
// example: ContentLengthBetween(0, 5000)
func NewContentLengthBetween() routing.PredicateSpec { return &contentLengthBetweenSpec{} }

func (*contentLengthBetweenSpec) Name() string {
	return predicates.ContentLengthBetweenName
}

// Create a predicate instance that evaluates content length header value range
func (*contentLengthBetweenSpec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	minLength, _ := args[0].(int64)
	maxLength, _ := args[1].(int64)
	if minLength < 0 || maxLength < 0 || minLength > maxLength {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	return &contentLengthBetweenPredicate{
		min: minLength,
		max: maxLength,
	}, nil
}

func (p *contentLengthBetweenPredicate) Match(req *http.Request) bool {
	length := req.ContentLength
	if length >= p.min && length <= p.max {
		return true
	}
	return false
}
