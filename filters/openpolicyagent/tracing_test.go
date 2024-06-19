package openpolicyagent

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/open-policy-agent/opa/config"
	"github.com/open-policy-agent/opa/plugins"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
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
	tracer := &tracingtest.Tracer{}

	testCases := []struct {
		name       string
		req        *http.Request
		tracer     opentracing.Tracer
		parentSpan opentracing.Span
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
			parentSpan: tracer.StartSpan("open-policy-agent"),
			resp:       &http.Response{StatusCode: http.StatusOK},
		},
		{
			name: "Sub-span created with parent span without tracer set",
			req: &http.Request{
				Header: map[string][]string{},
				Method: "GET",
				Host:   "example.com",
				URL:    &url.URL{Path: "/test", Scheme: "http", Host: "example.com"},
			},
			tracer:     tracer,
			parentSpan: tracer.StartSpan("open-policy-agent"),
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
			tracer.Reset("")

			tr := f.NewTransport(&MockTransport{tc.resp, tc.resperr}, buildTracingOptions(tc.tracer, "bundle", &plugins.Manager{
				ID: "manager-id",
				Config: &config.Config{
					Labels: map[string]string{"label": "value"},
				},
			}))

			if tc.parentSpan != nil {
				ctx := opentracing.ContextWithSpan(context.Background(), tc.parentSpan)
				tc.req = tc.req.WithContext(ctx)
			}

			resp, err := tr.RoundTrip(tc.req)
			if tc.parentSpan != nil {
				tc.parentSpan.Finish()
			}

			createdSpan, ok := tracer.FindSpan("open-policy-agent.http")
			assert.True(t, ok, "No span was created")

			if tc.resperr == nil {
				assert.NoError(t, err)
				if tc.resp.StatusCode > 399 {
					assert.Equal(t, true, createdSpan.Tags["error"], "Error tag was not set")
				}

				assert.Equal(t, tc.resp.StatusCode, createdSpan.Tags[proxy.HTTPStatusCodeTag], "http status tag was not set")
			} else {
				assert.Equal(t, true, createdSpan.Tags["error"], "Error tag was not set")
				assert.Equal(t, tc.resperr, err, "Error was not propagated")
			}

			assert.Equal(t, tc.resp, resp, "Response was not propagated")

			assert.Equal(t, tc.req.Method, createdSpan.Tags["http.method"])
			assert.Equal(t, tc.req.URL.String(), createdSpan.Tags["http.url"])
			assert.Equal(t, tc.req.Host, createdSpan.Tags["hostname"])
			assert.Equal(t, tc.req.URL.Path, createdSpan.Tags["http.path"])
			assert.Equal(t, "skipper", createdSpan.Tags["component"])
			assert.Equal(t, "client", createdSpan.Tags["span.kind"])
			assert.Equal(t, "bundle", createdSpan.Tags["opa.bundle_name"])
			assert.Equal(t, "value", createdSpan.Tags["opa.label.label"])
		})
	}
}
