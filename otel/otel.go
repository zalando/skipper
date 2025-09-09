// Package otel provides [OpenTelemetry] integration for Skipper.
//
// [OpenTelemetry]: https://opentelemetry.io/
package otel

import (
	"context"
	"errors"
	"os"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
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
	Initialized bool
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
		"OTEL_BSP_MAX_QUEUE_SIZE",
		"OTEL_BSP_MAX_EXPORT_BATCH_SIZE",
		"OTEL_BSP_SCHEDULE_DELAY",
		"OTEL_BSP_EXPORT_TIMEOUT",
	} {
		log.Debugf("%s: %s", name, os.Getenv(name))
	}

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
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Similar to "console" but writes into debug logger
	autoexport.RegisterSpanExporter("skipper-debug", func(ctx context.Context) (trace.SpanExporter, error) {
		exp, _ := stdouttrace.New(stdouttrace.WithWriter(writerFunc(func(p []byte) (int, error) {
			log.Debugf("Span: %s", p)
			return len(p), nil
		})))
		shutdownFuncs = append(shutdownFuncs, exp.Shutdown)
		return exp, nil
	})

	// Reads environment variables:
	// 	OTEL_TRACES_EXPORTER
	// 	OTEL_EXPORTER_OTLP_PROTOCOL
	// 	OTEL_EXPORTER_OTLP_ENDPOINT
	// 	OTEL_EXPORTER_OTLP_HEADERS
	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		handleErr(err)
		return
	}

	// Reads environment variables:
	// 	OTEL_RESOURCE_ATTRIBUTES
	resourceOpt := trace.WithResource(resource.Environment())

	// Reads environment variables:
	// 	OTEL_BSP_MAX_QUEUE_SIZE
	// 	OTEL_BSP_MAX_EXPORT_BATCH_SIZE
	// 	OTEL_BSP_SCHEDULE_DELAY
	// 	OTEL_BSP_EXPORT_TIMEOUT
	batcherOpt := trace.WithBatcher(spanExporter)

	tracerProvider := trace.NewTracerProvider(batcherOpt, resourceOpt)
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)

	otel.SetTracerProvider(tracerProvider)

	// Reads environment variables:
	// 	OTEL_PROPAGATORS
	textMapPropagator := autoprop.NewTextMapPropagator()
	otel.SetTextMapPropagator(textMapPropagator)

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { log.Error(err) }))
	otel.SetLogger(logrusr.New(log))

	return
}

type writerFunc func([]byte) (int, error)

func (wf writerFunc) Write(p []byte) (n int, err error) {
	return wf(p)
}
