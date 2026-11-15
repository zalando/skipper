package tracing_test

import (
	"context"
	"strings"
	"testing"

	originstana "github.com/instana/go-sensor"
	origlightstep "github.com/lightstep/lightstep-tracer-go"
	origbasic "github.com/opentracing/basictracer-go"
	origjaeger "github.com/uber/jaeger-client-go"

	ot "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/zalando/skipper/tracing"
	"github.com/zalando/skipper/tracing/tracingtest"
)

type testSpan struct {
	ot.Span
	spanContext ot.SpanContext
}

func (s *testSpan) Context() ot.SpanContext { return s.spanContext }

type otelSpanContext struct {
	traceID trace.TraceID
}

func (c otelSpanContext) TraceID() trace.TraceID { return c.traceID }

func (otelSpanContext) ForeachBaggageItem(func(k, v string) bool) {}

type unknownSpanContext struct{}

func (unknownSpanContext) ForeachBaggageItem(func(k, v string) bool) {}

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

// LoadPlugin is DEPRECATED but tests are still good for overall coverage
func TestLoadPlugin(t *testing.T) {
	tracer, err := tracing.LoadPlugin(t.TempDir(), []string{"noop"})
	require.NoError(t, err)
	assert.Equal(t, &ot.NoopTracer{}, tracer)
}

// LoadPlugin is DEPRECATED but tests are still good for overall coverage
func TestLoadPlugin_Error(t *testing.T) {
	for _, tt := range []struct {
		name    string
		opts    []string
		expectedErr string
	}{
		{
			name:    "no options",
			expectedErr: "opentracing: the implementation parameter is mandatory",
		},
		{
			name:    "missing plugin file",
			opts:    []string{"missing", "foo=bar"},
			expectedErr: "open module",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tracer, err := tracing.LoadPlugin(t.TempDir(), tt.opts)
			require.ErrorContains(t, err, tt.expectedErr)
			assert.Nil(t, tracer)
		})
	}
}

func TestLoadTracingPlugin(t *testing.T) {
	// `plugin.Open()` not tested because it requires built .so file
	for _, tt := range []struct {
		name       string
		pluginDirs []string
		opts       []string
		expected       ot.Tracer
	}{
		{
			name: "no plugin directories",
			opts: []string{"noop"},
		},
		{
			name:       "noop",
			pluginDirs: []string{t.TempDir()},
			opts:       []string{"noop"},
			expected:       &ot.NoopTracer{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tracer, err := tracing.LoadTracingPlugin(tt.pluginDirs, tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, tracer)
		})
	}
}

func TestLoadTracingPlugin_Errors(t *testing.T) {
	for _, tt := range []struct {
		name       string
		pluginDirs []string
		opts       []string
		expectedErr    string
	}{
		{
			name:       "missing options",
			pluginDirs: []string{t.TempDir()},
			expectedErr:    "opentracing: the implementation parameter is mandatory",
		},
		{
			name:       "missing plugin file in all directories",
			pluginDirs: []string{t.TempDir(), t.TempDir()},
			opts:       []string{"missing"},
			expectedErr:    "open module",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tracer, err := tracing.LoadTracingPlugin(tt.pluginDirs, tt.opts)
			require.ErrorContains(t, err, tt.expectedErr)
			assert.Nil(t, tracer)
		})
	}
}

func TestCreateSpan(t *testing.T) {
	t.Run("without parent span", func(t *testing.T) {
		tracer := tracingtest.NewTracer()

		span := tracing.CreateSpan("test", context.Background(), tracer)
		span.Finish()

		finished := tracer.FindSpan("test")
		require.NotNil(t, finished)
		assert.Zero(t, finished.ParentID)
	})

	t.Run("with parent span", func(t *testing.T) {
		tracer := tracingtest.NewTracer()

		parent := tracer.StartSpan("parent")
		ctx := ot.ContextWithSpan(context.Background(), parent)

		span := tracing.CreateSpan("child", ctx, tracer)
		span.Finish()
		parent.Finish()

		finishedParent := tracer.FindSpan("parent")
		require.NotNil(t, finishedParent)
		finishedChild := tracer.FindSpan("child")
		require.NotNil(t, finishedChild)

		assert.Equal(t, finishedParent.SpanContext.SpanID, finishedChild.ParentID)
		assert.Equal(t, finishedParent.SpanContext.TraceID, finishedChild.SpanContext.TraceID)
	})
}

func TestLogKV(t *testing.T) {
	t.Run("without span", func(t *testing.T) {
		assert.NotPanics(t, func() {
			tracing.LogKV("key", "value", context.Background())
		})
	})

	t.Run("with span", func(t *testing.T) {
		tracer := tracingtest.NewTracer()

		span := tracer.StartSpan("test")
		ctx := ot.ContextWithSpan(context.Background(), span)

		tracing.LogKV("key", "value", ctx)
		span.Finish()

		finished := tracer.FindSpan("test")
		require.NotNil(t, finished)

		logs := finished.Logs()
		require.Len(t, logs, 1)
		require.Len(t, logs[0].Fields, 1)
		assert.Equal(t, "key", logs[0].Fields[0].Key)
		assert.Equal(t, "value", logs[0].Fields[0].ValueString)
	})
}

func TestGetTraceID(t *testing.T) {
	traceHex := strings.Repeat("abcd", 8) // len must equal 32 else error
	otelTraceID, err := trace.TraceIDFromHex(traceHex)
	require.NoError(t, err)

	for _, tt := range []struct {
		name string
		span ot.Span
		expected string
	}{
		{
			name: "nil span",
		},
		{
			name: "nil span context",
			span: &testSpan{},
		},
		{
			name: "unknown span context",
			span: &testSpan{spanContext: unknownSpanContext{}},
		},
		{
			name: "basic",
			span: &testSpan{spanContext: origbasic.SpanContext{TraceID: 0xabc}},
			expected: "abc",
		},
		{
			name: "instana",
			span: &testSpan{spanContext: originstana.SpanContext{TraceID: 0xabc}},
			expected: "abc",
		},
		{
			name: "jaeger",
			span: &testSpan{spanContext: origjaeger.NewSpanContext(origjaeger.TraceID{Low: 0xabc}, 1, 0, true, nil)},
			expected: "0000000000000abc",
		},
		{
			name: "jaeger with 128 bit trace id",
			span: &testSpan{spanContext: origjaeger.NewSpanContext(origjaeger.TraceID{High: 0x1, Low: 0xabc}, 1, 0, true, nil)},
			expected: "00000000000000010000000000000abc",
		},
		{
			name: "lightstep",
			span: &testSpan{spanContext: origlightstep.SpanContext{TraceID: 0xabc}},
			expected: "abc",
		},
		{
			name: "opentelemetry",
			span: &testSpan{spanContext: otelSpanContext{traceID: otelTraceID}},
			expected: "abcdabcdabcdabcdabcdabcdabcdabcd",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tracing.GetTraceID(tt.span))
		})
	}
}
