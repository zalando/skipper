package openpolicyagent

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/open-policy-agent/opa/v1/config"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/tracing/tracingtest"
)

type MockTransport struct {
	resp *http.Response
	err  error
}

func (t *MockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return t.resp, t.err
}

func TestTracingFactory(t *testing.T) {
	tracer := tracingtest.NewTracer()

	testCases := []struct {
		name       string
		req        *http.Request
		tracer     opentracing.Tracer
		parentSpan string
		resp       *http.Response
		resperr    error
	}{
		{
			name: "Sub-span created with parent span without tracer set",
			req: &http.Request{
				Header: map[string][]string{},
				Method: "GET",
				Host:   "example.com",
				URL:    &url.URL{Path: "/test", Scheme: "http", Host: "example.com"},
			},
			tracer:     nil,
			parentSpan: "open-policy-agent",
			resp:       &http.Response{StatusCode: http.StatusOK},
		},
		{
			name: "Sub-span created with parent span with tracer set",
			req: &http.Request{
				Header: map[string][]string{},
				Method: "GET",
				Host:   "example.com",
				URL:    &url.URL{Path: "/test", Scheme: "http", Host: "example.com"},
			},
			tracer:     tracer,
			parentSpan: "open-policy-agent",
			resp:       &http.Response{StatusCode: http.StatusOK},
		},
		{
			name: "Sub-span created without parent span",
			req: &http.Request{
				Header: map[string][]string{},
				Method: "GET",
				Host:   "example.com",
				URL:    &url.URL{Path: "/test", Scheme: "http", Host: "example.com"},
			},
			tracer: tracer,
			resp:   &http.Response{StatusCode: http.StatusOK},
		},
		{
			name: "Span contains error information",
			req: &http.Request{
				Header: map[string][]string{},
				Method: "GET",
				Host:   "example.com",
				URL:    &url.URL{Path: "/test", Scheme: "http", Host: "example.com"},
			},
			tracer:  tracer,
			resperr: assert.AnError,
		},
		{
			name: "Response has a 4xx response",
			req: &http.Request{
				Header: map[string][]string{},
				Method: "GET",
				Host:   "example.com",
				URL:    &url.URL{Path: "/test", Scheme: "http", Host: "example.com"},
			},
			tracer: tracer,
			resp:   &http.Response{StatusCode: http.StatusUnauthorized},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := &tracingFactory{}
			tracer.Reset()

			tr := f.NewTransport(&MockTransport{tc.resp, tc.resperr}, buildTracingOptions(tc.tracer, "bundle", &plugins.Manager{
				ID: "manager-id",
				Config: &config.Config{
					Labels: map[string]string{"label": "value"},
				},
			}))

			var parentSpan opentracing.Span
			if tc.parentSpan != "" {
				parentSpan = tracer.StartSpan(tc.parentSpan)

				ctx := opentracing.ContextWithSpan(context.Background(), parentSpan)
				tc.req = tc.req.WithContext(ctx)
			}

			resp, err := tr.RoundTrip(tc.req)
			if parentSpan != nil {
				parentSpan.Finish()
			}

			createdSpan := tracer.FindSpan("open-policy-agent.http")
			require.NotNil(t, createdSpan, "No span was created")

			if tc.resperr == nil {
				assert.NoError(t, err)
				if tc.resp.StatusCode > 399 {
					assert.Equal(t, true, createdSpan.Tag("error"), "Error tag was not set")
				}

				assert.Equal(t, tc.resp.StatusCode, createdSpan.Tag(proxy.HTTPStatusCodeTag), "http status tag was not set")
			} else {
				assert.Equal(t, true, createdSpan.Tag("error"), "Error tag was not set")
				assert.Equal(t, tc.resperr, err, "Error was not propagated")
			}

			assert.Equal(t, tc.resp, resp, "Response was not propagated")

			assert.Equal(t, tc.req.Method, createdSpan.Tag("http.method"))
			assert.Equal(t, tc.req.URL.String(), createdSpan.Tag("http.url"))
			assert.Equal(t, tc.req.Host, createdSpan.Tag("hostname"))
			assert.Equal(t, tc.req.URL.Path, createdSpan.Tag("http.path"))
			assert.Equal(t, "skipper", createdSpan.Tag("component"))
			assert.Equal(t, "client", createdSpan.Tag("span.kind"))
			assert.Equal(t, "bundle", createdSpan.Tag("opa.bundle_name"))
			assert.Equal(t, "value", createdSpan.Tag("opa.label.label"))
		})
	}
}
