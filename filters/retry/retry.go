package retry

import (
	"github.com/zalando/skipper/filters"
)

type retry struct{}

// NewRetry creates a filter specification for the retry() filter
func NewRetry() filters.Spec { return retry{} }

func (retry) Name() string                                       { return filters.RetryName }
func (retry) CreateFilter([]interface{}) (filters.Filter, error) { return retry{}, nil }
func (retry) Response(filters.FilterContext)                     {}

func (retry) Request(ctx filters.FilterContext) {
	ctx.StateBag()[filters.RetryName] = struct{}{}
}
