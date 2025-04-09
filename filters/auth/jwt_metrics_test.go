package auth_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing/tracingtest"
)

func TestJwtMetrics(t *testing.T) {
	const validAuthHeader = "Bearer foobarbaz"

	testAuthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != validAuthHeader {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.Write([]byte(`{"foo": "bar"}`))
		}
	}))
	defer testAuthServer.Close()

	for _, tc := range []struct {
		name        string
		filters     string
		request     *http.Request
		status      int
		expected    map[string]int64
		expectedTag string
	}{
		{
			name:     "ignores 401 response",
			filters:  `jwtMetrics("{issuers: [foo, bar]}")`,
			request:  &http.Request{Method: "GET", Host: "foo.test"},
			status:   http.StatusUnauthorized,
			expected: map[string]int64{},
		},
		{
			name:     "ignores 403 response",
			filters:  `jwtMetrics("{issuers: [foo, bar]}")`,
			request:  &http.Request{Method: "GET", Host: "foo.test"},
			status:   http.StatusForbidden,
			expected: map[string]int64{},
		},
		{
			name:     "ignores 404 response",
			filters:  `jwtMetrics("{issuers: [foo, bar]}")`,
			request:  &http.Request{Method: "GET", Host: "foo.test"},
			status:   http.StatusNotFound,
			expected: map[string]int64{},
		},
		{
			name:    "missing-token",
			filters: `jwtMetrics("{issuers: [foo, bar]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test"},
			status:  http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.missing-token": 1,
			},
			expectedTag: "missing-token",
		},
		{
			name:    "missing-token with claims",
			filters: `jwtMetrics("{claims: [{iss: foo}, {iss: bar}]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test"},
			status:  http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.missing-token": 1,
			},
			expectedTag: "missing-token",
		},
		{
			name:    "invalid-token-type",
			filters: `jwtMetrics("{issuers: [foo, bar]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{"Basic foobarbaz"}},
			},
			status: http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.invalid-token-type": 1,
			},
			expectedTag: "invalid-token-type",
		},
		{
			name:    "invalid-token-type with claims",
			filters: `jwtMetrics("{claims: [{iss: foo}, {iss: bar}]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{"Basic foobarbaz"}},
			},
			status: http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.invalid-token-type": 1,
			},
			expectedTag: "invalid-token-type",
		},
		{
			name:    "invalid-token",
			filters: `jwtMetrics("{issuers: [foo, bar]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{"Bearer invalid-token"}},
			},
			status: http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.invalid-token": 1,
			},
			expectedTag: "invalid-token",
		},
		{
			name:    "invalid-token with claims",
			filters: `jwtMetrics("{claims: [{iss: foo}, {iss: bar}]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{"Bearer invalid-token"}},
			},
			status: http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.invalid-token": 1,
			},
			expectedTag: "invalid-token",
		},
		{
			name:    "missing-issuer",
			filters: `jwtMetrics("{issuers: [foo, bar]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"sub": "baz"}) + ".signature",
				}},
			},
			status: http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.missing-issuer": 1,
			},
			expectedTag: "missing-issuer",
		},
		{
			name:    "invalid-issuer",
			filters: `jwtMetrics("{issuers: [foo, bar]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"iss": "baz"}) + ".signature",
				}},
			},
			status: http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.invalid-issuer": 1,
			},
			expectedTag: "invalid-issuer",
		},
		{
			name:    "invalid-claims with one claim key",
			filters: `jwtMetrics("{claims: [{iss: foo}, {iss: bar}]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"iss": "baz"}) + ".signature",
				}},
			},
			status: http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.invalid-claims": 1,
			},
			expectedTag: "invalid-claims",
		},
		{
			name:    "no invalid-issuer for empty issuers/claims",
			filters: `jwtMetrics()`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"iss": "baz"}) + ".signature",
				}},
			},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name:    "no invalid-issuer when matches first",
			filters: `jwtMetrics("{issuers: [foo, bar]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"iss": "foo"}) + ".signature",
				}},
			},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name:    "no invalid-issuer when matches second",
			filters: `jwtMetrics("{issuers: [foo, bar]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"iss": "bar"}) + ".signature",
				}},
			},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name:    "no invalid-claims when matches first",
			filters: `jwtMetrics("{claims: [{iss: foo, bat: ball}, {iss: bar}]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"iss": "foo", "bat": "ball"}) + ".signature",
				}},
			},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name:    "no invalid-claims when matches second",
			filters: `jwtMetrics("{claims: [{iss: foo, bar: baz}, {iss: bar}]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"iss": "bar"}) + ".signature",
				}},
			},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name:    "invalid-claims when no full claim matches",
			filters: `jwtMetrics("{claims: [{iss: foo, bar: baz}, {iss: bar}]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"iss": "foo", "bar": "bat"}) + ".signature",
				}},
			},
			status: http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.invalid-claims": 1,
			},
			expectedTag: "invalid-claims",
		},
		{
			name:    "no invalid-claims when full claim matches and token has extra keys",
			filters: `jwtMetrics("{claims: [{iss: foo, bar: baz}, {iss: bar}]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					"Bearer header." + marshalBase64JSON(t, map[string]any{"iss": "foo", "bar": "baz", "bat": "ball"}) + ".signature",
				}},
			},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name:    "missing-token without opt-out",
			filters: `jwtMetrics("{issuers: [foo, bar], optOutAnnotations: [oauth.disabled]}")`,
			request: &http.Request{Method: "GET", Host: "foo.test"},
			status:  http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.missing-token": 1,
			},
			expectedTag: "missing-token",
		},
		{
			name: "no metrics when opted-out by annotation",
			filters: `
				annotate("oauth.disabled", "this endpoint is public") ->
				jwtMetrics("{issuers: [foo, bar], optOutAnnotations: [oauth.disabled, jwtMetrics.disabled]}")
			`,
			request:  &http.Request{Method: "GET", Host: "foo.test"},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name: "no metrics when opted-out by alternative annotation",
			filters: `
				annotate("jwtMetrics.disabled", "skip jwt metrics collection") ->
				jwtMetrics("{issuers: [foo, bar], optOutAnnotations: [oauth.disabled, jwtMetrics.disabled]}")
			`,
			request:  &http.Request{Method: "GET", Host: "foo.test"},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name: "counts missing-token when annotation does not match",
			filters: `
				annotate("foo", "bar") ->
				jwtMetrics("{issuers: [foo, bar], optOutAnnotations: [oauth.disabled, jwtMetrics.disabled]}")
			`,
			request: &http.Request{Method: "GET", Host: "foo.test"},
			status:  http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.missing-token": 1,
			},
			expectedTag: "missing-token",
		},
		{
			name: "no metrics when opted-out by state bag",
			// oauthTokeninfoAnyKV stores token info in the state bag using the key "tokeninfo"
			filters: `
				oauthTokeninfoAnyKV("foo", "bar") ->
				jwtMetrics("{issuers: [foo, bar], optOutStateBag: [tokeninfo]}")
			`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					validAuthHeader,
				}},
			},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name: "counts invalid-token when state bag does not match",
			// oauthTokeninfoAnyKV stores token info in the state bag using the key "tokeninfo"
			filters: `
				oauthTokeninfoAnyKV("foo", "bar") ->
				jwtMetrics("{issuers: [foo, bar], optOutStateBag: [does.not.match]}")
			`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					validAuthHeader,
				}},
			},
			status: http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.invalid-token": 1,
			},
			expectedTag: "invalid-token",
		},
		{
			name:     "no metrics when host matches opted-out domain",
			filters:  `jwtMetrics("{issuers: [foo, bar], optOutHosts: [ '^.+[.]domain[.]test$', '^exact[.]test$' ]}")`,
			request:  &http.Request{Method: "GET", Host: "foo.domain.test"},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name:     "no metrics when second level host matches opted-out domain",
			filters:  `jwtMetrics("{issuers: [foo, bar], optOutHosts: [ '^.+[.]domain[.]test$', '^exact[.]test$' ]}")`,
			request:  &http.Request{Method: "GET", Host: "foo.bar.domain.test"},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name:     "no metrics when host matches opted-out host exactly",
			filters:  `jwtMetrics("{issuers: [foo, bar], optOutHosts: [ '^.+[.]domain[.]test$', '^exact[.]test$' ]}")`,
			request:  &http.Request{Method: "GET", Host: "exact.test"},
			status:   http.StatusOK,
			expected: map[string]int64{},
		},
		{
			name: "counts invalid-token when host does not match",
			filters: `
				jwtMetrics("{issuers: [foo, bar], optOutHosts: [ '^.+[.]domain[.]test$', '^exact[.]test$' ]}")
			`,
			request: &http.Request{Method: "GET", Host: "foo.test",
				Header: http.Header{"Authorization": []string{
					validAuthHeader,
				}},
			},
			status: http.StatusOK,
			expected: map[string]int64{
				"jwtMetrics.custom.GET.foo_test.200.invalid-token": 1,
			},
			expectedTag: "invalid-token",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &metricstest.MockMetrics{}
			defer m.Close()
			tracer := tracingtest.NewTracer()

			fr := builtin.MakeRegistry()
			fr.Register(auth.NewJwtMetrics())
			fr.Register(auth.NewOAuthTokeninfoAnyKVWithOptions(auth.TokeninfoOptions{URL: testAuthServer.URL}))
			p := proxytest.Config{
				RoutingOptions: routing.Options{
					FilterRegistry: fr,
				},
				Routes: eskip.MustParse(fmt.Sprintf(`* -> %s -> status(%d) -> <shunt>`, tc.filters, tc.status)),
				ProxyParams: proxy.Params{
					Metrics: m,
					OpenTracing: &proxy.OpenTracingParams{
						Tracer:             tracer,
						DisableFilterSpans: true,
					},
				},
			}.Create()
			defer p.Close()

			u, err := url.Parse(p.URL)
			require.NoError(t, err)
			tc.request.URL = u

			resp, err := p.Client().Do(tc.request)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, tc.status, resp.StatusCode)

			require.Equal(t, 1, len(tracer.FinishedSpans()))

			span := tracer.FinishedSpans()[0]
			if tc.expectedTag == "" {
				assert.Nil(t, span.Tag("jwt"))
			} else {
				assert.Equal(t, tc.expectedTag, span.Tag("jwt"))
			}

			m.WithCounters(func(counters map[string]int64) {
				// add incoming counter to simplify comparison
				tc.expected["incoming.HTTP/1.1"] = 1

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
			`jwtMetrics("{issuers: [foo, bar]}")`,
		} {
			t.Run(def, func(t *testing.T) {
				f := eskip.MustParseFilters(def)[0]
				require.Equal(t, spec.Name(), f.Name)

				_, err := spec.CreateFilter(f.Args)
				assert.NoError(t, err)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		for _, def := range []string{
			`jwtMetrics("iss")`,
			`jwtMetrics(1)`,
			`jwtMetrics("iss", 1)`,
			`jwtMetrics("{optOutHosts: [ '[' ]}")`, // invalid regexp
		} {
			t.Run(def, func(t *testing.T) {
				f := eskip.MustParseFilters(def)[0]
				require.Equal(t, spec.Name(), f.Name)

				_, err := spec.CreateFilter(f.Args)
				t.Logf("%v", err)
				assert.Error(t, err)
			})
		}
	})
}

func marshalBase64JSON(t testing.TB, v any) string {
	d, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal json: %v, %v", v, err)
	}
	return base64.RawURLEncoding.EncodeToString(d)
}

func BenchmarkJwtMetrics_CreateFilter(b *testing.B) {
	spec := auth.NewJwtMetrics()
	args := []any{`{issuers: [foo, bar], optOutAnnotations: [oauth.disabled], optOutHosts: [ '^.+[.]domain[.]test$' ]}`}

	_, err := spec.CreateFilter(args)
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = spec.CreateFilter(args)
	}
}

func BenchmarkJwtMetrics(b *testing.B) {
	spec := auth.NewJwtMetrics()
	args := []any{`{issuers: [foo, bar], optOutAnnotations: [oauth.disabled], optOutHosts: [ '^.+[.]domain[.]test$' ]}`}

	f, err := spec.CreateFilter(args)
	require.NoError(b, err)

	m := &metricstest.MockMetrics{}
	defer m.Close()

	ctx := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET", Host: "foo.test",
			Header: http.Header{
				"Authorization": []string{
					"Bearer header." + marshalBase64JSON(b, map[string]any{"iss": "baz"}) + ".signature",
				},
			},
		},
		FResponse: &http.Response{StatusCode: http.StatusOK},
		FMetrics:  m,
	}

	f.Request(ctx)
	f.Response(ctx)
	m.WithCounters(func(counters map[string]int64) {
		require.Equal(b, map[string]int64{"GET.foo_test.200.invalid-issuer": 1}, counters)
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Request(ctx)
		f.Response(ctx)
	}
}
