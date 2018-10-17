package apimonitoring

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
	"net/http"
	"testing"
)

func createFilterForTest() (filters.Filter, error) {
	spec := apiMonitoringFilterSpec{}
	args := []interface{}{`{
		"apis": [
			{
				"application_id": "my_app",
				"api_id": "my_api",
	  			"path_templates": [
					"foo/orders",
					"foo/orders/:order-id",
					"foo/orders/:order-id/order-items/{order-item-id}",
					"/foo/customers/",
					"/foo/customers/{customer-id}/"
				]
			}
		]
	}`}
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
	expect func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64),
) {
	filter, err := filterCreate()
	assert.NoError(t, err)

	metricsMock := &metricstest.MockMetrics{
		Prefix: "apimonitoring.custom.",
	}

	req, err := http.NewRequest(method, url, bytes.NewBufferString(reqBody))
	if err != nil {
		t.Error(err)
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
		metricsMock,
		int64(len(reqBody)),
		responseContentLength,
	)
}

func Test_Filter_NoPathTemplate(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"GET",
		"https://www.example.org/a/b/c",
		"",
		nil,
		200,
		0,
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			// no path matching: tracked as unknown
			assert.Equal(t,
				map[string]int64{
					"apimonitoring.custom.<unknown>.<unknown>.GET.<unknown>.http_count":    1,
					"apimonitoring.custom.<unknown>.<unknown>.GET.<unknown>.http2xx_count": 1,
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apimonitoring.custom.<unknown>.<unknown>.GET.<unknown>.latency")
		})
}

func Test_Filter_PathTemplateNoVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"POST",
		"https://www.example.org/foo/orders",
		"asd",
		nil,
		400,
		6,
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apimonitoring.custom.my_app.my_api.POST.foo/orders.http_count":    1,
					"apimonitoring.custom.my_app.my_api.POST.foo/orders.http4xx_count": 1,
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apimonitoring.custom.my_app.my_api.POST.foo/orders.latency")
		})
}

func Test_Filter_PathTemplateWithVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"POST",
		"https://www.example.org/foo/orders/1234",
		"asd",
		nil,
		200,
		6,
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apimonitoring.custom.my_app.my_api.POST.foo/orders/:order-id.http_count":    1,
					"apimonitoring.custom.my_app.my_api.POST.foo/orders/:order-id.http2xx_count": 1,
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apimonitoring.custom.my_app.my_api.POST.foo/orders/:order-id.latency")
		})
}

func Test_Filter_PathTemplateWithMultipleVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"POST",
		"https://www.example.org/foo/orders/1234/order-items/123",
		"asd",
		nil,
		300,
		6,
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apimonitoring.custom.my_app.my_api.POST.foo/orders/:order-id/order-items/:order-item-id.http_count":    1,
					"apimonitoring.custom.my_app.my_api.POST.foo/orders/:order-id/order-items/:order-item-id.http3xx_count": 1,
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apimonitoring.custom.my_app.my_api.POST.foo/orders/:order-id/order-items/:order-item-id.latency")
		})
}

func Test_Filter_PathTemplateFromSecondConfiguredApi(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"POST",
		"https://www.example.org/foo/customers/loremipsum",
		"asd",
		nil,
		500,
		6,
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apimonitoring.custom.my_app.my_api.POST.foo/customers/:customer-id.http_count":    1,
					"apimonitoring.custom.my_app.my_api.POST.foo/customers/:customer-id.http5xx_count": 1,
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apimonitoring.custom.my_app.my_api.POST.foo/customers/:customer-id.latency")
		})
}

func Test_Filter_StatusCodeUnder200IsMonitored(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"POST",
		"https://www.example.org/foo/orders",
		"asd",
		nil,
		100,
		6,
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apimonitoring.custom.my_app.my_api.POST.foo/orders.http_count": 1,
					//"apimonitoring.custom.my_app.my_api.POST.foo/orders.http*xx_count" <--- no code group tracked
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apimonitoring.custom.my_app.my_api.POST.foo/orders.latency")
		})
}

func Test_Filter_StatusCodeOver599IsMonitored(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"POST",
		"https://www.example.org/foo/orders",
		"asd",
		nil,
		600,
		6,
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apimonitoring.custom.my_app.my_api.POST.foo/orders.http_count": 1,
					//"apimonitoring.custom.my_app.my_api.POST.foo/orders.http*xx_count" <--- no code group tracked
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apimonitoring.custom.my_app.my_api.POST.foo/orders.latency")
		})
}

func Test_Filter_NonConfiguredPathTrackedUnderUnknown(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"GET",
		"https://www.example.org/lapin/malin",
		"asd",
		nil,
		200,
		6,
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"apimonitoring.custom.<unknown>.<unknown>.GET.<unknown>.http_count":    1,
					"apimonitoring.custom.<unknown>.<unknown>.GET.<unknown>.http2xx_count": 1,
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "apimonitoring.custom.<unknown>.<unknown>.GET.<unknown>.latency")
		})
}
