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
	if NewTagFromResponseIfStatus().Name() != filters.TracingTagFromResponseIfStatusName {
		t.Error("Wrong tag spec name")
	}
}

func TestTagCreateFilter(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []interface{}
		spec filters.Spec
		want error
	}{
		{
			name: "create filter with unknown filter",
			spec: &tagSpec{},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with no args",
			spec: NewTag(),
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with no args",
			spec: NewTagFromResponse(),
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with no args",
			spec: NewTagFromResponseIfStatus(),
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with one args",
			spec: NewTag(),
			args: []interface{}{"foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with one args",
			spec: NewTagFromResponse(),
			args: []interface{}{"foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with one args",
			spec: NewTagFromResponseIfStatus(),
			args: []interface{}{"foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTag(),
			args: []interface{}{"foo", 3},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTagFromResponse(),
			args: []interface{}{"foo", 3},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTagFromResponseIfStatus(),
			args: []interface{}{"foo", 3},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTag(),
			args: []interface{}{3, "foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTagFromResponse(),
			args: []interface{}{3, "foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTagFromResponseIfStatus(),
			args: []interface{}{3, "foo"},
			want: filters.ErrInvalidFilterParameters,
		},

		// special
		{
			name: "create filter with three args want 2",
			spec: NewTag(),
			args: []interface{}{"foo", "bar", "qux"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with three args want 2",
			spec: NewTagFromResponse(),
			args: []interface{}{"foo", "bar", "qux"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args want 3 args",
			spec: NewTagFromResponseIfStatus(),
			args: []interface{}{"foo", "bar"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with three args wrong args",
			spec: NewTagFromResponseIfStatus(),
			args: []interface{}{"foo", "bar", 5},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with three args wrong args",
			spec: NewTagFromResponseIfStatus(),
			args: []interface{}{"foo", "bar", ">"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with three args wrong args",
			spec: NewTagFromResponseIfStatus(),
			args: []interface{}{"foo", "bar", "==500"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with three args wrong args",
			spec: NewTagFromResponseIfStatus(),
			args: []interface{}{"foo", "bar", "500"},
			want: filters.ErrInvalidFilterParameters,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.spec.CreateFilter(tt.args); err != tt.want {
				t.Errorf("Failed to create filter: Want %v, got %v", tt.want, err)
			}
		})
	}
}

func TestTracingTag(t *testing.T) {
	tracer := mocktracer.New()

	for _, ti := range []struct {
		name     string
		spec     filters.Spec
		values   []string
		context  *filtertest.Context
		tagName  string
		expected interface{}
	}{{
		"plain key value",
		NewTag(),
		[]string{"test_tag", "test_value"},
		&filtertest.Context{
			FRequest: &http.Request{},
		},
		"test_tag",
		"test_value",
	}, {
		"tag from header",
		NewTag(),
		[]string{"test_tag", "${request.header.X-Flow-Id}"},
		&filtertest.Context{
			FRequest: &http.Request{
				Header: http.Header{
					"X-Flow-Id": []string{"foo"},
				},
			},
		},
		"test_tag",
		"foo",
	}, {
		"tag from response",
		NewTagFromResponse(),
		[]string{"test_tag", "${response.header.X-Fallback}"},
		&filtertest.Context{
			FRequest: &http.Request{},
			FResponse: &http.Response{
				Header: http.Header{
					"X-Fallback": []string{"true"},
				},
			},
		},
		"test_tag",
		"true",
	}, {
		"tag from response if condition is true",
		NewTagFromResponseIfStatus(),
		[]string{"Error", "true", ">499"},
		&filtertest.Context{
			FRequest: &http.Request{},
			FResponse: &http.Response{
				StatusCode: 500,
			},
		},
		"Error",
		"true",
	}, {
		"tag from response if condition is true",
		NewTagFromResponseIfStatus(),
		[]string{"Error", "true", "=500"},
		&filtertest.Context{
			FRequest: &http.Request{},
			FResponse: &http.Response{
				StatusCode: 500,
			},
		},
		"Error",
		"true",
	}, {
		"tag from response if condition is true",
		NewTagFromResponseIfStatus(),
		[]string{"Error", "true", "<505"},
		&filtertest.Context{
			FRequest: &http.Request{},
			FResponse: &http.Response{
				StatusCode: 500,
			},
		},
		"Error",
		"true",
	}, {
		"tag from missing header",
		NewTag(),
		[]string{"test_tag", "${request.header.missing}"},
		&filtertest.Context{
			FRequest: &http.Request{},
		},
		"test_tag",
		nil,
	}, {
		"tracingTag is not processed on response",
		NewTag(),
		[]string{"test_tag", "${response.header.X-Fallback}"},
		&filtertest.Context{
			FRequest: &http.Request{},
			FResponse: &http.Response{
				Header: http.Header{
					"X-Fallback": []string{"true"},
				},
			},
		},
		"test_tag",
		nil,
	},
	} {
		t.Run(ti.name, func(t *testing.T) {
			span := tracer.StartSpan("proxy").(*mocktracer.MockSpan)
			defer span.Finish()

			requestContext := &filtertest.Context{
				FRequest: ti.context.FRequest.WithContext(opentracing.ContextWithSpan(ti.context.FRequest.Context(), span)),
			}

			values := make([]interface{}, 0)
			for _, v := range ti.values {
				values = append(values, v)
			}
			f, err := ti.spec.CreateFilter(values)
			if err != nil {
				t.Fatal(err)
			}

			f.Request(requestContext)

			requestContext.FResponse = ti.context.FResponse

			f.Response(requestContext)

			if got := span.Tag(ti.tagName); got != ti.expected {
				t.Errorf("unexpected tag %q value '%v' != '%v'", ti.tagName, got, ti.expected)
			}
		})
	}
}
