package openpolicyagent

import (
	"context"
	"fmt"
	"time"

	ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	"github.com/open-policy-agent/opa-envoy-plugin/opa/decisionlog"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/server"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/opentracing/opentracing-go"
)

func (opa *OpenPolicyAgentInstance) Eval(ctx context.Context, req *ext_authz_v3.CheckRequest) (*envoyauth.EvalResult, error) {

	decisionId, err := opa.idGenerator.Generate()
	if err != nil {
		opa.Logger().WithFields(map[string]interface{}{"err": err}).Error("Unable to generate decision ID.")
		return nil, err
	}

	result, stopeval, err := envoyauth.NewEvalResult(withDecisionID(decisionId))
	if err != nil {
		opa.Logger().WithFields(map[string]interface{}{"err": err}).Error("Unable to generate new result with decision ID.")
		return nil, err
	}

	span := opentracing.SpanFromContext(ctx)
	if span != nil {
		span.SetTag("opa.decision_id", result.DecisionID)
	}

	var input map[string]interface{}
	defer func() {
		stopeval()
		if topdown.IsCancel(err) {
			// If the evaluation was canceled, we don't want to log the decision.
			return
		}

		err := opa.logDecision(ctx, input, result, err)
		if err != nil {
			opa.Logger().WithFields(map[string]interface{}{"err": err}).Error("Unable to log decision to control plane.")
		}
	}()

	if ctx.Err() != nil {
		return nil, fmt.Errorf("check request timed out before query execution: %w", ctx.Err())
	}

	logger := opa.manager.Logger().WithFields(map[string]interface{}{"decision-id": result.DecisionID})
	input, err = envoyauth.RequestToInput(req, logger, nil, opa.EnvoyPluginConfig().SkipRequestBodyParse)
	if err != nil {
		return nil, fmt.Errorf("failed to convert request to input: %w", err)
	}

	inputValue, err := ast.InterfaceToValue(input)
	if err != nil {
		return nil, err
	}

	err = envoyauth.Eval(ctx, opa, inputValue, result, rego.DistributedTracingOpts(opa.DistributedTracing()))
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (opa *OpenPolicyAgentInstance) logDecision(ctx context.Context, input interface{}, result *envoyauth.EvalResult, err error) error {
	info := &server.Info{
		Timestamp: time.Now(),
		Input:     &input,
	}

	if opa.EnvoyPluginConfig().Path != "" {
		info.Path = opa.EnvoyPluginConfig().Path
	}

	return decisionlog.LogDecision(ctx, opa.manager, info, result, err)
}

func withDecisionID(decisionID string) func(*envoyauth.EvalResult) {
	return func(result *envoyauth.EvalResult) {
		result.DecisionID = decisionID
	}
}
