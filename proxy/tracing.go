package proxy

import (
	ot "github.com/opentracing/opentracing-go"

	"github.com/zalando/skipper/net"
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
	SpanKindClient        = net.SpanKindClient
	SpanKindServer        = "server"

	EndEvent           = net.EndEvent
	StartEvent         = net.StartEvent
	StreamHeadersEvent = "stream_Headers"
	StreamBodyEvent    = "streamBody.byte"
	StreamBodyError    = "streamBody error"

	ClientTraceDNS            = net.ClientTraceDNS
	ClientTraceConnect        = net.ClientTraceConnect
	ClientTraceTLS            = net.ClientTraceTLS
	ClientTraceGetConn        = net.ClientTraceGetConn
	ClientTraceGot100Continue = net.ClientTraceGot100Continue
	ClientTraceWroteHeaders   = net.ClientTraceWroteHeaders
	ClientTraceWroteRequest   = net.ClientTraceWroteRequest
	ClientTraceGotFirstByte   = net.ClientTraceGotFirstByte
	ClientTraceHTTPRoundTrip  = "http_roundtrip"
)

type proxyTracing struct {
	tracer                   ot.Tracer
	initialOperationName     string
	clientTraceByTag         bool
	disableFilterSpans       bool
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
		clientTraceByTag:         p.ClientTraceByTag,
		disableFilterSpans:       p.DisableFilterSpans,
		logFilterLifecycleEvents: p.LogFilterEvents,
		logStreamEvents:          p.LogStreamEvents,
		excludeTags:              excludedTags,
	}
}

func (pt *proxyTracing) logEvent(span ot.Span, eventName, eventValue string) {
	if pt.clientTraceByTag {
		pt.setTag(span, eventName, eventValue)
	} else {
		pt.logKV(span, eventName, eventValue)
	}
}

func (pt *proxyTracing) logKV(span ot.Span, key string, value any) {
	if span == nil {
		return
	}
	if s, ok := value.(string); ok {
		span.LogKV(key, ensureUTF8(s))
	} else {
		span.LogKV(key, value)
	}
}

func (pt *proxyTracing) setTag(span ot.Span, key string, value any) *proxyTracing {
	if span == nil {
		return pt
	}

	if !pt.excludeTags[key] {
		if s, ok := value.(string); ok {
			span.SetTag(key, ensureUTF8(s))
		} else {
			span.SetTag(key, value)
		}
	}

	return pt
}

func (pt *proxyTracing) logStreamEvent(span ot.Span, eventName, eventValue string) {
	if !pt.logStreamEvents {
		return
	}

	pt.logEvent(span, eventName, eventValue)
}

func (pt *proxyTracing) startFilterTracing(operation string, ctx *context) *filterTracing {
	if pt.disableFilterSpans {
		return nil
	}
	span := tracing.CreateSpan(operation, ctx.request.Context(), pt.tracer)
	ctx.parentSpan = span

	return &filterTracing{
		span:             span,
		logEvents:        pt.logFilterLifecycleEvents,
		clientTraceByTag: pt.clientTraceByTag,
	}
}

type filterTracing struct {
	span             ot.Span
	logEvents        bool
	clientTraceByTag bool
}

func (ft *filterTracing) finish() {
	if ft != nil {
		ft.span.Finish()
	}
}

func (ft *filterTracing) logStart(filterName string) {
	if ft != nil && ft.logEvents {
		ft.span.LogKV(filterName, StartEvent)
	}
}

func (ft *filterTracing) logEnd(filterName string) {
	if ft != nil && ft.logEvents {
		ft.span.LogKV(filterName, EndEvent)
	}
}
