/*
Package cron implements custom predicates to match routes
only when they also match the system time matches the given
cron-like expressions.

Package includes a single predicate: Cron.

For supported & unsupported features refer to the "cronmask" package
documentation (https://github.com/sarslanhan/cronmask).
*/
package cron

import (
	"github.com/sarslanhan/cronmask"
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
	"net/http"
	"time"
)

type clock func() time.Time

type spec struct {
}

func (*spec) Name() string {
	return predicates.CronName
}

func (*spec) Create(args []any) (routing.Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	expr, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	mask, err := cronmask.New(expr)
	if err != nil {
		return nil, err
	}

	return &predicate{
		mask:    mask,
		getTime: time.Now,
	}, nil
}

type predicate struct {
	mask    *cronmask.CronMask
	getTime clock
}

func (p *predicate) Match(r *http.Request) bool {
	now := p.getTime()

	return p.mask.Match(now)
}

func New() routing.PredicateSpec {
	return &spec{}
}
