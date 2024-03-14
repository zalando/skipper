package tracing

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/tracing"
	"github.com/zalando/skipper/tracing/tracingtest"
)

func TestOtelBaggageItemNameToTag(t *testing.T) {
	for _, ti := range []struct {
		msg              string
		baggageItemName  string
		baggageItemValue string
		tagName          string
	}{{
		"should add span tag for baggage item",
		"baggage_name",
		"push",
		"tag_name",
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			req := &http.Request{Header: http.Header{}}

			tr := &tracingtest.OtelTracer{}
			sCtx, span := tr.Start(req.Context(), "start_span")
			req = req.WithContext(sCtx)
			bCtx, err := tracing.SetBaggageMember(req.Context(), span, ti.baggageItemName, ti.baggageItemValue)
			if err != nil {
				t.Error(err)
				return
			}
			req = req.WithContext(bCtx)
			ctx := &filtertest.Context{FRequest: req, FTracer: tr}

			f, err := NewBaggageToTagFilter().CreateFilter([]interface{}{ti.baggageItemName, ti.tagName})
			if err != nil {
				t.Error(err)
				return
			}

			f.Request(ctx)
			span.End()

			s, ok := span.(*tracingtest.OtelSpan)
			if !ok {
				t.Fatal("Expected *tracingtest.OtelSpan")
			}

			if tagValue := s.Attributes[ti.tagName]; ti.baggageItemValue != tagValue {
				t.Error("couldn't set span tag from baggage item")
			}
		})
	}
}

func TestBaggageItemNameToTag(t *testing.T) {
	for _, ti := range []struct {
		msg              string
		baggageItemName  string
		baggageItemValue string
		tagName          string
	}{{
		"should add span tag for baggage item",
		"baggage_name",
		"push",
		"tag_name",
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			req := &http.Request{Header: http.Header{}}

			tr := &tracing.TracerWrapper{Ot: &tracingtest.OtTracer{}}
			sCtx, span := tr.Start(req.Context(), "start_span")
			req = req.WithContext(sCtx)

			bCtx, err := tracing.SetBaggageMember(req.Context(), span, ti.baggageItemName, ti.baggageItemValue)
			if err != nil {
				t.Error(err)
				return
			}
			req = req.WithContext(bCtx)
			ctx := &filtertest.Context{FRequest: req, FTracer: tr}

			f, err := NewBaggageToTagFilter().CreateFilter([]interface{}{ti.baggageItemName, ti.tagName})
			if err != nil {
				t.Error(err)
				return
			}

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

			if tagValue := otSpan.Tags[ti.tagName]; ti.baggageItemValue != tagValue {
				t.Error("couldn't set span tag from baggage item")
			}
		})
	}
}

func TestCreateFilter(t *testing.T) {
	for _, ti := range []struct {
		msg             string
		baggageItemName string
		tagName         string
		err             error
	}{{
		"should create filter with baggage item and span tag names",
		"baggage_name",
		"tag_name",
		nil,
	}, {
		"should not have empty baggage name or tag name",
		"",
		"",
		filters.ErrInvalidFilterParameters,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			var err error
			if ti.tagName == "" {
				_, err = NewBaggageToTagFilter().CreateFilter([]interface{}{
					ti.baggageItemName,
				})
			} else {
				_, err = NewBaggageToTagFilter().CreateFilter([]interface{}{
					ti.baggageItemName,
					ti.tagName,
				})
			}

			if err != ti.err {
				t.Error(err)
				return
			}

		})
	}
}

func TestOtelFallbackToBaggageNameForTag(t *testing.T) {
	for _, ti := range []struct {
		msg              string
		baggageItemName  string
		baggageItemValue string
		err              error
	}{{
		"should create filter and use baggage name when tag name is not provided",
		"baggage_name",
		"baggageValue",
		nil,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			req := &http.Request{Header: http.Header{}}

			tr := &tracingtest.OtelTracer{}
			sCtx, span := tr.Start(req.Context(), "start_span")
			req = req.WithContext(sCtx)

			bCtx, err := tracing.SetBaggageMember(req.Context(), span, ti.baggageItemName, ti.baggageItemValue)
			if err != nil {
				t.Error(err)
				return
			}
			req = req.WithContext(bCtx)
			ctx := &filtertest.Context{FRequest: req, FTracer: tr}

			f, err := NewBaggageToTagFilter().CreateFilter([]interface{}{ti.baggageItemName})
			if err != nil {
				t.Error(err)
				return
			}

			f.Request(ctx)
			span.End()

			s, ok := span.(*tracingtest.OtelSpan)
			if !ok {
				t.Fatal("Expected *tracingtest.OtelSpan")
			}
			if tagValue := s.Attributes[ti.baggageItemName]; ti.baggageItemValue != tagValue {
				t.Error("couldn't set span tag from baggage item")
			}
		})
	}
}

func TestFallbackToBaggageNameForTag(t *testing.T) {
	for _, ti := range []struct {
		msg              string
		baggageItemName  string
		baggageItemValue string
		err              error
	}{{
		"should create filter and use baggage name when tag name is not provided",
		"baggage_name",
		"baggageValue",
		nil,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			req := &http.Request{Header: http.Header{}}

			tr := &tracing.TracerWrapper{Ot: &tracingtest.OtTracer{}}
			sCtx, span := tr.Start(req.Context(), "start_span")
			req = req.WithContext(sCtx)

			bCtx, err := tracing.SetBaggageMember(req.Context(), span, ti.baggageItemName, ti.baggageItemValue)
			if err != nil {
				t.Error(err)
				return
			}
			req = req.WithContext(bCtx)
			ctx := &filtertest.Context{FRequest: req, FTracer: tr}

			f, err := NewBaggageToTagFilter().CreateFilter([]interface{}{ti.baggageItemName})
			if err != nil {
				t.Error(err)
				return
			}

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

			if tagValue := otSpan.Tags[ti.baggageItemName]; ti.baggageItemValue != tagValue {
				t.Error("couldn't set span tag from baggage item")
			}
		})
	}
}
