package openpolicyagent

import (
	"net/http"

	opatracing "github.com/open-policy-agent/opa/tracing"
	"github.com/opentracing/opentracing-go"
)

func init() {
	opatracing.RegisterHTTPTracing(&tracingFactory{})
}

type tracingFactory struct{}

type transport struct {
	opa     *OpenPolicyAgentInstance
	wrapped http.RoundTripper
}

func (*tracingFactory) NewTransport(tr http.RoundTripper, opts opatracing.Options) http.RoundTripper {
	return &transport{
		opa:     opts[0].(*OpenPolicyAgentInstance),
		wrapped: tr,
	}
}

func (*tracingFactory) NewHandler(f http.Handler, label string, opts opatracing.Options) http.Handler {
	return f
}

func (tr *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	parentSpan := opentracing.SpanFromContext(ctx)

	if parentSpan != nil {
		span := parentSpan.Tracer().StartSpan("http.send", opentracing.ChildOf(parentSpan.Context()))
		defer span.Finish()
		req = req.WithContext(opentracing.ContextWithSpan(ctx, span))

		carrier := opentracing.HTTPHeadersCarrier(req.Header)
		span.Tracer().Inject(span.Context(), opentracing.HTTPHeaders, carrier)
	}

	return tr.wrapped.RoundTrip(req)
}
