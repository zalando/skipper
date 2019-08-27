package jaeger

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go/config"
)

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    []string
		want    *config.Configuration
		wantErr bool
	}{
		{
			name: "defaults",
			opts: []string{},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{},
				Reporter:    &config.ReporterConfig{},
			},
		},
		{
			name: "set service-name",
			opts: []string{"service-name=skipper-internal"},
			want: &config.Configuration{
				ServiceName: "skipper-internal",
				Sampler:     &config.SamplerConfig{},
				Reporter:    &config.ReporterConfig{},
			},
		},
		{
			name: "set use-rpc-metrics",
			opts: []string{"use-rpc-metrics"},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{},
				Reporter:    &config.ReporterConfig{},
				RPCMetrics:  true,
			},
		},
		{
			name: "set sampler-type const",
			opts: []string{"sampler-type=const"},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{Type: "const", Param: 1},
				Reporter:    &config.ReporterConfig{},
			},
		},
		{
			name: "set sampler-type probabilistic",
			opts: []string{"sampler-type=probabilistic:0.01"},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{Type: "probabilistic", Param: 0.01},
				Reporter:    &config.ReporterConfig{},
			},
		},
		{
			name: "set sampler-type rateLimiting",
			opts: []string{"sampler-type=rateLimiting:0.01"},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{Type: "rateLimiting", Param: 0.01},
				Reporter:    &config.ReporterConfig{},
			},
		},
		{
			name: "set sampler-type remote",
			opts: []string{"sampler-type=remote:0.01"},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{Type: "remote", Param: 0.01},
				Reporter:    &config.ReporterConfig{},
			},
		},
		{
			name: "set sampler-url",
			opts: []string{"sampler-url=http://example"},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{SamplingServerURL: "http://example"},
				Reporter:    &config.ReporterConfig{},
			},
		},
		{
			name: "set reporter-queue",
			opts: []string{"reporter-queue=10"},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{},
				Reporter:    &config.ReporterConfig{QueueSize: 10},
			},
		},
		{
			name: "set reporter-interval",
			opts: []string{"reporter-interval=1h"},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{},
				Reporter:    &config.ReporterConfig{BufferFlushInterval: 1 * time.Hour},
			},
		},
		{
			name: "set local-agent",
			opts: []string{"local-agent=127.0.0.1:6811"},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{},
				Reporter:    &config.ReporterConfig{LocalAgentHostPort: "127.0.0.1:6811"},
			},
		},
		{
			name: "set tag",
			opts: []string{"tag=environment=production"},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{},
				Reporter:    &config.ReporterConfig{},
				Tags:        []opentracing.Tag{{Key: "environment", Value: "production"}},
			},
		},
		{
			name: "set tag multiple",
			opts: []string{
				"tag=environment=production",
				"tag=region=eu-west-1",
			},
			want: &config.Configuration{
				ServiceName: defServiceName,
				Sampler:     &config.SamplerConfig{},
				Reporter:    &config.ReporterConfig{},
				Tags: []opentracing.Tag{
					{Key: "environment", Value: "production"},
					{Key: "region", Value: "eu-west-1"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptions(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("jaeger.parseOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Logf("diff: %v", cmp.Diff(tt.want, got))
				t.Errorf("parseOptions() = %v, want %v", got, tt.want)
			}
		})
	}
}
