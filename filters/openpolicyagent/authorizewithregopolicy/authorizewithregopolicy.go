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

type spec struct {
	registry *openpolicyagent.OpenPolicyAgentRegistry
	opts     []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error
}

func NewAuthorizeWithRegoPolicySpec(registry *openpolicyagent.OpenPolicyAgentRegistry, opts ...func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error) filters.Spec {
	return &spec{
		registry: registry,
		opts:     opts,
	}
}

func (s *spec) Name() string {
	return filters.AuthorizeWithRegoPolicyName
}

func (s *spec) CreateFilter(config []interface{}) (filters.Filter, error) {
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

	envoyContextExtensions := map[string]string{}
	if len(sargs) > 1 {
		err = yaml.Unmarshal([]byte(sargs[1]), &envoyContextExtensions)
		if err != nil {
			return nil, err
		}
	}

	opaConfig, err := openpolicyagent.NewOpenPolicyAgentConfig(configOptions...)
	if err != nil {
		return nil, err
	}

	opa, err := s.registry.NewOpenPolicyAgentInstance(bundleName, *opaConfig, s.Name())

	if err != nil {
		return nil, err
	}

	return &authorizeWithRegoPolicyFilter{
		opa:                    opa,
		registry:               s.registry,
		envoyContextExtensions: envoyContextExtensions,
	}, nil
}

type authorizeWithRegoPolicyFilter struct {
	opa                    *openpolicyagent.OpenPolicyAgentInstance
	registry               *openpolicyagent.OpenPolicyAgentRegistry
	envoyContextExtensions map[string]string
}

func (f *authorizeWithRegoPolicyFilter) Request(fc filters.FilterContext) {
	req := fc.Request()
	span, ctx := f.opa.StartSpanFromFilterContext(fc)
	defer span.Finish()

	authzreq := envoy.AdaptToExtAuthRequest(req, f.opa.InstanceConfig().GetEnvoyMetadata(), f.envoyContextExtensions)

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

func (*authorizeWithRegoPolicyFilter) Response(filters.FilterContext) {}

func (f *authorizeWithRegoPolicyFilter) Close() error {
	return f.registry.ReleaseInstance(f.opa)
}
