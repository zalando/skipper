package tee

import (
	"github.com/zalando/skipper/predicates"
	"net/http"
)

const (
	// The eskip name of the predicate.

	HeaderKey = "x-tee-loopback-key"
)

type spec struct{}

type predicate struct {
	key string
}

func New() predicates.PredicateSpec { return &spec{} }

func (s *spec) Name() string { return predicates.TeeName }

func (s *spec) Create(args []interface{}) (predicates.Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	teeKey, _ := args[0].(string)
	if teeKey == "" {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	return &predicate{
		key: teeKey,
	}, nil
}

func (p *predicate) Match(r *http.Request) bool {
	v := r.Header.Get(HeaderKey)
	return v == p.key
}
