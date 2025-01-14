package lightstepotelbridge

import (
	"context"
	"github.com/google/go-cmp/cmp"
	"github.com/opentracing/opentracing-go"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/contrib/propagators/ot"
	"go.opentelemetry.io/otel/attribute"
	opentracing2 "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"os"
	"reflect"
	"testing"
	"time"
	"unsafe"
)

func Test_parseOptions(t *testing.T) {

	var (
		batchTimeout       = defaultBatchTimeout
		processorQueueSize = defaultProcessorQueueSize
		batchSize          = defaultBatchSize
		exportTimeout      = defaultExportTimeout
		hostname, _        = os.Hostname()
	)

	token := "mytoken"

	tests := []struct {
		name    string
		opts    []string
		want    Options
		wantErr bool
	}{
		{
			name:    "test without token should fail",
			opts:    []string{},
			want:    Options{},
			wantErr: true,
		},
		{
			name: "test with token works",
			opts: []string{"access-token=" + token},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
			wantErr: false,
		},
		{
			name: "test with token works and environment set",
			opts: []string{"access-token=" + token, "environment=production"},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        "production",
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
			wantErr: false,
		},
		{
			name: "test with token works and set service name",
			opts: []string{"access-token=" + token, "service-name=myservice"},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        "myservice",
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
			wantErr: false,
		},
		{
			name: "test with token works and set service version",
			opts: []string{"access-token=" + token, "service-version=1.3.4"},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     "1.3.4",
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
			wantErr: false,
		},
		{
			name: "test with token works and component set",
			opts: []string{"access-token=" + token, "component-name=mycomponent"},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      "mycomponent",
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
			wantErr: false,
		},
		{
			name: "test with token set collector",
			opts: []string{
				"access-token=" + token,
				"collector=collector.example.com:8888",
			},
			want: Options{
				AccessToken:        token,
				Collector:          "collector.example.com:8888",
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
			wantErr: false,
		},
		{
			name: "test with token set collector wrong format",
			opts: []string{
				"access-token=" + token,
				"collector=collector.example.com=8888",
			},
			want:    Options{},
			wantErr: true,
		},
		{
			name: "test with token set collector wrong port",
			opts: []string{
				"access-token=" + token,
				"collector=collector.example.com:abc",
			},
			want:    Options{},
			wantErr: true,
		},
		{
			name: "test with token set component name",
			opts: []string{
				"access-token=" + token,
				"component-name=skipper-ingress",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      "skipper-ingress",
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
			wantErr: false,
		},
		{
			name: "test with token set protocol to use grpc",
			opts: []string{
				"access-token=" + token,
				"protocol=grpc",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
		},
		{
			name: "test with token set protocol to use http",
			opts: []string{
				"access-token=" + token,
				"protocol=http",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseHttp:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
		},
		{
			name: "test with token set and wrong protocol",
			opts: []string{
				"access-token=" + token,
				"protocol=wrong",
			},
			wantErr: true,
			want:    Options{},
		},
		{
			name: "test with token set protocol to use grpc and insecure",
			opts: []string{
				"access-token=" + token,
				"protocol=grpc",
				"insecure-connection=true",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				UsePlainText:       true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
		},
		{
			name: "test with token set protocol to use grpc and insecure incorrect value",
			opts: []string{
				"access-token=" + token,
				"protocol=grpc",
				"insecure-connection=wrong",
			},
			wantErr: true,
			want:    Options{},
		},
		{
			name: "test with token set protocol to use http and insecure",
			opts: []string{
				"access-token=" + token,
				"protocol=http",
				"insecure-connection=true",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseHttp:            true,
				UsePlainText:       true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
		},
		{
			name: "test with token set and max queue size set",
			opts: []string{
				"access-token=" + token,
				"processor-queue-size=100",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				ProcessorQueueSize: 100,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
		},
		{
			name: "test with token set and max queue size set to non numeric",
			opts: []string{
				"access-token=" + token,
				"processor-queue-size=wrong",
			},
			wantErr: true,
			want:    Options{},
		},
		{
			name: "test with token set and batch size set",
			opts: []string{
				"access-token=" + token,
				"batch-size=100",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				BatchSize:          100,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				ProcessorQueueSize: processorQueueSize,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
		},
		{
			name: "test with token set and batch size set to non numeric",
			opts: []string{
				"access-token=" + token,
				"batch-size=wrong",
			},
			wantErr: true,
			want:    Options{},
		},
		{
			name: "test with token set and export timeout set",
			opts: []string{
				"access-token=" + token,
				"export-timeout=100",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				BatchSize:          batchSize,
				ExportTimeout:      100 * time.Millisecond,
				Hostname:           hostname,
				ProcessorQueueSize: processorQueueSize,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
		},
		{
			name: "test with token set and export timeout set to non numeric",
			opts: []string{
				"access-token=" + token,
				"export-timeout=wrong",
			},
			wantErr: true,
			want:    Options{},
		},
		{
			name: "test with token set and batch timeout set",
			opts: []string{
				"access-token=" + token,
				"batch-timeout=100",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       100 * time.Millisecond,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				ProcessorQueueSize: processorQueueSize,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
			},
		},
		{
			name: "test with token set and batch timeout set to non numeric",
			opts: []string{
				"access-token=" + token,
				"batch-timeout=wrong",
			},
			wantErr: true,
			want:    Options{},
		},
		{
			name: "test with token set and propagators set",
			opts: []string{
				"access-token=" + token,
				"propagators=ottrace,baggage,b3,tracecontext",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				ProcessorQueueSize: processorQueueSize,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, propagation.Baggage{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader)), propagation.TraceContext{}},
			},
		},
		{
			name: "test with token set and propagators set and b3 removed",
			opts: []string{
				"access-token=" + token,
				"propagators=ottrace,baggage,tracecontext",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				ProcessorQueueSize: processorQueueSize,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, propagation.Baggage{}, propagation.TraceContext{}},
			},
		},
		{
			name: "test with token set and propagators set and b3 removed",
			opts: []string{
				"access-token=" + token,
				"propagators=wro,ng",
			},
			wantErr: true,
			want:    Options{},
		},
		{
			name: "test with token works with global tag",
			opts: []string{
				"access-token=" + token,
				"tag=foo=bar",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
				GlobalAttributes:   []attribute.KeyValue{attribute.String("foo", "bar")},
			},
			wantErr: false,
		},
		{
			name: "test with token works with multiple global tag",
			opts: []string{
				"access-token=" + token,
				"tag=foo=bar",
				"tag=bar=foo",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
				GlobalAttributes:   []attribute.KeyValue{attribute.String("foo", "bar"), attribute.String("bar", "foo")},
			},
			wantErr: false,
		},
		{
			name: "test with token works with global tag empty",
			opts: []string{
				"access-token=" + token,
				"tag=",
			},
			want: Options{
				AccessToken:        token,
				Collector:          defaultEndpoint,
				Environment:        defaultEnvironment,
				ServiceName:        defaultServiceName,
				ServiceVersion:     defaultServiceVersion,
				ComponentName:      defaultComponentName,
				BatchTimeout:       batchTimeout,
				ProcessorQueueSize: processorQueueSize,
				BatchSize:          batchSize,
				ExportTimeout:      exportTimeout,
				Hostname:           hostname,
				UseGrpc:            true,
				Propagators:        []propagation.TextMapPropagator{ot.OT{}, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))},
				//GlobalAttributes:   []attribute.KeyValue{attribute.String("foo", "bar")},
			},
			wantErr: false,
		},
		{
			name: "test with token works with global tag wrong format",
			opts: []string{
				"access-token=" + token,
				"tag=wrong",
			},
			wantErr: true,
			want:    Options{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptions(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			//if !reflect.DeepEqual(got.Propagators, tt.propagators) {
			//	t.Logf("diff: %v", cmp.Diff(tt.propagators, got.Propagators))
			//	t.Errorf("propagators = %v, want %v", got.Propagators, tt.propagators)
			//}
			//got.Propagators = nil

			if !reflect.DeepEqual(got, tt.want) {
				t.Logf("diff: %v", cmp.Diff(tt.want, got))
				t.Errorf("parseOptions() = %v, want %v", got, tt.want)
			}
		})
	}

}

func Test_newExporter(t *testing.T) {
	type args struct {
		ctx context.Context
		opt Options
	}
	tests := []struct {
		name          string
		args          args
		wantErr       bool
		want          interface{}
		wantPlainText bool
	}{
		{
			name: "test with grpc",
			args: args{
				ctx: context.Background(),
				opt: Options{},
			},
			wantErr: false,
			want:    otlptracegrpc.NewClient(),
		},
		{
			name: "test with grpc and plain text",
			args: args{
				ctx: context.Background(),
				opt: Options{
					UsePlainText: true,
				},
			},
			wantErr:       false,
			want:          otlptracegrpc.NewClient(),
			wantPlainText: true,
		},
		{
			name: "test with http",
			args: args{
				ctx: context.Background(),
				opt: Options{
					UseHttp: true,
				},
			},
			wantErr: false,
			want:    otlptracehttp.NewClient(),
		},
		{
			name: "test with http and plain text",
			args: args{
				ctx: context.Background(),
				opt: Options{
					UseHttp:      true,
					UsePlainText: true,
				},
			},
			wantErr:       false,
			want:          otlptracehttp.NewClient(),
			wantPlainText: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, plainText, err := newExporter(tt.args.ctx, tt.args.opt)
			if (err != nil) != tt.wantErr {
				t.Errorf("newExporter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			gotClient := reflect.ValueOf(got).Elem().FieldByName("client")
			// Use unsafe to get the underlying value of the unexported field
			ptr := unsafe.Pointer(gotClient.UnsafeAddr())
			concreteValue := reflect.NewAt(gotClient.Type(), ptr).Elem().Interface()
			concreteType := reflect.TypeOf(concreteValue)

			if plainText != tt.wantPlainText {
				t.Errorf("newExporter() = %v, want %v", plainText, tt.wantPlainText)
			}

			if !reflect.DeepEqual(concreteType, reflect.TypeOf(tt.want)) {
				t.Errorf("newExporter() = %v, want %v", concreteType, reflect.TypeOf(tt.want))
			}
		})
	}
}

func Test_newTraceProvider(t *testing.T) {

	exp := &otlptrace.Exporter{}
	opts := Options{}
	r := resource.Default()
	tp := sdktrace.NewTracerProvider()

	type args struct {
		exp       *otlptrace.Exporter
		schemaUrl string
		r         *resource.Resource
		opt       Options
	}
	tests := []struct {
		name    string
		args    args
		want    interface{}
		wantErr bool
	}{
		{
			name: "test newTraceProvider fail resource.Merge",
			args: args{
				exp:       exp,
				schemaUrl: "wrongschema",
				opt:       opts,
				r:         r,
			},
			wantErr: true,
		},
		{
			name: "test newTraceProvider success",
			args: args{
				exp:       exp,
				schemaUrl: semconv.SchemaURL,
				opt:       opts,
				r:         r,
			},
			wantErr: false,
			want:    tp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newTraceProvider(tt.args.exp, tt.args.schemaUrl, tt.args.r, tt.args.opt)
			if err != nil {
				if (err != nil) != tt.wantErr {
					t.Errorf("newTraceProvider() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if reflect.TypeOf(got) != reflect.TypeOf(tt.want) {
				t.Errorf("newTraceProvider() got = %v, want %v", reflect.TypeOf(got), reflect.TypeOf(tt.want))
			}
		})
	}
}

func Test_setupOTelSDK(t *testing.T) {
	type args struct {
		ctx       context.Context
		schemaUrl string
		opts      Options
	}
	tests := []struct {
		name         string
		args         args
		wantShutdown func(context.Context) error
		wantErr      bool
	}{
		{
			name: "test setupOTelSDK success",
		},
		{
			name: "test setupOTelSDK fail",
			args: args{
				ctx:       context.Background(),
				opts:      Options{},
				schemaUrl: "wrong",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotShutdown, err := setupOTelSDK(tt.args.ctx, tt.args.schemaUrl, tt.args.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("setupOTelSDK() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if reflect.TypeOf(gotShutdown) != reflect.TypeOf(tt.wantShutdown) {
				t.Errorf("setupOTelSDK() gotShutdown = %v, want %v", reflect.TypeOf(gotShutdown), reflect.TypeOf(tt.wantShutdown))
			}
		})
	}
}

func TestInitTracer(t *testing.T) {

	type args struct {
		opts []string
	}
	tests := []struct {
		name string
		args args
		want interface{}
	}{
		{
			name: "test InitTracer successful",
			want: &opentracing2.BridgeTracer{},
			args: args{
				opts: []string{"access-token=mytoken", "collector=example.com:8443"},
			},
		},
		{
			name: "test InitTracer fail parse options and return no op tracer",
			want: &opentracing.NoopTracer{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InitTracer(tt.args.opts)
			if reflect.TypeOf(got) != reflect.TypeOf(tt.want) {
				t.Errorf("InitTracer() got = %v, want %v", reflect.TypeOf(got), reflect.TypeOf(tt.want))
			}
		})
	}
}
