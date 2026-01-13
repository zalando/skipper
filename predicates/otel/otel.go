package otel

import (
	"net/http"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
	"go.opentelemetry.io/otel/baggage"
)

type baggageSpec struct{}

type baggagePredicate struct {
	key string
}

// NewBaggage provides a predicate spec to create a Predicate instance that matches a baggage by key.
func NewBaggage() routing.PredicateSpec { return &baggageSpec{} }

func (*baggageSpec) Name() string {
	return predicates.OTelBaggageName
}

// Create a predicate instance that always evaluates to baggage
func (*baggageSpec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	var key string
	if sarg, ok := args[0].(string); ok {
		key = sarg
	} else {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	return &baggagePredicate{
		key: key,
	}, nil
}

func (p *baggagePredicate) Match(r *http.Request) bool {
	bp := baggage.FromContext(r.Context())
	return bp.Member(p.key).Key() == p.key
}
