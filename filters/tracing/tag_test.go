package tracing

import (
	"net/http"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestTracingTagNil(t *testing.T) {
	context := &filtertest.Context{
		FRequest: &http.Request{},
	}
	context.FRequest = context.FRequest.WithContext(opentracing.ContextWithSpan(context.FRequest.Context(), nil))

	s := NewTag()
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

func TestTagName(t *testing.T) {
	if NewTag().Name() != filters.TracingTagName {
		t.Error("Wrong tag spec name")
	}
	if NewTagFromResponse().Name() != filters.TracingTagFromResponseName {
		t.Error("Wrong tag spec name")
	}
}

func TestTagCreateFilter(t *testing.T) {
	spec := tagSpec{}
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

func TestTracingTag(t *testing.T) {
	tracer := mocktracer.New()

	for _, ti := range []struct {
		name     string
		spec     filters.Spec
		value    string
		context  *filtertest.Context
		expected interface{}
	}{{
		"plain key value",
		NewTag(),
		"test_value",
		&filtertest.Context{
			FRequest: &http.Request{},
		},
		"test_value",
	}, {
		"tag from header",
		NewTag(),
		"${request.header.X-Flow-Id}",
		&filtertest.Context{
			FRequest: &http.Request{
				Header: http.Header{
					"X-Flow-Id": []string{"foo"},
				},
			},
		},
		"foo",
	}, {
		"tag from response",
		NewTagFromResponse(),
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
		NewTag(),
		"${request.header.missing}",
		&filtertest.Context{
			FRequest: &http.Request{},
		},
		nil,
	},
	} {
		t.Run(ti.name, func(t *testing.T) {
			span := tracer.StartSpan("proxy").(*mocktracer.MockSpan)
			defer span.Finish()

			ti.context.FRequest = ti.context.FRequest.WithContext(opentracing.ContextWithSpan(ti.context.FRequest.Context(), span))

			f, err := ti.spec.CreateFilter([]interface{}{"test_tag", ti.value})
			if err != nil {
				t.Fatal(err)
			}

			f.Request(ti.context)
			f.Response(ti.context)

			if got := span.Tag("test_tag"); got != ti.expected {
				t.Errorf("unexpected tag value '%v' != '%v'", got, ti.expected)
			}
		})
	}
}

func TestTagFilterIgnoresResponse(t *testing.T) {
	tracer := mocktracer.New()
	span := tracer.StartSpan("proxy").(*mocktracer.MockSpan)
	defer span.Finish()

	requestContext := &filtertest.Context{
		FRequest: &http.Request{},
	}
	requestContext.FRequest = requestContext.FRequest.WithContext(opentracing.ContextWithSpan(requestContext.FRequest.Context(), span))

	f, err := NewTag().CreateFilter([]interface{}{"test_tag", "${response.header.X-Fallback}"})
	if err != nil {
		t.Fatal(err)
	}

	f.Request(requestContext)

	requestContext.FResponse = &http.Response{
		Header: http.Header{
			"X-Fallback": []string{"true"},
		},
	}

	f.Response(requestContext)

	if got := span.Tag("test_tag"); got != nil {
		t.Errorf("unexpected tag value '%v' != '%v'", got, nil)
	}
}

func TestTagFromResponseFilterIgnoresRequest(t *testing.T) {
	tracer := mocktracer.New()
	span := tracer.StartSpan("proxy").(*mocktracer.MockSpan)
	defer span.Finish()

	requestContext := &filtertest.Context{
		FRequest: &http.Request{
			Header: http.Header{
				"X-Flow-Id": []string{"a-flow-id"},
			},
		},
	}
	requestContext.FRequest = requestContext.FRequest.WithContext(opentracing.ContextWithSpan(requestContext.FRequest.Context(), span))

	f, err := NewTagFromResponse().CreateFilter([]interface{}{"test_tag", "${request.header.X-Flow-Id}"})
	if err != nil {
		t.Fatal(err)
	}

	f.Request(requestContext)

	responseContext := &filtertest.Context{FRequest: &http.Request{}}
	responseContext.FRequest = responseContext.FRequest.WithContext(opentracing.ContextWithSpan(responseContext.FRequest.Context(), span))

	f.Response(responseContext)

	if got := span.Tag("test_tag"); got != nil {
		t.Errorf("unexpected tag value '%v' != '%v'", got, nil)
	}
}
