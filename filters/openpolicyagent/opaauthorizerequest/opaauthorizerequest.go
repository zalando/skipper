package opaauthorizerequest

import (
	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"net/http"
	"time"

	"github.com/zalando/skipper/filters"
	"gopkg.in/yaml.v2"

	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/envoy"
)

const responseHeadersKey = "open-policy-agent:decision-response-headers"

type spec struct {
	registry *openpolicyagent.OpenPolicyAgentRegistry
	opts     []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error
}

func NewOpaAuthorizeRequestSpec(registry *openpolicyagent.OpenPolicyAgentRegistry, opts ...func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error) filters.Spec {
	return &spec{
		registry: registry,
		opts:     opts,
	}
}

func (s *spec) Name() string {
	return filters.OpaAuthorizeRequestName
}

func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	var err error

	if len(args) < 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	if len(args) > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	bundleName, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	envoyContextExtensions := map[string]string{}
	if len(args) > 1 {
		_, ok := args[1].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
		err = yaml.Unmarshal([]byte(args[1].(string)), &envoyContextExtensions)
		if err != nil {
			return nil, err
		}
	}

	configOptions := s.opts

	opaConfig, err := openpolicyagent.NewOpenPolicyAgentConfig(configOptions...)
	if err != nil {
		return nil, err
	}

	opa, err := s.registry.NewOpenPolicyAgentInstance(bundleName, *opaConfig, s.Name())

	if err != nil {
		return nil, err
	}

	return &opaAuthorizeRequestFilter{
		opa:                    opa,
		registry:               s.registry,
		envoyContextExtensions: envoyContextExtensions,
	}, nil
}

type opaAuthorizeRequestFilter struct {
	opa                    *openpolicyagent.OpenPolicyAgentInstance
	registry               *openpolicyagent.OpenPolicyAgentRegistry
	envoyContextExtensions map[string]string
}

func (f *opaAuthorizeRequestFilter) Request(fc filters.FilterContext) {
	req := fc.Request()
	span, ctx := f.opa.StartSpanFromFilterContext(fc)
	defer span.Finish()

	authzreq := envoy.AdaptToExtAuthRequest(req, f.opa.InstanceConfig().GetEnvoyMetadata(), f.envoyContextExtensions)

	start := time.Now()
	result, err := f.opa.Eval(ctx, authzreq)
	fc.Metrics().MeasureSince(f.opa.MetricsKey("eval_time"), start)

	if err != nil {
		f.opa.HandleInvalidDecisionError(fc, span, result, err, !f.opa.EnvoyPluginConfig().DryRun)
		return
	}

	if f.opa.EnvoyPluginConfig().DryRun {
		return
	}

	allowed, err := result.IsAllowed()
	if err != nil {
		f.opa.HandleInvalidDecisionError(fc, span, result, err, !f.opa.EnvoyPluginConfig().DryRun)
		return
	}
	if !allowed {
		fc.Metrics().IncCounter(f.opa.MetricsKey("decision.deny"))
		f.opa.ServeResponse(fc, span, result)
		return
	}

	fc.Metrics().IncCounter(f.opa.MetricsKey("decision.allow"))

	headersToRemove, err := result.GetRequestHTTPHeadersToRemove()
	if err != nil {
		f.opa.HandleInvalidDecisionError(fc, span, result, err, !f.opa.EnvoyPluginConfig().DryRun)
		return
	}
	removeRequestHeaders(fc, headersToRemove)

	headers, err := result.GetResponseHTTPHeaders()
	if err != nil {
		f.opa.HandleInvalidDecisionError(fc, span, result, err, !f.opa.EnvoyPluginConfig().DryRun)
		return
	}
	addRequestHeaders(fc, headers)

	if responseHeaders, err := result.GetResponseHTTPHeadersToAdd(); err != nil {
		f.opa.HandleInvalidDecisionError(fc, span, result, err, !f.opa.EnvoyPluginConfig().DryRun)
		return
	} else if len(responseHeaders) > 0 {
		fc.StateBag()[responseHeadersKey] = responseHeaders
	}
}

func removeRequestHeaders(fc filters.FilterContext, headersToRemove []string) {
	for _, header := range headersToRemove {
		fc.Request().Header.Del(header)
	}
}

func addRequestHeaders(fc filters.FilterContext, headers http.Header) {
	for key, values := range headers {
		for _, value := range values {
			// This is the default behavior from https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/base.proto#config-core-v3-headervalueoption
			fc.Request().Header.Add(key, value)
		}
	}
}
func (f *opaAuthorizeRequestFilter) Response(fc filters.FilterContext) {
	if headers, ok := fc.StateBag()[responseHeadersKey].([]*ext_authz_v3_core.HeaderValueOption); ok {
		addResponseHeaders(fc, headers)
	}
}

func addResponseHeaders(fc filters.FilterContext, headersToAdd []*ext_authz_v3_core.HeaderValueOption) {
	for _, headerToAdd := range headersToAdd {
		header := headerToAdd.GetHeader()
		fc.Response().Header.Add(header.GetKey(), header.GetValue())
	}
}

func (f *opaAuthorizeRequestFilter) OpenPolicyAgent() *openpolicyagent.OpenPolicyAgentInstance {
	return f.opa
}
