package otel

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/lightstep/lightstep-tracer-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelBridge "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"

	ot "github.com/opentracing/opentracing-go"
)

const (
	defServiceName = "skipper"
)

type tracerBuilder struct {
	exporter *otlptrace.Exporter
	provider *sdk.TracerProvider
	opt      options
}

type options struct {
	customHeaders  map[string]string
	host           string
	port           int
	useHTTP        bool
	ServiceName    string
	ServiceVersion string
	Environment    string
	customPath     string
	insecure       bool

	exportTimeout      time.Duration
	batchTimeout       time.Duration
	maxExportBatchSize int
	maxSpanQueueSize   int
	compressionMethod  string
	tags               map[string]string
}

func InitTracer(ctx context.Context, opts []string) (trace.Tracer, error) {
	opt, err := parseOptions(opts)
	if err != nil {
		return nil, err
	}

	builder := tracerBuilder{
		opt: opt,
	}

	return builder.Build(ctx)
}

func WithBridgeTracer(ctx context.Context, opts []string) (trace.Tracer, ot.Tracer, error) {
	opt, err := parseOptions(opts)
	if err != nil {
		return nil, nil, err
	}

	builder := tracerBuilder{
		opt: opt,
	}

	t, err := builder.Build(ctx)
	if err != nil {
		return nil, nil, err
	}

	_, otTracer, wrapper := otelBridge.NewTracerPairWithContext(ctx, t)
	otel.SetTracerProvider(wrapper)
	return t, otTracer, nil
}

func parseOptions(opts []string) (options, error) {
	config := options{}
	globalTags := make(map[string]string)

	for _, o := range opts {
		key, val, _ := strings.Cut(o, "=")
		switch key {
		case "custom-headers":
			config.customHeaders = make(map[string]string)
			err := json.Unmarshal([]byte(val), &config.customHeaders)
			if err != nil {
				return options{}, fmt.Errorf("failed to parse %q as key-value custom headers: %w", val, err)
			}
		case "service-name":
			if val != "" {
				config.ServiceName = val
			}
		case "service-version":
			if val != "" {
				config.ServiceVersion = val
			}
		case "max-export-batch-size":
			v, err := strconv.Atoi(val)
			if err != nil {
				return options{}, fmt.Errorf("failed to parse %q as int max-export-batch-size: %w", val, err)
			}
			config.maxExportBatchSize = v
		case "max-span-queue-size":
			v, err := strconv.Atoi(val)
			if err != nil {
				return options{}, fmt.Errorf("failed to parse %q as int max-export-batch-size: %w", val, err)
			}
			config.maxSpanQueueSize = v
		case "batch-timeout":
			v, err := time.ParseDuration(val)
			if err != nil {
				return options{}, fmt.Errorf("failed to parse %q as time.Duration min-period : %w", val, err)
			}
			config.batchTimeout = v
		case "export-timeout":
			v, err := time.ParseDuration(val)
			if err != nil {
				return options{}, fmt.Errorf("failed to parse %q as time.Duration min-period : %w", val, err)
			}
			config.exportTimeout = v
		case "tag":
			if val != "" {
				tag, tagVal, found := strings.Cut(val, "=")
				if !found {
					return options{}, fmt.Errorf("missing value for tag %q", val)
				}
				globalTags[tag] = tagVal
			}
		case "otlp-exporter-endpoint":
			var err error
			var sport string

			config.host, sport, err = net.SplitHostPort(val)
			if err != nil {
				return options{}, err
			}

			config.port, err = strconv.Atoi(sport)
			if err != nil {
				return options{}, fmt.Errorf("failed to parse %q as int: %w", sport, err)
			}
		case "compression-method":
			config.compressionMethod = val
		case "insecure":
			var err error
			config.insecure, err = strconv.ParseBool(val)
			if err != nil {
				return options{}, fmt.Errorf("failed to parse %q as bool: %w", val, err)
			}
		case "use-http":
			var err error
			config.useHTTP, err = strconv.ParseBool(val)
			if err != nil {
				return options{}, fmt.Errorf("failed to parse %q as bool: %w", val, err)
			}
		}
	}

	if config.ServiceName == "" {
		config.ServiceName = defServiceName
	}

	if config.host == "" {
		return options{}, fmt.Errorf("'otlp-exporter-endpoint' option not found, this option is mandatory")
	}

	config.tags = map[string]string{
		lightstep.ComponentNameKey: config.ServiceName,
	}

	for k, v := range globalTags {
		config.tags[k] = v
	}

	return config, nil
}

func (tb *tracerBuilder) setupGRPCExporter(ctx context.Context) error {
	clientOptions := []otlptracegrpc.Option{
		otlptracegrpc.WithHeaders(tb.opt.customHeaders),
		otlptracegrpc.WithEndpoint(net.JoinHostPort(tb.opt.host, strconv.Itoa(tb.opt.port))),
	}

	if tb.opt.insecure {
		clientOptions = append(clientOptions, otlptracegrpc.WithInsecure())
	}

	// OpenTelemetry only supports gzip or no compression:
	// https://github.com/open-telemetry/opentelemetry-go/blob/exporters/otlp/otlptrace/v1.17.0/exporters/otlp/otlptrace/otlptracegrpc/options.go#L91
	if tb.opt.compressionMethod != "gzip" && len(tb.opt.compressionMethod) != 0 {
		return fmt.Errorf("compression method %q is not supported", tb.opt.compressionMethod)
	}

	if tb.opt.compressionMethod == "gzip" {
		clientOptions = append(clientOptions, otlptracegrpc.WithCompressor(tb.opt.compressionMethod))
	}

	client := otlptracegrpc.NewClient(clientOptions...)

	e, err := otlptrace.New(ctx, client)
	if err != nil {
		return err
	}

	tb.exporter = e
	return nil
}

func (tb *tracerBuilder) setupHTTPExporter(ctx context.Context) error {
	clientOptions := []otlptracehttp.Option{
		otlptracehttp.WithHeaders(tb.opt.customHeaders),
		otlptracehttp.WithURLPath(tb.opt.customPath),
		otlptracehttp.WithEndpoint(net.JoinHostPort(tb.opt.host, strconv.Itoa(tb.opt.port))),
	}

	if tb.opt.insecure {
		clientOptions = append(clientOptions, otlptracehttp.WithInsecure())
	}

	// OpenTelemetry only supports gzip or no compression:
	// https://github.com/open-telemetry/opentelemetry-go/blob/3c476ce1816ae6f38758e90cc36d8b77ebcc223b/exporters/otlp/otlptrace/internal/otlpconfig/optiontypes.go#L29
	if tb.opt.compressionMethod != "gzip" && len(tb.opt.compressionMethod) != 0 {
		return fmt.Errorf("compression method %q is not supported", tb.opt.compressionMethod)
	}

	if tb.opt.compressionMethod == "gzip" {
		clientOptions = append(clientOptions, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	}

	client := otlptracehttp.NewClient(clientOptions...)

	e, err := otlptrace.New(ctx, client)
	if err != nil {
		return err
	}

	tb.exporter = e
	return nil
}

func (tb *tracerBuilder) Build(ctx context.Context) (trace.Tracer, error) {
	var err error
	if tb.opt.useHTTP {
		err = tb.setupHTTPExporter(ctx)
	} else {
		err = tb.setupGRPCExporter(ctx)
	}
	if err != nil {
		return nil, err
	}

	tags := []attribute.KeyValue{
		semconv.ServiceNameKey.String(tb.opt.ServiceName),
		semconv.ServiceVersionKey.String(tb.opt.ServiceVersion),
		attribute.String("environment", tb.opt.Environment),
	}

	for k, v := range tb.opt.tags {
		tags = append(tags, attribute.String(k, v))
	}

	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, tags...),
	)
	if err != nil {
		return nil, err
	}

	var batcherOptions []sdk.BatchSpanProcessorOption

	if tb.opt.batchTimeout != 0 {
		batcherOptions = append(batcherOptions, sdk.WithBatchTimeout(tb.opt.batchTimeout))
	}

	if tb.opt.exportTimeout != 0 {
		batcherOptions = append(batcherOptions, sdk.WithExportTimeout(tb.opt.exportTimeout))
	}

	if tb.opt.maxExportBatchSize != 0 {
		batcherOptions = append(batcherOptions, sdk.WithMaxExportBatchSize(tb.opt.maxExportBatchSize))
	}

	if tb.opt.maxSpanQueueSize != 0 {
		batcherOptions = append(batcherOptions, sdk.WithMaxQueueSize(tb.opt.maxSpanQueueSize))
	}

	tb.provider = sdk.NewTracerProvider(
		sdk.WithBatcher(tb.exporter, batcherOptions...),
		sdk.WithResource(r),
	)

	otel.SetTracerProvider(tb.provider)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return tb.provider.Tracer(
		tb.opt.ServiceName,
		trace.WithInstrumentationVersion(tb.opt.ServiceName),
	), nil
}
