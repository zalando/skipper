package apiusagemonitoring

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
)

func Test_Filter_NoPathTemplate(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodGet,
		"https://www.example.org/a/b/c",
		299,
		func(pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.{no-tag}.{unknown}.GET.{no-match}.*.*."
			// no path matching: tracked as unknown
			m.WithCounters(func(counters map[string]int64) {
				assert.Equal(t,
					map[string]int64{
						pre + "http_count":    int64(pass),
						pre + "http2xx_count": int64(pass),
					},
					counters,
				)
			})
			m.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Contains(t, measures, pre+"latency")
			})
		})
}

func Test_Filter_PathTemplateNoVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		400,
		func(pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_tag.my_api.POST.foo/orders.*.*."
			m.WithCounters(func(counters map[string]int64) {
				assert.Equal(t,
					map[string]int64{
						pre + "http_count":    int64(pass),
						pre + "http4xx_count": int64(pass),
					},
					counters,
				)
			})
			m.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Contains(t, measures, pre+"latency")
			})
		})
}

func Test_Filter_PathTemplateWithVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders/1234",
		204,
		func(pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_tag.my_api.POST.foo/orders/{order-id}.*.*."
			m.WithCounters(func(counters map[string]int64) {
				assert.Equal(t,
					map[string]int64{
						pre + "http_count":    int64(pass),
						pre + "http2xx_count": int64(pass),
					},
					counters,
				)
			})
			m.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Contains(t, measures, pre+"latency")
			})
		})
}

func Test_Filter_PathTemplateWithMultipleVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders/1234/order-items/123",
		301,
		func(pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_tag.my_api.POST.foo/orders/{order-id}/order-items/{order-item-id}.*.*."
			m.WithCounters(func(counters map[string]int64) {
				assert.NotContains(
					t, counters,
					"apiUsageMonitoring.custom.my_app.my_tag.my_api.POST.foo/orders/{order-id}.http_count",
					"Matched `foo/orders/{order-id}` instead of `foo/orders/{order-id}`/order-items/{order-item-id}")

				assert.Equal(t,
					map[string]int64{
						pre + "http_count":    int64(pass),
						pre + "http3xx_count": int64(pass),
					},
					counters,
				)
			})
			m.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Contains(t, measures, pre+"latency")
			})
		})
}

func Test_Filter_PathTemplateFromSecondConfiguredApi(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/customers/loremipsum",
		502,
		func(pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_tag.my_api.POST.foo/customers/{customer-id}.*.*."
			m.WithCounters(func(counters map[string]int64) {
				assert.Equal(t,
					map[string]int64{
						pre + "http_count":    int64(pass),
						pre + "http5xx_count": int64(pass),
					},
					counters,
				)
			})
			m.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Contains(t, measures, pre+"latency")
			})
		})
}

func Test_Filter_StatusCodes1xxAreMonitored(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		100,
		func(pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_tag.my_api.POST.foo/orders.*.*."
			m.WithCounters(func(counters map[string]int64) {
				assert.Equal(t,
					map[string]int64{
						pre + "http_count":    int64(pass),
						pre + "http1xx_count": int64(pass),
					},
					counters,
				)
			})
			m.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Contains(t, measures, pre+"latency")
			})
		})
}

func Test_Filter_StatusCodeOver599IsMonitored(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		600,
		func(pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_tag.my_api.POST.foo/orders.*.*."
			m.WithCounters(func(counters map[string]int64) {
				assert.Equal(t,
					map[string]int64{
						pre + "http_count":    int64(pass),
						pre + "httpxxx_count": int64(pass),
					},
					counters,
				)
			})
			m.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Contains(t, measures, pre+"latency")
			})
		})
}

func Test_Filter_StatusCodeUnder100IsMonitoredWithoutHttpStatusCount(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		99,
		func(pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_tag.my_api.POST.foo/orders.*.*."
			m.WithCounters(func(counters map[string]int64) {
				assert.Equal(t,
					map[string]int64{
						pre + "http_count":    int64(pass),
						pre + "httpxxx_count": int64(pass),
					},
					counters,
				)
			})
			m.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Contains(t, measures, pre+"latency")
			})
		})
}

func Test_Filter_NonConfiguredPathTrackedUnderNoMatch(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodGet,
		"https://www.example.org/lapin/malin",
		200,
		func(pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.{no-tag}.{unknown}.GET.{no-match}.*.*."
			m.WithCounters(func(counters map[string]int64) {
				assert.Equal(t,
					map[string]int64{
						pre + "http_count":    int64(pass),
						pre + "http2xx_count": int64(pass),
					},
					counters,
				)
			})
			m.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Contains(t, measures, pre+"latency")
			})
		})
}

func Test_Filter_AllHttpMethodsAreSupported(t *testing.T) {
	for _, testCase := range []struct {
		method                 string
		expectedMethodInMetric string
	}{
		{http.MethodGet, "GET"},
		{http.MethodHead, "HEAD"},
		{http.MethodPost, "POST"},
		{http.MethodPut, "PUT"},
		{http.MethodPatch, "PATCH"},
		{http.MethodDelete, "DELETE"},
		{http.MethodConnect, "CONNECT"},
		{http.MethodOptions, "OPTIONS"},
		{http.MethodTrace, "TRACE"},
		{"", "{unknown}"},
		{"foo", "{unknown}"},
	} {
		t.Run(testCase.method, func(t *testing.T) {
			testWithFilter(
				t,
				createFilterForTest,
				testCase.method,
				"https://www.example.org/lapin/malin",
				200,
				func(pass int, m *metricstest.MockMetrics) {
					pre := fmt.Sprintf(
						"apiUsageMonitoring.custom.my_app.{no-tag}.{unknown}.%s.{no-match}.*.*.",
						testCase.expectedMethodInMetric)
					m.WithCounters(func(counters map[string]int64) {
						assert.Equal(t,
							map[string]int64{
								pre + "http_count":    int64(pass),
								pre + "http2xx_count": int64(pass),
							},
							counters,
						)
					})
					m.WithMeasures(func(measures map[string][]time.Duration) {
						assert.Contains(t, measures, pre+"latency")
					})
				})
		})
	}
}

func Test_Filter_PathTemplateMatchesInternalSlashes(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders/1/2/3/order-items/123",
		204,
		func(pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_tag.my_api.POST.foo/orders/{order-id}/order-items/{order-item-id}.*.*."
			m.WithCounters(func(counters map[string]int64) {
				assert.Equal(t,
					map[string]int64{
						pre + "http_count":    int64(pass),
						pre + "http2xx_count": int64(pass),
					},
					counters,
				)
			})
			m.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Contains(t, measures, pre+"latency")
			})
		})
}

func Test_Filter_PathTemplateMatchesInternalSlashesTooFollowingVarPart(t *testing.T) {
	filterCreate := func() (filters.Filter, error) {
		args := []any{`{
				"application_id": "my_app",
                "tag": "my_tag",
				"api_id": "my_api",
				"path_templates": [
					"foo/:a",
					"foo/:a/:b",
					"foo/:a/:b/:c",
					"bar/{a}-{b}/{c}"
				]
			}`}
		spec := NewApiUsageMonitoring(true, "", "", "")
		return spec.CreateFilter(args)
	}
	for _, c := range []struct {
		requestPath                 string
		expectedMatchedPathTemplate string
	}{
		{"foo/1", "foo/{a}"},
		{"foo/1/2", "foo/{a}/{b}"},
		{"foo/1/2/3", "foo/{a}/{b}/{c}"},
		{"foo/1/2/3/4", "foo/{a}/{b}/{c}"},
		{"foo/1/2/3/4/5", "foo/{a}/{b}/{c}"},
		{"bar/1/2-3/4/5", "bar/{a}-{b}/{c}"},
	} {
		subTestName := strings.ReplaceAll(c.requestPath, "/", "_")
		t.Run(subTestName, func(t *testing.T) {
			testWithFilter(
				t,
				filterCreate,
				http.MethodGet,
				fmt.Sprintf("https://www.example.org/%s", c.requestPath),
				204,
				func(pass int, m *metricstest.MockMetrics) {
					pre := "apiUsageMonitoring.custom.my_app.my_tag.my_api.GET." + c.expectedMatchedPathTemplate + ".*.*."
					m.WithCounters(func(counters map[string]int64) {
						assert.Equal(t,
							map[string]int64{
								pre + "http_count":    int64(pass),
								pre + "http2xx_count": int64(pass),
							},
							counters,
						)
					})
					m.WithMeasures(func(measures map[string][]time.Duration) {
						assert.Contains(t, measures, pre+"latency")
					})
				})
		})
	}
}

func Test_Filter_PathTemplateMatchesPathFromRequestChain(t *testing.T) {
	filterCreate := func() (filters.Filter, error) {
		args := []any{`{
				"application_id": "my_app",
                "tag": "my_tag",
				"api_id": "my_api",
				"path_templates": [
					"foo/:a",
					"bar/:a"
				]
			}`}
		spec := NewApiUsageMonitoring(true, "", "", "")
		return spec.CreateFilter(args)
	}
	for _, c := range []struct {
		requestPath                 string
		modifiedPath                string
		expectedMatchedPathTemplate string
	}{
		{"foo/x", "bar/x", "foo/{a}"},
	} {
		subTestName := strings.ReplaceAll(c.requestPath, "/", "_")
		t.Run(subTestName, func(t *testing.T) {
			testWithFilterModifyContext(
				t,
				filterCreate,
				http.MethodGet,
				fmt.Sprintf("https://www.example.org/%s", c.requestPath),
				204,
				func(ctx *filtertest.Context) {
					ctx.FRequest.URL.Path = c.modifiedPath
				},
				func(pass int, m *metricstest.MockMetrics) {
					pre := "apiUsageMonitoring.custom.my_app.my_tag.my_api.GET." + c.expectedMatchedPathTemplate + ".*.*."
					m.WithCounters(func(counters map[string]int64) {
						assert.Equal(t,
							map[string]int64{
								pre + "http_count":    int64(pass),
								pre + "http2xx_count": int64(pass),
							},
							counters,
						)
					})
					m.WithMeasures(func(measures map[string][]time.Duration) {
						assert.Contains(t, measures, pre+"latency")
					})
				})
		})
	}
}
