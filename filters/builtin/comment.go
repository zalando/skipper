package builtin

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

type comment struct{}

// NewComment is a filter to comment a filter chain. It does nothing
func NewComment() filters.Spec {
	return &comment{}
}

func (*comment) Name() string {
	return filters.CommentName
}

func (c *comment) CreateFilter(args []interface{}) (filters.Filter, error) {
	return c, nil
}

func (*comment) Request(filters.FilterContext) {}

func (*comment) Response(filters.FilterContext) {}

type CommentPostProcessor struct{}

func (CommentPostProcessor) Do(routes []*routing.Route) []*routing.Route {
	for _, r := range routes {
		var ff []*routing.RouteFilter
		for _, f := range r.Filters {
			if f.Name != filters.CommentName {
				ff = append(ff, f)
			}
		}
		r.Filters = ff
	}
	return routes
}
