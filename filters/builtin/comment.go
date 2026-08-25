package builtin

import (
	"slices"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

type comment struct{}

// NewComment is a filter to comment a filter chain. It does nothing
func NewComment() filters.Spec {
	return comment{}
}

func (comment) Name() string {
	return filters.CommentName
}

func (c comment) CreateFilter(args []any) (filters.Filter, error) {
	return c, nil
}

func (comment) Request(filters.FilterContext) {}

func (comment) Response(filters.FilterContext) {}

type CommentPostProcessor struct{}

func (CommentPostProcessor) Do(routes []*routing.Route) []*routing.Route {
	for _, r := range routes {
		r.Filters = slices.DeleteFunc(r.Filters, func(f *routing.RouteFilter) bool {
			return f.Name == filters.CommentName
		})
	}
	return routes
}
