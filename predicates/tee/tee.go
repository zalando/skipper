package tee

import (
	"net/http"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	// Deprecated, use predicates.TeeName instead
	PredicateName = predicates.TeeName

	HeaderKey = "x-tee-loopback-key"
)

type spec struct{}

type predicate struct {
	key string
}

func New() routing.PredicateSpec { return &spec{} }

func (s *spec) Name() string { return predicates.TeeName }

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
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
