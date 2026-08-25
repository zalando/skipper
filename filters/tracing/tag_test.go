package tracing

import (
	"net/http"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/tracing/tracingtest"
)

func TestTracingTagNil(t *testing.T) {
	context := &filtertest.Context{
		FRequest: &http.Request{},
	}
	context.FRequest = context.FRequest.WithContext(opentracing.ContextWithSpan(context.FRequest.Context(), nil))

	s := NewTag()
	f, err := s.CreateFilter([]any{"test_tag", "foo"})
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
		args []any
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
			args: []any{"foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with one args",
			spec: NewTagFromResponse(),
			args: []any{"foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with one args",
			spec: NewTagFromResponseIfStatus(),
			args: []any{"foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTag(),
			args: []any{"foo", 3},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTagFromResponse(),
			args: []any{"foo", 3},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTagFromResponseIfStatus(),
			args: []any{"foo", 3},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTag(),
			args: []any{3, "foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTagFromResponse(),
			args: []any{3, "foo"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args one wrong",
			spec: NewTagFromResponseIfStatus(),
			args: []any{3, "foo"},
			want: filters.ErrInvalidFilterParameters,
		},

		// special
		{
			name: "create filter with three args want 2",
			spec: NewTag(),
			args: []any{"foo", "bar", "qux"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with three args want 2",
			spec: NewTagFromResponse(),
			args: []any{"foo", "bar", "qux"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with two args want 4 args",
			spec: NewTagFromResponseIfStatus(),
			args: []any{"foo", "bar"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with three args want 4",
			spec: NewTagFromResponseIfStatus(),
			args: []any{"foo", "bar", 300},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with four args wrong args",
			spec: NewTagFromResponseIfStatus(),
			args: []any{"foo", "bar", 300, "500"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with four args wrong args",
			spec: NewTagFromResponseIfStatus(),
			args: []any{"foo", "bar", "300", 500},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with four args wrong args",
			spec: NewTagFromResponseIfStatus(),
			args: []any{"foo", "bar", "300", "500"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with four args wrong args",
			spec: NewTagFromResponseIfStatus(),
			args: []any{"foo", "bar", 300, 600},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with four args wrong args",
			spec: NewTagFromResponseIfStatus(),
			args: []any{"foo", "bar", 500, 400},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "create filter with four args wrong args",
			spec: NewTagFromResponseIfStatus(),
			args: []any{"foo", "bar", -1, 400},
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
	tracer := tracingtest.NewTracer()

	for _, ti := range []struct {
		name     string
		doc      string
		context  *filtertest.Context
		tagName  string
		expected any
	}{{
		"plain key value",
		`tracingTag("test_tag", "test_value")`,
		&filtertest.Context{
			FRequest: &http.Request{},
		},
		"test_tag",
		"test_value",
	}, {
		"tag from header",
		`tracingTag("test_tag", "${request.header.X-Flow-Id}")`,
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
		`tracingTagFromResponse("test_tag", "${response.header.X-Fallback}")`,
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
		`tracingTagFromResponseIfStatus("error", "true", 500, 599)`,
		&filtertest.Context{
			FRequest: &http.Request{},
			FResponse: &http.Response{
				StatusCode: 500,
			},
		},
		"error",
		"true",
	}, {
		"tag from response if condition is true",
		`tracingTagFromResponseIfStatus("error", "true", 500, 500)`,
		&filtertest.Context{
			FRequest: &http.Request{},
			FResponse: &http.Response{
				StatusCode: 500,
			},
		},
		"error",
		"true",
	}, {
		"tag from response if condition is true",
		`tracingTagFromResponseIfStatus("error", "true", 500, 599)`,
		&filtertest.Context{
			FRequest: &http.Request{},
			FResponse: &http.Response{
				StatusCode: 599,
			},
		},
		"error",
		"true",
	}, {
		"do not tag from response if condition is false",
		`tracingTagFromResponseIfStatus("error", "true", 500, 599)`,
		&filtertest.Context{
			FRequest: &http.Request{},
			FResponse: &http.Response{
				StatusCode: 499,
			},
		},
		"error",
		nil,
	}, {
		"tag from missing header",
		`tracingTag("test_tag", "${request.header.missing}")`,
		&filtertest.Context{
			FRequest: &http.Request{},
		},
		"test_tag",
		nil,
	}, {
		"tracingTag is not processed on response",
		`tracingTag("test_tag", "${request.header.X-Fallback}")`,
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
			span := tracer.StartSpan("proxy").(*tracingtest.MockSpan)
			defer span.Finish()

			requestContext := &filtertest.Context{
				FRequest: ti.context.FRequest.WithContext(opentracing.ContextWithSpan(ti.context.FRequest.Context(), span)),
			}

			fEskip := eskip.MustParseFilters(ti.doc)[0]

			fr := make(filters.Registry)
			fr.Register(NewTag())
			fr.Register(NewTagFromResponse())
			fr.Register(NewTagFromResponseIfStatus())

			spec, ok := fr[fEskip.Name]
			if !ok {
				t.Fatalf("Failed to find filter spec: %q", fEskip.Name)
			}
			f, err := spec.CreateFilter(fEskip.Args)
			if err != nil {
				t.Fatalf("Failed to create filter %q with %v: %v", fEskip.Name, fEskip.Args, err)
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
