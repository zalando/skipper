package proxy

import (
	ot "github.com/opentracing/opentracing-go"
)

const (
	ComponentTag      = "component"
	ErrorTag          = "error"
	FlowIDTag         = "component"
	HostnameTag       = "hostname"
	HTTPHostTag       = "http.host"
	HTTPMethodTag     = "http.method"
	HTTPRemoteAddrTag = "http.remote_addr"
	HTTPPathTag       = "http.path"
	HTTPUrlTag        = "http.url"
	HTTPStatusCodeTag = "http.status_code"
	SkipperRouteTag   = "skipper.route"
	SkipperRouteIDTag = "skipper.route_id"
	SpanKindTag       = "span.kind"
	SpanKindClient    = "client"
	SpanKindServer    = "server"
)

type proxyTracing struct {
	tracer                   ot.Tracer
	initialOperationName     string
	logFilterLifecycleEvents bool
	excludeTags              map[string]bool
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

	excludedTags := map[string]bool{}

	for _, t := range p.ExcludeTags {
		excludedTags[t] = true
	}

	return &proxyTracing{
		tracer:                   p.Tracer,
		initialOperationName:     p.InitialSpan,
		logFilterLifecycleEvents: p.LogFilterEvents,
		excludeTags:              excludedTags,
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

func (t *proxyTracing) setTag(span ot.Span, key string, value interface{}) *proxyTracing {
	_, excluded := t.excludeTags[key]
	if !excluded {
		span.SetTag(key, value)
	}

	return t
}
