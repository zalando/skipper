package apiusagemonitoring

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
	"net/http"
	"strings"
	"testing"
)

var (
	args = []interface{}{`{
		"application_id": "my_app",
		"api_id": "my_api",
	  	"path_templates": [
			"foo/orders",
			"foo/orders/:order-id",
			"foo/orders/:order-id/order-items/{order-item-id}",
			"/foo/customers/",
			"/foo/customers/{customer-id}/"
		]
	}`}
)

func createFilterForTest() (filters.Filter, error) {
	spec := NewApiUsageMonitoring(true, "", "", "")
	return spec.CreateFilter(args)
}

func testWithFilter(
	t *testing.T,
	filterCreate func() (filters.Filter, error),
	method string,
	url string,
	resStatus int,
	expect func(t *testing.T, pass int, m *metricstest.MockMetrics),
) {
	filter, err := filterCreate()
	assert.NoError(t, err)
	assert.NotNil(t, filter)

	metricsMock := &metricstest.MockMetrics{
		Prefix: "apiUsageMonitoring.custom.",
	}

	// performing multiple passes to make sure that the caching of metrics keys
	// does not fail.
	for pass := 1; pass <= 3; pass++ {
		t.Run(fmt.Sprintf("pass %d", pass), func(t *testing.T) {
			req, err := http.NewRequest(method, url, nil)
			if method == "" {
				req.Method = ""
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx := &filtertest.Context{
				FRequest: req,
				FResponse: &http.Response{
					StatusCode: resStatus,
				},
				FStateBag: make(map[string]interface{}),
				FMetrics:  metricsMock,
			}
			filter.Request(ctx)
			filter.Response(ctx)

			expect(
				t,
				pass,
				metricsMock,
			)
		})
	}
}

func Test_Filter_NoPathTemplate(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodGet,
		"https://www.example.org/a/b/c",
		299,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.*.*."
			// no path matching: tracked as unknown
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_NoConfiguration(t *testing.T) {
	testWithFilter(
		t,
		func() (filters.Filter, error) {
			spec := NewApiUsageMonitoring(true, "", "", "")
			return spec.CreateFilter([]interface{}{})
		},
		http.MethodGet,
		"https://www.example.org/a/b/c",
		200,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.*.*."
			// no path matching: tracked as unknown
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_PathTemplateNoVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		400,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http4xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_PathTemplateWithVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders/1234",
		204,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_PathTemplateWithMultipleVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders/1234/order-items/123",
		301,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id/order-items/:order-item-id.*.*."
			if !assert.NotContains(
				t, m.Counters,
				"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id.http_count",
				"Matched `foo/orders/:order-id` instead of `foo/orders/:order-id`/order-items/:order-item-id") {
			}
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http3xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_PathTemplateFromSecondConfiguredApi(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/customers/loremipsum",
		502,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/customers/:customer-id.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http5xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_StatusCodes1xxAreMonitored(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		100,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http1xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_StatusCodeOver599IsMonitored(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		600,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count": int64(pass),
					//pre + "http*xx_count" <--- no code group tracked
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_StatusCodeUnder100IsMonitoredWithoutHttpStatusCount(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		99,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count": int64(pass),
					//pre + "http*xx_count" <--- no code group tracked
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_NonConfiguredPathTrackedUnderUnknown(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodGet,
		"https://www.example.org/lapin/malin",
		200,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_AllHttpMethodsAreSupported(t *testing.T) {
	t.Parallel()
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
		{"", "<unknown>"},
		{"foo", "<unknown>"},
	} {
		t.Run(testCase.method, func(t *testing.T) {
			testWithFilter(
				t,
				createFilterForTest,
				testCase.method,
				"https://www.example.org/lapin/malin",
				200,
				func(t *testing.T, pass int, m *metricstest.MockMetrics) {
					pre := fmt.Sprintf(
						"apiUsageMonitoring.custom.<unknown>.<unknown>.%s.<unknown>.*.*.",
						testCase.expectedMethodInMetric)
					assert.Equal(t,
						map[string]int64{
							pre + "http_count":    int64(pass),
							pre + "http2xx_count": int64(pass),
						},
						m.Counters,
					)
					assert.Contains(t, m.Measures, pre+"latency")
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
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id/order-items/:order-item-id.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_PathTemplateMatchesInternalSlashesTooFollowingVarPart(t *testing.T) {
	filterCreate := func() (filters.Filter, error) {
		args = []interface{}{`{
				"application_id": "my_app",
				"api_id": "my_api",
				"path_templates": [
					"foo/:a",
					"foo/:a/:b",
					"foo/:a/:b/:c"
				]
			}`}
		spec := NewApiUsageMonitoring(true, "", "", "")
		return spec.CreateFilter(args)
	}
	for _, c := range []struct {
		requestPath                 string
		expectedMatchedPathTemplate string
	}{
		{"foo/1", "foo/:a"},
		{"foo/1/2", "foo/:a/:b"},
		{"foo/1/2/3", "foo/:a/:b/:c"},
		{"foo/1/2/3/4", "foo/:a/:b/:c"},
		{"foo/1/2/3/4/5", "foo/:a/:b/:c"},
	} {
		subTestName := strings.Replace(c.requestPath, "/", "_", -1)
		t.Run(subTestName, func(t *testing.T) {
			testWithFilter(
				t,
				filterCreate,
				http.MethodGet,
				fmt.Sprintf("https://www.example.org/%s", c.requestPath),
				204,
				func(t *testing.T, pass int, m *metricstest.MockMetrics) {
					pre := "apiUsageMonitoring.custom.my_app.my_api.GET." + c.expectedMatchedPathTemplate + ".*.*."
					assert.Equal(t,
						map[string]int64{
							pre + "http_count":    int64(pass),
							pre + "http2xx_count": int64(pass),
						},
						m.Counters,
					)
					assert.Contains(t, m.Measures, pre+"latency")
				})
		})
	}
}
