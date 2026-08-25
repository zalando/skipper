package openpolicyagent

import (
	"bytes"
	"io"

	"net/http"

	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
)

func (opa *OpenPolicyAgentInstance) ServeInvalidDecisionError(fc filters.FilterContext, span opentracing.Span, result *envoyauth.EvalResult, err error) {
	opa.HandleInvalidDecisionError(fc, span, result, err, true)
}

func (opa *OpenPolicyAgentInstance) HandleInvalidDecisionError(fc filters.FilterContext, span opentracing.Span, result *envoyauth.EvalResult, err error, serve bool) {
	opa.HandleEvaluationError(fc, span, result, err, serve, http.StatusInternalServerError)
}

func (opa *OpenPolicyAgentInstance) HandleInstanceNotReadyError(fc filters.FilterContext, span opentracing.Span, serve bool) {
	span.SetTag("error", true)

	span.LogKV(
		"event", "error",
		"message", "Open Policy Agent instance is not ready yet",
	)

	opa.Logger().Info("Open Policy Agent instance is not ready yet, returning unavailable")

	if serve {
		resp := http.Response{}
		resp.StatusCode = http.StatusServiceUnavailable

		fc.Serve(&resp)
	}
}

func (opa *OpenPolicyAgentInstance) HandleEvaluationError(fc filters.FilterContext, span opentracing.Span, result *envoyauth.EvalResult, err error, serve bool, status int) {
	fc.Metrics().IncCounter(opa.MetricsKey("decision.err"))
	span.SetTag("error", true)

	if result != nil {
		span.LogKV(
			"event", "error",
			"opa.decision_id", result.DecisionID,
			"message", err.Error(),
		)

		opa.Logger().WithFields(map[string]any{
			"decision":    result.Decision,
			"err":         err,
			"decision_id": result.DecisionID,
		}).Info("Rejecting request because of an invalid decision")
	} else {
		span.LogKV(
			"event", "error",
			"message", err.Error(),
		)

		opa.Logger().WithFields(map[string]any{
			"err": err,
		}).Info("Rejecting request because of an error")
	}

	if serve {
		resp := http.Response{}
		resp.StatusCode = status

		fc.Serve(&resp)
	}
}

func (opa *OpenPolicyAgentInstance) ServeResponse(fc filters.FilterContext, span opentracing.Span, result *envoyauth.EvalResult) {
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
