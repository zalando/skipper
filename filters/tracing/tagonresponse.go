package tracing

import (
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

type tagOnResponseSpec struct {
}

type tagOnResponseFilter struct {
	tagName  string
	tagValue *eskip.Template
}

// NewTagOnResponse creates a filter specification for the tracingTagOnResponse filter.
func NewTagOnResponse() filters.Spec {
	return &tagOnResponseSpec{}
}

func (s *tagOnResponseSpec) Name() string {
	return filters.TracingTagOnResponseName
}

func (s *tagOnResponseSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	tagName, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	tagValue, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &tagOnResponseFilter{
		tagName:  tagName,
		tagValue: eskip.NewTemplate(tagValue),
	}, nil
}

func (f *tagOnResponseFilter) Request(filters.FilterContext) {}

func (f *tagOnResponseFilter) Response(ctx filters.FilterContext) {
	req := ctx.Request()
	span := opentracing.SpanFromContext(req.Context())
	if span == nil {
		return
	}

	if v, ok := f.tagValue.ApplyContext(ctx); ok {
		span.SetTag(f.tagName, v)
	}
}
