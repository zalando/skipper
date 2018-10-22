package apiusagemonitoring

import (
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"testing"
)

func Test_Name(t *testing.T) {
	spec := NewApiUsageMonitoring()
	assert.Equal(t, "apiUsageMonitoring", spec.Name())
}

func assertApiUsageMonitoringFilter(t *testing.T, filter filters.Filter, asserter func(t *testing.T, filter *apiUsageMonitoringFilter)) {
	assert.NotNil(t, filter)
	if assert.IsType(t, &apiUsageMonitoringFilter{}, filter) {
		asserter(t, filter.(*apiUsageMonitoringFilter))
	}
}

func assertNoopFilter(t *testing.T, filter filters.Filter, asserter func(t *testing.T, filter *noopFilter)) {
	assert.NotNil(t, filter)
	if assert.IsType(t, &noopFilter{}, filter) {
		asserter(t, filter.(*noopFilter))
	}
}

func Test_CreateFilter_NoParam(t *testing.T) {
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{})
	assert.NoError(t, err)
	assertNoopFilter(t, filter, func(t *testing.T, filter *noopFilter) {
		assert.Contains(t, filter.reason, "Configuration yielded no path to monitor")
	})
}

func Test_CreateFilter_EmptyString(t *testing.T) {
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{""})
	assert.NoError(t, err)
	assertNoopFilter(t, filter, func(t *testing.T, filter *noopFilter) {
		assert.Contains(t, filter.reason, "Configuration yielded no path to monitor")
	})
}

func Test_CreateFilter_NotAString(t *testing.T) {
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{1234})
	assert.NoError(t, err)
	assertNoopFilter(t, filter, func(t *testing.T, filter *noopFilter) {
		assert.Contains(t, filter.reason, "Configuration yielded no path to monitor")
	})
}

func Test_CreateFilter_NotJson(t *testing.T) {
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{"I am not JSON"})
	assert.NoError(t, err)
	assertNoopFilter(t, filter, func(t *testing.T, filter *noopFilter) {
		assert.Contains(t, filter.reason, "Configuration yielded no path to monitor")
	})
}

func Test_CreateFilter_EmptyJson(t *testing.T) {
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{"{}"})
	assert.NoError(t, err)
	assertNoopFilter(t, filter, func(t *testing.T, filter *noopFilter) {
		assert.Contains(t, filter.reason, "Configuration yielded no path to monitor")
	})
}

func Test_CreateFilter_NoPathTemplate(t *testing.T) {
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
		"path_templates": []
	}`})
	assert.NoError(t, err)
	assertNoopFilter(t, filter, func(t *testing.T, filter *noopFilter) {
		assert.Contains(t, filter.reason, "Configuration yielded no path to monitor")
	})
}

func Test_CreateFilter_EmptyPathTemplate(t *testing.T) {
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
		"api_id": "my_api",
		"path_templates": [
			""
		]
	}`})
	assert.NoError(t, err)
	assertNoopFilter(t, filter, func(t *testing.T, filter *noopFilter) {
		assert.Contains(t, filter.reason, "Configuration yielded no path to monitor")
	})
}

func Test_CreateFilter_TypoInPropertyNamesFail(t *testing.T) {
	spec := NewApiUsageMonitoring()
	// path_template has no `s` and should cause a JSON decoding error.
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
		"api_id": "my_api",
		"path_template": [
			""
		]
	}`})
	assert.NoError(t, err)
	assertNoopFilter(t, filter, func(t *testing.T, filter *noopFilter) {
		assert.Contains(t, filter.reason, "Configuration yielded no path to monitor")
	})
}

func Test_CreateFilter_NonParseableParametersShouldBeLoggedAndIgnored(t *testing.T) {
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{
		`{
			"application_id": "my_app",
			"api_id": "my_api",
			"path_templates": [
				"test"
			]
		}`,
		123456,
		123.456,
		"I am useless...", // poor little depressed parameter :'(
	})
	assert.NoError(t, err)
	assertApiUsageMonitoringFilter(t, filter, func(t *testing.T, filter *apiUsageMonitoringFilter) {

		assert.Len(t, filter.paths, 1)

		assert.Equal(t, "my_app", filter.paths[0].ApplicationId)
		assert.Equal(t, "my_api", filter.paths[0].ApiId)
		assert.Equal(t, "test", filter.paths[0].PathTemplate)
		assert.Equal(t, "^[\\/]*test[\\/]*$", filter.paths[0].Matcher.String())
	})
}

func Test_CreateFilter_FullConfigSingleApi(t *testing.T) {
	// Includes paths:
	//   - normal (no variable part)
	//   - with {name} variable paths
	//   - with :name variable paths
	//   - with/without head/trailing slash
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
		"api_id": "my_api",
		"path_templates": [
			"foo/orders",
			"foo/orders/:order-id",
			"foo/orders/:order-id/order_item/{order-item-id}",
			"/foo/customers/",
			"/foo/customers/{customer-id}/"
		]
	}`})
	assert.NoError(t, err)
	assertApiUsageMonitoringFilter(t, filter, func(t *testing.T, filter *apiUsageMonitoringFilter) {

		assert.Len(t, filter.paths, 5)

		assert.Equal(t, "my_app", filter.paths[0].ApplicationId)
		assert.Equal(t, "my_api", filter.paths[0].ApiId)
		assert.Equal(t, "foo/orders", filter.paths[0].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders[\\/]*$", filter.paths[0].Matcher.String())

		assert.Equal(t, "my_app", filter.paths[1].ApplicationId)
		assert.Equal(t, "my_api", filter.paths[1].ApiId)
		assert.Equal(t, "foo/orders/:order-id", filter.paths[1].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$", filter.paths[1].Matcher.String())

		assert.Equal(t, "my_app", filter.paths[2].ApplicationId)
		assert.Equal(t, "my_api", filter.paths[2].ApiId)
		assert.Equal(t, "foo/orders/:order-id/order_item/:order-item-id", filter.paths[2].PathTemplate) // normalized to `:id`
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$", filter.paths[2].Matcher.String())

		assert.Equal(t, "my_app", filter.paths[3].ApplicationId)
		assert.Equal(t, "my_api", filter.paths[3].ApiId)
		assert.Equal(t, "foo/customers", filter.paths[3].PathTemplate) // without the head/tail slashes
		assert.Equal(t, "^[\\/]*foo\\/customers[\\/]*$", filter.paths[3].Matcher.String())

		assert.Equal(t, "my_app", filter.paths[4].ApplicationId)
		assert.Equal(t, "my_api", filter.paths[4].ApiId)
		assert.Equal(t, "foo/customers/:customer-id", filter.paths[4].PathTemplate) // without the head/tail slashes, normalized to `:id`
		assert.Equal(t, "^[\\/]*foo\\/customers\\/[^\\/]+[\\/]*$", filter.paths[4].Matcher.String())
	})
}

func Test_CreateFilter_FullConfigMultipleApis(t *testing.T) {
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
			"application_id": "my_app",
			"api_id": "orders_api",
			"path_templates": [
				"foo/orders",
				"foo/orders/:order-id",
				"foo/orders/:order-id/order_item/{order-item-id}"
			]
		}`, `{
			"application_id": "my_app",
			"api_id": "customers_api",
			"path_templates": [
				"/foo/customers/",
				"/foo/customers/{customer-id}/"
			]
		}`})
	assert.NoError(t, err)
	assertApiUsageMonitoringFilter(t, filter, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Len(t, filter.paths, 5)

		assert.Equal(t, "my_app", filter.paths[0].ApplicationId)
		assert.Equal(t, "orders_api", filter.paths[0].ApiId)
		assert.Equal(t, "foo/orders", filter.paths[0].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders[\\/]*$", filter.paths[0].Matcher.String())

		assert.Equal(t, "my_app", filter.paths[1].ApplicationId)
		assert.Equal(t, "orders_api", filter.paths[1].ApiId)
		assert.Equal(t, "foo/orders/:order-id", filter.paths[1].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$", filter.paths[1].Matcher.String())

		assert.Equal(t, "my_app", filter.paths[2].ApplicationId)
		assert.Equal(t, "orders_api", filter.paths[2].ApiId)
		assert.Equal(t, "foo/orders/:order-id/order_item/:order-item-id", filter.paths[2].PathTemplate) // normalized to `:id`
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$", filter.paths[2].Matcher.String())

		assert.Equal(t, "my_app", filter.paths[3].ApplicationId)
		assert.Equal(t, "customers_api", filter.paths[3].ApiId)
		assert.Equal(t, "foo/customers", filter.paths[3].PathTemplate) // without the head/tail slashes
		assert.Equal(t, "^[\\/]*foo\\/customers[\\/]*$", filter.paths[3].Matcher.String())

		assert.Equal(t, "my_app", filter.paths[4].ApplicationId)
		assert.Equal(t, "customers_api", filter.paths[4].ApiId)
		assert.Equal(t, "foo/customers/:customer-id", filter.paths[4].PathTemplate) // without the head/tail slashes, normalized to `:id`
		assert.Equal(t, "^[\\/]*foo\\/customers\\/[^\\/]+[\\/]*$", filter.paths[4].Matcher.String())
	})
}

func Test_CreateFilter_FullConfigWithApisWithoutPaths(t *testing.T) {
	spec := NewApiUsageMonitoring()
	// There is a valid object for the 2nd api (customers_api), but no path_templates.
	// Since the end result is that there are a total to observable paths > 0, it should
	// be accepted.
	filter, err := spec.CreateFilter([]interface{}{`{
			"application_id": "my_app",
			"api_id": "orders_api",
			"path_templates": [
				"foo/orders",
				"foo/orders/:order-id",
				"foo/orders/:order-id/order_item/{order-item-id}"
			]
		}`, `{
			"application_id": "my_app",
			"api_id": "customers_api",
			"path_templates": [
			]
		}`})
	assert.NoError(t, err)
	assertApiUsageMonitoringFilter(t, filter, func(t *testing.T, filter *apiUsageMonitoringFilter) {

		assert.Len(t, filter.paths, 3)

		assert.Equal(t, "my_app", filter.paths[0].ApplicationId)
		assert.Equal(t, "foo/orders", filter.paths[0].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders[\\/]*$", filter.paths[0].Matcher.String())

		assert.Equal(t, "my_app", filter.paths[1].ApplicationId)
		assert.Equal(t, "foo/orders/:order-id", filter.paths[1].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$", filter.paths[1].Matcher.String())

		assert.Equal(t, "my_app", filter.paths[2].ApplicationId)
		assert.Equal(t, "foo/orders/:order-id/order_item/:order-item-id", filter.paths[2].PathTemplate) // normalized to `:id`
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$", filter.paths[2].Matcher.String())
	})
}

func Test_CreateFilter_DuplicatePathTemplatesAreIgnored(t *testing.T) {
	// PathTemplate "foo" and "/foo/" after normalising are the same.
	// That causes an error, even if under different application or API IDs.
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
		"api_id": "orders_api",
		"path_templates": [
			"foo",
			"/foo/"
		]
	}`})
	assert.NoError(t, err)
	assertApiUsageMonitoringFilter(t, filter, func(t *testing.T, filter *apiUsageMonitoringFilter) {

		assert.Len(t, filter.paths, 1)

		assert.Equal(t, filter.paths[0].ApplicationId, "my_app")
		assert.Equal(t, filter.paths[0].PathTemplate, "foo")
		assert.Equal(t, filter.paths[0].Matcher.String(), "^[\\/]*foo[\\/]*$")
	})
}

func Test_CreateFilter_DuplicateMatchersAreIgnored(t *testing.T) {
	// PathTemplate "/foo/:a" and "/foo/:b" yield the same RegExp
	spec := NewApiUsageMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
		"api_id": "orders_api",
		"path_templates": [
			"foo/:a",
			"foo/:b"
		]
	}`})
	assert.NoError(t, err)
	assertApiUsageMonitoringFilter(t, filter, func(t *testing.T, filter *apiUsageMonitoringFilter) {

		assert.Len(t, filter.paths, 1)

		assert.Equal(t, "my_app", filter.paths[0].ApplicationId)
		assert.Equal(t, "foo/:a", filter.paths[0].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/[^\\/]+[\\/]*$", filter.paths[0].Matcher.String())
	})
}

func Test_CreateFilter_RegExCompileFailureCausesError(t *testing.T) {
	spec := &apiUsageMonitoringSpec{}
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
		"api_id": "orders_api",
		"path_templates": [
			"(["
		]
	}`})
	assert.NoError(t, err)
	assert.NoError(t, err)
	assertNoopFilter(t, filter, func(t *testing.T, filter *noopFilter) {
		assert.Contains(t, filter.reason, "Configuration yielded no path to monitor")
	})
}
