//go:build opa

package skipper

import (
	ot "github.com/opentracing/opentracing-go"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/filters/openpolicyagent/opaauthorizerequest"
	"github.com/zalando/skipper/filters/openpolicyagent/opaserveresponse"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

func newOpenPolicyAgent(o *Options, tracer ot.Tracer, mtr metrics.Metrics) (*openPolicyAgent, error) {
	if !o.EnableOpenPolicyAgent {
		return &openPolicyAgent{close: func() {}}, nil
	}

	opts := make([]func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error, 0)
	opts = append(opts,
		openpolicyagent.WithConfigTemplateFile(o.OpenPolicyAgentConfigTemplate))
	if o.OpenPolicyAgentEnvoyMetadata != "" {
		opts = append(opts, openpolicyagent.WithEnvoyMetadataFile(o.OpenPolicyAgentEnvoyMetadata))
	}

	opaRegistryOpts := []func(*openpolicyagent.OpenPolicyAgentRegistry) error{
		openpolicyagent.WithMaxRequestBodyBytes(o.OpenPolicyAgentMaxRequestBodySize),
		openpolicyagent.WithMaxMemoryBodyParsing(o.OpenPolicyAgentMaxMemoryBodyParsing),
		openpolicyagent.WithReadBodyBufferSize(o.OpenPolicyAgentRequestBodyBufferSize),
		openpolicyagent.WithCleanInterval(o.OpenPolicyAgentCleanerInterval),
		openpolicyagent.WithInstanceStartupTimeout(o.OpenPolicyAgentStartupTimeout),
		openpolicyagent.WithTracer(tracer),
		openpolicyagent.WithEnableCustomControlLoop(o.EnableOpenPolicyAgentCustomControlLoop),
		openpolicyagent.WithEnablePrintTracing(o.EnableOpenPolicyAgentPrintTracing),
		openpolicyagent.WithControlLoopInterval(o.OpenPolicyAgentControlLoopInterval),
		openpolicyagent.WithControlLoopMaxJitter(o.OpenPolicyAgentControlLoopMaxJitter),
		openpolicyagent.WithEnableDataPreProcessingOptimization(o.EnableOpenPolicyAgentDataPreProcessingOptimization),
		openpolicyagent.WithPreloadingEnabled(o.EnableOpenPolicyAgentPreloading),
		openpolicyagent.WithAsyncDecisionLogging(o.EnableOpenPolicyAgentAsyncDecisionLogging),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...),
	}

	if promMetrics, ok := mtr.(metrics.PrometheusMetrics); ok {
		opaRegistryOpts = append(opaRegistryOpts,
			openpolicyagent.WithPrometheusRegisterer(promMetrics.ScopedPrometheusRegisterer("openpolicyagent")))
	}

	opaRegistry, err := openpolicyagent.NewOpenPolicyAgentRegistry(opaRegistryOpts...)
	if err != nil {
		return nil, err
	}

	opa := &openPolicyAgent{
		filters: []filters.Spec{
			opaauthorizerequest.NewOpaAuthorizeRequestSpec(opaRegistry),
			opaauthorizerequest.NewOpaAuthorizeRequestWithBodySpec(opaRegistry),
			opaserveresponse.NewOpaServeResponseSpec(opaRegistry),
			opaserveresponse.NewOpaServeResponseWithReqBodySpec(opaRegistry),
		},
		postProcessors: []routing.PostProcessor{opaRegistry},
		close:          opaRegistry.Close,
	}
	if o.EnableOpenPolicyAgentPreloading {
		opa.preProcessors = []routing.PreProcessor{opaRegistry.NewPreProcessor()}
	}
	return opa, nil
}
