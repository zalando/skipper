package loadbalancer

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	loadBalancerDecideName = "LoadDecide"
	loadBalancerName       = "LoadBalancer2"
	decisionHeaderFmt      = "X-Skipper-Load-Balancer-Decision-%s"
)

type counter chan int

type specDecide struct{}

type predicateDecide struct {
	counter        counter
	decisionHeader string
	size           int
}

type spec2 struct{}

type predicate2 struct {
	decisionHeader string
	indexStr       string
}

func newCounter() counter {
	c := make(counter, 1)
	c <- 0
	return c
}

func (c counter) inc(groupSize int) int {
	v := <-c
	c <- v + 1
	return v % groupSize
}

func NewDecide() routing.PredicateSpec {
	return &specDecide{}
}

func (s *specDecide) Name() string { return loadBalancerDecideName }

func (s *specDecide) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	group, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	size, ok := args[0].(int)
	if !ok {
		fsize, ok := args[1].(float64)
		if !ok {
			return nil, predicates.ErrInvalidPredicateParameters
		}

		size = int(fsize)
	}

	return &predicateDecide{
		counter:        newCounter(),
		decisionHeader: fmt.Sprintf(decisionHeaderFmt, group),
		size:           size,
	}, nil
}

func (p *predicateDecide) Match(r *http.Request) bool {
	if r.Header.Get(p.decisionHeader) != "" {
		return false
	}

	d := p.counter.inc(p.size)
	r.Header.Set(p.decisionHeader, strconv.Itoa(d))
	return true
}

func NewBalance() routing.PredicateSpec {
	return &spec2{}
}

func (s *spec2) Name() string { return loadBalancerName }

func (s *spec2) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	group, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	index, ok := args[0].(int)
	if !ok {
		findex, ok := args[1].(float64)
		if !ok {
			return nil, predicates.ErrInvalidPredicateParameters
		}

		index = int(findex)
	}

	return &predicate2{
		decisionHeader: fmt.Sprintf(decisionHeaderFmt, group),
		indexStr:       strconv.Itoa(index),
	}, nil
}

func (p *predicate2) Match(r *http.Request) bool {
	m := r.Header.Get(p.decisionHeader) == p.indexStr
	if m {
		r.Header.Del(p.decisionHeader)
	}

	return m
}
