package tracing

import (
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

const (
	name = "tracingTag"
)

type tagSpec struct {
}

type tagFilter struct {
	tagName  string
	tagValue *eskip.Template
}

// NewTag creates a filter specification for the tracingTag filter.
func NewTag() filters.Spec {
	return tagSpec{}
}

func (s tagSpec) Name() string {
	return name
}

func (s tagSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
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

	return tagFilter{
		tagName:  tagName,
		tagValue: eskip.NewTemplate(tagValue),
	}, nil
}

func (f tagFilter) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	span := opentracing.SpanFromContext(req.Context())
	if span == nil {
		return
	}

	if v, ok := f.tagValue.ApplyRequestContext(ctx); ok {
		span.SetTag(f.tagName, v)
	}
}

func (f tagFilter) Response(filters.FilterContext) {}
