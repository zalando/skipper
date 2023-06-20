package openpolicyagent

import (
	"bytes"
	"io"

	"net/http"

	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
)

func (opa *OpenPolicyAgentInstance) RejectInvalidDecisionError(fc filters.FilterContext, span opentracing.Span, result *envoyauth.EvalResult, err error) {
	resp := http.Response{}

	fc.Metrics().IncCounter(opa.MetricsKey("decision.err"))

	resp.StatusCode = http.StatusInternalServerError
	span.SetTag("error", true)

	if result != nil {
		span.LogKV(
			"event", "error",
			"opa.decision_id", result.DecisionID,
			"message", err.Error(),
		)

		opa.Logger().WithFields(map[string]interface{}{
			"decision":    result.Decision,
			"err":         err,
			"decision_id": result.DecisionID,
		}).Info("Rejecting request because of an invalid decision")
	} else {
		span.LogKV(
			"event", "error",
			"message", err.Error(),
		)

		opa.Logger().WithFields(map[string]interface{}{
			"err": err,
		}).Info("Rejecting request because of an invalid decision")
	}

	fc.Serve(&resp)
}

func (opa *OpenPolicyAgentInstance) ServeResponse(fc filters.FilterContext, span opentracing.Span, result *envoyauth.EvalResult) {
	resp := http.Response{}

	var err error
	resp.StatusCode, err = result.GetResponseHTTPStatus()
	if err != nil {
		opa.RejectInvalidDecisionError(fc, span, result, err)
		return
	}

	resp.Header, err = result.GetResponseHTTPHeaders()
	if err != nil {
		opa.RejectInvalidDecisionError(fc, span, result, err)
		return
	}

	hasbody := result.HasResponseBody()

	if hasbody {
		body, err := result.GetResponseBody()
		if err != nil {
			opa.RejectInvalidDecisionError(fc, span, result, err)
			return
		}

		resp.Body = io.NopCloser(bytes.NewReader([]byte(body)))
	}

	fc.Serve(&resp)
}
