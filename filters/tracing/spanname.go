/*
Package tracing provides filters to instrument distributed tracing.
*/
package tracing

import (
	"github.com/zalando/skipper/filters"
)

const (
	// SpanNameFilterName is the name of the filter in eskip.
	SpanNameFilterName = "tracingSpanName"

	// OpenTracingProxySpanKey is the key used in the state bag to pass the span name to the proxy.
	OpenTracingProxySpanKey = "statebag:opentracing:proxy:span"
)

type spec struct{}

type filter struct {
	spanName string
}

// NewSpanName creates a filter spec for setting the name of the outgoing span. (By default "proxy".)
//
// 	tracingSpanName("example-operation")
//
// WARNING: this filter is experimental, and the name and the arguments can change until marked as stable.
func NewSpanName() filters.Spec {
	return &spec{}
}

func (s *spec) Name() string { return SpanNameFilterName }

func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
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
