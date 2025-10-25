// Package host provides HTTP host header matching related predicates.
package host

import (
	"net/http"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

type anySpec struct{}

type anyPredicate struct {
	hosts []string
}

// NewAny creates a predicate specification, whose instances match request host.
//
// The HostAny predicate requires one or more string hostnames and matches if request host
// exactly equals to any of the hostnames.
func NewAny() routing.PredicateSpec { return &anySpec{} }

func (*anySpec) Name() string {
	return predicates.HostAnyName
}

// Create a predicate instance that always evaluates to true
func (*anySpec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) == 0 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	p := &anyPredicate{}
	for _, arg := range args {
		if host, ok := arg.(string); ok {
			p.hosts = append(p.hosts, host)
		} else {
			return nil, predicates.ErrInvalidPredicateParameters
		}
	}
	return p, nil
}

func (ap *anyPredicate) Match(r *http.Request) bool {
	for _, host := range ap.hosts {
		if host == r.Host {
			return true
		}
	}
	return false
}
