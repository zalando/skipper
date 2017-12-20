package loadbalancer

import (
	"net/http"
	"sync"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	PredicateName = "LoadBalancer"
)

type spec struct {
	mu       sync.RWMutex
	counters map[string]int
}

type predicate struct {
	group string
	index int
	count int

	mu       sync.RWMutex
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
	if index >= count {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	s.mu.Lock()
	if s.counters == nil {
		s.counters = make(map[string]int)
	}
	if _, ok := s.counters[group]; !ok {
		s.counters[group] = 0
	}
	s.mu.Unlock()
	return &predicate{
		group:    group,
		index:    index,
		count:    count,
		counters: s.counters,
	}, nil
}

func (p *predicate) Match(r *http.Request) bool {
	p.mu.RLock()
	current := p.counters[p.group]
	p.mu.RUnlock()
	matched := current == p.index
	if matched {
		p.mu.Lock()
		current = (current + 1) % p.count
		p.counters[p.group] = current
		p.mu.Unlock()
	}
	return matched
}
