package builtin

import "github.com/zalando/skipper/filters"

type statusSpec struct{}

type statusFilter int

// NewStatus creates a filter specification whose instances set the
// status of the response to a fixed value regardless of
// backend response.
func NewStatus() filters.Spec { return new(statusSpec) }

func (s *statusSpec) Name() string { return filters.StatusName }

func (s *statusSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	switch c := args[0].(type) {
	case int:
		return statusFilter(c), nil
	case float64:
		return statusFilter(c), nil
	default:
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (f statusFilter) Request(filters.FilterContext) {}

func (f statusFilter) Response(ctx filters.FilterContext) {
	ctx.Response().StatusCode = int(f)
}
