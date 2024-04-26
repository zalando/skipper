package openpolicyagent

import (
	"context"
	"encoding/json"
	"fmt"
	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	"github.com/open-policy-agent/opa-envoy-plugin/opa/decisionlog"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/server"
	"github.com/open-policy-agent/opa/tracing"
	"github.com/opentracing/opentracing-go"
	pbstruct "google.golang.org/protobuf/types/known/structpb"
	"time"
)

func (opa *OpenPolicyAgentInstance) Eval(ctx context.Context, req *ext_authz_v3.CheckRequest) (*envoyauth.EvalResult, error) {

	decisionId, err := opa.idGenerator.Generate()
	if err != nil {
		opa.Logger().WithFields(map[string]interface{}{"err": err}).Error("Unable to generate decision ID.")
		return nil, err
	}

	setDecisionIdInRequest(req, decisionId)

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

	err = envoyauth.Eval(ctx, opa, inputValue, result, rego.DistributedTracingOpts(tracing.Options{opa}))
	if err != nil {
		return nil, err
	}

	return result, nil
}

func setDecisionIdInRequest(req *ext_authz_v3.CheckRequest, decisionId string) {
	if metaDataContextDoesNotExist(req) {
		req.Attributes.MetadataContext = formFilterMetadata(decisionId)
	} else {
		req.Attributes.MetadataContext.FilterMetadata["open_policy_agent"] = formOpenPolicyAgentMetaDataObject(decisionId)
	}
}

func metaDataContextDoesNotExist(req *ext_authz_v3.CheckRequest) bool {
	return req.Attributes.MetadataContext == nil
}

func formFilterMetadata(decisionId string) *ext_authz_v3_core.Metadata {
	metaData := &ext_authz_v3_core.Metadata{
		FilterMetadata: map[string]*pbstruct.Struct{
			"open_policy_agent": {
				Fields: map[string]*pbstruct.Value{
					"decision_id": {
						Kind: &pbstruct.Value_StringValue{StringValue: decisionId},
					},
				},
			},
		},
	}
	return metaData
}

func formOpenPolicyAgentMetaDataObject(decisionId string) *pbstruct.Struct {
	nestedStruct := &pbstruct.Struct{}
	nestedStruct.Fields = make(map[string]*pbstruct.Value)

	innerFields := make(map[string]interface{})
	innerFields["decision_id"] = decisionId

	innerBytes, _ := json.Marshal(innerFields)
	_ = json.Unmarshal(innerBytes, &nestedStruct)

	return nestedStruct
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
