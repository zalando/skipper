package auth_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
)

func TestJwtMetrics(t *testing.T) {
	args := eskip.MustParseFilters(`jwtMetrics("{claims: {iss: [foo, qux], sub: [bar]}, ignore_status_codes: [401, 403, 404]}")`)[0].Args

	spec := auth.NewJwtMetrics()
	filter, err := spec.CreateFilter(args)
	require.NoError(t, err)

	for _, tc := range []struct {
		name     string
		request  *http.Request
		response *http.Response
		expected map[string]int64
	}{
		{
			name:     "ignores 401 response",
			request:  &http.Request{Method: "GET", Host: "foo.test"},
			response: &http.Response{StatusCode: http.StatusUnauthorized},
			expected: map[string]int64{},
		},
		{
			name:     "ignores 403 response",
			request:  &http.Request{Method: "GET", Host: "foo.test"},
			response: &http.Response{StatusCode: http.StatusUnauthorized},
			expected: map[string]int64{},
		},
		{
			name:     "ignores custom response",
			request:  &http.Request{Method: "GET", Host: "foo.test"},
			response: &http.Response{StatusCode: http.StatusNotFound},
			expected: map[string]int64{},
		},
		{
			name:     "missing-token",
			request:  &http.Request{Method: "GET", Host: "foo.test"},
			response: &http.Response{StatusCode: http.StatusOK},
			expected: map[string]int64{
				"GET.foo_test.200.missing-token": 1,
			},
		},
		{
			name: "invalid-token-type",
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{"Basic foobarbaz"}},
			},
			response: &http.Response{StatusCode: http.StatusOK},
			expected: map[string]int64{
				"GET.foo_test.200.invalid-token-type": 1,
			},
		},
		{
			name: "invalid-token",
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{"Bearer invalid-token"}},
			},
			response: &http.Response{StatusCode: http.StatusOK},
			expected: map[string]int64{
				"GET.foo_test.200.invalid-token": 1,
			},
		},
		{
			name: "missing-claim",
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"iss": "baz"}) + ".signature",
				}},
			},
			response: &http.Response{StatusCode: http.StatusOK},
			expected: map[string]int64{
				"GET.foo_test.200.missing-claim": 1,
			},
		},
		{
			name: "first claim present",
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"iss": "foo"}) + ".signature",
				}},
			},
			response: &http.Response{StatusCode: http.StatusOK},
			expected: map[string]int64{},
		},
		{
			name: "second claim present",
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"sub": "bar"}) + ".signature",
				}},
			},
			response: &http.Response{StatusCode: http.StatusOK},
			expected: map[string]int64{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metrics := &metricstest.MockMetrics{}
			ctx := &filtertest.Context{
				FRequest: tc.request,
				FMetrics: metrics,
			}
			filter.Request(ctx)
			ctx.FResponse = tc.response
			filter.Response(ctx)

			metrics.WithCounters(func(counters map[string]int64) {
				assert.Equal(t, tc.expected, counters)
			})
		})
	}
}

func TestJwtMetricsArgs(t *testing.T) {
	spec := auth.NewJwtMetrics()

	t.Run("valid", func(t *testing.T) {
		for _, def := range []string{
			`jwtMetrics()`,
			`jwtMetrics("{claims: {iss: [foo], sub: [bar]}}")`,
			`jwtMetrics("{claims: {iss: [foo, bar]}}")`,
		} {
			t.Run(def, func(t *testing.T) {
				args := eskip.MustParseFilters(def)[0].Args

				_, err := spec.CreateFilter(args)
				assert.NoError(t, err)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		for _, def := range []string{
			`jwtMetrics("iss")`,
			`jwtMetrics("iss", "foo",  "bar")`,
			`jwtMetrics("iss", 1)`,
		} {
			t.Run(def, func(t *testing.T) {
				args := eskip.MustParseFilters(def)[0].Args

				_, err := spec.CreateFilter(args)
				assert.Error(t, err)
			})
		}
	})
}

func marshalBase64JSON(t *testing.T, v any) string {
	d, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal json: %v, %v", v, err)
	}
	return base64.RawURLEncoding.EncodeToString(d)
}
