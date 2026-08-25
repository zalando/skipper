package lightstep

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	lightstep "github.com/lightstep/lightstep-tracer-go"
	opentracing "github.com/opentracing/opentracing-go"
)

func TestParseOptions(t *testing.T) {
	token := "mytoken"
	defPropagator := lightstep.PropagatorStack{}
	defPropagator.PushPropagator(lightstep.LightStepPropagator)
	b3Propagator := lightstep.PropagatorStack{}
	b3Propagator.PushPropagator(lightstep.B3Propagator)

	tests := []struct {
		name        string
		opts        []string
		want        lightstep.Options
		wantErr     bool
		propagators map[opentracing.BuiltinFormat]lightstep.Propagator
	}{
		{
			name:    "test without token should fail",
			opts:    []string{},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token should work",
			opts: []string{"token=" + token},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token set collector",
			opts: []string{
				"token=" + token,
				"collector=collector.example.com:8888",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: "collector.example.com",
					Port: 8888,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token set component name",
			opts: []string{
				"token=" + token,
				"component-name=skipper-ingress",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: "skipper-ingress",
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token set component name",
			opts: []string{
				"token=" + token,
				"component-name=skipper-ingress",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: "skipper-ingress",
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token set protocol to grpc use grpc",
			opts: []string{
				"token=" + token,
				"protocol=grpc",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token set protocol to foo returns error",
			opts: []string{
				"token=" + token,
				"protocol=foo",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token set protocol to http does not use grpc",
			opts: []string{
				"token=" + token,
				"protocol=http",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: false,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token set max buffered spans",
			opts: []string{
				"token=" + token,
				"max-buffered-spans=8192",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
				MaxBufferedSpans:            8192,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token set wrong max buffered spans",
			opts: []string{
				"token=" + token,
				"max-buffered-spans=foo",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token set max log key length",
			opts: []string{
				"token=" + token,
				"max-log-key-len=20",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				Tags:                        opentracing.Tags{"lightstep.component_name": string("skipper")},
				UseGRPC:                     true,
				MaxLogKeyLen:                20,
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token set no max log key length",
			opts: []string{
				"token=" + token,
				"max-log-key-len=",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token set wrong max log key length",
			opts: []string{
				"token=" + token,
				"max-log-key-len=foo",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token set max log value length",
			opts: []string{
				"token=" + token,
				"max-log-value-len=100",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				Tags:                        opentracing.Tags{"lightstep.component_name": string("skipper")},
				UseGRPC:                     true,
				MaxLogValueLen:              100,
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token set no max log value length",
			opts: []string{
				"token=" + token,
				"max-log-value-len=",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token set max logs per span",
			opts: []string{
				"token=" + token,
				"max-logs-per-span=25",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				Tags:                        opentracing.Tags{"lightstep.component_name": string("skipper")},
				UseGRPC:                     true,
				MaxLogsPerSpan:              25,
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token set wrong max logs per span",
			opts: []string{
				"token=" + token,
				"max-logs-per-span=2a5",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token set max call message size",
			opts: []string{
				"token=" + token,
				"grpc-max-msg-size=8192",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: 8192,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token max call message size no number",
			opts: []string{
				"token=" + token,
				"grpc-max-msg-size=foo",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token set reporting periods",
			opts: []string{
				"token=" + token,
				"min-period=100ms",
				"max-period=2100ms",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             2100 * time.Millisecond,
				MinReportingPeriod:          100 * time.Millisecond,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token and wrong reporting period values should fail",
			opts: []string{
				"token=" + token,
				"min-period=2100ms",
				"max-period=100ms",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token and not parseable min period values should fail",
			opts: []string{
				"token=" + token,
				"min-period=foo",
				"max-period=100ms",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token and not parseable max period values should fail",
			opts: []string{
				"token=" + token,
				"min-period=100ms",
				"max-period=foo",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token and set tags",
			opts: []string{
				"token=" + token,
				"tag=foo=bar",
				"tag=teapot=418",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
					"foo":                      "bar",
					"teapot":                   "418",
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with token and set wrong tag",
			opts: []string{
				"token=" + token,
				"tag=foo",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with token and set empty tag",
			opts: []string{
				"token=" + token,
				"tag=foo=",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
					"foo":                      "",
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with plaintext set",
			opts: []string{
				"token=" + token,
				"collector=collector.example.com:8888",
				"plaintext=true",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host:      "collector.example.com",
					Port:      8888,
					Plaintext: true,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with wrong plaintext set",
			opts: []string{
				"token=" + token,
				"plaintext=foo",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with b3 propagator",
			opts: []string{
				"token=" + token,
				"collector=collector.example.com:8888",
				"propagators=b3",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: "collector.example.com",
					Port: 8888,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: b3Propagator},
		},
		{
			name: "test with ls propagator",
			opts: []string{
				"token=" + token,
				"collector=collector.example.com:8888",
				"propagators=ls",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: "collector.example.com",
					Port: 8888,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with wrong propagator",
			opts: []string{
				"token=" + token,
				"collector=collector.example.com:8888",
				"propagators=foo",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with wrong collector",
			opts: []string{
				"token=" + token,
				"collector=collector.example.com:http",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with wrong collector hostport",
			opts: []string{
				"token=" + token,
				"collector=collector.example.com",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
		{
			name: "test with cmdline",
			opts: []string{
				"token=" + token,
				"cmd-line=hello world",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
					lightstep.CommandLineKey:   "hello world",
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
		{
			name: "test with logevents",
			opts: []string{
				"token=" + token,
				"log-events",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]any{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr:     false,
			propagators: map[opentracing.BuiltinFormat]lightstep.Propagator{opentracing.HTTPHeaders: defPropagator},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptions(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got.Propagators, tt.propagators) {
				t.Logf("diff: %v", cmp.Diff(tt.propagators, got.Propagators))
				t.Errorf("propagators = %v, want %v", got.Propagators, tt.propagators)
			}
			got.Propagators = nil

			if !reflect.DeepEqual(got, tt.want) {
				t.Logf("diff: %v", cmp.Diff(tt.want, got))
				t.Errorf("parseOptions() = %v, want %v", got, tt.want)
			}
		})
	}
}
