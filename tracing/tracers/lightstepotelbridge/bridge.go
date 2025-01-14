package lightstepotelbridge

import (
	"context"
	"errors"
	"fmt"
	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/propagators/b3"
	ottrace "go.opentelemetry.io/contrib/propagators/ot"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelBridge "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultEndpoint           = "ingest.lightstep.com:443"
	defaultServiceName        = "skipper"
	defaultServiceVersion     = "0.1.0"
	defaultEnvironment        = "dev"
	defaultTracerName         = "lightstep-otel-bridge"
	defaultComponentName      = "skipper"
	urlPath                   = "traces/otlp/v0.9"
	defaultPropagators        = "ottrace,b3"
	hostNameKey               = "hostname"
	lsEnvironmentKey          = "environment"
	defaultBatchTimeout       = 2500 * time.Millisecond
	defaultProcessorQueueSize = 10000
	defaultBatchSize          = 512
	defaultExportTimeout      = 5000 * time.Millisecond
)

type Options struct {
	Collector          string
	AccessToken        string
	Environment        string
	UseHttp            bool
	UseGrpc            bool
	UsePlainText       bool
	ComponentName      string
	ServiceName        string
	ServiceVersion     string
	BatchTimeout       time.Duration
	ProcessorQueueSize int
	BatchSize          int
	ExportTimeout      time.Duration
	Hostname           string
	Propagators        []propagation.TextMapPropagator
	GlobalAttributes   []attribute.KeyValue
}

func parseOptions(opts []string) (Options, error) {

	var (
		serviceName                    string
		serviceVersion                 string
		endpoint                       string
		lsToken                        string
		lsEnvironment                  string
		componentName                  string
		propagators                    string
		useHttp, useGrpc, usePlainText bool
		err                            error
		globalTags                     []attribute.KeyValue
		hostname, _                    = os.Hostname()
		batchTimeout                   = defaultBatchTimeout
		processorQueueSize             = defaultProcessorQueueSize
		batchSize                      = defaultBatchSize
		exportTimeout                  = defaultExportTimeout
	)

	for _, o := range opts {
		key, val, _ := strings.Cut(o, "=")
		switch key {
		case "collector":
			var sport string

			_, sport, err = net.SplitHostPort(val)
			if err != nil {
				return Options{}, err
			}

			_, err = strconv.Atoi(sport)
			if err != nil {
				return Options{}, fmt.Errorf("failed to parse %s as int: %w", sport, err)
			}
			endpoint = val

		case "access-token":
			lsToken = val
		case "environment":
			lsEnvironment = val
		case "protocol":
			if strings.ToLower(val) == "http" {
				useHttp = true
			} else if strings.ToLower(val) == "grpc" {
				useGrpc = true
			} else {
				return Options{}, fmt.Errorf("unsupported protocol %s", val)
			}
		case "insecure-connection":
			usePlainText, err = strconv.ParseBool(val)
			if err != nil {
				return Options{}, fmt.Errorf("failed to parse %s as bool: %w", val, err)
			}
		case "component-name":
			componentName = val
		case "service-name":
			serviceName = val
		case "service-version":
			serviceVersion = val
		case "batch-timeout":
			intVal, err := strconv.Atoi(val)
			if err != nil {
				return Options{}, errors.New("failed to parse batch-timeout as int")
			}
			batchTimeout = time.Duration(intVal) * time.Millisecond
		case "processor-queue-size":
			intVal, err := strconv.Atoi(val)
			if err != nil {
				return Options{}, errors.New("failed to parse processor-queue-size as int")
			}
			processorQueueSize = intVal
		case "batch-size":
			intVal, err := strconv.Atoi(val)
			if err != nil {
				return Options{}, errors.New("failed to parse batch-size as int")
			}
			batchSize = intVal
		case "export-timeout":
			intVal, err := strconv.Atoi(val)
			if err != nil {
				return Options{}, errors.New("failed to parse export-timeout as int")
			}
			exportTimeout = time.Duration(intVal) * time.Millisecond
		case "propagators":
			for _, p := range strings.Split(val, ",") {
				switch strings.ToLower(p) {
				case "tracecontext":
					// no-op
				case "baggage":
					// no-op
				case "ottrace":
					// no-op
				case "b3":
					// no-op
				default:
					return Options{}, fmt.Errorf("unsupported propagator %s", p)
				}
			}
			propagators = val
		case "tag":
			if val != "" {
				tag, tagVal, found := strings.Cut(val, "=")
				if !found {
					return Options{}, fmt.Errorf("missing value for tag %s", val)
				}
				globalTags = append(globalTags, attribute.String(tag, tagVal))
			}
		}
	}

	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	if lsToken == "" {
		return Options{}, errors.New("missing Lightstep access token")
	}

	if lsEnvironment == "" {
		lsEnvironment = defaultEnvironment
	}

	if !useHttp {
		useGrpc = true
	}

	if componentName == "" {
		componentName = defaultComponentName
	}

	if serviceName == "" {
		serviceName = defaultServiceName
	}

	if serviceVersion == "" {
		serviceVersion = defaultServiceVersion
	}

	if propagators == "" {
		propagators = defaultPropagators
	}

	var textMapPropagator []propagation.TextMapPropagator

	for _, prop := range strings.Split(propagators, ",") {
		switch strings.ToLower(prop) {
		case "tracecontext":
			textMapPropagator = append(textMapPropagator, propagation.TraceContext{})
		case "baggage":
			textMapPropagator = append(textMapPropagator, propagation.Baggage{})
		case "ottrace":
			textMapPropagator = append(textMapPropagator, ottrace.OT{})
		case "b3":
			textMapPropagator = append(textMapPropagator, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader|b3.B3SingleHeader)))
		default:
			return Options{}, fmt.Errorf("unsupported propagator %s", prop)
		}
	}
	log.Infof("serviceName: %s, serviceVersion: %s, endpoint: %s, lsEnvironment: %s, componentName: %s, useHttp: %t, useGrpc: %t, usePlainText: %t, batchTimeout: %s, processorQueueSize: %d, batchSize: %d, exportTimeout: %s, hostname: %s, propagators: %s, globalTags: %v", serviceName, serviceVersion, endpoint, lsEnvironment, componentName, useHttp, useGrpc, usePlainText, batchTimeout, processorQueueSize, batchSize, exportTimeout, hostname, propagators, globalTags)
	return Options{
		Collector:          endpoint,
		AccessToken:        lsToken,
		Environment:        lsEnvironment,
		UseHttp:            useHttp,
		UseGrpc:            useGrpc,
		UsePlainText:       usePlainText,
		ComponentName:      componentName,
		ServiceName:        serviceName,
		ServiceVersion:     serviceVersion,
		BatchTimeout:       batchTimeout,
		ProcessorQueueSize: processorQueueSize,
		BatchSize:          batchSize,
		ExportTimeout:      exportTimeout,
		Hostname:           hostname,
		Propagators:        textMapPropagator,
		GlobalAttributes:   globalTags,
	}, nil
}

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOTelSDK(ctx context.Context, schemaUrl string, opts Options) (shutdown func(context.Context) error, err error) {
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

	// Set up propagator.
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			opts.Propagators...,
		),
	)

	// Set up trace provider.
	exp, _, err := newExporter(ctx, opts)
	if err != nil {
		handleErr(err)
		return
	}

	tracerProvider, err := newTraceProvider(exp, schemaUrl, resource.Default(), opts)
	if err != nil {
		handleErr(err)
		return
	}

	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	return
}

func newExporter(ctx context.Context, opt Options) (*otlptrace.Exporter, bool, error) {

	var headers = map[string]string{
		"lightstep-access-token": opt.AccessToken,
	}

	var client otlptrace.Client
	var isSecure = false

	if opt.UseHttp {
		var tOpt []otlptracehttp.Option
		tOpt = append(tOpt, otlptracehttp.WithHeaders(headers))
		tOpt = append(tOpt, otlptracehttp.WithEndpoint(opt.Collector))
		tOpt = append(tOpt, otlptracehttp.WithURLPath(urlPath))
		if opt.UsePlainText {
			tOpt = append(tOpt, otlptracehttp.WithInsecure())
			isSecure = true
		}
		client = otlptracehttp.NewClient(tOpt...)
	} else {
		var tOpt []otlptracegrpc.Option
		tOpt = append(tOpt, otlptracegrpc.WithHeaders(headers))
		tOpt = append(tOpt, otlptracegrpc.WithEndpoint(opt.Collector))
		if opt.UsePlainText {
			tOpt = append(tOpt, otlptracegrpc.WithInsecure())
			isSecure = true
		}
		client = otlptracegrpc.NewClient(tOpt...)
	}

	exp, err := otlptrace.New(ctx, client)
	return exp, isSecure, err
}

func newTraceProvider(exp *otlptrace.Exporter, schemaUrl string, r *resource.Resource, opt Options) (*sdktrace.TracerProvider, error) {

	opts := opt.GlobalAttributes

	opts = append(
		opts,
		semconv.ServiceName(opt.ServiceName),
		semconv.HostName(opt.Hostname),
		semconv.ServiceVersionKey.String(opt.ServiceVersion),
		attribute.String(lsEnvironmentKey, opt.Environment),
		attribute.String(hostNameKey, opt.Hostname),
	)

	r, err := resource.Merge(
		r,
		resource.NewWithAttributes(
			schemaUrl,
			opts...,
		),
	)

	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			exp,
			sdktrace.WithBatchTimeout(opt.BatchTimeout),
			sdktrace.WithMaxExportBatchSize(opt.BatchSize),
			sdktrace.WithMaxQueueSize(opt.ProcessorQueueSize),
			sdktrace.WithExportTimeout(opt.ExportTimeout),
		),
		sdktrace.WithResource(r),
	), nil
}

func InitTracer(opts []string) opentracing.Tracer {

	options, err := parseOptions(opts)
	if err != nil {
		log.WithError(err).Error("failed to parse options")
		return &opentracing.NoopTracer{}
	}

	_, err = setupOTelSDK(context.Background(), semconv.SchemaURL, options)
	if err != nil {
		log.WithError(err).Error("failed to set up OpenTelemetry SDK")
		return &opentracing.NoopTracer{}
	}

	provider := otel.GetTracerProvider()
	otelTracer := provider.Tracer(defaultTracerName)
	bridgeTracer, wrapperTracerProvider := otelBridge.NewTracerPair(otelTracer)
	otel.SetTracerProvider(wrapperTracerProvider)

	log.Infof("OpenTelemetry Lightstep bridge tracer initialized")

	return bridgeTracer
}
