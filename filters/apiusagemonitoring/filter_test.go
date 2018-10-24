package apiusagemonitoring

import (
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
	"net/http"
	"testing"
)

func init() {
	log.Logger.SetLevel(logrus.DebugLevel)
}

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
	spec := NewApiUsageMonitoring(true)
	return spec.CreateFilter(args)
}

func testWithFilter(
	t *testing.T,
	filterCreate func() (filters.Filter, error),
	method string,
	url string,
	reqBody string,
	bypassReqContentLength *int64,
	resStatus int,
	responseContentLength int64,
	expect func(t *testing.T, pass int, m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64),
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
			req, err := http.NewRequest(method, url, bytes.NewBufferString(reqBody))
			if err != nil {
				t.Fatal(err)
			}
			if bypassReqContentLength != nil {
				req.ContentLength = *bypassReqContentLength
			}
			ctx := &filtertest.Context{
				FRequest: req,
				FResponse: &http.Response{
					StatusCode:    resStatus,
					ContentLength: responseContentLength,
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
				int64(len(reqBody)),
				responseContentLength,
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
		"",
		nil,
		299,
		0,
		func(t *testing.T, pass int, m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			// no path matching: tracked as unknown
			assert.Equal(t,
				map[string]int64{
					"apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.http_count":    int64(pass),
					"apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.latency")
		})
}

func Test_Filter_PathTemplateNoVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		"asd",
		nil,
		400,
		6,
		func(t *testing.T, pass int, m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.http_count":    int64(pass),
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.http4xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.latency")
		})
}

func Test_Filter_PathTemplateWithVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders/1234",
		"asd",
		nil,
		204,
		6,
		func(t *testing.T, pass int, m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id.http_count":    int64(pass),
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id.http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id.latency")
		})
}

func Test_Filter_PathTemplateWithMultipleVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders/1234/order-items/123",
		"asd",
		nil,
		301,
		6,
		func(t *testing.T, pass int, m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id/order-items/:order-item-id.http_count":    int64(pass),
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id/order-items/:order-item-id.http3xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id/order-items/:order-item-id.latency")
		})
}

func Test_Filter_PathTemplateFromSecondConfiguredApi(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/customers/loremipsum",
		"asd",
		nil,
		502,
		6,
		func(t *testing.T, pass int, m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/customers/:customer-id.http_count":    int64(pass),
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/customers/:customer-id.http5xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apiUsageMonitoring.custom.my_app.my_api.POST.foo/customers/:customer-id.latency")
		})
}

func Test_Filter_StatusCodes1xxAreMonitored(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		"asd",
		nil,
		100,
		6,
		func(t *testing.T, pass int, m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.http_count":    int64(pass),
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.http1xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.latency")
		})
}

func Test_Filter_StatusCodeOver599IsMonitored(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		"asd",
		nil,
		600,
		6,
		func(t *testing.T, pass int, m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.http_count": int64(pass),
					//"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.http*xx_count" <--- no code group tracked
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.latency")
		})
}

func Test_Filter_StatusCodeUnder100IsMonitoredWithoutHttpStatusCount(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		"asd",
		nil,
		99,
		6,
		func(t *testing.T, pass int, m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.http_count": int64(pass),
					//"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.http*xx_count" <--- no code group tracked
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.latency")
		})
}

func Test_Filter_NonConfiguredPathTrackedUnderUnknown(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodGet,
		"https://www.example.org/lapin/malin",
		"asd",
		nil,
		200,
		6,
		func(t *testing.T, pass int, m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.http_count":    int64(pass),
					"apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.latency")
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
		//{"", "<unknown>"}, // not tested because `http.NewRequest` default to `GET` when method is empty
		{"foo", "<unknown>"},
		{"this_does_not_make_any_sense", "<unknown>"},
	} {
		t.Run(testCase.method, func(t *testing.T) {
			testWithFilter(
				t,
				createFilterForTest,
				testCase.method,
				"https://www.example.org/lapin/malin",
				"asd",
				nil,
				200,
				6,
				func(t *testing.T, pass int, m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {

					httpCountMetricKey := fmt.Sprintf(
						"apiUsageMonitoring.custom.<unknown>.<unknown>.%s.<unknown>.http_count",
						testCase.expectedMethodInMetric)
					httpStatusClassCountMetricKey := fmt.Sprintf(
						"apiUsageMonitoring.custom.<unknown>.<unknown>.%s.<unknown>.http2xx_count",
						testCase.expectedMethodInMetric)

					assert.Equal(t,
						map[string]int64{
							httpCountMetricKey:            int64(pass),
							httpStatusClassCountMetricKey: int64(pass),
						},
						m.Counters,
					)

					latencyMetricKey := fmt.Sprintf(
						"apiUsageMonitoring.custom.<unknown>.<unknown>.%s.<unknown>.latency",
						testCase.expectedMethodInMetric)

					assert.Contains(t, m.Measures, latencyMetricKey)
				})
		})
	}

}
