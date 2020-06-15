package tee

import (
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
	"net/http"
)

type ContextKey int

const (
	// The eskip name of the predicate.
	PredicateName = "Tee"
	ContextTeeKey ContextKey = iota
)

type spec struct{}

type predicate struct {
	key string
}

func New() routing.PredicateSpec { return &spec{} }

func (s *spec) Name() string { return PredicateName }

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	return &predicate{
		key: args[0].(string),
	}, nil
}

func (p *predicate) Match(r *http.Request) bool {
	ctx := r.Context()
	teeRegistry, ok := ctx.Value(ContextTeeKey).(map[string]bool)
	if !ok {
		return false
	}
	if _, ok:=teeRegistry[p.key]; ok {
		return true
	}
	return false
}
