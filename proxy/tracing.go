package proxy

import (
	"github.com/zalando/skipper/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
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
	trace.Tracer
	initialOperationName     string
	disableFilterSpans       bool
	logFilterLifecycleEvents bool
	logStreamEvents          bool
	excludeTags              map[string]bool
}

type filterTracing struct {
	span      trace.Span
	logEvents bool
}

func newProxyTracing(p *OpenTracingParams) *proxyTracing {
	if p == nil {
		p = &OpenTracingParams{}
	}

	if p.InitialSpan == "" {
		p.InitialSpan = "ingress"
	}

	excludedTags := map[string]bool{}

	for _, t := range p.ExcludeTags {
		excludedTags[t] = true
	}

	if p.OtelTracer == nil {
		if p.Tracer != nil {
			p.OtelTracer = &tracing.TracerWrapper{Ot: p.Tracer}
		} else {
			p.OtelTracer = nooptrace.NewTracerProvider().Tracer("noop tracer")
		}
	}

	return &proxyTracing{
		Tracer:                   p.OtelTracer,
		initialOperationName:     p.InitialSpan,
		disableFilterSpans:       p.DisableFilterSpans,
		logFilterLifecycleEvents: p.LogFilterEvents,
		logStreamEvents:          p.LogStreamEvents,
		excludeTags:              excludedTags,
	}
}

func (t *proxyTracing) logEvent(span trace.Span, eventName, eventValue string) {
	if span == nil {
		return
	}

	span.AddEvent(eventName, trace.WithAttributes(attribute.String(eventName, eventValue)))
}

func (t *proxyTracing) logErrorEvent(span trace.Span, message string) {
	if span == nil {
		return
	}

	span.AddEvent(tracing.ErrorTag, trace.WithAttributes(attribute.String("message", message)))
}

func (t *proxyTracing) setTag(span trace.Span, key string, value string) *proxyTracing {
	if span == nil {
		return t
	}

	if !t.excludeTags[key] {
		span.SetAttributes(attribute.String(key, value))
	}

	return t
}

func (t *proxyTracing) logStreamEvent(span trace.Span, eventName, eventValue string) {
	if !t.logStreamEvents {
		return
	}

	t.logEvent(span, eventName, ensureUTF8(eventValue))
}

func (t *proxyTracing) startFilterTracing(operation string, ctx *context) *filterTracing {
	if t.disableFilterSpans {
		return nil
	}

	_, span := t.Start(ctx.request.Context(), operation)
	ctx.parentSpan = span

	return &filterTracing{span, t.logFilterLifecycleEvents}
}

func (t *filterTracing) finish() {
	if t != nil {
		t.span.End()
	}
}

func (t *filterTracing) logStart(filterName string) {
	if t != nil && t.logEvents {
		t.span.AddEvent(filterName, trace.WithAttributes(attribute.String(filterName, StartEvent)))
	}
}

func (t *filterTracing) logEnd(filterName string) {
	if t != nil && t.logEvents {
		t.span.AddEvent(filterName, trace.WithAttributes(attribute.String(filterName, EndEvent)))
	}
}
