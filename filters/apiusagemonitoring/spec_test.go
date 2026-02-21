package apiusagemonitoring

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	filter, err := spec.CreateFilter([]any{})
	assert.NoError(t, err)
	assert.Equal(t, filter, &noopFilter{})
}

type pathMatcher struct {
	ApplicationId string
	Tag           string
	ApiId         string
	PathTemplate  string
	Matcher       *string
}

func unknownPath(applicationId string) pathMatcher {
	return pathMatcher{
		PathTemplate:  "{no-match}",
		ApplicationId: applicationId,
		Tag:           "{no-tag}",
		ApiId:         "{unknown}",
		Matcher:       nil,
	}
}

func matcher(matcher string) *string {
	return &matcher
}

func assertPath(t *testing.T, actualPath *pathInfo, expectedPath pathMatcher) {
	assert.Equalf(t, expectedPath.ApiId, actualPath.ApiId, "AppId")
	assert.Equalf(t, expectedPath.ApplicationId, actualPath.ApplicationId, "ApplicationId")
	assert.Equalf(t, expectedPath.Tag, actualPath.Tag, "tag")
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
		assert.Equalf(t, expected.Tag, actual.Tag, "tag[%d]", i)
		assert.Equalf(t, expected.PathTemplate, actual.PathTemplate, "PathTemplate[%d]", i)
		if expected.Matcher != nil {
			assert.Equalf(t, *expected.Matcher, actual.Matcher.String(), "Matcher[%d]", i)
		}
	}
}

func Test_FeatureNotEnabled_TypeNameAndCreatedFilterAreRight(t *testing.T) {
	spec := NewApiUsageMonitoring(false, "", "", "")
	assert.Equal(t, "apiUsageMonitoring", spec.Name())

	filter, err := spec.CreateFilter([]any{})

	assert.NoError(t, err)
	assert.Equal(t, filter, &noopFilter{})
}

func Test_CreateFilter_NoParam_ShouldReturnNoopFilter(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "", "", "")

	filter, err := spec.CreateFilter([]any{})

	assert.Nil(t, err)
	assert.Equal(t, noopFilter{}, filter)
}

func Test_CreateFilter_EmptyString_ShouldReturnNoopFilter(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "", "", "")

	filter, err := spec.CreateFilter([]any{""})

	assert.Nil(t, err)
	assert.Equal(t, noopFilter{}, filter)
}

func Test_CreateFilter_NotAString_ShouldReturnNoopFilter(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "", "", "")

	filter, err := spec.CreateFilter([]any{1234})

	assert.Nil(t, err)
	assert.Equal(t, noopFilter{}, filter)
}

func Test_CreateFilter_NotJson_ShouldReturnNoopFilter(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "", "", "")

	filter, err := spec.CreateFilter([]any{"I am not JSON"})

	assert.Nil(t, err)
	assert.Equal(t, noopFilter{}, filter)
}

func Test_CreateFilter_EmptyJson_ShouldReturnNoopFilter(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "", "", "")

	filter, err := spec.CreateFilter([]any{"{}"})

	assert.Nil(t, err)
	assert.Equal(t, noopFilter{}, filter)
}

func Test_CreateFilter_NoPathTemplate_ShouldReturnNoopFilter(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "", "", "")

	filter, err := spec.CreateFilter([]any{`{
		"application_id": "app",
		"api_id": "api",
		"path_templates": []
	}`})

	assert.Nil(t, err)
	assert.Equal(t, noopFilter{}, filter)
}

func Test_CreateFilter_EmptyPathTemplate_ShouldReturnNoopFilter(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "", "", "")

	filter, err := spec.CreateFilter([]any{`{
		"application_id": "my_app",
		"api_id": "my_api",
		"path_templates": [
			""
		]
	}`})

	assert.Nil(t, err)
	assert.Equal(t, noopFilter{}, filter)
}

func Test_CreateFilter_TypoInPropertyNames_ShouldReturnNoopFilter(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "", "", "")

	// path_template has no `s` and should cause a JSON decoding error.
	filter, err := spec.CreateFilter([]any{`{
		"application_id": "my_app",
		"api_id": "my_api",
		"path_template": [
			""
		]
	}`})

	assert.Nil(t, err)
	assert.Equal(t, noopFilter{}, filter)
}

func Test_CreateFilter_NonParsableParametersShouldBeLoggedAndIgnored(t *testing.T) {
	args := []any{
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
	spec := NewApiUsageMonitoring(true, "", "", "")

	rawFilter, err := spec.CreateFilter(args)
	assert.NoError(t, err)
	assert.NotNil(t, rawFilter)

	filter := rawFilter.(*apiUsageMonitoringFilter)
	assertPaths(t, filter.Paths, []pathMatcher{
		{
			ApplicationId: "my_app",
			Tag:           "{no-tag}",
			ApiId:         "my_api",
			PathTemplate:  "test",
			Matcher:       matcher("^/*test/*$"),
		},
	})
	assertPath(t, filter.UnknownPath, unknownPath("my_app"))
}

func Test_CreateFilter_FullConfigSingleApi(t *testing.T) {
	// Includes paths:
	//   - normal (no variable part)
	//   - with {name} variable paths
	//   - with :name variable paths
	//   - with/without head/trailing slash
	args := []any{`{
		"application_id": "my_app",
        "tag": "staging",
		"api_id": "my_api",
		"path_templates": [
			"foo/orders",
			"foo/orders/:order-id",
			"/foo/order-items/{order-id}:{order-item-id}/"
		]
	}`}
	spec := NewApiUsageMonitoring(true, "", "", "")

	rawFilter, err := spec.CreateFilter(args)
	assert.NoError(t, err)
	assert.NotNil(t, rawFilter)

	filter := rawFilter.(*apiUsageMonitoringFilter)
	assertPaths(t, filter.Paths, []pathMatcher{
		{
			ApplicationId: "my_app",
			Tag:           "staging",
			ApiId:         "my_api",
			PathTemplate:  "foo/orders/{order-id}",
			Matcher:       matcher("^/*foo/+orders/+.+/*$"),
		},
		{
			ApplicationId: "my_app",
			Tag:           "staging",
			ApiId:         "my_api",
			PathTemplate:  "foo/orders",
			Matcher:       matcher("^/*foo/+orders/*$"),
		},
		{
			ApplicationId: "my_app",
			Tag:           "staging",
			ApiId:         "my_api",
			PathTemplate:  "foo/order-items/{order-id}:{order-item-id}",
			Matcher:       matcher("^/*foo/+order-items/+.+:.+/*$"),
		},
	})
	assertPath(t, filter.UnknownPath, unknownPath("my_app"))
}

func Test_CreateFilter_NoApplicationId_ShouldReturnNoopFilter(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "", "", "")

	filter, err := spec.CreateFilter([]any{`{
		"api_id": "api",
		"path_templates": [
			"foo/orders"
		]
	}`})

	assert.Nil(t, err)
	assert.Equal(t, noopFilter{}, filter)

}

func Test_CreateFilter_NoApiId_ShouldReturnNoopFilter(t *testing.T) {
	spec := NewApiUsageMonitoring(true, "", "", "")

	filter, err := spec.CreateFilter([]any{`{
		"application_id": "api",
		"path_templates": [
			"foo/orders"
		]
	}`})

	assert.Nil(t, err)
	assert.Equal(t, noopFilter{}, filter)
}

func Test_CreateFilter_FullConfigMultipleApis(t *testing.T) {
	args := []any{
		`{
			"application_id": "my_app",
			"api_id": "orders_api",
			"path_templates": [
				"foo/orders",
				"foo/orders/:order-id",
				"/foo/order-items/{order-id}:{order-item-id}"
			]
		}`,
		`{
			"application_id": "my_app:tag",
			"api_id": "customers_api",
			"path_templates": [
				"/foo/customers/",
				"/foo/customers/{customer-id}/"
			],
			"client_tracking_pattern": ".+"
		}`,
	}
	spec := NewApiUsageMonitoring(true, "", "", "")

	rawFilter, err := spec.CreateFilter(args)
	assert.NoError(t, err)
	assert.NotNil(t, rawFilter)

	filter := rawFilter.(*apiUsageMonitoringFilter)
	assertPaths(t, filter.Paths, []pathMatcher{
		{
			ApplicationId: "my_app",
			Tag:           "{no-tag}",
			ApiId:         "orders_api",
			PathTemplate:  "foo/orders/{order-id}",
			Matcher:       matcher("^/*foo/+orders/+.+/*$"),
		},
		{
			ApplicationId: "my_app",
			Tag:           "{no-tag}",
			ApiId:         "orders_api",
			PathTemplate:  "foo/orders",
			Matcher:       matcher("^/*foo/+orders/*$"),
		},
		{
			ApplicationId: "my_app",
			Tag:           "{no-tag}",
			ApiId:         "orders_api",
			PathTemplate:  "foo/order-items/{order-id}:{order-item-id}",
			Matcher:       matcher("^/*foo/+order-items/+.+:.+/*$"),
		},
		{
			ApplicationId: "my_app",
			Tag:           "tag",
			ApiId:         "customers_api",
			PathTemplate:  "foo/customers/{customer-id}",
			Matcher:       matcher("^/*foo/+customers/+.+/*$"),
		},
		{
			ApplicationId: "my_app",
			Tag:           "tag",
			ApiId:         "customers_api",
			PathTemplate:  "foo/customers",
			Matcher:       matcher("^/*foo/+customers/*$"),
		},
	})
	assertPath(t, filter.UnknownPath, unknownPath("my_app"))
}

func Test_CreateFilter_FullConfigWithApisWithoutPaths(t *testing.T) {
	// There is a valid object for the 2nd api (customers_api), but no path_templates.
	// Since the end result is that there are a total to observable paths > 0, it should
	// be accepted.
	args := []any{`{
			"application_id": "my_order_app",
			"tag": "staging",
			"api_id": "orders_api",
			"path_templates": [
				"foo/orders",
				"foo/orders/:order-id"
			]
		}`, `{
			"application_id": "my_customer_app",
			"api_id": "customers_api",
			"path_templates": [
			]
		}`}
	spec := NewApiUsageMonitoring(true, "", "", "")

	rawFilter, err := spec.CreateFilter(args)
	assert.NoError(t, err)
	assert.NotNil(t, rawFilter)

	filter := rawFilter.(*apiUsageMonitoringFilter)
	assertPaths(t, filter.Paths, []pathMatcher{
		{
			ApplicationId: "my_order_app",
			Tag:           "staging",
			ApiId:         "orders_api",
			PathTemplate:  "foo/orders/{order-id}",
			Matcher:       matcher("^/*foo/+orders/+.+/*$"),
		},
		{
			ApplicationId: "my_order_app",
			Tag:           "staging",
			ApiId:         "orders_api",
			PathTemplate:  "foo/orders",
			Matcher:       matcher("^/*foo/+orders/*$"),
		},
	})
	assertPath(t, filter.UnknownPath, unknownPath("my_order_app"))
}

func Test_CreateFilter_DuplicatePathTemplatesAreIgnored(t *testing.T) {
	// PathTemplate "foo" and "/foo/" after normalising are the same.
	// That causes an error, even if under different application or API IDs.
	args := []any{`{
		"application_id": "my_app",
		"api_id": "orders_api",
		"path_templates": [
			"foo",
			"/foo/"
		]
	}`}
	spec := NewApiUsageMonitoring(true, "", "", "")

	rawFilter, err := spec.CreateFilter(args)
	assert.NoError(t, err)
	assert.NotNil(t, rawFilter)

	filter := rawFilter.(*apiUsageMonitoringFilter)
	assertPaths(t, filter.Paths, []pathMatcher{
		{
			ApplicationId: "my_app",
			Tag:           "{no-tag}",
			ApiId:         "orders_api",
			PathTemplate:  "foo",
			Matcher:       matcher("^/*foo/*$"),
		},
	})
	assertPath(t, filter.UnknownPath, unknownPath("my_app"))
}

func Test_CreateFilter_DuplicateMatchersAreIgnored(t *testing.T) {
	// PathTemplate "/foo/:a" and "/foo/:b" yield the same RegExp
	args := []any{`{
		"application_id": "my_app",
		"api_id": "orders_api",
		"path_templates": [
			"foo/:a",
			"foo/:b"
		]
	}`}
	spec := NewApiUsageMonitoring(true, "", "", "")

	rawFilter, err := spec.CreateFilter(args)
	assert.NoError(t, err)
	assert.NotNil(t, rawFilter)

	filter := rawFilter.(*apiUsageMonitoringFilter)

	assertPaths(t, filter.Paths, []pathMatcher{
		{
			ApplicationId: "my_app",
			Tag:           "{no-tag}",
			ApiId:         "orders_api",
			PathTemplate:  "foo/{a}",
			Matcher:       matcher("^/*foo/+.+/*$"),
		},
	})
	assertPath(t, filter.UnknownPath, unknownPath("my_app"))
}

type identPathHandler struct{}

func (ph identPathHandler) normalizePathTemplate(path string) string {
	return path
}
func (ph identPathHandler) createPathPattern(path string) string {
	return path
}

func Test_CreateFilter_RegExCompileFailureIgnoresPath(t *testing.T) {
	args := []any{`{
		"application_id": "my_app",
		"api_id": "orders_api",
		"path_templates": [
			"([",
			"orders/"
		]
	}`}

	spec := NewApiUsageMonitoring(true, "", "", "")
	spec.(*apiUsageMonitoringSpec).pathHandler = identPathHandler{}

	rawFilter, err := spec.CreateFilter(args)
	assert.NoError(t, err)
	assert.NotNil(t, rawFilter)

	filter := rawFilter.(*apiUsageMonitoringFilter)
	assert.Equal(t, 1, len(filter.Paths))
	assertPath(t, filter.UnknownPath, unknownPath("my_app"))
}

func Test_NormalizePathTemplate(t *testing.T) {
	args := map[string]struct {
		originalPath         string
		expectedPathTemplate string
	}{
		"without variables": {
			originalPath:         "foo/orders",
			expectedPathTemplate: "foo/orders",
		},
		"with single column variable": {
			originalPath:         "foo/orders/:order-id",
			expectedPathTemplate: "foo/orders/{order-id}",
		},
		"with multiple column variables": {
			originalPath:         "foo/orders/:order-id/order-items/:order-item-id",
			expectedPathTemplate: "foo/orders/{order-id}/order-items/{order-item-id}",
		},

		"with single curly bracket variable": {
			originalPath:         "bar/orders/{order-id}",
			expectedPathTemplate: "bar/orders/{order-id}",
		},
		"with multiple curly bracket variables": {
			originalPath:         "bar/orders/{order-id}/order-items/{order-item-id?}",
			expectedPathTemplate: "bar/orders/{order-id}/order-items/{order-item-id}",
		},
		"with compound key curly bracket variables": {
			originalPath:         "bar/order-items/{order-id}:{order-item-id?}",
			expectedPathTemplate: "bar/order-items/{order-id}:{order-item-id}",
		},

		"with additional leading and trailing slashes": {
			originalPath:         "/bas/customers/",
			expectedPathTemplate: "bas/customers",
		},
		"with multi additional leading, middle, and trailing slashes": {
			originalPath:         "//bas//customers///:customer-id//",
			expectedPathTemplate: "bas/customers/{customer-id}",
		},
	}

	unit := defaultPathHandler{}
	for message, path := range args {
		actualPathTemplate := unit.normalizePathTemplate(path.originalPath)
		assert.Equalf(t, path.expectedPathTemplate, actualPathTemplate, message)
	}
}

func Test_CreatePathPattern(t *testing.T) {
	args := map[string]struct {
		originalPath        string
		expectedPathPattern string
	}{
		"without variables": {
			originalPath:        "foo/orders",
			expectedPathPattern: "^/*foo/+orders/*$",
		},
		"with single column variable": {
			originalPath:        "foo/orders/:order-id",
			expectedPathPattern: "^/*foo/+orders/+.+/*$",
		},
		"with multiple column variables": {
			originalPath:        "foo/orders/:order-id/order-items/:order-item-id",
			expectedPathPattern: "^/*foo/+orders/+.+/+order-items/+.+/*$",
		},

		"with single curly bracket variable": {
			originalPath:        "bar/orders/{order-id}",
			expectedPathPattern: "^/*bar/+orders/+.+/*$",
		},
		"with multiple curly bracket variables": {
			originalPath:        "bar/orders/{order-id}/order-items/{order-item-id?}",
			expectedPathPattern: "^/*bar/+orders/+.+/+order-items/+.*/*$",
		},
		"with compound key curly bracket variables": {
			originalPath:        "bar/order-items/{order-id}:{order-item-id?}",
			expectedPathPattern: "^/*bar/+order-items/+.+:.*/*$",
		},

		"with additional leading and trailing slashes": {
			originalPath:        "/bas/customers",
			expectedPathPattern: "^/*bas/+customers/*$",
		},
		"with multi additional leading, middle, and trailing slashes": {
			originalPath:        "//bas//customers///:customer-id//",
			expectedPathPattern: "^/*bas/+customers/+.+/*$",
		},
		"with escape characters": {
			originalPath:        "/bas/*(cust\\omers)?.id+/",
			expectedPathPattern: "^/*bas/+\\*\\(cust\\\\omers\\)\\?\\.id\\+/*$",
		},
	}

	unit := defaultPathHandler{}
	for message, path := range args {
		actualPathPattern := unit.createPathPattern(path.originalPath)
		assert.Equalf(t, path.expectedPathPattern, actualPathPattern, message)
	}
}

func Benchmark_CreateFilter_FullConfigSingleApiNakadi(b *testing.B) {
	// Includes paths:
	//   - normal (no variable part)
	//   - with {name} variable paths
	//   - with :name variable paths
	//   - with/without head/trailing slash
	spec := NewApiUsageMonitoring(
		true,
		"https://identity.zalando.com/realm",
		"https://identity.zalando.com/managed-id,sub",
		"services[.].*")

	args := []any{`{
		"application_id": "my_app",
		"tag": "staging",
		"api_id": "my_api",
		"path_templates": [
			"/event-types",
			"/event-types/{name}",
			"/event-types/{name}/cursor-distances",
			"/event-types/{name}/cursors-lag",
			"/event-types/{name}/deleted-events",
			"/event-types/{name}/events",
			"/event-types/{name}/partition-count",
			"/event-types/{name}/partitions",
			"/event-types/{name}/partitions/{partition}",
			"/event-types/{name}/schemas",
			"/event-types/{name}/schemas/{version}",
			"/event-types/{name}/shifted-cursors",
			"/event-types/{name}/timelines",
			"/metrics",
			"/registry/enrichment-strategies",
			"/registry/partition-strategies",
			"/settings/admins",
			"/settings/blacklist",
			"/settings/blacklist/{blacklist_type}/{name}",
			"/settings/features",
			"/storages",
			"/storages/default/{id}",
			"/storages/{id}",
			"/subscriptions",
			"/subscriptions/{subscription_id}",
			"/subscriptions/{subscription_id}/cursors",
			"/subscriptions/{subscription_id}/events",
			"/subscriptions/{subscription_id}/stats"
		]
	}`}
	for n := 0; n < b.N; n++ {
		f, err := spec.CreateFilter(args)
		if err != nil {
			b.Fatalf("Failed to run CreateFilter: %v", err)
		}
		filter, ok := f.(*apiUsageMonitoringFilter)
		if !ok || filter == nil {
			b.Fatal("Failed to convert filter")
		}
	}
}
