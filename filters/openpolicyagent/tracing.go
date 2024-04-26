package openpolicyagent

import (
	"net/http"

	"github.com/open-policy-agent/opa/plugins"
	opatracing "github.com/open-policy-agent/opa/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/proxy"
)

const (
	spanNameHttpOut = "open-policy-agent.http"
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

func WithTracingOptTracer(tracer opentracing.Tracer) func(*transport) {
	return func(t *transport) {
		t.tracer = tracer
	}
}

func WithTracingOptBundleName(bundleName string) func(*transport) {
	return func(t *transport) {
		t.bundleName = bundleName
	}
}

func WithTracingOptManager(manager *plugins.Manager) func(*transport) {
	return func(t *transport) {
		t.manager = manager
	}
}

func (*tracingFactory) NewTransport(tr http.RoundTripper, opts opatracing.Options) http.RoundTripper {
	log := &logging.DefaultLog{}

	wrapper := &transport{
		wrapped: tr,
	}

	for _, o := range opts {
		opt, ok := o.(func(*transport))
		if !ok {
			log.Warnf("invalid type for OPA tracing option, expected func(*transport) got %T, tracing information might be incomplete", o)
		} else {
			opt(wrapper)
		}
	}

	return wrapper
}

func (*tracingFactory) NewHandler(f http.Handler, label string, opts opatracing.Options) http.Handler {
	return f
}

func (tr *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	parentSpan := opentracing.SpanFromContext(ctx)
	var span opentracing.Span

	if parentSpan != nil {
		span = parentSpan.Tracer().StartSpan(spanNameHttpOut, opentracing.ChildOf(parentSpan.Context()))
	} else if tr.tracer != nil {
		span = tr.tracer.StartSpan(spanNameHttpOut)
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
