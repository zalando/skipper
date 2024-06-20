package opaserveresponse

import (
	"io"
	"net/http"
	"time"

	"github.com/zalando/skipper/filters"
	"gopkg.in/yaml.v2"

	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/envoy"
)

type spec struct {
	registry    *openpolicyagent.OpenPolicyAgentRegistry
	opts        []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error
	name        string
	bodyParsing bool
}

func NewOpaServeResponseSpec(registry *openpolicyagent.OpenPolicyAgentRegistry, opts ...func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error) filters.Spec {
	return &spec{
		registry:    registry,
		opts:        opts,
		name:        filters.OpaServeResponseName,
		bodyParsing: false,
	}
}

func NewOpaServeResponseWithReqBodySpec(registry *openpolicyagent.OpenPolicyAgentRegistry, opts ...func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error) filters.Spec {
	return &spec{
		registry:    registry,
		opts:        opts,
		name:        filters.OpaServeResponseWithReqBodyName,
		bodyParsing: true,
	}
}

func (s *spec) Name() string {
	return s.name
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

	return &opaServeResponseFilter{
		opa:                    opa,
		registry:               s.registry,
		envoyContextExtensions: envoyContextExtensions,
		bodyParsing:            s.bodyParsing,
	}, nil
}

type opaServeResponseFilter struct {
	opa                    *openpolicyagent.OpenPolicyAgentInstance
	registry               *openpolicyagent.OpenPolicyAgentRegistry
	envoyContextExtensions map[string]string
	bodyParsing            bool
}

func (f *opaServeResponseFilter) Request(fc filters.FilterContext) {
	span, ctx := f.opa.StartSpanFromFilterContext(fc)
	defer span.Finish()
	req := fc.Request()

	var rawBodyBytes []byte
	if f.bodyParsing {
		var body io.ReadCloser
		var err error
		var finalizer func()
		body, rawBodyBytes, finalizer, err = f.opa.ExtractHttpBodyOptionally(req)
		defer finalizer()
		if err != nil {
			f.opa.ServeInvalidDecisionError(fc, span, nil, err)
			return
		}
		req.Body = body
	}

	authzreq, err := envoy.AdaptToExtAuthRequest(fc.Request(), f.opa.InstanceConfig().GetEnvoyMetadata(), f.envoyContextExtensions, rawBodyBytes)
	if err != nil {
		f.opa.HandleEvaluationError(fc, span, nil, err, !f.opa.EnvoyPluginConfig().DryRun, http.StatusBadRequest)
		return
	}

	start := time.Now()
	result, err := f.opa.Eval(ctx, authzreq)
	fc.Metrics().MeasureSince(f.opa.MetricsKey("eval_time"), start)
	if err != nil {
		f.opa.ServeInvalidDecisionError(fc, span, result, err)

		return
	}

	f.opa.ServeResponse(fc, span, result)
}

func (f *opaServeResponseFilter) Response(fc filters.FilterContext) {}

func (f *opaServeResponseFilter) OpenPolicyAgent() *openpolicyagent.OpenPolicyAgentInstance {
	return f.opa
}
