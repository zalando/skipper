// Package otel provides [OpenTelemetry] integration for Skipper.
//
// [OpenTelemetry]: https://opentelemetry.io/
package otel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/bombsimon/logrusr/v4"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("package", "otel")

// Options configure OpenTelemetry pipeline.
type Options struct {
	// Initialized indicates whether the OpenTelemetry pipeline has been initialized externally.
	// If true [Init] returns immediately without doing anything.
	Initialized bool `yaml:"-"`

	TracesExporter     string             `yaml:"tracesExporter"`
	ExporterOtlp       ExporterOtlp       `yaml:"exporterOtlp"`
	ResourceAttributes map[string]string  `yaml:"resourceAttributes"`
	Propagators        []string           `yaml:"propagators"`
	BatchSpanProcessor BatchSpanProcessor `yaml:"batchSpanProcessor"`

	// Sampler configures the Sampler used by the TracerProvider to decide
	// whether a span is recorded and marked as sampled.
	//
	// Supported values, matching the OTEL_TRACES_SAMPLER environment variable:
	//
	//   - always_on
	//   - always_off
	//   - traceidratio
	//   - parentbased_always_on (default)
	//   - parentbased_always_off
	//   - parentbased_traceidratio
	//
	// If empty, the sampler is taken from the OTEL_TRACES_SAMPLER (and
	// OTEL_TRACES_SAMPLER_ARG) environment variables, falling back to the SDK
	// default of parentbased_always_on if those are not set either.
	//
	// Note that "parentbased_*" samplers respect the sampled flag of an
	// incoming traceparent header and will stop sampling (and therefore stop
	// propagating sampled=1) for the rest of the trace once an upstream hop
	// marks it as not sampled. Use "always_on" to make skipper always record
	// and always propagate sampled=1 regardless of the incoming trace context.
	Sampler string `yaml:"sampler"`

	// SamplerArg configures the argument for the Sampler, if applicable.
	// Only used by the "traceidratio" and "parentbased_traceidratio" samplers,
	// where it is the sampling ratio in the range [0.0, 1.0].
	//
	// If empty, the argument is taken from the OTEL_TRACES_SAMPLER_ARG
	// environment variable.
	SamplerArg string `yaml:"samplerArg"`
}

type ExporterOtlp struct {
	Protocol string            `yaml:"protocol"`
	Endpoint string            `yaml:"endpoint"`
	Headers  map[string]string `yaml:"headers"`
}

type BatchSpanProcessor struct {
	ScheduleDelay      time.Duration `yaml:"scheduleDelay"`
	ExportTimeout      time.Duration `yaml:"exportTimeout"`
	MaxQueueSize       int           `yaml:"maxQueueSize"`
	MaxExportBatchSize int           `yaml:"maxExportBatchSize"`
}

// Init bootstraps the OpenTelemetry pipeline using environment variables and provided options.
// Make sure to call shutdown for proper cleanup if err is nil.
//
// Supported environment variables:
//
//   - OTEL_TRACES_EXPORTER
//   - OTEL_EXPORTER_OTLP_PROTOCOL
//   - OTEL_EXPORTER_OTLP_ENDPOINT
//   - OTEL_EXPORTER_OTLP_HEADERS
//   - OTEL_RESOURCE_ATTRIBUTES
//   - OTEL_PROPAGATORS
//   - OTEL_TRACES_SAMPLER
//   - OTEL_TRACES_SAMPLER_ARG
//   - OTEL_BSP_MAX_QUEUE_SIZE
//   - OTEL_BSP_MAX_EXPORT_BATCH_SIZE
//   - OTEL_BSP_SCHEDULE_DELAY
//   - OTEL_BSP_EXPORT_TIMEOUT
//
// See:
//   - [go.opentelemetry.io/contrib/exporters/autoexport]
//   - [go.opentelemetry.io/contrib/propagators/autoprop].
//   - https://opentelemetry.io/docs/languages/sdk-configuration/general/
//   - https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/
//   - https://github.com/open-telemetry/opentelemetry-specification/blob/main/spec-compliance-matrix.md#environment-variables
func Init(ctx context.Context, o *Options) (shutdown func(context.Context) error, err error) {
	if o.Initialized {
		log.Debug("OpenTelemetry pipeline initialized externally")
		return func(context.Context) error { return nil }, nil
	}

	for _, name := range []string{
		"OTEL_TRACES_EXPORTER",
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		// "OTEL_EXPORTER_OTLP_HEADERS", // may contain sensitive data
		"OTEL_RESOURCE_ATTRIBUTES",
		"OTEL_PROPAGATORS",
		"OTEL_TRACES_SAMPLER",
		"OTEL_TRACES_SAMPLER_ARG",
		"OTEL_BSP_MAX_QUEUE_SIZE",
		"OTEL_BSP_MAX_EXPORT_BATCH_SIZE",
		"OTEL_BSP_SCHEDULE_DELAY",
		"OTEL_BSP_EXPORT_TIMEOUT",
	} {
		log.Debugf("%s: %q", name, os.Getenv(name))
	}
	log.Debugf("Options: %+v", o)

	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) (func(context.Context) error, error) {
		return nil, errors.Join(inErr, shutdown(ctx))
	}

	spanExporter, err := newSpanExporter(ctx, o)
	if err != nil {
		return handleErr(err)
	}
	shutdownFuncs = append(shutdownFuncs, spanExporter.Shutdown)

	batcherOpt := withBatcher(spanExporter, o)

	resourceOpt, err := withResource(o)
	if err != nil {
		return handleErr(err)
	}

	samplerOpt, err := withSampler(o)
	if err != nil {
		return handleErr(err)
	}

	tracerProvider := trace.NewTracerProvider(batcherOpt, resourceOpt, samplerOpt)
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)

	otel.SetTracerProvider(tracerProvider)

	propagator, err := textMapPropagator(o)
	if err != nil {
		return handleErr(err)
	}
	otel.SetTextMapPropagator(propagator)

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { log.Error(err) }))
	otel.SetLogger(logrusr.New(log))

	return
}

// newSpanExporter creates a span exporter based on the provided options or
// environment variables if the options do not specify traces exporter.
// Reads environment variables:
//
//	OTEL_TRACES_EXPORTER
//	OTEL_EXPORTER_OTLP_PROTOCOL
//	OTEL_EXPORTER_OTLP_ENDPOINT
//	OTEL_EXPORTER_OTLP_HEADERS
func newSpanExporter(ctx context.Context, o *Options) (trace.SpanExporter, error) {
	if o.TracesExporter == "otlp" {
		if o.ExporterOtlp.Endpoint == "" {
			return nil, fmt.Errorf("OTLP endpoint is required")
		}
		switch o.ExporterOtlp.Protocol {
		case "grpc":
			return otlptracegrpc.New(ctx,
				otlptracegrpc.WithEndpointURL(o.ExporterOtlp.Endpoint+"/v1/traces"),
				otlptracegrpc.WithHeaders(o.ExporterOtlp.Headers),
			)
		case "":
			fallthrough // to default http/protobuf
		case "http/protobuf":
			return otlptracehttp.New(ctx,
				otlptracehttp.WithEndpointURL(o.ExporterOtlp.Endpoint+"/v1/traces"),
				otlptracehttp.WithHeaders(o.ExporterOtlp.Headers),
			)
		default:
			return nil, fmt.Errorf("invalid OTLP protocol %s - should be one of ['grpc', 'http/protobuf']", o.ExporterOtlp.Protocol)
		}
	} else if o.TracesExporter == "console" {
		return stdouttrace.New()
	} else if o.TracesExporter == "skipper-debug" {
		return skipperDebugSpanExporter(ctx)
	} else {
		log.Debugf("Configuring span exporter using environment variables")
		return autoexport.NewSpanExporter(ctx)
	}
}

// withBatcher registers the exporter with the TracerProvider using
// environment variables possibly overridden by the provided options.
// Reads environment variables:
//
//	OTEL_BSP_MAX_QUEUE_SIZE
//	OTEL_BSP_MAX_EXPORT_BATCH_SIZE
//	OTEL_BSP_SCHEDULE_DELAY
//	OTEL_BSP_EXPORT_TIMEOUT
func withBatcher(spanExporter trace.SpanExporter, o *Options) trace.TracerProviderOption {
	return trace.WithBatcher(spanExporter, func(bspo *trace.BatchSpanProcessorOptions) {
		if o.BatchSpanProcessor.ScheduleDelay != 0 {
			bspo.BatchTimeout = o.BatchSpanProcessor.ScheduleDelay
		}
		if o.BatchSpanProcessor.ExportTimeout != 0 {
			bspo.ExportTimeout = o.BatchSpanProcessor.ExportTimeout
		}
		if o.BatchSpanProcessor.MaxQueueSize != 0 {
			bspo.MaxQueueSize = o.BatchSpanProcessor.MaxQueueSize
		}
		if o.BatchSpanProcessor.MaxExportBatchSize != 0 {
			bspo.MaxExportBatchSize = o.BatchSpanProcessor.MaxExportBatchSize
		}
	})
}

// withResource configures the resource for the TracerProvider using
// environment variables possibly overridden by the provided options.
// Reads environment variable:
//
//	OTEL_RESOURCE_ATTRIBUTES
func withResource(o *Options) (trace.TracerProviderOption, error) {
	r := resource.Environment()
	if len(o.ResourceAttributes) > 0 {
		var attrs []attribute.KeyValue
		for key, val := range o.ResourceAttributes {
			attrs = append(attrs, attribute.String(key, val))
		}

		var err error
		r, err = resource.Merge(r, resource.NewSchemaless(attrs...))
		if err != nil {
			return nil, err
		}
	}
	return trace.WithResource(r), nil
}

// withSampler configures the Sampler for the TracerProvider based on the
// provided options. If Options.Sampler is empty, trace.WithSampler(nil) is
// returned, which is a no-op: the TracerProvider then falls back to the
// OTEL_TRACES_SAMPLER/OTEL_TRACES_SAMPLER_ARG environment variables, or the
// SDK default (parentbased_always_on) if those are not set either.
func withSampler(o *Options) (trace.TracerProviderOption, error) {
	s, err := samplerFromOptions(o)
	if err != nil {
		return nil, err
	}
	return trace.WithSampler(s), nil
}

// samplerFromOptions builds a [trace.Sampler] out of Options.Sampler and
// Options.SamplerArg. It returns a nil Sampler and no error if Options.Sampler
// is empty, letting the caller fall back to environment variables or the SDK
// default.
func samplerFromOptions(o *Options) (trace.Sampler, error) {
	switch o.Sampler {
	case "":
		return nil, nil
	case "always_on":
		return trace.AlwaysSample(), nil
	case "always_off":
		return trace.NeverSample(), nil
	case "traceidratio":
		ratio, err := samplerRatio(o.SamplerArg)
		if err != nil {
			return nil, err
		}
		return trace.TraceIDRatioBased(ratio), nil
	case "parentbased_always_on":
		return trace.ParentBased(trace.AlwaysSample()), nil
	case "parentbased_always_off":
		return trace.ParentBased(trace.NeverSample()), nil
	case "parentbased_traceidratio":
		ratio, err := samplerRatio(o.SamplerArg)
		if err != nil {
			return nil, err
		}
		return trace.ParentBased(trace.TraceIDRatioBased(ratio)), nil
	default:
		return nil, fmt.Errorf("invalid sampler %q - should be one of "+
			"['always_on', 'always_off', 'traceidratio', 'parentbased_always_on', "+
			"'parentbased_always_off', 'parentbased_traceidratio']", o.Sampler)
	}
}

// samplerRatio parses arg as the sampling ratio used by the traceidratio and
// parentbased_traceidratio samplers. An empty arg defaults to a ratio of 1.0,
// matching the OTEL_TRACES_SAMPLER_ARG behavior in the OTel SDK.
func samplerRatio(arg string) (float64, error) {
	if arg == "" {
		return 1.0, nil
	}
	ratio, err := strconv.ParseFloat(arg, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid sampler argument %q: %w", arg, err)
	}
	if ratio < 0.0 || ratio > 1.0 {
		return 0, fmt.Errorf("invalid sampler argument %q: trace ID ratio must be in range [0.0, 1.0]", arg)
	}
	return ratio, nil
}

// textMapPropagator creates a composite TextMapPropagator using
// environment variable possibly overridden by the provided options.
// Reads environment variable:
//
//	OTEL_PROPAGATORS
func textMapPropagator(o *Options) (propagation.TextMapPropagator, error) {
	if len(o.Propagators) > 0 {
		return autoprop.TextMapPropagator(o.Propagators...)
	} else {
		return autoprop.NewTextMapPropagator(), nil
	}
}

func skipperDebugSpanExporter(ctx context.Context) (trace.SpanExporter, error) {
	return stdouttrace.New(stdouttrace.WithWriter(writerFunc(func(p []byte) (int, error) {
		log.Debugf("Span: %s", p)
		return len(p), nil
	})))
}

type writerFunc func([]byte) (int, error)

func (wf writerFunc) Write(p []byte) (n int, err error) {
	return wf(p)
}
