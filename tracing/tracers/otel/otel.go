package otel

import (
	"context"
	"errors"
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
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	HostNameKey           = "hostname"
	LSHostNameKey         = "lightstep.hostname"
	LSEnvironmentKey      = "environment"
	LSComponentNameKey    = "lightstep.component_name"
	DefaultEndpoint       = "ingest.lightstep.com:443"
	DefaultServiceName    = "skipper"
	DefaultServiceVersion = "0.1.0"
	DefaultEnvironment    = "dev"
	DefaultTracerName     = "otel-lightstep-bridge"
	DefaultComponentName  = "skipper"
)

var (
	serviceName        = os.Getenv("LS_SERVICE_NAME")
	urlPath            = "traces/otlp/v0.9"
	serviceVersion     = os.Getenv("LS_SERVICE_VERSION")
	endpoint           = os.Getenv("LS_SATELLITE_URL")
	lsToken            = os.Getenv("LS_ACCESS_TOKEN")
	lsEnvironment      = os.Getenv("LS_ENVIRONMENT")
	hostname, _        = os.Hostname()
	batchTimeout       = 2500 * time.Millisecond
	processorQueueSize = 10000
	batchSize          = 512
	exportTimeout      = 5000 * time.Millisecond
)

type Options struct {
	Collector      string
	AccessToken    string
	Environment    string
	UseHttp        bool
	UseGrpc        bool
	UsePlainText   bool
	ComponentName  string
	ServiceName    string
	ServiceVersion string
}

func parseOptions(opts []string) (Options, error) {
	var (
		collector, accessToken, environment, componentName, serviceName, serviceVersion string
		useHttp, useGrpc, usePlainText                                                  bool
	)

	for _, o := range opts {
		key, val, _ := strings.Cut(o, "=")
		switch key {
		case "collector":
			collector = val
		case "access-token":
			accessToken = val
		case "environment":
			environment = val
		case "use-http":
			useHttp = true
		case "use-grpc":
			useGrpc = true
		case "use-plain-text":
			usePlainText = true
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
		}
	}

	return Options{
		Collector:      collector,
		AccessToken:    accessToken,
		Environment:    environment,
		UseHttp:        useHttp,
		UseGrpc:        useGrpc,
		UsePlainText:   usePlainText,
		ComponentName:  componentName,
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
	}, nil
}

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOTelSDK(ctx context.Context, opts Options) (shutdown func(context.Context) error, err error) {
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
			//propagation.TraceContext{},
			//propagation.Baggage{},
			ottrace.OT{},
			b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader|b3.B3SingleHeader)),
		),
	)

	// Set up trace provider.
	exp, err := newExporter(ctx, opts)
	if err != nil {
		handleErr(err)
		return
	}

	tracerProvider, err := newTraceProvider(exp, opts)
	if err != nil {
		handleErr(err)
		return
	}

	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	return
}

func newExporter(ctx context.Context, opt Options) (*otlptrace.Exporter, error) {

	if len(opt.Collector) > 0 {
		endpoint = opt.Collector
		log.Infof("Using custom LS endpoint %s/%s", endpoint, urlPath)
	} else if len(endpoint) == 0 {
		endpoint = DefaultEndpoint
		log.Infof("Using default LS endpoint %s/%s", endpoint, urlPath)
	}

	if len(opt.AccessToken) > 0 {
		lsToken = opt.AccessToken
		log.Infof("Using custom LS token")
	}

	if len(lsToken) == 0 {
		return nil, errors.New("missing Lightstep access token")
	}

	var headers = map[string]string{
		"lightstep-access-token": lsToken,
	}

	var client otlptrace.Client

	if opt.UseGrpc {
		var tOpt []otlptracegrpc.Option
		tOpt = append(tOpt, otlptracegrpc.WithHeaders(headers))
		tOpt = append(tOpt, otlptracegrpc.WithEndpoint(endpoint))
		if opt.UsePlainText {
			tOpt = append(tOpt, otlptracegrpc.WithInsecure())
		}
		client = otlptracegrpc.NewClient(tOpt...)
	} else {
		var tOpt []otlptracehttp.Option
		tOpt = append(tOpt, otlptracehttp.WithHeaders(headers))
		tOpt = append(tOpt, otlptracehttp.WithEndpoint(endpoint))
		tOpt = append(tOpt, otlptracehttp.WithURLPath(urlPath))
		if opt.UsePlainText {
			tOpt = append(tOpt, otlptracehttp.WithInsecure())
		}
		client = otlptracehttp.NewClient(tOpt...)
	}

	return otlptrace.New(ctx, client)
}

func newTraceProvider(exp *otlptrace.Exporter, opt Options) (*sdktrace.TracerProvider, error) {

	var componentName = DefaultComponentName
	if len(opt.ComponentName) > 0 {
		componentName = opt.ComponentName
		log.Infof("Using custom component name %s", componentName)
	}

	if len(opt.ServiceName) > 0 {
		serviceName = opt.ServiceName
		log.Infof("Using custom service name %s", serviceName)
	} else if len(serviceName) == 0 {
		serviceName = DefaultServiceName
		log.Infof("Using default service name %s", serviceName)
	}

	if len(opt.ServiceVersion) > 0 {
		serviceVersion = opt.ServiceVersion
		log.Infof("Using custom service version %s", serviceVersion)
	} else if len(serviceVersion) == 0 {
		serviceVersion = DefaultServiceVersion
		log.Infof("Using default service version %s", serviceVersion)
	}

	if len(opt.Environment) > 0 {
		lsEnvironment = opt.Environment
		log.Infof("Using custom environment %s", lsEnvironment)
	} else if len(lsEnvironment) == 0 {
		lsEnvironment = DefaultEnvironment
		log.Infof("Using default environment %s", lsEnvironment)
	}

	r, err :=
		resource.Merge(
			resource.Default(),
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(serviceName),
				semconv.ServiceVersionKey.String(serviceVersion),
				attribute.String(LSEnvironmentKey, lsEnvironment),
				attribute.String(LSComponentNameKey, componentName),
				// TODO does not work
				attribute.String(LSHostNameKey, hostname),
				attribute.String(HostNameKey, hostname),
			),
		)

	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			exp,
			sdktrace.WithBatchTimeout(batchTimeout),
			sdktrace.WithMaxExportBatchSize(batchSize),
			sdktrace.WithMaxQueueSize(processorQueueSize),
			sdktrace.WithExportTimeout(exportTimeout),
		),
		sdktrace.WithResource(r),
	), nil
}

func InitTracer(opts []string) opentracing.Tracer {

	options, err := parseOptions(opts)

	_, err = setupOTelSDK(context.Background(), options)
	if err != nil {
		log.WithError(err).Error("failed to set up OpenTelemetry SDK")
		return nil
	}

	provider := otel.GetTracerProvider()
	otelTracer := provider.Tracer(DefaultTracerName)
	bridgeTracer, wrapperTracerProvider := otelBridge.NewTracerPair(otelTracer)
	otel.SetTracerProvider(wrapperTracerProvider)

	log.Infof("OpenTelemetry Lightstep bridge tracer initialized")

	return bridgeTracer
}
