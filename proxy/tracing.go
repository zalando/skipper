package proxy

import (
	ot "github.com/opentracing/opentracing-go"

	"github.com/zalando/skipper/tracing"
)

const (
	ClientRequestStateTag = "client.request"
	ComponentTag          = "component"
	ErrorTag              = "error"
	BlockTag              = "blocked"
	FlowIDTag             = "flow_id"
	HostnameTag           = "hostname"
	HTTPHostTag           = "http.host"
	HTTPMethodTag         = "http.method"
	HTTPRemoteIPTag       = "http.remote_ip"
	HTTPPathTag           = "http.path"
	HTTPUrlTag            = "http.url"
	NetworkPeerAddressTag = "network.peer.address"
	HTTPStatusCodeTag     = "http.status_code"
	SkipperRouteIDTag     = "skipper.route_id"
	SpanKindTag           = "span.kind"

	ClientRequestCanceled = "canceled"
	SpanKindClient        = "client"
	SpanKindServer        = "server"

	EndEvent           = "end"
	StartEvent         = "start"
	StreamHeadersEvent = "stream_Headers"
	StreamBodyEvent    = "streamBody.byte"
	StreamBodyError    = "streamBody error"
)

type proxyTracing struct {
	tracer                   ot.Tracer
	initialOperationName     string
	disableFilterSpans       bool
	logFilterLifecycleEvents bool
	logStreamEvents          bool
	excludeTags              map[string]bool
}

type filterTracing struct {
	span      ot.Span
	logEvents bool
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
		disableFilterSpans:       p.DisableFilterSpans,
		logFilterLifecycleEvents: p.LogFilterEvents,
		logStreamEvents:          p.LogStreamEvents,
		excludeTags:              excludedTags,
	}
}

func (t *proxyTracing) logEvent(span ot.Span, eventName, eventValue string) {
	if span == nil {
		return
	}

	span.LogKV(eventName, ensureUTF8(eventValue))
}

func (t *proxyTracing) setTag(span ot.Span, key string, value interface{}) *proxyTracing {
	if span == nil {
		return t
	}

	if !t.excludeTags[key] {
		if s, ok := value.(string); ok {
			span.SetTag(key, ensureUTF8(s))
		} else {
			span.SetTag(key, value)
		}
	}

	return t
}

func (t *proxyTracing) logStreamEvent(span ot.Span, eventName, eventValue string) {
	if !t.logStreamEvents {
		return
	}

	t.logEvent(span, eventName, ensureUTF8(eventValue))
}

func (t *proxyTracing) startFilterTracing(operation string, ctx *context) *filterTracing {
	if t.disableFilterSpans {
		return nil
	}
	span := tracing.CreateSpan(operation, ctx.request.Context(), t.tracer)
	ctx.parentSpan = span

	return &filterTracing{span, t.logFilterLifecycleEvents}
}

func (t *filterTracing) finish() {
	if t != nil {
		t.span.Finish()
	}
}

func (t *filterTracing) logStart(filterName string) {
	if t != nil && t.logEvents {
		t.span.LogKV(filterName, StartEvent)
	}
}

func (t *filterTracing) logEnd(filterName string) {
	if t != nil && t.logEvents {
		t.span.LogKV(filterName, EndEvent)
	}
}
