package serveresponsewithregopolicy

import (
	"time"

	"github.com/zalando/skipper/filters"
	"gopkg.in/yaml.v2"

	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/envoy"
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
		f.opa.ServeInvalidDecisionError(fc, span, result, err)

		return
	}

	f.opa.ServeResponse(fc, span, result)
}

func (f *serveResponseWithRegoPolicyFilter) Response(fc filters.FilterContext) {}

func (f *serveResponseWithRegoPolicyFilter) OpenPolicyAgent() *openpolicyagent.OpenPolicyAgentInstance {
	return f.opa
}
