package lightstep

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	lightstep "github.com/lightstep/lightstep-tracer-go"
)

func TestParseOptions(t *testing.T) {
	token := "mytoken"

	tests := []struct {
		name    string
		opts    []string
		want    lightstep.Options
		wantErr bool
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
				Tags: map[string]interface{}{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr: false,
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
				Tags: map[string]interface{}{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr: false,
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
				Tags: map[string]interface{}{
					lightstep.ComponentNameKey: "skipper-ingress",
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr: false,
		},
		{
			name: "test with token set tags",
			opts: []string{
				"token=" + token,
				"tag=cluster=my-test",
				"tag=foo=bar",
			},
			want: lightstep.Options{
				AccessToken: token,
				Collector: lightstep.Endpoint{
					Host: lightstep.DefaultGRPCCollectorHost,
					Port: lightstep.DefaultSecurePort,
				},
				UseGRPC: true,
				Tags: map[string]interface{}{
					lightstep.ComponentNameKey: defComponentName,
					"cluster":                  "my-test",
					"foo":                      "bar",
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr: false,
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
				Tags: map[string]interface{}{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
				MaxBufferedSpans:            8192,
			},
			wantErr: false,
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
				Tags: map[string]interface{}{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: 8192,
				ReportingPeriod:             lightstep.DefaultMaxReportingPeriod,
				MinReportingPeriod:          lightstep.DefaultMinReportingPeriod,
			},
			wantErr: false,
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
				Tags: map[string]interface{}{
					lightstep.ComponentNameKey: defComponentName,
				},
				GRPCMaxCallSendMsgSizeBytes: defaultGRPMaxMsgSize,
				ReportingPeriod:             2100 * time.Millisecond,
				MinReportingPeriod:          100 * time.Millisecond,
			},
			wantErr: false,
		},
		{
			name: "test with token and wront reporting period values should fail",
			opts: []string{
				"token=" + token,
				"min-period=2100ms",
				"max-period=100ms",
			},
			want:    lightstep.Options{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptions(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Logf("diff: %v", cmp.Diff(tt.want, got))
				t.Errorf("parseOptions() = %v, want %v", got, tt.want)
			}
		})
	}
}
