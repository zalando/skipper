package serveresponsewithregopolicy

import (
	"time"

	"github.com/zalando/skipper/filters"
	"gopkg.in/yaml.v2"

	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/envoy"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/util"
)

type Spec struct {
	factory    openpolicyagent.OpenPolicyAgentFactory
	configOpts []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error
}

func NewServeResponseWithRegoPolicySpec(factory openpolicyagent.OpenPolicyAgentFactory, opts ...func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error) Spec {
	return Spec{
		factory:    factory,
		configOpts: opts,
	}
}

func (s Spec) Name() string {
	return "serveResponseWithRegoPolicy"
}

const (
	paramBundleName int = iota
	paramEnvoyContextExtensions
)

func (s Spec) CreateFilter(config []interface{}) (filters.Filter, error) {
	var err error

	sargs, err := util.GetStrings(config)
	if err != nil {
		return nil, err
	}

	if len(sargs) < 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	if len(sargs) > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	bundleName := sargs[paramBundleName]

	configOptions := s.configOpts

	if len(sargs) > 1 {
		envoyContextExtensions := map[string]string{}
		err = yaml.Unmarshal([]byte(sargs[paramEnvoyContextExtensions]), &envoyContextExtensions)
		if err != nil {
			return nil, err
		}
		configOptions = append(configOptions, openpolicyagent.WithEnvoyContextExtensions(envoyContextExtensions))
	}

	opaConfig, err := openpolicyagent.NewOpenPolicyAgentConfig(configOptions...)
	if err != nil {
		return nil, err
	}

	opa, err := s.factory.NewOpenPolicyAgentEnvoyInstance(bundleName, *opaConfig, s.Name())

	if err != nil {
		return nil, err
	}

	return serveResponseWithRegoPolicyFilter{
		opa: opa,
	}, nil
}

type serveResponseWithRegoPolicyFilter struct {
	opa *openpolicyagent.OpenPolicyAgentInstance
}

func (f serveResponseWithRegoPolicyFilter) Request(fc filters.FilterContext) {
	start := time.Now()

	req := envoy.AdaptToEnvoyExtAuthRequest(fc.Request(), f.opa.InstanceConfig().GetPolicyType(), f.opa.InstanceConfig().GetEnvoyContextExtensions())

	result, err := f.opa.Eval(fc.Request().Context(), req)

	if err != nil {
		f.opa.RejectInvalidDecisionError(fc, result, err)
		return
	}

	f.opa.Logger().WithFields(map[string]interface{}{
		"query":               f.opa.EnvoyPluginConfig().ParsedQuery.String(),
		"dry-run":             f.opa.EnvoyPluginConfig().DryRun,
		"decision":            result.Decision,
		"err":                 err,
		"txn":                 result.TxnID,
		"metrics":             result.Metrics.All(),
		"total_decision_time": time.Since(start),
	}).Debug("Authorizing request with decision.")

	if !f.opa.EnvoyPluginConfig().DryRun {
		if err != nil {
			f.opa.RejectInvalidDecisionError(fc, result, err)
		} else {
			f.opa.ServeResponse(fc, result)
		}
	}
}

func (f serveResponseWithRegoPolicyFilter) Response(fc filters.FilterContext) {}
