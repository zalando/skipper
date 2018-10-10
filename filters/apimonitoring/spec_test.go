package apimonitoring

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_Name(t *testing.T) {
	spec := New(false)
	assert.Equal(t, "apimonitoring", spec.Name())
}

func Test_CreateFilter_NoParam(t *testing.T) {
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expecting one parameter (JSON configuration of the filter)")
	assert.Nil(t, filter)
}

func Test_CreateFilter_EmptyString(t *testing.T) {
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{""})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error reading JSON configuration")
	assert.Nil(t, filter)
}

func Test_CreateFilter_NotAString(t *testing.T) {
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{1234})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expecting first parameter to be a string")
	assert.Nil(t, filter)
}

func Test_CreateFilter_NotJson(t *testing.T) {
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{"I am not JSON"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error reading JSON configuration")
	assert.Nil(t, filter)
}

func Test_CreateFilter_EmptyJson(t *testing.T) {
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{"{}"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no `application_id` provided")
	assert.Nil(t, filter)
}

func Test_CreateFilter_NoPathTemplate(t *testing.T) {
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app"
	}`})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no path to monitor")
	assert.Nil(t, filter)
}

func Test_CreateFilter_EmptyPathTemplate(t *testing.T) {
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
		"path_templates": [
			""
		]
	}`})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty path at index 0")
	assert.Nil(t, filter)
}

func Test_CreateFilter_ExtraParametersAreIgnored(t *testing.T) {
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{
		`{
			"application_id": "my_app",
		  	"path_templates": [
				"test"
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

	assert.False(t, actual.verbose)

	assert.Len(t, actual.paths, 1)

	assert.Equal(t, "my_app", actual.paths[0].ApplicationId)
	assert.Equal(t, "test", actual.paths[0].PathTemplate)
	assert.Equal(t, "^[\\/]*test[\\/]*$", actual.paths[0].Matcher.String())
}

func Test_CreateFilter_VerboseIsFalseIfNotSpecified(t *testing.T) {
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
	  	"path_templates": [
			"test"
		]
	}`})
	assert.NoError(t, err)
	assert.NotNil(t, filter)
	actual, ok := filter.(*apiMonitoringFilter)
	assert.True(t, ok)

	assert.False(t, actual.verbose)

	assert.Len(t, actual.paths, 1)

	assert.Equal(t, "my_app", actual.paths[0].ApplicationId)
	assert.Equal(t, "test", actual.paths[0].PathTemplate)
	assert.Equal(t, "^[\\/]*test[\\/]*$", actual.paths[0].Matcher.String())
}

func Test_CreateFilter_VerboseIsForcedByGlobalFilterConfiguration(t *testing.T) {
	spec := New(true) // <---	this parameter forces all filters to be verbose, even if they
	//							are explicitly configured not to.
	filter, err := spec.CreateFilter([]interface{}{`{
		"verbose": false,
		"application_id": "my_app",
	  	"path_templates": [
			"test"
		]
	}`})
	assert.NoError(t, err)
	assert.NotNil(t, filter)
	actual, ok := filter.(*apiMonitoringFilter)
	assert.True(t, ok)

	assert.True(t, actual.verbose)

	assert.Len(t, actual.paths, 1)

	assert.Equal(t, "my_app", actual.paths[0].ApplicationId)
	assert.Equal(t, "test", actual.paths[0].PathTemplate)
	assert.Equal(t, "^[\\/]*test[\\/]*$", actual.paths[0].Matcher.String())
}

func Test_CreateFilter_FullConfig(t *testing.T) {
	// Includes paths:
	//   - normal (no variable part)
	//   - with {name} variable paths
	//   - with :name variable paths
	//   - with/without head/trailing slash
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{`{
		"verbose": true,
		"application_id": "my_app",
	  	"path_templates": [
			"foo/orders",
			"foo/orders/:order-id",
			"foo/orders/:order-id/order_item/{order-item-id}",
			"/foo/customers/",
			"/foo/customers/{customer-id}/"
		]
	}`})
	assert.NoError(t, err)
	assert.NotNil(t, filter)
	actual, ok := filter.(*apiMonitoringFilter)
	assert.True(t, ok)

	assert.True(t, actual.verbose)

	assert.Len(t, actual.paths, 5)

	assert.Equal(t, "my_app", actual.paths[0].ApplicationId)
	assert.Equal(t, "foo/orders", actual.paths[0].PathTemplate)
	assert.Equal(t, "^[\\/]*foo\\/orders[\\/]*$", actual.paths[0].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[1].ApplicationId)
	assert.Equal(t, "foo/orders/:order-id", actual.paths[1].PathTemplate)
	assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$", actual.paths[1].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[2].ApplicationId)
	assert.Equal(t, "foo/orders/:order-id/order_item/:order-item-id", actual.paths[2].PathTemplate) // normalized to `:id`
	assert.Equal(t, "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$", actual.paths[2].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[3].ApplicationId)
	assert.Equal(t, "foo/customers", actual.paths[3].PathTemplate) // without the head/tail slashes
	assert.Equal(t, "^[\\/]*foo\\/customers[\\/]*$", actual.paths[3].Matcher.String())

	assert.Equal(t, "my_app", actual.paths[4].ApplicationId)
	assert.Equal(t, "foo/customers/:customer-id", actual.paths[4].PathTemplate) // without the head/tail slashes, normalized to `:id`
	assert.Equal(t, "^[\\/]*foo\\/customers\\/[^\\/]+[\\/]*$", actual.paths[4].Matcher.String())
}

func Test_CreateFilter_DuplicatePathTemplatesAreIgnored(t *testing.T) {
	// PathTemplate "foo" and "/foo/" after normalising are the same.
	// That causes an error, even if under different application or API IDs.
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
		"path_templates": [
			"foo",
			"/foo/"
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
	spec := New(false)
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
		"path_templates": [
			"foo/:a",
			"foo/:b"
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
		"application_id": "my_app",
		"path_templates": [
			"(["
		]
	}`})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error compiling regular expression")
	assert.Nil(t, filter)
}
