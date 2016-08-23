package priority

import (
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const Name = "Priority"

type priority struct{}

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
