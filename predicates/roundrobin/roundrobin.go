package roundrobin

import (
	"net/http"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	PredicateName = "RoundRobin"
)

type spec struct {
	counters map[string]int
}

type predicate struct {
	group    string
	index    int
	count    int
	counters map[string]int
}

func New() routing.PredicateSpec {
	return &spec{
		counters: make(map[string]int),
	}
}

func (*spec) Name() string {
	return PredicateName
}

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 3 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	group, ok := args[0].(string)
	if !ok || group == "" {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	index, ok := args[1].(int)
	if !ok {
		findex, ok := args[1].(float64)
		if !ok {
			return nil, predicates.ErrInvalidPredicateParameters
		}
		index = int(findex)
	}
	if index < 0 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	count, ok := args[2].(int)
	if !ok {
		fcount, ok := args[2].(float64)
		if !ok {
			return nil, predicates.ErrInvalidPredicateParameters
		}
		count = int(fcount)
	}
	if count <= 0 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	// TODO: Sync needed
	if s.counters == nil {
		s.counters = make(map[string]int)
	}
	if _, ok := s.counters[group]; !ok {
		s.counters[group] = 0
	}
	return &predicate{
		group:    group,
		index:    index,
		count:    count,
		counters: s.counters,
	}, nil
}

// TODO: Sync needed
func (p *predicate) Match(r *http.Request) bool {
	current := p.counters[p.group]
	matched := current == p.index
	if matched {
		current = (current + 1) % p.count
		p.counters[p.group] = current
	}
	return matched
}
