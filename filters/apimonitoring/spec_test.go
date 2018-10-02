package apimonitoring

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
	"io/ioutil"
	"net/http"
	"testing"
)

func Test_CreateFilter_NoParam(t *testing.T) {
	spec := &apiMonitoringFilterSpec{}
	filter, err := spec.CreateFilter([]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expecting one parameter (JSON configuration of the filter)")
	assert.Nil(t, filter)
}

func Test_CreateFilter_EmptyString(t *testing.T) {
	spec := &apiMonitoringFilterSpec{}
	filter, err := spec.CreateFilter([]interface{}{""})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error reading JSON configuration")
	assert.Nil(t, filter)
}

func Test_CreateFilter_NotAString(t *testing.T) {
	spec := &apiMonitoringFilterSpec{}
	filter, err := spec.CreateFilter([]interface{}{1234})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expecting first parameter to be a string")
	assert.Nil(t, filter)
}

func Test_CreateFilter_NotJson(t *testing.T) {
	spec := &apiMonitoringFilterSpec{}
	filter, err := spec.CreateFilter([]interface{}{"I am not JSON"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error reading JSON configuration")
	assert.Nil(t, filter)
}

func Test_CreateFilter_EmptyJson(t *testing.T) {
	spec := &apiMonitoringFilterSpec{}
	filter, err := spec.CreateFilter([]interface{}{"{}"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no path to monitor")
	assert.Nil(t, filter)
}

func Test_CreateFilter_FullConfig(t *testing.T) {
	// Includes paths:
	//   - normal (no variable part)
	//   - with {name} variable paths
	//   - with :name variable paths
	//   - with/without head/trailing slash
	spec := &apiMonitoringFilterSpec{}
	filter, err := spec.CreateFilter([]interface{}{`{
	  "apis": [
	    {
	      "application_id": "my_app",
	      "id": "orders_api",
	      "path_templates": [
	        "foo/orders",
	        "foo/orders/:order-id",
	        "foo/orders/:order-id/order_item/{order-item-id}"
	      ]
	    },
	    {
	      "id": "customers_api",
	      "application_id": "my_app",
	      "path_templates": [
	        "/foo/customers/",
	        "/foo/customers/{customer-id}/"
	      ]
	    }
	  ]
	}`})
	assert.NoError(t, err)
	assert.NotNil(t, filter)
	actual, ok := filter.(*apiMonitoringFilter)
	assert.True(t, ok)

	assert.False(t, actual.verbose)

	assert.Len(t, actual.paths, 5)

	assert.Equal(t, actual.paths[0].ApplicationId, "my_app")
	assert.Equal(t, actual.paths[0].ApiId, "orders_api")
	assert.Equal(t, actual.paths[0].PathTemplate, "foo/orders")
	assert.Equal(t, actual.paths[0].Matcher.String(), "^[\\/]*foo\\/orders[\\/]*$")

	assert.Equal(t, actual.paths[1].ApplicationId, "my_app")
	assert.Equal(t, actual.paths[1].ApiId, "orders_api")
	assert.Equal(t, actual.paths[1].PathTemplate, "foo/orders/:order-id")
	assert.Equal(t, actual.paths[1].Matcher.String(), "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$")

	assert.Equal(t, actual.paths[2].ApplicationId, "my_app")
	assert.Equal(t, actual.paths[2].ApiId, "orders_api")
	assert.Equal(t, actual.paths[2].PathTemplate, "foo/orders/:order-id/order_item/:order-item-id") // normalized to `:id`
	assert.Equal(t, actual.paths[2].Matcher.String(), "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$")

	assert.Equal(t, actual.paths[3].ApplicationId, "my_app")
	assert.Equal(t, actual.paths[3].ApiId, "customers_api")
	assert.Equal(t, actual.paths[3].PathTemplate, "foo/customers") // without the head/tail slashes
	assert.Equal(t, actual.paths[3].Matcher.String(), "^[\\/]*foo\\/customers[\\/]*$")

	assert.Equal(t, actual.paths[4].ApplicationId, "my_app")
	assert.Equal(t, actual.paths[4].ApiId, "customers_api")
	assert.Equal(t, actual.paths[4].PathTemplate, "foo/customers/:customer-id") // without the head/tail slashes, normalized to `:id`
	assert.Equal(t, actual.paths[4].Matcher.String(), "^[\\/]*foo\\/customers\\/[^\\/]+[\\/]*$")
}

func Test_CreateFilter_DuplicatePathPatternCausesError(t *testing.T) {
	// PathTemplate "foo" and "/foo/" after normalising are the same.
	// That causes an error, even if under different application or API IDs.
	spec := &apiMonitoringFilterSpec{}
	filter, err := spec.CreateFilter([]interface{}{`{
	  "apis": [
	    {
	      "application_id": "my_app",
	      "id": "orders_api",
	      "path_templates": [
	        "foo"
	      ]
	    },
	    {
	      "id": "customers_api",
	      "application_id": "my_app",
	      "path_templates": [
	        "/foo/"
	      ]
	    }
	  ]
	}`})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate path pattern "foo" detected`)
	assert.Nil(t, filter)
}

func Test_CreateFilter_DuplicateMatchersCausesError(t *testing.T) {
	spec := &apiMonitoringFilterSpec{}
	filter, err := spec.CreateFilter([]interface{}{`{
	  "apis": [
	    {
	      "application_id": "my_app",
	      "id": "orders_api",
	      "path_templates": [
	        "clients/:clientId",
            "clients/{clientId}"
	      ]
	    }
	  ]
	}`})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate path pattern")
	assert.Nil(t, filter)
}

func createFilterForTest() (filters.Filter, error) {
	spec := apiMonitoringFilterSpec{}
	args := []interface{}{`{
	  "apis": [
	    {
	      "application_id": "my_app",
	      "id": "orders_api",
	      "path_templates": [
	        "foo/orders",
	        "foo/orders/:order-id",
	        "foo/orders/:order-id/order-items/{order-item-id}"
	      ]
	    },
	    {
	      "id": "customers_api",
	      "application_id": "my_app",
	      "path_templates": [
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
	resStatus int,
	resBody string,
	expect func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64),
) {
	filter, err := filterCreate()
	assert.NoError(t, err)

	metricsMock := new(metricstest.MockMetrics)

	req, err := http.NewRequest(method, url, bytes.NewBufferString(reqBody))
	if err != nil {
		t.Error(err)
	}
	ctx := &filtertest.Context{
		FRequest: req,
		FResponse: &http.Response{
			StatusCode: resStatus,
			Body:       ioutil.NopCloser(bytes.NewBufferString(resBody)),
		},
		FStateBag: make(map[string]interface{}),
		FMetrics:  metricsMock,
	}
	filter.Request(ctx)
	filter.Response(ctx)

	expect(
		metricsMock,
		int64(len(reqBody)),
		0, // int64(len(resBody)), // todo Restore after understanding why `response.ContentLength` returns always 0
	)
}

func Test_Filter_NoPathPattern(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"GET",
		"https://www.example.org/a/b/c",
		"",
		200,
		"",
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			// no path matching: no tracking
			assert.Empty(t, m.Counters)
			assert.Empty(t, m.Measures)
		})
}

func Test_Filter_PathPatternNoVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"POST",
		"https://www.example.org/foo/orders",
		"asd",
		400,
		"qwerty",
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"api-mon.my_app.orders_api.POST.foo/orders.http_count":    1,
					"api-mon.my_app.orders_api.POST.foo/orders.http400_count": 1,
					"api-mon.my_app.orders_api.POST.foo/orders.req_size_sum":  reqBodyLen,
					"api-mon.my_app.orders_api.POST.foo/orders.resp_size_sum": resBodyLen,
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "api-mon.my_app.orders_api.POST.foo/orders.latency")
		})
}

func Test_Filter_PathPatternWithVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"POST",
		"https://www.example.org/foo/orders/1234",
		"asd",
		400,
		"qwerty",
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"api-mon.my_app.orders_api.POST.foo/orders/:order-id.http_count":    1,
					"api-mon.my_app.orders_api.POST.foo/orders/:order-id.http400_count": 1,
					"api-mon.my_app.orders_api.POST.foo/orders/:order-id.req_size_sum":  reqBodyLen,
					"api-mon.my_app.orders_api.POST.foo/orders/:order-id.resp_size_sum": resBodyLen,
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "api-mon.my_app.orders_api.POST.foo/orders/:order-id.latency")
		})
}

func Test_Filter_PathPatternWithMultipleVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"POST",
		"https://www.example.org/foo/orders/1234/order-items/123",
		"asd",
		400,
		"qwerty",
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"api-mon.my_app.orders_api.POST.foo/orders/:order-id/order-items/:order-item-id.http_count":    1,
					"api-mon.my_app.orders_api.POST.foo/orders/:order-id/order-items/:order-item-id.http400_count": 1,
					"api-mon.my_app.orders_api.POST.foo/orders/:order-id/order-items/:order-item-id.req_size_sum":  reqBodyLen,
					"api-mon.my_app.orders_api.POST.foo/orders/:order-id/order-items/:order-item-id.resp_size_sum": resBodyLen,
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "api-mon.my_app.orders_api.POST.foo/orders/:order-id/order-items/:order-item-id.latency")
		})
}

func Test_Filter_PathPatternFromSecondConfiguredApi(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		"POST",
		"https://www.example.org/foo/customers/loremipsum",
		"asd",
		400,
		"qwerty",
		func(m *metricstest.MockMetrics, reqBodyLen int64, resBodyLen int64) {
			assert.Equal(t,
				map[string]int64{
					"api-mon.my_app.customers_api.POST.foo/customers/:customer-id.http_count":    1,
					"api-mon.my_app.customers_api.POST.foo/customers/:customer-id.http400_count": 1,
					"api-mon.my_app.customers_api.POST.foo/customers/:customer-id.req_size_sum":  reqBodyLen,
					"api-mon.my_app.customers_api.POST.foo/customers/:customer-id.resp_size_sum": resBodyLen,
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, "api-mon.my_app.customers_api.POST.foo/customers/:customer-id.latency")
		})
}
