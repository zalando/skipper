package tracing

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/tracing"
	"github.com/zalando/skipper/tracing/tracingtest"
)

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

			span := &tracing.SpanWrapper{Ot: tracingtest.NewSpan("start_span")}
			span.Ot.SetBaggageItem(ti.baggageItemName, ti.baggageItemValue)
			req = req.WithContext(tracing.ContextWithSpan(req.Context(), span))
			ctx := &filtertest.Context{FRequest: req}

			f, err := NewBaggageToTagFilter().CreateFilter([]interface{}{ti.baggageItemName, ti.tagName})
			if err != nil {
				t.Error(err)
				return
			}

			f.Request(ctx)

			otSpan, ok := span.Ot.(*tracingtest.OtSpan)
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

			span := &tracing.SpanWrapper{Ot: tracingtest.NewSpan("start_span")}
			span.Ot.SetBaggageItem(ti.baggageItemName, ti.baggageItemValue)
			req = req.WithContext(tracing.ContextWithSpan(req.Context(), span))
			ctx := &filtertest.Context{FRequest: req}

			f, err := NewBaggageToTagFilter().CreateFilter([]interface{}{ti.baggageItemName})
			if err != nil {
				t.Error(err)
				return
			}

			f.Request(ctx)

			otSpan, ok := span.Ot.(*tracingtest.OtSpan)
			if !ok {
				t.Fatal("Expected span.Ot to be of type *tracingtest.Span")
			}

			if tagValue := otSpan.Tags[ti.baggageItemName]; ti.baggageItemValue != tagValue {
				t.Error("couldn't set span tag from baggage item")
			}
		})
	}
}
