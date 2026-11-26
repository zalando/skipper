package hedge

import (
	"github.com/zalando/skipper/filters"
)

type disableHedge struct{}

// NewDisableHedge creates a filter spec for the disableHedge() filter.
// The filter opts the route out of hedged requests.
//
// Name: "disableHedge".
func NewDisableHedge() filters.Spec { return disableHedge{} }

func (disableHedge) Name() string { return filters.DisableHedgeName }

func (disableHedge) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	return disableHedge{}, nil
}

func (disableHedge) Request(ctx filters.FilterContext) {
	ctx.StateBag()[filters.BackendHedgeDisabled] = true
}

func (disableHedge) Response(filters.FilterContext) {}
