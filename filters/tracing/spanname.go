/*
Package tracing provides filters to instrument distributed tracing.
*/
package tracing

import (
	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.TracingSpanNameName instead
	SpanNameFilterName = filters.TracingSpanNameName

	// OpenTracingProxySpanKey is the key used in the state bag to pass the span name to the proxy.
	OpenTracingProxySpanKey = "statebag:opentracing:proxy:span"
)

type spec struct{}

type filter struct {
	spanName string
}

// NewSpanName creates a filter spec for setting the name of the outgoing span. (By default "proxy".)
//
//	tracingSpanName("example-operation")
func NewSpanName() filters.Spec {
	return &spec{}
}

func (s *spec) Name() string { return filters.TracingSpanNameName }

func (s *spec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	name, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &filter{spanName: name}, nil
}

func (f *filter) Request(ctx filters.FilterContext) {
	bag := ctx.StateBag()
	bag[OpenTracingProxySpanKey] = f.spanName
}

func (f *filter) Response(filters.FilterContext) {}
