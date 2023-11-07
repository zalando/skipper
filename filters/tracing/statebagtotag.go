package tracing

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/tracing"
)

const (
	// Deprecated, use filters.StateBagToTagName instead
	StateBagToTagFilterName = filters.StateBagToTagName
)

type stateBagToTagSpec struct{}

type stateBagToTagFilter struct {
	stateBagItemName string
	tagName          string
}

func (stateBagToTagSpec) Name() string {
	return filters.StateBagToTagName
}

func (stateBagToTagSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	stateBagItemName, ok := args[0].(string)
	if !ok || stateBagItemName == "" {
		return nil, filters.ErrInvalidFilterParameters
	}

	tagName := stateBagItemName
	if len(args) > 1 {
		tagNameArg, ok := args[1].(string)
		if !ok || tagNameArg == "" {
			return nil, filters.ErrInvalidFilterParameters
		}
		tagName = tagNameArg
	}

	return &stateBagToTagFilter{
		stateBagItemName: stateBagItemName,
		tagName:          tagName,
	}, nil
}

func NewStateBagToTag() filters.Spec {
	return stateBagToTagSpec{}
}

func (f *stateBagToTagFilter) Request(ctx filters.FilterContext) {
	value, ok := ctx.StateBag()[f.stateBagItemName]
	if !ok {
		return
	}

	span := tracing.SpanFromContext(ctx.Request().Context(), ctx.Tracer())
	if span == nil {
		return
	}

	span.SetAttributes(attribute.String(f.tagName, fmt.Sprint(value)))
}

func (*stateBagToTagFilter) Response(ctx filters.FilterContext) {}
