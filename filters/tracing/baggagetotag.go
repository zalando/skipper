package tracing

import (
	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.TracingBaggageToTagName instead
	BaggageToTagFilterName = filters.TracingBaggageToTagName
)

type baggageToTagSpec struct{}

type baggageToTagFilter struct {
	baggageItemName string
	tagName         string
}

func (baggageToTagSpec) Name() string {
	return filters.TracingBaggageToTagName
}

func (baggageToTagSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) < 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	baggageItemName, ok := args[0].(string)
	if !ok || baggageItemName == "" {
		return nil, filters.ErrInvalidFilterParameters
	}

	tagName := baggageItemName
	if len(args) > 1 {
		tagNameArg, ok := args[1].(string)
		if !ok || tagNameArg == "" {
			return nil, filters.ErrInvalidFilterParameters
		}
		tagName = tagNameArg
	}

	return baggageToTagFilter{
		baggageItemName,
		tagName,
	}, nil
}

func NewBaggageToTagFilter() filters.Spec {
	return baggageToTagSpec{}
}

func (f baggageToTagFilter) Request(ctx filters.FilterContext) {

	span := opentracing.SpanFromContext(ctx.Request().Context())
	if span == nil {
		return
	}
	baggageItem := span.BaggageItem(f.baggageItemName)

	if baggageItem == "" {
		return
	}

	span.SetTag(f.tagName, baggageItem)
}

func (baggageToTagFilter) Response(ctx filters.FilterContext) {}
