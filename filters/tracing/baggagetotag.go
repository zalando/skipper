package tracing

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/tracing"
	"go.opentelemetry.io/otel/attribute"
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

func (baggageToTagSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
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
	reqCtx := ctx.Request().Context()
	span := tracing.SpanFromContext(reqCtx, ctx.Tracer())
	if span == nil {
		return
	}

	baggageItem := tracing.GetBaggageMember(reqCtx, span, f.baggageItemName)
	if baggageItem.Value() == "" {
		return
	}

	span.SetAttributes(attribute.String(f.tagName, baggageItem.Value()))
}

func (baggageToTagFilter) Response(ctx filters.FilterContext) {}
