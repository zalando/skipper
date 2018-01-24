package loadbalancer

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	groupPredicateName  = "LBGroup"
	memberPredicateName = "LBMember"
	decideFilterName    = "lbDecide"

	decisionHeader = "X-Load-Balancer-Member"
)

type counter chan int

type groupSpec struct{}

type groupPredicate struct {
	group string
}

type memberSpec struct{}

type memberPredicate struct {
	group       string
	indexString string
}

type decideSpec struct{}

type decideFilter struct {
	group   string
	size    int
	counter counter
}

func getGroupDecision(h http.Header, group string) (string, bool) {
	for _, header := range h[decisionHeader] {
		decision := strings.Split(header, "=")
		if len(decision) != 2 {
			continue
		}

		if decision[0] == group {
			return decision[1], true
		}
	}

	return "", false
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

func (c counter) value() int {
	v := <-c
	c <- v
	return v
}

func NewGroup() routing.PredicateSpec {
	return &groupSpec{}
}

func (s *groupSpec) Name() string { return groupPredicateName }

func (s *groupSpec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	group, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	return &groupPredicate{group: group}, nil
}

func (p *groupPredicate) Match(req *http.Request) bool {
	_, has := getGroupDecision(req.Header, p.group)
	return !has
}

func NewMember() routing.PredicateSpec {
	return &memberSpec{}
}

func (s *memberSpec) Name() string { return memberPredicateName }

func (s *memberSpec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	group, ok := args[0].(string)
	if !ok {
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

	return &memberPredicate{
		group:       group,
		indexString: strconv.Itoa(index), // we only need it as a string
	}, nil
}

func (p *memberPredicate) Match(req *http.Request) bool {
	member, _ := getGroupDecision(req.Header, p.group)
	return member == p.indexString
}

func NewDecide() filters.Spec { return &decideSpec{} }

func (s *decideSpec) Name() string { return decideFilterName }

func (s *decideSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	group, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	size, ok := args[1].(int)
	if !ok {
		fsize, ok := args[1].(float64)
		if !ok {
			return nil, predicates.ErrInvalidPredicateParameters
		}

		size = int(fsize)
	}

	return &decideFilter{
		group:   group,
		size:    size,
		counter: newCounter(),
	}, nil
}

func (f *decideFilter) Request(ctx filters.FilterContext) {
	current := f.counter.inc(f.size)
	ctx.Request().Header.Set(decisionHeader, fmt.Sprintf("%s=%d", f.group, current))
}

func (f *decideFilter) Response(filters.FilterContext) {}
