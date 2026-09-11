package tls

import (
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

type tlsClientPredicateSpec struct{}
type tlsClientPredicate struct{}

func NewTLSClientPredicate() routing.PredicateSpec {
	return &tlsClientPredicateSpec{}
}

func (m *tlsClientPredicateSpec) Name() string {
	return predicates.TLSClientName
}

func (m *tlsClientPredicateSpec) Create([]any) (routing.Predicate, error) {
	return nil, nil
}
