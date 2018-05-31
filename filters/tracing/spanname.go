package tracing

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy"
)

const SpanNameFilterName = "tracingSpanName"

type spec struct{}

type filter struct {
	spanName string
}

func New() filters.Spec {
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
	bag[proxy.OpenTracingProxySpanKey] = f.spanName
}

func (f *filter) Response(filters.FilterContext) {}
