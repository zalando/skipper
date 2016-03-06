package builtin

import "github.com/zalando/skipper/filters"

type statusSpec struct{}

type statusFilter struct {
	code int
}

func NewStatus() filters.Spec { return new(statusSpec) }

func (s *statusSpec) Name() string { return StatusName }

func (s *statusSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	if c, ok := args[0].(float64); ok {
		return &statusFilter{int(c)}, nil
	} else {
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (f *statusFilter) Request(filters.FilterContext) {}

func (f *statusFilter) Response(ctx filters.FilterContext) {
	ctx.Response().StatusCode = f.code
}
