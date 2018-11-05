package apiusagemonitoring

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_CreateSpec(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "realm", "    abc,def, ,ghi,xyz   ")
	assert.Equal(t, "apiUsageMonitoring", spec.Name())
	if assert.IsType(t, new(apiUsageMonitoringSpec), spec) {
		s := spec.(*apiUsageMonitoringSpec)
		assert.Equal(t, []string{"abc", "def", "ghi", "xyz"}, s.clientIdKey)
		assert.Equal(t, "realm", s.realmKey)
		assert.NotNil(t, s.unknownPath)
	}
}

func Test_FeatureDisableCreateNilFilters(t *testing.T) {
	spec := NewApiUsageMonitoring(false, "", "")
	assert.IsType(t, &noopSpec{}, spec)
	filter, err := spec.CreateFilter([]interface{}{})
	assert.NoError(t, err)
	assert.Equal(t, filter, &noopFilter{})
}

func assertApiUsageMonitoringFilter(t *testing.T, filterArgs []interface{}, asserter func(t *testing.T, filter *apiUsageMonitoringFilter)) {
	spec := NewApiUsageMonitoring(true, "", "")
	filter, err := spec.CreateFilter(filterArgs)
	assert.NoError(t, err)
	assert.NotNil(t, filter)
	if assert.IsType(t, &apiUsageMonitoringFilter{}, filter) {
		asserter(t, filter.(*apiUsageMonitoringFilter))
	}
}

type pathMatcher struct {
	ApplicationId string
	ApiId         string
	PathTemplate  string
	Matcher       string
}

func assertPaths(t *testing.T, paths []*pathInfo, expectedPaths []pathMatcher) {
	if !assert.Len(t, paths, len(expectedPaths)) {
		return
	}
	for i, actual := range paths {
		expected := expectedPaths[i]
		if !assert.Equal(t, expected.PathTemplate, actual.PathTemplate, fmt.Sprintf("Index %d", i)) {
			continue // don't test this one further, it's an ordering problem and the template in results is enough
		}
		assert.Equal(t, expected.ApplicationId, actual.ApplicationId, fmt.Sprintf("Index %d", i))
		assert.Equal(t, expected.ApiId, actual.ApiId, fmt.Sprintf("Index %d", i))
		assert.Equal(t, expected.Matcher, actual.Matcher.String(), fmt.Sprintf("Index %d", i))
	}
}

func Test_FeatureNotEnabled_TypeNameAndCreatedFilterAreRight(t *testing.T) {
	spec := NewApiUsageMonitoring(false, "", "")
	assert.Equal(t, "apiUsageMonitoring", spec.Name())
	filter, err := spec.CreateFilter([]interface{}{})
	assert.NoError(t, err)
	assert.Equal(t, filter, &noopFilter{})
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
		assertPaths(t, filter.Paths, []pathMatcher{
			{
				PathTemplate:  "test",
				ApplicationId: "my_app",
				ApiId:         "my_api",
				Matcher:       "^\\/*test\\/*$",
			},
		})
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
		assertPaths(t, filter.Paths, []pathMatcher{
			{
				PathTemplate:  "foo/orders/:order-id/order_item/:order-item-id",
				ApplicationId: "my_app",
				ApiId:         "my_api",
				Matcher:       "^\\/*foo\\/orders\\/.+\\/order_item\\/.+\\/*$",
			},
			{
				PathTemplate:  "foo/orders/:order-id",
				ApplicationId: "my_app",
				ApiId:         "my_api",
				Matcher:       "^\\/*foo\\/orders\\/.+\\/*$",
			},
			{
				PathTemplate:  "foo/orders",
				ApplicationId: "my_app",
				ApiId:         "my_api",
				Matcher:       "^\\/*foo\\/orders\\/*$",
			},
			{
				PathTemplate:  "foo/customers/:customer-id",
				ApplicationId: "my_app",
				ApiId:         "my_api",
				Matcher:       "^\\/*foo\\/customers\\/.+\\/*$",
			},
			{
				PathTemplate:  "foo/customers",
				ApplicationId: "my_app",
				ApiId:         "my_api",
				Matcher:       "^\\/*foo\\/customers\\/*$",
			},
		})
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
		assertPaths(t, filter.Paths, []pathMatcher{
			{
				PathTemplate:  "foo/orders/:order-id/order_item/:order-item-id",
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				Matcher:       "^\\/*foo\\/orders\\/.+\\/order_item\\/.+\\/*$",
			},
			{
				PathTemplate:  "foo/orders/:order-id",
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				Matcher:       "^\\/*foo\\/orders\\/.+\\/*$",
			},
			{
				PathTemplate:  "foo/orders",
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				Matcher:       "^\\/*foo\\/orders\\/*$",
			},
			{
				PathTemplate:  "foo/customers/:customer-id",
				ApplicationId: "my_app",
				ApiId:         "customers_api",
				Matcher:       "^\\/*foo\\/customers\\/.+\\/*$",
			},
			{
				PathTemplate:  "foo/customers",
				ApplicationId: "my_app",
				ApiId:         "customers_api",
				Matcher:       "^\\/*foo\\/customers\\/*$",
			},
		})
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
		assertPaths(t, filter.Paths, []pathMatcher{
			{
				PathTemplate:  "foo/orders/:order-id/order_item/:order-item-id",
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				Matcher:       "^\\/*foo\\/orders\\/.+\\/order_item\\/.+\\/*$",
			},
			{
				PathTemplate:  "foo/orders/:order-id",
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				Matcher:       "^\\/*foo\\/orders\\/.+\\/*$",
			},
			{
				PathTemplate:  "foo/orders",
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				Matcher:       "^\\/*foo\\/orders\\/*$",
			},
		})
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
		assertPaths(t, filter.Paths, []pathMatcher{
			{
				PathTemplate:  "foo",
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				Matcher:       "^\\/*foo\\/*$",
			},
		})
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
		assertPaths(t, filter.Paths, []pathMatcher{
			{
				PathTemplate:  "foo/:a",
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				Matcher:       "^\\/*foo\\/.+\\/*$",
			},
		})
	})
}

func Test_CreateFilter_RegExCompileFailureIgnoresPath(t *testing.T) {
	args := []interface{}{`{
		"application_id": "my_app",
		"api_id": "orders_api",
		"path_templates": [
			"(["
		]
	}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
	})
}
