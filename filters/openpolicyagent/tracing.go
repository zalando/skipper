package openpolicyagent

import (
	"net/http"

	opatracing "github.com/open-policy-agent/opa/tracing"
	"github.com/zalando/skipper/tracing"
	"go.opentelemetry.io/otel/trace"
)

type tracingFactory struct {
	tracer trace.Tracer
}

type transport struct {
	opa     *OpenPolicyAgentInstance
	tracer  trace.Tracer
	wrapped http.RoundTripper
}

func (tf *tracingFactory) NewTransport(tr http.RoundTripper, opts opatracing.Options) http.RoundTripper {
	return &transport{
		opa:     opts[0].(*OpenPolicyAgentInstance),
		wrapped: tr,
		tracer:  tf.tracer,
	}
}

func (*tracingFactory) NewHandler(f http.Handler, label string, opts opatracing.Options) http.Handler {
	return f
}

func (tr *transport) RoundTrip(req *http.Request) (*http.Response, error) {
    if tr.tracer == nil {
	    return tr.wrapped.RoundTrip(req)
    }



	ctx, span := tr.tracer.Start(req.Context(), "http.send")
	defer span.End()
	req = req.WithContext(ctx)
	tracing.Inject(ctx, req, span, tr.tracer)

	return tr.wrapped.RoundTrip(req)
}
