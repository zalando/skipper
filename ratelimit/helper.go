package ratelimit

import (
	"context"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

func setError(span opentracing.Span, typ, msg string, tagValue string) {
	if span != nil {
		ext.Error.Set(span, true)
		span.SetTag(typ+".error", tagValue)
		span.LogKV("log", msg)
	}
}

func parentSpan(ctx context.Context) opentracing.Span {
	return opentracing.SpanFromContext(ctx)
}
