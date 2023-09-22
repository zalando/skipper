package tracing

import (
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

type tagSpec struct {
	typ string
}

type tagFilter struct {
	tagFromResponse bool

	tagName  string
	tagValue *eskip.Template
}

// NewTag creates a filter specification for the tracingTag filter.
func NewTag() filters.Spec {
	return &tagSpec{typ: filters.TracingTagName}
}

// NewTagFromResponse creates a filter similar to NewTag, but applies tags after the request has been processed.
func NewTagFromResponse() filters.Spec {
	return &tagSpec{typ: filters.TracingTagFromResponseName}
}

func (s *tagSpec) Name() string {
	return s.typ
}

func (s *tagSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
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

	return &tagFilter{
		tagFromResponse: s.typ == filters.TracingTagFromResponseName,

		tagName:  tagName,
		tagValue: eskip.NewTemplate(tagValue),
	}, nil
}

func (f *tagFilter) Request(ctx filters.FilterContext) {
	if !f.tagFromResponse {
		f.setTag(ctx)
	}
}

func (f *tagFilter) Response(ctx filters.FilterContext) {
	if f.tagFromResponse {
		f.setTag(ctx)
	}
}

func (f *tagFilter) setTag(ctx filters.FilterContext) {
	span := opentracing.SpanFromContext(ctx.Request().Context())
	if span == nil {
		return
	}

	if v, ok := f.tagValue.ApplyContext(ctx); ok {
		span.SetTag(f.tagName, v)
	}
}
