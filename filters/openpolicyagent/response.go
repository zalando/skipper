package openpolicyagent

import (
	"bytes"
	"io"

	"net/http"

	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (opa *OpenPolicyAgentInstance) ServeInvalidDecisionError(fc filters.FilterContext, span trace.Span, result *envoyauth.EvalResult, err error) {
	opa.HandleInvalidDecisionError(fc, span, result, err, true)
}

func (opa *OpenPolicyAgentInstance) HandleInvalidDecisionError(fc filters.FilterContext, span trace.Span, result *envoyauth.EvalResult, err error, serve bool) {
	opa.HandleEvaluationError(fc, span, result, err, serve, http.StatusInternalServerError)
}

func (opa *OpenPolicyAgentInstance) HandleEvaluationError(fc filters.FilterContext, span trace.Span, result *envoyauth.EvalResult, err error, serve bool, status int) {
	fc.Metrics().IncCounter(opa.MetricsKey("decision.err"))
	span.SetAttributes(attribute.Bool(tracing.ErrorTag, true))

	if result != nil {
		span.AddEvent(
			"error",
			trace.WithAttributes(attribute.String("opa.decision_id", result.DecisionID)),
			trace.WithAttributes(attribute.String("message", err.Error())),
		)

		opa.Logger().WithFields(map[string]interface{}{
			"decision":    result.Decision,
			"err":         err,
			"decision_id": result.DecisionID,
		}).Info("Rejecting request because of an invalid decision")
	} else {
		span.AddEvent(
			"error",
			trace.WithAttributes(attribute.String("message", err.Error())),
		)

		opa.Logger().WithFields(map[string]interface{}{
			"err": err,
		}).Info("Rejecting request because of an error")
	}

	if serve {
		resp := http.Response{}
		resp.StatusCode = status

		fc.Serve(&resp)
	}
}

func (opa *OpenPolicyAgentInstance) ServeResponse(fc filters.FilterContext, span trace.Span, result *envoyauth.EvalResult) {
	resp := http.Response{}

	var err error
	resp.StatusCode, err = result.GetResponseHTTPStatus()
	if err != nil {
		opa.ServeInvalidDecisionError(fc, span, result, err)
		return
	}

	resp.Header, err = result.GetResponseHTTPHeaders()
	if err != nil {
		opa.ServeInvalidDecisionError(fc, span, result, err)
		return
	}

	if result.HasResponseBody() {
		body, err := result.GetResponseBody()
		if err != nil {
			opa.ServeInvalidDecisionError(fc, span, result, err)
			return
		}

		resp.Body = io.NopCloser(bytes.NewReader([]byte(body)))
	}

	fc.Serve(&resp)
}
