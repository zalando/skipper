package proxy

import (
	ot "github.com/opentracing/opentracing-go"
)

var (
	SkipperRouteTag   = "skipper.route"
	SkipperRouteIDTag = "skipper.route_id"
)

type proxyTracing struct {
	tracer                   ot.Tracer
	initialOperationName     string
	logFilterLifecycleEvents bool
	includeTags              map[string]bool
}

func newProxyTracing(p *OpenTracingParams) *proxyTracing {
	if p == nil {
		p = &OpenTracingParams{}
	}

	if p.InitialSpan == "" {
		p.InitialSpan = "ingress"
	}

	if p.Tracer == nil {
		p.Tracer = &ot.NoopTracer{}
	}

	includedTags := map[string]bool{}

	for _, t := range p.IncludeTags {
		includedTags[t] = true
	}

	return &proxyTracing{
		tracer:                   p.Tracer,
		initialOperationName:     p.InitialSpan,
		logFilterLifecycleEvents: p.LogFilterEvents,
	}
}

func (t *proxyTracing) logFilterLifecycleEvent(span ot.Span, filterName, event string) {
	if !t.logFilterLifecycleEvents {
		return
	}

	span.LogKV(filterName, event)
}

func (t *proxyTracing) logFilterStart(span ot.Span, filterName string) {
	t.logFilterLifecycleEvent(span, filterName, "start")
}

func (t *proxyTracing) logFilterEnd(span ot.Span, filterName string) {
	t.logFilterLifecycleEvent(span, filterName, "done")
}

func (t *proxyTracing) setTag(span ot.Span, key string, value interface{}) ot.Span {
	if included, ok := t.includeTags[key]; included && ok {
		span.SetTag(key, value)
	}

	return span
}
