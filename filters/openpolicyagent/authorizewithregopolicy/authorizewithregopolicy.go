package authorizewithregopolicy

import (
	"net/http"
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

func NewAuthorizeWithRegoPolicySpec(factory openpolicyagent.OpenPolicyAgentFactory, opts ...func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error) Spec {
	return Spec{
		factory:    factory,
		configOpts: opts,
	}
}

func (s Spec) Name() string {
	return "authorizeWithRegoPolicy"
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

	return authorizeWithRegoPolicyFilter{
		opa: opa,
	}, nil
}

type authorizeWithRegoPolicyFilter struct {
	opa *openpolicyagent.OpenPolicyAgentInstance
}

func (f authorizeWithRegoPolicyFilter) Request(fc filters.FilterContext) {
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
		allowed, err := result.IsAllowed()

		if err != nil {
			f.opa.RejectInvalidDecisionError(fc, result, err)
			return
		}

		if !allowed {
			f.opa.ServeResponse(fc, result)
			return
		}

		if result.HasResponseBody() {
			body, err := result.GetResponseBody()
			if err != nil {
				f.opa.RejectInvalidDecisionError(fc, result, err)
				return
			}
			fc.StateBag()[openpolicyagent.OpenPolicyAgentDecisionBodyKey] = body
		}

		headers, err := result.GetResponseHTTPHeaders()
		if err != nil {
			f.opa.RejectInvalidDecisionError(fc, result, err)
			return
		}
		addRequestHeaders(fc, headers)

		headersToRemove, err := result.GetRequestHTTPHeadersToRemove()
		if err != nil {
			f.opa.RejectInvalidDecisionError(fc, result, err)
			return
		}
		removeHeaders(fc, headersToRemove)
	}
}

func removeHeaders(fc filters.FilterContext, headersToRemove []string) {
	for _, header := range headersToRemove {
		fc.Request().Header.Del(header)
	}
}

func addRequestHeaders(fc filters.FilterContext, headers http.Header) {
	for key, values := range headers {
		for _, value := range values {
			fc.Request().Header.Add(key, value)
		}
	}
}

func (f authorizeWithRegoPolicyFilter) Response(fc filters.FilterContext) {}
