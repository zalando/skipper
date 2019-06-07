package proxy

import (
	ot "github.com/opentracing/opentracing-go"
)

const (
	ClientRequestStateTag = "client.request"
	ComponentTag          = "component"
	ErrorTag              = "error"
	FlowIDTag             = "component"
	HostnameTag           = "hostname"
	HTTPHostTag           = "http.host"
	HTTPMethodTag         = "http.method"
	HTTPRemoteAddrTag     = "http.remote_addr"
	HTTPPathTag           = "http.path"
	HTTPUrlTag            = "http.url"
	HTTPStatusCodeTag     = "http.status_code"
	SkipperRouteTag       = "skipper.route"
	SkipperRouteIDTag     = "skipper.route_id"
	SpanKindTag           = "span.kind"

	ClientRequestCanceled = "canceled"
	SpanKindClient        = "client"
	SpanKindServer        = "server"

	EndEvent           = "end"
	StartEvent         = "start"
	StreamHeadersEvent = "stream_Headers"
)

type proxyTracing struct {
	tracer                   ot.Tracer
	initialOperationName     string
	logFilterLifecycleEvents bool
	logStreamEvents          bool
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
		logStreamEvents:          p.LogStreamEvents,
		excludeTags:              excludedTags,
	}
}

func (t *proxyTracing) logEvent(span ot.Span, eventName, eventValue string) {
	if span == nil {
		return
	}

	span.LogKV(eventName, eventValue)
}

func (t *proxyTracing) setTag(span ot.Span, key string, value interface{}) *proxyTracing {
	if span == nil {
		return t
	}

	if !t.excludeTags[key] {
		span.SetTag(key, value)
	}

	return t
}

func (t *proxyTracing) logFilterEvent(span ot.Span, filterName, event string) {
	if !t.logFilterLifecycleEvents {
		return
	}

	t.logEvent(span, filterName, event)
}

func (t *proxyTracing) logStreamEvent(span ot.Span, eventName, eventValue string) {
	if !t.logStreamEvents {
		return
	}

	t.logEvent(span, eventName, eventValue)
}

func (t *proxyTracing) logFilterStart(span ot.Span, filterName string) {
	t.logFilterEvent(span, filterName, StartEvent)
}

func (t *proxyTracing) logFilterEnd(span ot.Span, filterName string) {
	t.logFilterEvent(span, filterName, EndEvent)
}
