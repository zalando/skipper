package otel

import (
	"context"
	"os"
	"testing"
)

func TestOtel(t *testing.T) {
	for _, tt := range []struct {
		name      string
		opt       *Options
		env       map[string]string
		errString string
	}{
		{
			name: "test otel default",
			opt:  &Options{},
		},
		{
			name: "test otel initialized",
			opt:  &Options{Initialized: true},
		},
		{
			name: "test otel exporter otlp with endpoint",
			opt: &Options{
				TracesExporter: "otlp",
				ExporterOtlp: ExporterOtlp{
					Endpoint: "http://otlp-exporter.example",
				},
			},
		},
		{
			name: "test otel exporter otlp with endpoint and protcol grpc",
			opt: &Options{
				TracesExporter: "otlp",
				ExporterOtlp: ExporterOtlp{
					Endpoint: "http://otlp-exporter.example",
					Protocol: "grpc",
				},
			},
		},
		{
			name: "test otel exporter otlp with endpoint and protcol http/protobuf",
			opt: &Options{
				TracesExporter: "otlp",
				ExporterOtlp: ExporterOtlp{
					Endpoint: "http://otlp-exporter.example",
					Protocol: "http/protobuf",
				},
			},
		},
		{
			name: "test otel exporter otlp with endpoint and protcol unknown",
			opt: &Options{
				TracesExporter: "otlp",
				ExporterOtlp: ExporterOtlp{
					Endpoint: "http://otlp-exporter.example",
					Protocol: "unknown",
				},
			},
			errString: "invalid OTLP protocol unknown - should be one of ['grpc', 'http/protobuf']",
		},
		{
			name:      "test otel exporter otlp without endpoint",
			opt:       &Options{TracesExporter: "otlp"},
			errString: "OTLP endpoint is required",
		},
		{
			name: "test otel exporter console",
			opt:  &Options{TracesExporter: "console"},
		},
		{
			name: "test otel exporter skipper-debug",
			opt:  &Options{TracesExporter: "skipper-debug"},
		},
		{
			name: "test otel exporter auto",
			opt:  &Options{TracesExporter: "auto"},
			env: map[string]string{
				"OTEL_TRACES_EXPORTER": "console",
			},
		},
		{
			name: "test otel exporter auto with batchers",
			opt: &Options{
				TracesExporter: "auto",
				BatchSpanProcessor: BatchSpanProcessor{
					ScheduleDelay:      1,
					ExportTimeout:      1,
					MaxQueueSize:       1,
					MaxExportBatchSize: 1,
				},
			},
			env: map[string]string{
				"OTEL_TRACES_EXPORTER": "console",
			},
		},
		{
			name: "test otel exporter console with resources",
			opt: &Options{
				TracesExporter: "console",
				ResourceAttributes: map[string]string{
					"foo": "bar",
				},
			},
		},
		{
			name: "test otel exporter console with propagator none",
			opt: &Options{
				TracesExporter: "console",
				Propagators:    []string{"none"},
			},
		},
		{
			name: "test otel exporter console with propagator baggage",
			opt: &Options{
				TracesExporter: "console",
				Propagators:    []string{"baggage"},
			},
		},
		{
			name: "test otel sampler always_on",
			opt: &Options{
				TracesExporter: "console",
				Sampler:        "always_on",
			},
		},
		{
			name: "test otel sampler always_off",
			opt: &Options{
				TracesExporter: "console",
				Sampler:        "always_off",
			},
		},
		{
			name: "test otel sampler traceidratio without arg",
			opt: &Options{
				TracesExporter: "console",
				Sampler:        "traceidratio",
			},
		},
		{
			name: "test otel sampler traceidratio with arg",
			opt: &Options{
				TracesExporter: "console",
				Sampler:        "traceidratio",
				SamplerArg:     "0.5",
			},
		},
		{
			name: "test otel sampler parentbased_always_on",
			opt: &Options{
				TracesExporter: "console",
				Sampler:        "parentbased_always_on",
			},
		},
		{
			name: "test otel sampler parentbased_always_off",
			opt: &Options{
				TracesExporter: "console",
				Sampler:        "parentbased_always_off",
			},
		},
		{
			name: "test otel sampler parentbased_traceidratio",
			opt: &Options{
				TracesExporter: "console",
				Sampler:        "parentbased_traceidratio",
				SamplerArg:     "0.1",
			},
		},
		{
			name: "test otel sampler unknown",
			opt: &Options{
				TracesExporter: "console",
				Sampler:        "unknown",
			},
			errString: "invalid sampler \"unknown\" - should be one of ['always_on', 'always_off', 'traceidratio', 'parentbased_always_on', 'parentbased_always_off', 'parentbased_traceidratio']",
		},
		{
			name: "test otel sampler traceidratio with invalid arg",
			opt: &Options{
				TracesExporter: "console",
				Sampler:        "traceidratio",
				SamplerArg:     "not-a-float",
			},
			errString: `invalid sampler argument "not-a-float": strconv.ParseFloat: parsing "not-a-float": invalid syntax`,
		},
		{
			name: "test otel sampler traceidratio with out of range arg",
			opt: &Options{
				TracesExporter: "console",
				Sampler:        "traceidratio",
				SamplerArg:     "1.5",
			},
			errString: `invalid sampler argument "1.5": trace ID ratio must be in range [0.0, 1.0]`,
		},
		{
			name: "test otel sampler empty falls back to env",
			opt: &Options{
				TracesExporter: "console",
			},
			env: map[string]string{
				"OTEL_TRACES_SAMPLER": "always_on",
			},
		}} {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				if err := os.Setenv(k, v); err != nil {
					t.Fatalf("Failed to set env: %q -> %q: %v", k, v, err)
				}
			}
			shutdown, err := Init(context.Background(), tt.opt)
			if err != nil {
				if tt.errString == "" {
					t.Fatalf("Failed to init OTel want no error, got: %v", err)
				} else {
					if tt.errString != err.Error() {
						t.Fatalf("Failed to get error want: %q, got %q", tt.errString, err.Error())
					}
					return
				}
			} else {
				if tt.errString != "" {
					t.Fatalf("Failed to get wanted error: %q", tt.errString)
				}
			}
			err = shutdown(context.Background())
			if err != nil {
				t.Fatalf("Failed to shutdown OTel: %v", err)
			}
		})
	}

}
