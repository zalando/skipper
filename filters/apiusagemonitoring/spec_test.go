package apiusagemonitoring

import (
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"testing"
)

func init() {
	log.Logger.SetLevel(logrus.DebugLevel)
}

func Test_TypeAndName(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "", "", "")
	assert.Equal(t, &apiUsageMonitoringSpec{
		enabled:                 true,
		clientIdKeyName:         "",
		realmKeyName:            "",
		realmAndClientIdMatcher: "",
		realmAndClientIdRegExp:  nil,
	}, spec)
	assert.Equal(t, "apiUsageMonitoring", spec.Name())
}

func Test_FeatureDisableCreateNilFilters(t *testing.T) {
	spec := NewApiUsageMonitoring(false, "", "", "")
	assert.Equal(t, &apiUsageMonitoringSpec{
		enabled:                 false,
		clientIdKeyName:         "",
		realmKeyName:            "",
		realmAndClientIdMatcher: "",
		realmAndClientIdRegExp:  nil,
	}, spec)
	filter, err := spec.CreateFilter([]interface{}{})
	assert.NoError(t, err)
	assert.Nil(t, filter)
}

func assertApiUsageMonitoringFilter(t *testing.T, filterArgs []interface{}, asserter func(t *testing.T, filter *apiUsageMonitoringFilter)) {
	spec := NewApiUsageMonitoring(true, "", "", "")
	filter, err := spec.CreateFilter(filterArgs)
	assert.NoError(t, err)
	assert.NotNil(t, filter)
	if assert.IsType(t, &apiUsageMonitoringFilter{}, filter) {
		asserter(t, filter.(*apiUsageMonitoringFilter))
	}
}

func Test_FeatureNotEnabled_TypeNameAndCreatedFilterAreRight(t *testing.T) {
	spec := NewApiUsageMonitoring(false, "", "", "")
	assert.Equal(t, "apiUsageMonitoring", spec.Name())
	filter, err := spec.CreateFilter([]interface{}{})
	assert.NoError(t, err)
	assert.Nil(t, filter)
}

func Test_CreateFilter_NoParam(t *testing.T) {
	var args []interface{}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
	})
}

func Test_CreateFilter_EmptyString(t *testing.T) {
	args := []interface{}{""}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
	})
}

func Test_CreateFilter_NotAString(t *testing.T) {
	args := []interface{}{1234}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
	})
}

func Test_CreateFilter_NotJson(t *testing.T) {
	args := []interface{}{"I am not JSON"}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
	})
}

func Test_CreateFilter_EmptyJson(t *testing.T) {
	args := []interface{}{"{}"}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
	})
}

func Test_CreateFilter_NoPathTemplate(t *testing.T) {
	args := []interface{}{`{
		"path_templates": []
	}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
	})
}

func Test_CreateFilter_EmptyPathTemplate(t *testing.T) {
	args := []interface{}{`{
		"application_id": "my_app",
		"api_id": "my_api",
		"path_templates": [
			""
		]
	}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
	})
}

func Test_CreateFilter_TypoInPropertyNamesFail(t *testing.T) {
	// path_template has no `s` and should cause a JSON decoding error.
	args := []interface{}{`{
		"application_id": "my_app",
		"api_id": "my_api",
		"path_template": [
			""
		]
	}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
	})
}

func Test_CreateFilter_NonParseableParametersShouldBeLoggedAndIgnored(t *testing.T) {
	args := []interface{}{
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
	}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {

		assert.Len(t, filter.Paths, 1)

		assert.Equal(t, "my_app", filter.Paths[0].ApplicationId)
		assert.Equal(t, "my_api", filter.Paths[0].ApiId)
		assert.Equal(t, "test", filter.Paths[0].PathTemplate)
		assert.Equal(t, "^[\\/]*test[\\/]*$", filter.Paths[0].Matcher.String())
	})
}

func Test_CreateFilter_FullConfigSingleApi(t *testing.T) {
	// Includes paths:
	//   - normal (no variable part)
	//   - with {name} variable paths
	//   - with :name variable paths
	//   - with/without head/trailing slash
	args := []interface{}{`{
		"application_id": "my_app",
		"api_id": "my_api",
		"path_templates": [
			"foo/orders",
			"foo/orders/:order-id",
			"foo/orders/:order-id/order_item/{order-item-id}",
			"/foo/customers/",
			"/foo/customers/{customer-id}/"
		]
	}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {

		assert.Len(t, filter.Paths, 5)

		assert.Equal(t, "my_app", filter.Paths[0].ApplicationId)
		assert.Equal(t, "my_api", filter.Paths[0].ApiId)
		assert.Equal(t, "foo/orders", filter.Paths[0].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders[\\/]*$", filter.Paths[0].Matcher.String())

		assert.Equal(t, "my_app", filter.Paths[1].ApplicationId)
		assert.Equal(t, "my_api", filter.Paths[1].ApiId)
		assert.Equal(t, "foo/orders/:order-id", filter.Paths[1].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$", filter.Paths[1].Matcher.String())

		assert.Equal(t, "my_app", filter.Paths[2].ApplicationId)
		assert.Equal(t, "my_api", filter.Paths[2].ApiId)
		assert.Equal(t, "foo/orders/:order-id/order_item/:order-item-id", filter.Paths[2].PathTemplate) // normalized to `:id`
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$", filter.Paths[2].Matcher.String())

		assert.Equal(t, "my_app", filter.Paths[3].ApplicationId)
		assert.Equal(t, "my_api", filter.Paths[3].ApiId)
		assert.Equal(t, "foo/customers", filter.Paths[3].PathTemplate) // without the head/tail slashes
		assert.Equal(t, "^[\\/]*foo\\/customers[\\/]*$", filter.Paths[3].Matcher.String())

		assert.Equal(t, "my_app", filter.Paths[4].ApplicationId)
		assert.Equal(t, "my_api", filter.Paths[4].ApiId)
		assert.Equal(t, "foo/customers/:customer-id", filter.Paths[4].PathTemplate) // without the head/tail slashes, normalized to `:id`
		assert.Equal(t, "^[\\/]*foo\\/customers\\/[^\\/]+[\\/]*$", filter.Paths[4].Matcher.String())
	})
}

func Test_CreateFilter_FullConfigMultipleApis(t *testing.T) {
	args := []interface{}{`{
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
		}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Len(t, filter.Paths, 5)

		assert.Equal(t, "my_app", filter.Paths[0].ApplicationId)
		assert.Equal(t, "orders_api", filter.Paths[0].ApiId)
		assert.Equal(t, "foo/orders", filter.Paths[0].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders[\\/]*$", filter.Paths[0].Matcher.String())

		assert.Equal(t, "my_app", filter.Paths[1].ApplicationId)
		assert.Equal(t, "orders_api", filter.Paths[1].ApiId)
		assert.Equal(t, "foo/orders/:order-id", filter.Paths[1].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$", filter.Paths[1].Matcher.String())

		assert.Equal(t, "my_app", filter.Paths[2].ApplicationId)
		assert.Equal(t, "orders_api", filter.Paths[2].ApiId)
		assert.Equal(t, "foo/orders/:order-id/order_item/:order-item-id", filter.Paths[2].PathTemplate) // normalized to `:id`
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$", filter.Paths[2].Matcher.String())

		assert.Equal(t, "my_app", filter.Paths[3].ApplicationId)
		assert.Equal(t, "customers_api", filter.Paths[3].ApiId)
		assert.Equal(t, "foo/customers", filter.Paths[3].PathTemplate) // without the head/tail slashes
		assert.Equal(t, "^[\\/]*foo\\/customers[\\/]*$", filter.Paths[3].Matcher.String())

		assert.Equal(t, "my_app", filter.Paths[4].ApplicationId)
		assert.Equal(t, "customers_api", filter.Paths[4].ApiId)
		assert.Equal(t, "foo/customers/:customer-id", filter.Paths[4].PathTemplate) // without the head/tail slashes, normalized to `:id`
		assert.Equal(t, "^[\\/]*foo\\/customers\\/[^\\/]+[\\/]*$", filter.Paths[4].Matcher.String())
	})
}

func Test_CreateFilter_FullConfigWithApisWithoutPaths(t *testing.T) {
	// There is a valid object for the 2nd api (customers_api), but no path_templates.
	// Since the end result is that there are a total to observable paths > 0, it should
	// be accepted.
	args := []interface{}{`{
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
		}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {

		assert.Len(t, filter.Paths, 3)

		assert.Equal(t, "my_app", filter.Paths[0].ApplicationId)
		assert.Equal(t, "foo/orders", filter.Paths[0].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders[\\/]*$", filter.Paths[0].Matcher.String())

		assert.Equal(t, "my_app", filter.Paths[1].ApplicationId)
		assert.Equal(t, "foo/orders/:order-id", filter.Paths[1].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$", filter.Paths[1].Matcher.String())

		assert.Equal(t, "my_app", filter.Paths[2].ApplicationId)
		assert.Equal(t, "foo/orders/:order-id/order_item/:order-item-id", filter.Paths[2].PathTemplate) // normalized to `:id`
		assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$", filter.Paths[2].Matcher.String())
	})
}

func Test_CreateFilter_DuplicatePathTemplatesAreIgnored(t *testing.T) {
	// PathTemplate "foo" and "/foo/" after normalising are the same.
	// That causes an error, even if under different application or API IDs.
	args := []interface{}{`{
		"application_id": "my_app",
		"api_id": "orders_api",
		"path_templates": [
			"foo",
			"/foo/"
		]
	}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {

		assert.Len(t, filter.Paths, 1)

		assert.Equal(t, filter.Paths[0].ApplicationId, "my_app")
		assert.Equal(t, filter.Paths[0].PathTemplate, "foo")
		assert.Equal(t, filter.Paths[0].Matcher.String(), "^[\\/]*foo[\\/]*$")
	})
}

func Test_CreateFilter_DuplicateMatchersAreIgnored(t *testing.T) {
	// PathTemplate "/foo/:a" and "/foo/:b" yield the same RegExp
	args := []interface{}{`{
		"application_id": "my_app",
		"api_id": "orders_api",
		"path_templates": [
			"foo/:a",
			"foo/:b"
		]
	}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {

		assert.Len(t, filter.Paths, 1)

		assert.Equal(t, "my_app", filter.Paths[0].ApplicationId)
		assert.Equal(t, "foo/:a", filter.Paths[0].PathTemplate)
		assert.Equal(t, "^[\\/]*foo\\/[^\\/]+[\\/]*$", filter.Paths[0].Matcher.String())
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
	assert.Nil(t, filter)
}
