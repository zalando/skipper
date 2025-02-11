package openpolicyagent

import (
	"net/http"

	"github.com/open-policy-agent/opa/v1/plugins"
	opatracing "github.com/open-policy-agent/opa/v1/tracing"
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

	spanOpts := []opentracing.StartSpanOption{opentracing.Tags{
		proxy.HTTPMethodTag: req.Method,
		proxy.HTTPUrlTag:    req.URL.String(),
		proxy.HostnameTag:   req.Host,
		proxy.HTTPPathTag:   req.URL.Path,
		proxy.ComponentTag:  "skipper",
		proxy.SpanKindTag:   proxy.SpanKindClient,
	}}

	var span opentracing.Span
	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		spanOpts = append(spanOpts, opentracing.ChildOf(parentSpan.Context()))
		span = parentSpan.Tracer().StartSpan(spanNameHttpOut, spanOpts...)
	} else if tr.tracer != nil {
		span = tr.tracer.StartSpan(spanNameHttpOut, spanOpts...)
	}

	if span == nil {
		return tr.wrapped.RoundTrip(req)
	}

	defer span.Finish()

	setSpanTags(span, tr.bundleName, tr.manager)
	req = req.WithContext(opentracing.ContextWithSpan(ctx, span))

	carrier := opentracing.HTTPHeadersCarrier(req.Header)
	span.Tracer().Inject(span.Context(), opentracing.HTTPHeaders, carrier)

	resp, err := tr.wrapped.RoundTrip(req)

	if err != nil {
		span.SetTag("error", true)
		span.LogKV("event", "error", "message", err.Error())
		return resp, err
	}

	span.SetTag(proxy.HTTPStatusCodeTag, resp.StatusCode)

	if resp.StatusCode > 399 {
		span.SetTag("error", true)
		span.LogKV("event", "error", "message", resp.Status)
	}

	return resp, nil
}
