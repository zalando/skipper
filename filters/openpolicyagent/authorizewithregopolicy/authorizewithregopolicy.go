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
	factory openpolicyagent.OpenPolicyAgentFactory
	opts    []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error
}

func NewAuthorizeWithRegoPolicySpec(factory openpolicyagent.OpenPolicyAgentFactory, opts ...func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error) Spec {
	return Spec{
		factory: factory,
		opts:    opts,
	}
}

func (s Spec) Name() string {
	return filters.AuthorizeWithRegoPolicyName
}

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

	bundleName := sargs[0]

	configOptions := s.opts

	if len(sargs) > 1 {
		envoyContextExtensions := map[string]string{}
		err = yaml.Unmarshal([]byte(sargs[1]), &envoyContextExtensions)
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
	req := fc.Request()
	span, ctx := f.opa.StartSpanFromContext(req.Context())
	defer span.Finish()

	authzreq := envoy.AdaptToEnvoyExtAuthRequest(req, f.opa.InstanceConfig().GetEnvoyMetadata(), f.opa.InstanceConfig().GetEnvoyContextExtensions())

	start := time.Now()
	result, err := f.opa.Eval(ctx, authzreq)
	fc.Metrics().MeasureSince(f.opa.MetricsKey("eval_time"), start)

	if err != nil {
		f.opa.RejectInvalidDecisionError(fc, span, result, err)
		return
	}

	if f.opa.EnvoyPluginConfig().DryRun {
		return
	}

	allowed, err := result.IsAllowed()

	if err != nil {
		f.opa.RejectInvalidDecisionError(fc, span, result, err)
		return
	}

	if !allowed {
		fc.Metrics().IncCounter(f.opa.MetricsKey("decision.deny"))
		f.opa.ServeResponse(fc, span, result)
		return
	}

	fc.Metrics().IncCounter(f.opa.MetricsKey("decision.allow"))

	if result.HasResponseBody() {
		body, err := result.GetResponseBody()
		if err != nil {
			f.opa.RejectInvalidDecisionError(fc, span, result, err)
			return
		}
		fc.StateBag()[openpolicyagent.OpenPolicyAgentDecisionBodyKey] = body
	}

	headers, err := result.GetResponseHTTPHeaders()
	if err != nil {
		f.opa.RejectInvalidDecisionError(fc, span, result, err)
		return
	}
	addRequestHeaders(fc, headers)

	headersToRemove, err := result.GetRequestHTTPHeadersToRemove()
	if err != nil {
		f.opa.RejectInvalidDecisionError(fc, span, result, err)
		return
	}
	removeHeaders(fc, headersToRemove)
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
