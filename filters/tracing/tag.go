package tracing

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/tracing"
	"go.opentelemetry.io/otel/attribute"
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
	span := tracing.SpanFromContext(ctx.Request().Context(), ctx.Tracer())
	if span == nil {
		return
	}

	if v, ok := f.tagValue.ApplyContext(ctx); ok {
		span.SetAttributes(attribute.String(f.tagName, v))
	}
}
