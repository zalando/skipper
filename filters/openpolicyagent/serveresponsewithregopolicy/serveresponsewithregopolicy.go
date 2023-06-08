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
	factory openpolicyagent.OpenPolicyAgentFactory
	opts    []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error
}

func NewServeResponseWithRegoPolicySpec(factory openpolicyagent.OpenPolicyAgentFactory, opts ...func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error) Spec {
	return Spec{
		factory: factory,
		opts:    opts,
	}
}

func (s Spec) Name() string {
	return filters.ServeResponseWithRegoPolicyName
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

	return serveResponseWithRegoPolicyFilter{
		opa: opa,
	}, nil
}

type serveResponseWithRegoPolicyFilter struct {
	opa *openpolicyagent.OpenPolicyAgentInstance
}

func (f serveResponseWithRegoPolicyFilter) Request(fc filters.FilterContext) {
	req := fc.Request()
	span, ctx := f.opa.StartSpanFromContext(req.Context())
	defer span.Finish()

	authzreq := envoy.AdaptToEnvoyExtAuthRequest(fc.Request(), f.opa.InstanceConfig().GetEnvoyMetadata(), f.opa.InstanceConfig().GetEnvoyContextExtensions())

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

	if err != nil {
		f.opa.RejectInvalidDecisionError(fc, span, result, err)
	} else {
		f.opa.ServeResponse(fc, span, result)
	}
}

func (f serveResponseWithRegoPolicyFilter) Response(fc filters.FilterContext) {}
