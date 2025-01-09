package otel

import (
	"context"
	"errors"
	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
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
	"strings"
)

const (
	defComponentName     = "skipper"
	defaultGRPMaxMsgSize = 16 * 1024 * 1000
)

type Options struct {
	Collector      string
	AccessToken    string
	Environment    string
	UseHttp        bool
	UseGrpc        bool
	PlainText      bool
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
		case "plaintext":
			usePlainText = true
		case "component-name":
			componentName = val
		case "service-name":
			serviceName = val
		case "service-version":
			serviceVersion = val
		}
	}

	return Options{
		Collector:      collector,
		AccessToken:    accessToken,
		Environment:    environment,
		UseHttp:        useHttp,
		UseGrpc:        useGrpc,
		PlainText:      usePlainText,
		ComponentName:  componentName,
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
	}, nil
}

var (
	serviceName    = os.Getenv("LS_SERVICE_NAME")
	urlPath        = "traces/otlp/v0.9"
	serviceVersion = os.Getenv("LS_SERVICE_VERSION")
	endpoint       = os.Getenv("LS_SATELLITE_URL")
	lsToken        = os.Getenv("LS_ACCESS_TOKEN")
	lsEnvironment  = os.Getenv("LS_ENVIRONMENT")
)

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
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	// Set up trace provider.
	exp, err := newExporter(ctx, opts)
	if err != nil {
		handleErr(err)
		return
	}

	tracerProvider := newTraceProvider(exp, opts)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	return
}

func newExporter(ctx context.Context, o Options) (*otlptrace.Exporter, error) {

	if len(o.Collector) > 0 {
		endpoint = o.Collector
		log.Infof("Using custom LS endpoint %s/%s", endpoint, urlPath)
	} else if len(endpoint) == 0 {
		endpoint = "ingest.lightstep.com:443"
		log.Infof("Using default LS endpoint %s/%s", endpoint, urlPath)
	}

	if len(o.AccessToken) > 0 {
		lsToken = o.AccessToken
		log.Infof("Using custom LS token")
	}

	var headers = map[string]string{
		"lightstep-access-token": lsToken,
	}

	var client otlptrace.Client

	if o.UseGrpc {
		var tOpt []otlptracegrpc.Option
		tOpt = append(tOpt, otlptracegrpc.WithHeaders(headers))
		tOpt = append(tOpt, otlptracegrpc.WithEndpoint(endpoint))
		if o.PlainText {
			tOpt = append(tOpt, otlptracegrpc.WithInsecure())
		}
		client = otlptracegrpc.NewClient(tOpt...)
	} else {
		var tOpt []otlptracehttp.Option
		tOpt = append(tOpt, otlptracehttp.WithHeaders(headers))
		tOpt = append(tOpt, otlptracehttp.WithEndpoint(endpoint))
		tOpt = append(tOpt, otlptracehttp.WithURLPath(urlPath))
		if o.PlainText {
			tOpt = append(tOpt, otlptracehttp.WithInsecure())
		}
		client = otlptracehttp.NewClient(tOpt...)
	}

	return otlptrace.New(ctx, client)
}

func newTraceProvider(exp *otlptrace.Exporter, opt Options) *sdktrace.TracerProvider {

	if len(opt.ServiceName) > 0 {
		serviceName = opt.ServiceName
		log.Infof("Using custom service name %s", serviceName)
	} else if len(serviceName) == 0 {
		serviceName = "skipper"
		log.Infof("Using default service name %s", serviceName)
	}

	if len(opt.ServiceVersion) > 0 {
		serviceVersion = opt.ServiceVersion
		log.Infof("Using custom service version %s", serviceVersion)
	} else if len(serviceVersion) == 0 {
		serviceVersion = "0.1.0"
		log.Infof("Using default service version %s", serviceVersion)
	}

	if len(opt.Environment) > 0 {
		lsEnvironment = opt.Environment
		log.Infof("Using custom environment %s", lsEnvironment)
	} else if len(lsEnvironment) == 0 {
		lsEnvironment = "dev"
		log.Infof("Using default environment %s", lsEnvironment)
	}

	r, rErr :=
		resource.Merge(
			resource.Default(),
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(serviceName),
				semconv.ServiceVersionKey.String(serviceVersion),
				attribute.String("environment", lsEnvironment),
			),
		)

	if rErr != nil {
		panic(rErr)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(r),
	)
}

func InitTracer(opts []string) opentracing.Tracer {

	options, err := parseOptions(opts)

	_, err = setupOTelSDK(context.Background(), options)
	if err != nil {
		log.WithError(err).Error("failed to set up OpenTelemetry SDK")
		return nil
	}

	provider := otel.GetTracerProvider()
	otelTracer := provider.Tracer("otel-ls-brigde")
	bridgeTracer, wrapperTracerProvider := otelBridge.NewTracerPair(otelTracer)
	otel.SetTracerProvider(wrapperTracerProvider)

	log.Infof("OpenTelemetry bridge tracer initialized")

	return bridgeTracer
}
