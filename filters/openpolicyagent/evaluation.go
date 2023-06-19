package openpolicyagent

import (
	"context"
	"time"

	ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	"github.com/open-policy-agent/opa-envoy-plugin/opa/decisionlog"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/server"
	"github.com/pkg/errors"
)

func (opa *OpenPolicyAgentInstance) Eval(ctx context.Context, req *ext_authz_v3.CheckRequest) (*envoyauth.EvalResult, error) {
	resp, finalFunc, err := opa.eval(ctx, req)

	logErr := finalFunc()
	if logErr != nil {
		opa.Logger().WithFields(map[string]interface{}{"err": logErr}).Error("Unable to log decision to control plane.")
	}

	return resp, err
}

func (opa *OpenPolicyAgentInstance) eval(ctx context.Context, req *ext_authz_v3.CheckRequest) (*envoyauth.EvalResult, func() error, error) {
	var err error

	result, stopeval, err := envoyauth.NewEvalResult()

	if err != nil {
		opa.Logger().WithFields(map[string]interface{}{"err": err}).Error("Unable to generate decision ID.")
		return nil, func() error { return nil }, err
	}

	var input map[string]interface{}
	stop := func() error {
		stopeval()
		logErr := opa.logDecision(ctx, input, result, err)
		if logErr != nil {
			return logErr
		}
		return nil
	}

	if ctx.Err() != nil {
		err = errors.Wrap(ctx.Err(), "check request timed out before query execution")
		return nil, stop, err
	}

	logger := opa.manager.Logger().WithFields(map[string]interface{}{"decision-id": result.DecisionID})
	input, err = envoyauth.RequestToInput(req, logger, nil, true)
	if err != nil {
		return nil, stop, err
	}

	inputValue, err := ast.InterfaceToValue(input)
	if err != nil {
		return nil, stop, err
	}

	err = envoyauth.Eval(ctx, opa, inputValue, result)
	if err != nil {
		return nil, stop, err
	}

	return result, stop, nil
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
