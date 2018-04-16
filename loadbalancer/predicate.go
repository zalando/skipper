package loadbalancer

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	GroupPredicateName  = "LBGroup"
	MemberPredicateName = "LBMember"
)

type groupSpec struct{}

type groupPredicate struct {
	group string
}

type memberSpec struct{}

type memberPredicate struct {
	group       string
	indexString string
}

func getGroupDecision(h http.Header, group string) (string, bool) {
	for _, header := range h[DecisionHeader] {
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

// NewGroup creates a predicate spec identifying the entry route
// of a load balanced route group. E.g. eskip: LBGroup("my-group")
// where the single mandatory string argument is the name of the
// group, used as a reference in the LB decision filter and the
// the group member predicates.
//
// Typically, one such route is used in a load balancer setup and
// it contains the the decision filter (lbDecide("my-group", 4)).
// It is recommended to generate these routes with the
// loadbalancer.Balance() function that expects a single route and
// N backend endpoints as input and returns the loadbalanced set
// of routes representing the group.
func NewGroup() routing.PredicateSpec {
	return &groupSpec{}
}

func (s *groupSpec) Name() string { return GroupPredicateName }

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

// NewMember creates a predicate spec identifying a member route
// of a load balanced route group. E.g. eskip: LBMember("my-group", 2)
// where the first argument is the name of the group, while the
// second is the index of the current route.
//
// Typically, these routes are generated with the loadbalancer.Balance()
// function. See the description of LBGroup(), too.
func NewMember() routing.PredicateSpec {
	return &memberSpec{}
}

func (s *memberSpec) Name() string { return MemberPredicateName }

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
