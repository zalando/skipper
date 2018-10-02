package apimonitoring

import (
	"github.com/stretchr/testify/assert"
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
	assert.Contains(t, err.Error(), "no `application_id` provided")
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

	assert.False(t, actual.verbose)

	assert.Len(t, actual.paths, 5)

	assert.Equal(t, actual.paths[0].ApplicationId, "my_app")
	assert.Equal(t, actual.paths[0].PathTemplate, "foo/orders")
	assert.Equal(t, actual.paths[0].Matcher.String(), "^[\\/]*foo\\/orders[\\/]*$")

	assert.Equal(t, actual.paths[1].ApplicationId, "my_app")
	assert.Equal(t, actual.paths[1].PathTemplate, "foo/orders/:order-id")
	assert.Equal(t, actual.paths[1].Matcher.String(), "^[\\/]*foo\\/orders\\/[^\\/]+[\\/]*$")

	assert.Equal(t, actual.paths[2].ApplicationId, "my_app")
	assert.Equal(t, actual.paths[2].PathTemplate, "foo/orders/:order-id/order_item/:order-item-id") // normalized to `:id`
	assert.Equal(t, actual.paths[2].Matcher.String(), "^[\\/]*foo\\/orders\\/[^\\/]+\\/order_item\\/[^\\/]+[\\/]*$")

	assert.Equal(t, actual.paths[3].ApplicationId, "my_app")
	assert.Equal(t, actual.paths[3].PathTemplate, "foo/customers") // without the head/tail slashes
	assert.Equal(t, actual.paths[3].Matcher.String(), "^[\\/]*foo\\/customers[\\/]*$")

	assert.Equal(t, actual.paths[4].ApplicationId, "my_app")
	assert.Equal(t, actual.paths[4].PathTemplate, "foo/customers/:customer-id") // without the head/tail slashes, normalized to `:id`
	assert.Equal(t, actual.paths[4].Matcher.String(), "^[\\/]*foo\\/customers\\/[^\\/]+[\\/]*$")
}

func Test_CreateFilter_DuplicatePathPatternCausesError(t *testing.T) {
	// PathTemplate "foo" and "/foo/" after normalising are the same.
	// That causes an error, even if under different application or API IDs.
	spec := &apiMonitoringFilterSpec{}
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
		"path_templates": [
			"foo",
			"/foo/"
		]
	}`})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate path pattern "foo" detected`)
	assert.Nil(t, filter)
}

func Test_CreateFilter_DuplicateMatchersCausesError(t *testing.T) {
	spec := &apiMonitoringFilterSpec{}
	filter, err := spec.CreateFilter([]interface{}{`{
		"application_id": "my_app",
		"path_templates": [
			"clients/:clientId",
			"clients/{clientId}"
		]
	}`})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate path pattern")
	assert.Nil(t, filter)
}
