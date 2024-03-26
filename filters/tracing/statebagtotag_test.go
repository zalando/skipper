package tracing

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/tracing"
	"github.com/zalando/skipper/tracing/tracingtest"
)

func TestStateBagToOtelAttribute(t *testing.T) {
	req := &http.Request{Header: http.Header{}}

	tr := &tracingtest.OtelTracer{}
	sCtx, span := tr.Start(req.Context(), "start_span")
	req = req.WithContext(sCtx)

	req = req.WithContext(tracing.ContextWithSpan(req.Context(), span))
	ctx := &filtertest.Context{FRequest: req, FStateBag: map[string]interface{}{"item": "val"}, FTracer: tr}

	f, err := NewStateBagToTag().CreateFilter([]interface{}{"item", "tag"})
	require.NoError(t, err)

	f.Request(ctx)
	span.End()

	s, ok := span.(*tracingtest.OtelSpan)
	if !ok {
		t.Fatal("Expected *tracingtest.OtelSpan")
	}
	assert.Equal(t, "val", s.Attributes["tag"])
}

func TestStateBagToTag(t *testing.T) {
	req := &http.Request{Header: http.Header{}}
	tr := &tracing.TracerWrapper{Ot: &tracingtest.OtTracer{}}
	sCtx, span := tr.Start(req.Context(), "start_span")
	req = req.WithContext(sCtx)

	req = req.WithContext(tracing.ContextWithSpan(req.Context(), span))
	ctx := &filtertest.Context{FRequest: req, FStateBag: map[string]interface{}{"item": "val"}, FTracer: tr}

	f, err := NewStateBagToTag().CreateFilter([]interface{}{"item", "tag"})
	require.NoError(t, err)

	f.Request(ctx)
	span.End()

	s, ok := span.(*tracing.SpanWrapper)
	if !ok {
		t.Fatal("Expected span to be of type *tracing.SpanWrapper")
	}
	otSpan, ok := s.Ot.(*tracingtest.OtSpan)
	if !ok {
		t.Fatal("Expected span.Ot to be of type *tracingtest.Span")
	}

	assert.Equal(t, "val", otSpan.Tags["tag"])
}

func TestStateBagToTagAllocs(t *testing.T) {
	req := &http.Request{Header: http.Header{}}

	tr := noop.NewTracerProvider().Tracer("start_tracer")
	sCtx, span := tr.Start(req.Context(), "start_span")
	req = req.WithContext(sCtx)

	req = req.WithContext(tracing.ContextWithSpan(req.Context(), span))
	ctx := &filtertest.Context{FRequest: req, FStateBag: map[string]interface{}{"item": "val"}, FTracer: tr}

	f, err := NewStateBagToTag().CreateFilter([]interface{}{"item", "tag"})
	require.NoError(t, err)

	allocs := testing.AllocsPerRun(100, func() {
		f.Request(ctx)
	})
	if allocs != 0.0 {
		t.Errorf("Expected zero allocations, got %f", allocs)
	}
}

func TestStateBagToTag_CreateFilter(t *testing.T) {
	for _, ti := range []struct {
		msg      string
		args     []interface{}
		stateBag string
		tag      string
		err      error
	}{
		{
			msg:      "state bag and tag",
			args:     []interface{}{"state_bag", "tag"},
			stateBag: "state_bag",
			tag:      "tag",
		},
		{
			msg:      "only state bag",
			args:     []interface{}{"state_bag"},
			stateBag: "state_bag",
			tag:      "state_bag",
		},
		{
			msg:  "no args",
			args: []interface{}{},
			err:  filters.ErrInvalidFilterParameters,
		},
		{
			msg:  "empty arg",
			args: []interface{}{""},
			err:  filters.ErrInvalidFilterParameters,
		},
		{
			msg:  "too many args",
			args: []interface{}{"foo", "bar", "baz"},
			err:  filters.ErrInvalidFilterParameters,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			f, err := NewStateBagToTag().CreateFilter(ti.args)

			assert.Equal(t, ti.err, err)
			if err == nil {
				ff := f.(*stateBagToTagFilter)

				assert.Equal(t, ti.stateBag, ff.stateBagItemName)
				assert.Equal(t, ti.tag, ff.tagName)
			}
		})
	}
}

func BenchmarkStateBagToOtelAttribute_StringValue(b *testing.B) {
	f, err := NewStateBagToTag().CreateFilter([]interface{}{"item", "tag"})
	require.NoError(b, err)

	span := tracingtest.NewSpan("start_span")

	req := &http.Request{Header: http.Header{}}
	req = req.WithContext(tracing.ContextWithSpan(req.Context(), span))

	ctx := &filtertest.Context{FRequest: req, FStateBag: map[string]interface{}{"item": "val"}}
	f.Request(ctx)

	require.Equal(b, "val", span.Attributes["tag"])

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		f.Request(ctx)
	}
}

func BenchmarkStateBagToTag_StringValue(b *testing.B) {
	f, err := NewStateBagToTag().CreateFilter([]interface{}{"item", "tag"})
	require.NoError(b, err)

	span := tracingtest.NewOtSpan("start_span")
	sw := &tracing.SpanWrapper{Ot: span}

	req := &http.Request{Header: http.Header{}}
	req = req.WithContext(tracing.ContextWithSpan(req.Context(), sw))

	ctx := &filtertest.Context{FRequest: req, FStateBag: map[string]interface{}{"item": "val"}}
	f.Request(ctx)

	require.Equal(b, "val", span.Tags["tag"])

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		f.Request(ctx)
	}
}
