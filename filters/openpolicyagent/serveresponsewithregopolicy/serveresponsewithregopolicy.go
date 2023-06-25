package serveresponsewithregopolicy

import (
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

func NewServeResponseWithRegoPolicySpec(registry *openpolicyagent.OpenPolicyAgentRegistry, opts ...func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error) filters.Spec {
	return &spec{
		registry: registry,
		opts:     opts,
	}
}

func (s *spec) Name() string {
	return filters.ServeResponseWithRegoPolicyName
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

	return &serveResponseWithRegoPolicyFilter{
		opa:                    opa,
		registry:               s.registry,
		envoyContextExtensions: envoyContextExtensions,
	}, nil
}

type serveResponseWithRegoPolicyFilter struct {
	opa                    *openpolicyagent.OpenPolicyAgentInstance
	registry               *openpolicyagent.OpenPolicyAgentRegistry
	envoyContextExtensions map[string]string
}

func (f *serveResponseWithRegoPolicyFilter) Request(fc filters.FilterContext) {
	span, ctx := f.opa.StartSpanFromFilterContext(fc)
	defer span.Finish()

	authzreq := envoy.AdaptToExtAuthRequest(fc.Request(), f.opa.InstanceConfig().GetEnvoyMetadata(), f.envoyContextExtensions)

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

func (f *serveResponseWithRegoPolicyFilter) Response(fc filters.FilterContext) {}

func (f *serveResponseWithRegoPolicyFilter) Close() error {
	return f.registry.ReleaseInstance(f.opa)
}
