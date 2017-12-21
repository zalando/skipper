/*
Package loadbalancer implements a predicate which will match for different backends
in a round-robin fashion.

First parameter defines a group which determines the set of possible routes to match.

Second parameter is 0-based index of the route among the other routes in the same group.

Third parameter is the total number of routes in the group.

Eskip example:

	LoadBalancer("group-name", 0, 2) -> "https://www.example.org:8000";
	LoadBalancer("group-name", 1, 2) -> "https://www.example.org:8001";
*/
package loadbalancer

import (
	"net/http"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	PredicateName = "LoadBalancer"
)

type spec struct {
	mu       *sync.RWMutex
	counters map[string]int
}

type predicate struct {
	group string
	index int
	count int

	mu       *sync.RWMutex
	counters map[string]int
}

// New creates a new load balancer predicate spec.
func New() routing.PredicateSpec {
	return &spec{
		mu:       &sync.RWMutex{},
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
	return &predicate{
		mu:       s.mu,
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
	matched := current == p.index%p.count
	log.Infof(
		"lb predicate: matmched=%t group=%s index=%d count=%d current=%d",
		matched, p.group, p.index, p.count, current,
	)
	if matched {
		p.mu.Lock()
		current = (current + 1) % p.count
		p.counters[p.group] = current
		p.mu.Unlock()
	}
	return matched
}
