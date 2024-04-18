package openpolicyagent

import (
	"net/http"

	"github.com/open-policy-agent/opa/plugins"
	opatracing "github.com/open-policy-agent/opa/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/proxy"
)

const (
	spanName = "open-policy-agent.http"
)

func init() {
	opatracing.RegisterHTTPTracing(&tracingFactory{})
}

type tracingFactory struct{}

type transport struct {
	tracer     opentracing.Tracer
	bundleName string
	manager    *plugins.Manager

	wrapped http.RoundTripper
}

func (*tracingFactory) NewTransport(tr http.RoundTripper, opts opatracing.Options) http.RoundTripper {
	var tracer opentracing.Tracer

	if len(opts) < 3 {
		panic("insufficient tracing options provided for outbound http calls from Open Policy Agent")
	}

	if opts[0] != nil {
		var ok bool
		tracer, ok = opts[0].(opentracing.Tracer)
		if !ok {
			panic("invalid type for first argument of tracing options, expected opentracing.Tracer")
		}
	}

	bundleName, ok := opts[1].(string)
	if !ok {
		panic("invalid type for second argument of tracing options, expected string")
	}

	manager, ok := opts[2].(*plugins.Manager)
	if !ok {
		panic("invalid type third argument of tracing options, expected *plugins.Manager")
	}

	return &transport{
		tracer:     tracer,
		bundleName: bundleName,
		manager:    manager,
		wrapped:    tr,
	}
}

func (*tracingFactory) NewHandler(f http.Handler, label string, opts opatracing.Options) http.Handler {
	return f
}

func (tr *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	parentSpan := opentracing.SpanFromContext(ctx)
	var span opentracing.Span

	if parentSpan != nil {
		span = parentSpan.Tracer().StartSpan(spanName, opentracing.ChildOf(parentSpan.Context()))
	} else if tr.tracer != nil {
		span = tr.tracer.StartSpan(spanName)
	}

	if span != nil {
		defer span.Finish()

		span.SetTag(proxy.HTTPMethodTag, req.Method)
		span.SetTag(proxy.HTTPUrlTag, req.URL.String())
		span.SetTag(proxy.HostnameTag, req.Host)
		span.SetTag(proxy.HTTPPathTag, req.URL.Path)
		span.SetTag(proxy.ComponentTag, "skipper")
		span.SetTag(proxy.SpanKindTag, proxy.SpanKindClient)

		setSpanTags(span, tr.bundleName, tr.manager)
		req = req.WithContext(opentracing.ContextWithSpan(ctx, span))

		carrier := opentracing.HTTPHeadersCarrier(req.Header)
		span.Tracer().Inject(span.Context(), opentracing.HTTPHeaders, carrier)
	}

	resp, err := tr.wrapped.RoundTrip(req)
	if err != nil && span != nil {
		span.SetTag("error", true)
		span.LogKV("event", "error", "message", err.Error())
	}

	return resp, err
}
