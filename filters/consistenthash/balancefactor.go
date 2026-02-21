package consistenthash

import (
	"fmt"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/loadbalancer"
)

type consistentHashBalanceFactor struct {
	balanceFactor float64
}

// NewConsistentHashBalanceFactor creates a filter Spec, whose instances
// set the balancer factor used by the `consistentHash` algorithm to avoid
// popular hashes overloading a single endpoint
func NewConsistentHashBalanceFactor() filters.Spec { return &consistentHashBalanceFactor{} }
func (*consistentHashBalanceFactor) Name() string {
	return filters.ConsistentHashBalanceFactorName
}

func (*consistentHashBalanceFactor) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	value, ok := args[0].(float64)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	if value < 1 {
		return nil, fmt.Errorf("invalid consistentHashBalanceFactor filter value, must be >=1 but got %f", value)
	}

	return &consistentHashBalanceFactor{value}, nil
}

func (c *consistentHashBalanceFactor) Request(ctx filters.FilterContext) {
	ctx.StateBag()[loadbalancer.ConsistentHashBalanceFactor] = c.balanceFactor
}

func (*consistentHashBalanceFactor) Response(filters.FilterContext) {}
