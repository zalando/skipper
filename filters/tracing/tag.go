package tracing

import (
	"net/http"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

type tagSpec struct {
	typ string
}

type tagFilterType int

const (
	tagRequest tagFilterType = iota + 1
	tagResponse
	tagResponseCondition
)

type tagFilter struct {
	typ tagFilterType

	tagName  string
	tagValue *eskip.Template

	condition func(*http.Response) bool
}

// NewTag creates a filter specification for the tracingTag filter.
func NewTag() filters.Spec {
	return &tagSpec{typ: filters.TracingTagName}
}

// NewTagFromResponse creates a filter similar to NewTag, but applies tags after the request has been processed.
func NewTagFromResponse() filters.Spec {
	return &tagSpec{typ: filters.TracingTagFromResponseName}
}

func NewTagFromResponseIfStatus() filters.Spec {
	return &tagSpec{typ: filters.TracingTagFromResponseIfStatusName}
}

func (s *tagSpec) Name() string {
	return s.typ
}

func (s *tagSpec) CreateFilter(args []any) (filters.Filter, error) {
	var typ tagFilterType

	switch s.typ {
	case filters.TracingTagName:
		if len(args) != 2 {
			return nil, filters.ErrInvalidFilterParameters
		}
		typ = tagRequest
	case filters.TracingTagFromResponseName:
		if len(args) != 2 {
			return nil, filters.ErrInvalidFilterParameters
		}
		typ = tagResponse
	case filters.TracingTagFromResponseIfStatusName:
		if len(args) != 4 {
			return nil, filters.ErrInvalidFilterParameters
		}
		typ = tagResponseCondition
	default:
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

	f := &tagFilter{
		typ:      typ,
		tagName:  tagName,
		tagValue: eskip.NewTemplate(tagValue),
	}

	if len(args) == 4 {
		minValue, ok := args[2].(float64)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
		maxValue, ok := args[3].(float64)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
		minVal := int(minValue)
		maxVal := int(maxValue)
		if minVal < 0 || maxVal > 599 || minVal > maxVal {
			return nil, filters.ErrInvalidFilterParameters
		}

		f.condition = func(rsp *http.Response) bool {
			return minVal <= rsp.StatusCode && rsp.StatusCode <= maxVal
		}
	}

	return f, nil
}

func (f *tagFilter) Request(ctx filters.FilterContext) {
	if f.typ == tagRequest {
		f.setTag(ctx)
	}
}

func (f *tagFilter) Response(ctx filters.FilterContext) {
	switch f.typ {
	case tagResponse:
		f.setTag(ctx)
	case tagResponseCondition:
		if f.condition(ctx.Response()) {
			f.setTag(ctx)
		}
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
