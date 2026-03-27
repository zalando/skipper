package otel

import (
	"net/http"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
	"go.opentelemetry.io/otel/baggage"
)

type baggageSpec struct{}

type baggagePredicate struct {
	key   string
	value string
}

// NewBaggage provides a predicate spec to create a Predicate instance that matches a baggage by key.
func NewBaggage() routing.PredicateSpec { return &baggageSpec{} }

func (*baggageSpec) Name() string {
	return predicates.OTelBaggageName
}

// Create a predicate instance that always evaluates to baggage
func (*baggageSpec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) == 0 || len(args) > 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	var key string
	var value string

	if sarg, ok := args[0].(string); ok {
		key = sarg
	} else {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	if len(args) == 2 {
		if sarg, ok := args[1].(string); ok {
			value = sarg
		} else {
			return nil, predicates.ErrInvalidPredicateParameters
		}
	}

	return &baggagePredicate{
		key:   key,
		value: value,
	}, nil
}

func (p *baggagePredicate) Match(r *http.Request) bool {
	bp := baggage.FromContext(r.Context())
	member := bp.Member(p.key)
	if p.value != "" {
		return member.Key() == p.key && member.Value() == p.value
	}
	return member.Key() == p.key
}
