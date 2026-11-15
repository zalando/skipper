package tracing_test

import (
	"testing"

	origbasic "github.com/opentracing/basictracer-go"
	ot "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper/tracing"
)

func TestInitTracer(t *testing.T) {
	for _, tt := range []struct {
		name string
		opts []string
		expected ot.Tracer
	}{
		{
			name: "noop",
			opts: []string{"noop"},
			expected: &ot.NoopTracer{},
		},
		{
			name: "noop ignores arguments",
			opts: []string{"noop", "foo=bar"},
			expected: &ot.NoopTracer{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tracer, err := tracing.InitTracer(tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, tracer)
		})
	}
}

func TestInitTracer_Error(t *testing.T) {
	for _, tt := range []struct {
		name    string
		opts    []string
		expectedErr string
	}{
		{
			name:    "nil options",
			expectedErr: "opentracing: the implementation parameter is mandatory",
		},
		{
			name:    "empty options",
			opts:    []string{},
			expectedErr: "opentracing: the implementation parameter is mandatory",
		},
		{
			name:    "unsupported implementation",
			opts:    []string{"unknown"},
			expectedErr: "tracer 'unknown' not supported",
		},
		{
			name:    "basic with invalid argument",
			opts:    []string{"basic", "sample-modulo=foo"},
			expectedErr: "invalid argument for sample-modulo option",
		},
		{
			name:    "basic with missing argument",
			opts:    []string{"basic", "max-logs-per-span"},
			expectedErr: "missing argument for max-logs-per-span option",
		},
		// instana has no possible errors in initializer
		{
			name:    "jaeger with missing argument",
			opts:    []string{"jaeger", "sampler-type"},
			expectedErr: "missing argument for sampler-type option",
		},
		{
			name:    "lightstep without token",
			opts:    []string{"lightstep"},
			expectedErr: "missing token",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tracer, err := tracing.InitTracer(tt.opts)
			require.ErrorContains(t, err, tt.expectedErr)
			assert.Nil(t, tracer)
		})
	}
}

func TestInitTracerBasic(t *testing.T) {
	// do not test real instana/jaeger/lightstep init
	// because they start network reporters
	opts := []string{
		"basic",
		"drop-all-logs",
		"sample-modulo=2",
		"max-logs-per-span=5",
		"recorder=in-memory",
	}
	tracer, err := tracing.InitTracer(opts)
	require.NoError(t, err)
	require.NotNil(t, tracer)

	closer, ok := tracer.(interface{ Close() })
	require.True(t, ok)
	defer closer.Close()

	span := tracer.StartSpan("test")
	defer span.Finish()

	assert.IsType(t, origbasic.SpanContext{}, span.Context())
	assert.NotEmpty(t, tracing.GetTraceID(span))
}
