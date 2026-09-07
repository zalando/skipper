package otel

import (
	"context"
	"net/http"
	"os"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
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

// TestPropagatorExtractMalformedTraceparent documents a trace-disconnection
// bug. A spec-valid traceparent carrying a 64-bit ottrace id padded into W3C's
// 128-bit form is accepted by tracecontext, but the chain [tracecontext,
// ottrace, b3multi, baggage] lets b3multi run last and overwrite the
// SpanContext with an unrelated b3 trace-id from an intermediate hop, so spans
// exported under always_on appear disconnected in the backend UI.
//
// Subtests compare three configs:
//
//	(1) current prod (reproduces the bug)
//	(2) a reorder-only workaround keeping b3multi,
//	(3) the target config dropping b3multi, matching the pre-OTel LightStep
//	(ottrace only) baseline via lightstep.PropagatorStack + LightStepPropagator.
//
// wantSampled reflects the upstream-signalled flag on the extracted parent
// SpanContext.
func TestPropagatorExtractMalformedTraceparent(t *testing.T) {
	const (
		otTraceID64  = "a66e94ab3ffc4fa1"
		otTraceID128 = "0000000000000000a66e94ab3ffc4fa1"
		otSpanID     = "28c00ede49b44eb1"
		b3TraceID64  = "e132d03200cd1ed1"
		b3TraceID128 = "0000000000000000e132d03200cd1ed1"
		b3SpanID     = "e132d03200cd1ed1"
	)

	// Trace header from the upstream request that has smelly traceparent and b3
	// headers that are not consistent with the traceparent not ottrace headers.
	requestHeader := http.Header{
		"Traceparent":       []string{"00-" + otTraceID128 + "-" + otSpanID + "-01"},
		"Ot-Tracer-Traceid": []string{otTraceID64},
		"Ot-Tracer-Spanid":  []string{otSpanID},
		"Ot-Tracer-Sampled": []string{"true"},
		"X-B3-Traceid":      []string{b3TraceID64},
		"X-B3-Spanid":       []string{b3SpanID},
		"Baggage":           []string{"traffic_info=traffic_type=allowed-crawler"},
	}

	for _, tt := range []struct {
		name             string
		propagators      []string
		wantTraceID      string
		wantSpanID       string
		wantSampled      bool
		wantSpanValid    bool
		wantMatchesCause string
	}{
		{
			// b3multi runs last among span-context writers and overwrites the
			// SpanContext produced by tracecontext and ottrace with a b3 trace
			// id injected by an intermediate Finagle hop.
			name:             "current config - b3multi wins, trace disconnected",
			propagators:      []string{"tracecontext", "ottrace", "b3multi", "baggage"},
			wantTraceID:      b3TraceID128,
			wantSpanID:       b3SpanID,
			wantSampled:      false,
			wantSpanValid:    true,
			wantMatchesCause: "b3multi ran last among span-context writers; X-B3-Sampled absent so trace flag is 0",
		},
		{
			// Keeping b3multi. tracecontext still runs last and accepts the smelly
			// traceparent, so the resulting trace-id is the padded 128-bit form.
			// This happens to match ottrace's trace, so the trace stays connected
			// but only by coincidence.
			name:             "reorder to prefer tracecontext - accidentally connects to ottrace",
			propagators:      []string{"baggage", "b3multi", "ottrace", "tracecontext"},
			wantTraceID:      otTraceID128,
			wantSpanID:       otSpanID,
			wantSampled:      true,
			wantSpanValid:    true,
			wantMatchesCause: "tracecontext ran last and accepted the padded traceparent; span id happens to match ottrace",
		},
		{
			// Target config after removing b3multi. Rationale: the prior
			// OpenTracing config only had `propagators=lightstep` (ottrace
			// only) - see lightstep.PropagatorStack + LightStepPropagator - so
			// dropping b3multi does not regress against the pre-OTel baseline.
			//
			// Under this config the upstream request lands on the ottrace
			// SpanContext (ottrace runs after tracecontext and overwrites).
			// The smelly traceparent still parses in tracecontext, but ottrace
			// overwrites it with the same trace-id (64-bit id padded) and the
			// same span-id, so the trace stays connected.
			name:             "target config - drop b3multi, ottrace wins",
			propagators:      []string{"tracecontext", "ottrace", "baggage"},
			wantTraceID:      otTraceID128,
			wantSpanID:       otSpanID,
			wantSampled:      true,
			wantSpanValid:    true,
			wantMatchesCause: "ottrace runs after tracecontext and overwrites with the ottrace-derived trace-id",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, err := textMapPropagator(&Options{Propagators: tt.propagators})
			if err != nil {
				t.Fatalf("textMapPropagator: %v", err)
			}

			ctx := p.Extract(context.Background(), propagation.HeaderCarrier(requestHeader))
			sc := trace.SpanContextFromContext(ctx)

			if got, want := sc.IsValid(), tt.wantSpanValid; got != want {
				t.Fatalf("SpanContext.IsValid() = %v, want %v (%s)", got, want, tt.wantMatchesCause)
			}
			if !tt.wantSpanValid {
				return
			}
			if got, want := sc.TraceID().String(), tt.wantTraceID; got != want {
				t.Errorf("TraceID = %q, want %q (%s)", got, want, tt.wantMatchesCause)
			}
			if got, want := sc.SpanID().String(), tt.wantSpanID; got != want {
				t.Errorf("SpanID = %q, want %q (%s)", got, want, tt.wantMatchesCause)
			}
			if got, want := sc.IsSampled(), tt.wantSampled; got != want {
				t.Errorf("IsSampled = %v, want %v (%s)", got, want, tt.wantMatchesCause)
			}
		})
	}
}
