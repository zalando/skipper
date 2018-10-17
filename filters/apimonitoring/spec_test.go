package apimonitoring

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_Name(t *testing.T) {
	spec := NewApiMonitoring()
	assert.Equal(t, "apimonitoring", spec.Name())
}

func Test_CreateFilter_NoParam(t *testing.T) {
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expecting one parameter (JSON configuration of the filter)")
	assert.Nil(t, filter)
}

func Test_CreateFilter_EmptyString(t *testing.T) {
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{""})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error reading JSON configuration")
	assert.Nil(t, filter)
}

func Test_CreateFilter_NotAString(t *testing.T) {
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{1234})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expecting first parameter to be a string")
	assert.Nil(t, filter)
}

func Test_CreateFilter_NotJson(t *testing.T) {
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{"I am not JSON"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error reading JSON configuration")
	assert.Nil(t, filter)
}

func Test_CreateFilter_EmptyJson(t *testing.T) {
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{"{}"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no path to monitor")
	assert.Nil(t, filter)
}

func Test_CreateFilter_NoPathTemplate(t *testing.T) {
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
		"apis": [
			{}
		]
	}`})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no path to monitor")
	assert.Nil(t, filter)
}

func Test_CreateFilter_EmptyPathTemplate(t *testing.T) {
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
		"apis": [
			{
				"application_id": "my_app",
				"api_id": "my_api",
				"path_templates": [
					""
				]
			}
		]
	}`})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty path at API index 0, template index 0")
	assert.Nil(t, filter)
}

func Test_CreateFilter_TypoInPropertyNamesFail(t *testing.T) {
	spec := NewApiMonitoring()
	// path_template has no `s` and should cause a JSON decoding error.
	filter, err := spec.CreateFilter([]interface{}{`{
		"apis": [
			{
				"application_id": "my_app",
				"api_id": "my_api",
				"path_template": [
					""
				]
			}
		]
	}`})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error reading JSON configuration")
	assert.Contains(t, err.Error(), `json: unknown field "path_template"`)
	assert.Nil(t, filter)
}

func Test_CreateFilter_ExtraParametersAreIgnored(t *testing.T) {
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{
		`{
			"apis": [
				{
					"application_id": "my_app",
					"api_id": "my_api",
					"path_templates": [
						"test"
					]
				}
			]
		}`,
		123456,
		123.456,
		"I am useless...", // poor little depressed parameter :'(
	})
	assert.NoError(t, err)
	assert.NotNil(t, filter)
	actual, ok := filter.(*apiMonitoringFilter)
	assert.True(t, ok)

	assert.Len(t, actual.paths, 1)

	assert.Equal(t, "my_app", actual.paths[0].ApplicationId)
	assert.Equal(t, "my_api", actual.paths[0].ApiId)
	assert.Equal(t, "test", actual.paths[0].PathTemplate)
	assert.Equal(t, "^[\\/]*test[\\/]*$", actual.paths[0].Matcher.String())
}

func Test_CreateFilter_FullConfigSingleApi(t *testing.T) {
	// Includes paths:
	//   - normal (no variable part)
	//   - with {name} variable paths
	//   - with :name variable paths
	//   - with/without head/trailing slash
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
		"apis": [
			{
				"application_id": "my_app",
				"api_id": "my_api",
	  			"path_templates": [
					"foo/orders",
					"foo/orders/:order-id",
					"foo/orders/:order-id/order_item/{order-item-id}",
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

	assert.Len(t, actual.paths, 5)

	assert.Equal(t, "my_app", actual.paths[0].ApplicationId)
	assert.Equal(t, "my_api", actual.paths[0].ApiId)
	assert.Equal(t, "foo/orders", actual.paths[0].PathTemplate)
	assert.Equal(t, "^[\\/]*foo\\/orders[\\/]*$", actual.paths[0].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[1].ApplicationId)
	assert.Equal(t, "my_api", actual.paths[1].ApiId)
	assert.Equal(t, "foo/orders/:order-id", actual.paths[1].PathTemplate)
	assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$", actual.paths[1].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[2].ApplicationId)
	assert.Equal(t, "my_api", actual.paths[2].ApiId)
	assert.Equal(t, "foo/orders/:order-id/order_item/:order-item-id", actual.paths[2].PathTemplate) // normalized to `:id`
	assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$", actual.paths[2].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[3].ApplicationId)
	assert.Equal(t, "my_api", actual.paths[3].ApiId)
	assert.Equal(t, "foo/customers", actual.paths[3].PathTemplate) // without the head/tail slashes
	assert.Equal(t, "^[\\/]*foo\\/customers[\\/]*$", actual.paths[3].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[4].ApplicationId)
	assert.Equal(t, "my_api", actual.paths[4].ApiId)
	assert.Equal(t, "foo/customers/:customer-id", actual.paths[4].PathTemplate) // without the head/tail slashes, normalized to `:id`
	assert.Equal(t, "^[\\/]*foo\\/customers\\/[^\\/]+[\\/]*$", actual.paths[4].Matcher.String())
}

func Test_CreateFilter_FullConfigMultipleApis(t *testing.T) {
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
		"apis": [
			{
				"application_id": "my_app",
				"api_id": "orders_api",
	  			"path_templates": [
					"foo/orders",
					"foo/orders/:order-id",
					"foo/orders/:order-id/order_item/{order-item-id}"
				]
			},
			{
				"application_id": "my_app",
				"api_id": "customers_api",
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

	assert.Len(t, actual.paths, 5)

	assert.Equal(t, "my_app", actual.paths[0].ApplicationId)
	assert.Equal(t, "orders_api", actual.paths[0].ApiId)
	assert.Equal(t, "foo/orders", actual.paths[0].PathTemplate)
	assert.Equal(t, "^[\\/]*foo\\/orders[\\/]*$", actual.paths[0].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[1].ApplicationId)
	assert.Equal(t, "orders_api", actual.paths[1].ApiId)
	assert.Equal(t, "foo/orders/:order-id", actual.paths[1].PathTemplate)
	assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$", actual.paths[1].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[2].ApplicationId)
	assert.Equal(t, "orders_api", actual.paths[2].ApiId)
	assert.Equal(t, "foo/orders/:order-id/order_item/:order-item-id", actual.paths[2].PathTemplate) // normalized to `:id`
	assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$", actual.paths[2].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[3].ApplicationId)
	assert.Equal(t, "customers_api", actual.paths[3].ApiId)
	assert.Equal(t, "foo/customers", actual.paths[3].PathTemplate) // without the head/tail slashes
	assert.Equal(t, "^[\\/]*foo\\/customers[\\/]*$", actual.paths[3].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[4].ApplicationId)
	assert.Equal(t, "customers_api", actual.paths[4].ApiId)
	assert.Equal(t, "foo/customers/:customer-id", actual.paths[4].PathTemplate) // without the head/tail slashes, normalized to `:id`
	assert.Equal(t, "^[\\/]*foo\\/customers\\/[^\\/]+[\\/]*$", actual.paths[4].Matcher.String())
}

func Test_CreateFilter_FullConfigWithApisWithoutPaths(t *testing.T) {
	spec := NewApiMonitoring()
	// There is a valid object for the 2nd api (customers_api), but no path_templates.
	// Since the end result is that there are a total to observable paths > 0, it should
	// be accepted.
	filter, err := spec.CreateFilter([]interface{}{`{
		"apis": [
			{
				"application_id": "my_app",
				"api_id": "orders_api",
	  			"path_templates": [
					"foo/orders",
					"foo/orders/:order-id",
					"foo/orders/:order-id/order_item/{order-item-id}"
				]
			},
			{
				"application_id": "my_app",
				"api_id": "customers_api",
				"path_templates": [
				]
			}
		]
	}`})
	assert.NoError(t, err)
	assert.NotNil(t, filter)
	actual, ok := filter.(*apiMonitoringFilter)
	assert.True(t, ok)

	assert.Len(t, actual.paths, 3)

	assert.Equal(t, "my_app", actual.paths[0].ApplicationId)
	assert.Equal(t, "foo/orders", actual.paths[0].PathTemplate)
	assert.Equal(t, "^[\\/]*foo\\/orders[\\/]*$", actual.paths[0].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[1].ApplicationId)
	assert.Equal(t, "foo/orders/:order-id", actual.paths[1].PathTemplate)
	assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$", actual.paths[1].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[2].ApplicationId)
	assert.Equal(t, "foo/orders/:order-id/order_item/:order-item-id", actual.paths[2].PathTemplate) // normalized to `:id`
	assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$", actual.paths[2].Matcher.String())
}

func Test_CreateFilter_DuplicatePathTemplatesAreIgnored(t *testing.T) {
	// PathTemplate "foo" and "/foo/" after normalising are the same.
	// That causes an error, even if under different application or API IDs.
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
		"apis": [
			{
				"application_id": "my_app",
				"api_id": "orders_api",
	  			"path_templates": [
					"foo",
					"/foo/"
				]
			}
		]
	}`})
	assert.NoError(t, err)
	assert.NotNil(t, filter)
	actual, ok := filter.(*apiMonitoringFilter)
	assert.True(t, ok)

	assert.Len(t, actual.paths, 1)

	assert.Equal(t, actual.paths[0].ApplicationId, "my_app")
	assert.Equal(t, actual.paths[0].PathTemplate, "foo")
	assert.Equal(t, actual.paths[0].Matcher.String(), "^[\\/]*foo[\\/]*$")
}

func Test_CreateFilter_DuplicateMatchersAreIgnored(t *testing.T) {
	// PathTemplate "/foo/:a" and "/foo/:b" yield the same RegExp
	spec := NewApiMonitoring()
	filter, err := spec.CreateFilter([]interface{}{`{
		"apis": [
			{
				"application_id": "my_app",
				"api_id": "orders_api",
	  			"path_templates": [
					"foo/:a",
					"foo/:b"
				]
			}
		]
	}`})
	assert.NoError(t, err)
	assert.NotNil(t, filter)
	actual, ok := filter.(*apiMonitoringFilter)
	assert.True(t, ok)

	assert.Len(t, actual.paths, 1)

	assert.Equal(t, "my_app", actual.paths[0].ApplicationId)
	assert.Equal(t, "foo/:a", actual.paths[0].PathTemplate)
	assert.Equal(t, "^[\\/]*foo\\/[^\\/]+[\\/]*$", actual.paths[0].Matcher.String())
}

func Test_CreateFilter_RegExCompileFailureCausesError(t *testing.T) {
	spec := &apiMonitoringFilterSpec{}
	filter, err := spec.CreateFilter([]interface{}{`{
		"apis": [
			{
				"application_id": "my_app",
				"api_id": "orders_api",
	  			"path_templates": [
					"(["
				]
			}
		]
	}`})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error compiling regular expression")
	assert.Nil(t, filter)
}
