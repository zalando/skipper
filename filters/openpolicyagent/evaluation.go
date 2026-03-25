package openpolicyagent

import (
	"context"
	"fmt"
	"runtime/pprof"
	"time"

	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	"github.com/open-policy-agent/opa-envoy-plugin/opa/decisionlog"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/plugins/logs"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/server"
	"github.com/open-policy-agent/opa/v1/topdown"
	"github.com/open-policy-agent/opa/v1/topdown/print"
	"github.com/opentracing/opentracing-go"
	pbstruct "google.golang.org/protobuf/types/known/structpb"
)

func (opa *OpenPolicyAgentInstance) Eval(ctx context.Context, req *ext_authz_v3.CheckRequest) (*envoyauth.EvalResult, error) {

	decisionId, err := opa.idGenerator.Generate()
	if err != nil {
		opa.Logger().WithFields(map[string]interface{}{"err": err}).Error("Unable to generate decision ID.")
		return nil, err
	}

	err = setDecisionIdInRequest(req, decisionId)
	if err != nil {
		opa.Logger().WithFields(map[string]interface{}{"err": err}).Error("Unable to set decision ID in Request.")
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

	logger := opa.Logger().WithFields(map[string]interface{}{"decision-id": result.DecisionID})

	pprof.Do(ctx, pprof.Labels("opa.decision_id", decisionId), func(ctx context.Context) {
		input, err = envoyauth.RequestToInput(req, logger, nil, opa.EnvoyPluginConfig().SkipRequestBodyParse)
		if err != nil {
			err = fmt.Errorf("failed to convert request to input: %w", err)
			return
		}

		var inputValue ast.Value
		inputValue, err = ast.InterfaceToValue(input)
		if err != nil {
			return
		}

		evalOpts := []rego.EvalOption{}
		if span != nil && opa.registry.enablePrintTracing {
			evalOpts = append(evalOpts, rego.EvalPrintHook(&spanPrintHook{span: span}))
		}
		err = envoyauth.Eval(ctx, &evalContext{opa}, inputValue, result, evalOpts...)
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func setDecisionIdInRequest(req *ext_authz_v3.CheckRequest, decisionId string) error {
	if req.Attributes.MetadataContext == nil {
		req.Attributes.MetadataContext = &ext_authz_v3_core.Metadata{
			FilterMetadata: map[string]*pbstruct.Struct{},
		}
	}

	filterMeta, err := FormOpenPolicyAgentMetaDataObject(decisionId)
	if err != nil {
		return err
	}
	req.Attributes.MetadataContext.FilterMetadata["open_policy_agent"] = filterMeta
	return nil
}

func FormOpenPolicyAgentMetaDataObject(decisionId string) (*pbstruct.Struct, error) {

	innerFields := make(map[string]interface{})
	innerFields["decision_id"] = decisionId

	return pbstruct.NewStruct(innerFields)
}

func (opa *OpenPolicyAgentInstance) logDecision(ctx context.Context, input interface{}, result *envoyauth.EvalResult, err error) error {
	info := &server.Info{
		Timestamp: time.Now(),
		Input:     &input,
	}

	if opa.EnvoyPluginConfig().Path != "" {
		info.Path = opa.EnvoyPluginConfig().Path
	}

	plugin := logs.Lookup(opa.manager)
	if plugin == nil {
		return nil
	}

	return decisionlog.LogDecision(ctx, plugin, info, result, err)
}

func withDecisionID(decisionID string) func(*envoyauth.EvalResult) {
	return func(result *envoyauth.EvalResult) {
		result.DecisionID = decisionID
	}
}

type spanPrintHook struct {
	span opentracing.Span
}

func (h *spanPrintHook) Print(pctx print.Context, msg string) error {
	h.span.LogKV("event", "print", "opa.print.location", pctx.Location.String(), "message", msg)
	return nil
}
