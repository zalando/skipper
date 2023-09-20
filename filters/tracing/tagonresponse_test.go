package tracing

import (
	"net/http"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestTracingTagOnResponseNil(t *testing.T) {
	context := &filtertest.Context{
		FRequest: &http.Request{},
	}
	context.FRequest = context.FRequest.WithContext(opentracing.ContextWithSpan(context.FRequest.Context(), nil))

	s := NewTagOnResponse()
	f, err := s.CreateFilter([]interface{}{"test_tag", "foo"})
	if err != nil {
		t.Fatal(err)
	}

	f.Request(context)

	span := opentracing.SpanFromContext(context.Request().Context())
	if span != nil {
		t.Errorf("span should be nil, but is '%v'", span)
	}
}

func TestTracingTagOnResponseTagName(t *testing.T) {
	if (tagOnResponseSpec{}).Name() != filters.TracingTagOnResponseName {
		t.Error("Wrong tag spec name")
	}
}
func TestTracingTagOnResponseCreateFilter(t *testing.T) {
	spec := tagOnResponseSpec{}
	if _, err := spec.CreateFilter(nil); err != filters.ErrInvalidFilterParameters {
		t.Errorf("Create filter without args should return error")
	}

	if _, err := spec.CreateFilter([]interface{}{3, "foo"}); err != filters.ErrInvalidFilterParameters {
		t.Errorf("Create filter without first arg is string should return error")
	}

	if _, err := spec.CreateFilter([]interface{}{"foo", 3}); err != filters.ErrInvalidFilterParameters {
		t.Errorf("Create filter without second arg is string should return error")
	}
}

func TestTracingTagOnResponseTag(t *testing.T) {
	tracer := mocktracer.New()

	for _, ti := range []struct {
		name     string
		value    string
		context  *filtertest.Context
		expected interface{}
	}{{
		"plain key value",
		"test_value",
		&filtertest.Context{
			FResponse: &http.Response{},
		},
		"test_value",
	}, {
		"tag from header",
		"${response.header.X-Fallback}",
		&filtertest.Context{
			FRequest: &http.Request{},
			FResponse: &http.Response{
				Header: http.Header{
					"X-Fallback": []string{"true"},
				},
			},
		},
		"true",
	}, {
		"tag from missing header",
		"${response.header.missing}",
		&filtertest.Context{
			FResponse: &http.Response{},
		},
		nil,
	},
	} {
		t.Run(ti.name, func(t *testing.T) {
			span := tracer.StartSpan("proxy").(*mocktracer.MockSpan)
			defer span.Finish()

			ti.context.FRequest = ti.context.FRequest.WithContext(opentracing.ContextWithSpan(ti.context.FRequest.Context(), span))

			s := NewTagOnResponse()
			f, err := s.CreateFilter([]interface{}{"test_tag", ti.value})
			if err != nil {
				t.Fatal(err)
			}

			f.Request(ti.context)

			if got := span.Tag("test_tag"); got != ti.expected {
				t.Errorf("unexpected tag value '%v' != '%v'", got, ti.expected)
			}

			f.Response(ti.context)
		})
	}
}
