package apiusagemonitoring

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_CreateSpec(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "realm", "    abc,def, ,ghi,xyz   ", "")
	assert.Equal(t, "apiUsageMonitoring", spec.Name())
	if assert.IsType(t, new(apiUsageMonitoringSpec), spec) {
		s := spec.(*apiUsageMonitoringSpec)
		assert.Equal(t, []string{"abc", "def", "ghi", "xyz"}, s.clientKeys)
		assert.Equal(t, []string{"realm"}, s.realmKeys)
		assert.NotNil(t, s.unknownPath)
	}
}

func Test_FeatureDisableCreateNilFilters(t *testing.T) {
	spec := NewApiUsageMonitoring(false, "", "", "")
	assert.IsType(t, &noopSpec{}, spec)
	filter, err := spec.CreateFilter([]interface{}{})
	assert.NoError(t, err)
	assert.Equal(t, filter, &noopFilter{})
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

type pathMatcher struct {
	ApplicationId string
	ApiId         string
	PathTemplate  string
	Matcher       *string
}

func unknownPath(applicationId string) pathMatcher {
	return pathMatcher{
		PathTemplate:  "<unknown>",
		ApplicationId: applicationId,
		ApiId:         "<unknown>",
		Matcher:       nil,
	}
}

func matcher(matcher string) *string {
	return &matcher
}

func assertPath(t *testing.T, actualPath *pathInfo, expectedPath pathMatcher) {
	assert.Equalf(t, expectedPath.ApiId, actualPath.ApiId, "AppId")
	assert.Equalf(t, expectedPath.ApplicationId, actualPath.ApplicationId, "ApplicationId")
	assert.Equalf(t, expectedPath.PathTemplate, actualPath.PathTemplate, "PathTemplate")
	if expectedPath.Matcher != nil {
		assert.Equalf(t, *expectedPath.Matcher, actualPath.Matcher.String(), "Matcher")
	}
}

func assertPaths(t *testing.T, paths []*pathInfo, expectedPaths []pathMatcher) {
	if !assert.Len(t, paths, len(expectedPaths)) {
		return
	}
	for i, actual := range paths {
		expected := expectedPaths[i]
		assert.Equalf(t, expected.ApiId, actual.ApiId, "AppId[%d]", i)
		assert.Equalf(t, expected.ApplicationId, actual.ApplicationId, "ApplicationId[%d]", i)
		assert.Equalf(t, expected.PathTemplate, actual.PathTemplate, "PathTemplate[%d]", i)
		if expected.Matcher != nil {
			assert.Equalf(t, *expected.Matcher, actual.Matcher.String(), "Matcher[%d]", i)
		}
	}
}

func Test_FeatureNotEnabled_TypeNameAndCreatedFilterAreRight(t *testing.T) {
	spec := NewApiUsageMonitoring(false, "", "", "")
	assert.Equal(t, "apiUsageMonitoring", spec.Name())
	filter, err := spec.CreateFilter([]interface{}{})
	assert.NoError(t, err)
	assert.Equal(t, filter, &noopFilter{})
}

func Test_CreateFilter_NoParam(t *testing.T) {
	var args []interface{}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
		assertPath(t, filter.UnknownPath, unknownPath("<unknown>"))
	})
}

func Test_CreateFilter_EmptyString(t *testing.T) {
	args := []interface{}{""}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
		assertPath(t, filter.UnknownPath, unknownPath("<unknown>"))
	})
}

func Test_CreateFilter_NotAString(t *testing.T) {
	args := []interface{}{1234}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
		assertPath(t, filter.UnknownPath, unknownPath("<unknown>"))
	})
}

func Test_CreateFilter_NotJson(t *testing.T) {
	args := []interface{}{"I am not JSON"}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
		assertPath(t, filter.UnknownPath, unknownPath("<unknown>"))
	})
}

func Test_CreateFilter_EmptyJson(t *testing.T) {
	args := []interface{}{"{}"}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
		assertPath(t, filter.UnknownPath, unknownPath("<unknown>"))
	})
}

func Test_CreateFilter_NoPathTemplate(t *testing.T) {
	args := []interface{}{`{
		"path_templates": []
	}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assert.Empty(t, filter.Paths)
		assertPath(t, filter.UnknownPath, unknownPath("<unknown>"))
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
		assertPath(t, filter.UnknownPath, unknownPath("my_app"))
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
		assertPath(t, filter.UnknownPath, unknownPath("<unknown>"))
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
				ApplicationId: "my_app",
				ApiId:         "my_api",
				PathTemplate:  "test",
				Matcher:       matcher("^\\/*test\\/*$"),
			},
		})
		assertPath(t, filter.UnknownPath, unknownPath("my_app"))
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
				ApplicationId: "my_app",
				ApiId:         "my_api",
				PathTemplate:  "foo/orders/:order-id/order_item/:order-item-id",
				Matcher:       matcher("^\\/*foo\\/orders\\/.+\\/order_item\\/.+\\/*$"),
			},
			{
				ApplicationId: "my_app",
				ApiId:         "my_api",
				PathTemplate:  "foo/orders/:order-id",
				Matcher:       matcher("^\\/*foo\\/orders\\/.+\\/*$"),
			},
			{
				ApplicationId: "my_app",
				ApiId:         "my_api",
				PathTemplate:  "foo/orders",
				Matcher:       matcher("^\\/*foo\\/orders\\/*$"),
			},
			{
				ApplicationId: "my_app",
				ApiId:         "my_api",
				PathTemplate:  "foo/customers/:customer-id",
				Matcher:       matcher("^\\/*foo\\/customers\\/.+\\/*$"),
			},
			{
				ApplicationId: "my_app",
				ApiId:         "my_api",
				PathTemplate:  "foo/customers",
				Matcher:       matcher("^\\/*foo\\/customers\\/*$"),
			},
		})
		assertPath(t, filter.UnknownPath, unknownPath("my_app"))
	})
}

func Test_CreateFilter_NoApplicationOrApiId(t *testing.T) {
	args := []interface{}{`{
		"path_templates": [
			"foo/orders"
		]
	}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assertPaths(t, filter.Paths, []pathMatcher{
			{
				ApplicationId: "<unknown>",
				ApiId:         "<unknown>",
				PathTemplate:  "foo/orders",
				Matcher:       matcher("^\\/*foo\\/orders\\/*$"),
			},
		})
		assertPath(t, filter.UnknownPath, unknownPath("<unknown>"))
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
			],
			"client_tracking_pattern": ".*"
		}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assertPaths(t, filter.Paths, []pathMatcher{
			{
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				PathTemplate:  "foo/orders/:order-id/order_item/:order-item-id",
				Matcher:       matcher("^\\/*foo\\/orders\\/.+\\/order_item\\/.+\\/*$"),
			},
			{
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				PathTemplate:  "foo/orders/:order-id",
				Matcher:       matcher("^\\/*foo\\/orders\\/.+\\/*$"),
			},
			{
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				PathTemplate:  "foo/orders",
				Matcher:       matcher("^\\/*foo\\/orders\\/*$"),
			},
			{
				ApplicationId: "my_app",
				ApiId:         "customers_api",
				PathTemplate:  "foo/customers/:customer-id",
				Matcher:       matcher("^\\/*foo\\/customers\\/.+\\/*$"),
			},
			{
				ApplicationId: "my_app",
				ApiId:         "customers_api",
				PathTemplate:  "foo/customers",
				Matcher:       matcher("^\\/*foo\\/customers\\/*$"),
			},
		})
		assertPath(t, filter.UnknownPath, unknownPath("my_app"))
	})
}

func Test_CreateFilter_FullConfigWithApisWithoutPaths(t *testing.T) {
	// There is a valid object for the 2nd api (customers_api), but no path_templates.
	// Since the end result is that there are a total to observable paths > 0, it should
	// be accepted.
	args := []interface{}{`{
			"application_id": "my_order_app",
			"api_id": "orders_api",
			"path_templates": [
				"foo/orders",
				"foo/orders/:order-id",
				"foo/orders/:order-id/order_item/{order-item-id}"
			]
		}`, `{
			"application_id": "my_customer_app",
			"api_id": "customers_api",
			"path_templates": [
			]
		}`}
	assertApiUsageMonitoringFilter(t, args, func(t *testing.T, filter *apiUsageMonitoringFilter) {
		assertPaths(t, filter.Paths, []pathMatcher{
			{
				ApplicationId: "my_order_app",
				ApiId:         "orders_api",
				PathTemplate:  "foo/orders/:order-id/order_item/:order-item-id",
				Matcher:       matcher("^\\/*foo\\/orders\\/.+\\/order_item\\/.+\\/*$"),
			},
			{
				ApplicationId: "my_order_app",
				ApiId:         "orders_api",
				PathTemplate:  "foo/orders/:order-id",
				Matcher:       matcher("^\\/*foo\\/orders\\/.+\\/*$"),
			},
			{
				ApplicationId: "my_order_app",
				ApiId:         "orders_api",
				PathTemplate:  "foo/orders",
				Matcher:       matcher("^\\/*foo\\/orders\\/*$"),
			},
		})
		assertPath(t, filter.UnknownPath, unknownPath("<unknown>"))
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
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				PathTemplate:  "foo",
				Matcher:       matcher("^\\/*foo\\/*$"),
			},
		})
		assertPath(t, filter.UnknownPath, unknownPath("my_app"))
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
				ApplicationId: "my_app",
				ApiId:         "orders_api",
				PathTemplate:  "foo/:a",
				Matcher:       matcher("^\\/*foo\\/.+\\/*$"),
			},
		})
		assertPath(t, filter.UnknownPath, unknownPath("my_app"))
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
		assertPath(t, filter.UnknownPath, unknownPath("my_app"))
	})
}
