package apimonitoring

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
	"net/http"
	"strings"
	"testing"
	"time"
)

func Test_pathPatternToRegEx(t *testing.T) {
	cases := map[string]string{
		"/orders/{orderId}/orderItems/{orderItemId}": `\/orders\/[^\/]+\/orderItems\/[^\/]+[\/]*`,
	}
	for input, expected := range cases {
		t.Run(fmt.Sprintf("pathPatternToRegEx with %s", input), func(t *testing.T) {
			actual, err := pathPatternToRegEx(input)
			if err != nil {
				t.Errorf("pathPatternToRegEx with %q generated error: %v", input, err)
			}
			if actual.String() != expected {
				t.Errorf("pathPatternToRegEx:\n\tinput:    %s\n\texpected: %s\n\tactual:   %s", input, expected, actual)
			}
		})
	}
}

func Test_splitRawArg_integer(t *testing.T) {
	name, value, err := splitRawArg(123)
	assert.NotNil(t, err)
	assert.Equal(t, "expecting string parameters, received 123", err.Error())
	assert.Equal(t, "", name)
	assert.Equal(t, "", value)
}

func Test_splitRawArg_string_empty(t *testing.T) {
	name, value, err := splitRawArg("")
	assert.NotNil(t, err)
	assert.Equal(t, "expecting non empty string", err.Error())
	assert.Equal(t, "", name)
	assert.Equal(t, "", value)
}

func Test_splitRawArg_string_noSplitter(t *testing.T) {
	name, value, err := splitRawArg("asd")
	assert.NotNil(t, err)
	assert.Equal(t, "expecting ':' to split the name from the value: asd", err.Error())
	assert.Equal(t, "", name)
	assert.Equal(t, "", value)
}

func Test_splitRawArg_string_emptyName(t *testing.T) {
	name, value, err := splitRawArg(":/foo/bar")
	assert.NotNil(t, err)
	assert.Equal(t, "parameter with no name (starts with splitter ':'): :/foo/bar", err.Error())
	assert.Equal(t, "", name)
	assert.Equal(t, "", value)
}

func Test_splitRawArg_string_emptyValue(t *testing.T) {
	name, value, err := splitRawArg("pathpat:")
	assert.NotNil(t, err)
	assert.Equal(t, "parameter \"pathpat\" does not have any value: pathpat:", err.Error())
	assert.Equal(t, "pathpat", name)
	assert.Equal(t, "", value)
}

func Test_splitRawArg_string_valid(t *testing.T) {
	name, value, err := splitRawArg("pathpat: /foo/bar")
	assert.Nil(t, err)
	assert.Equal(t, "pathpat", name)
	assert.Equal(t, "/foo/bar", value)
}

func Test_CreateFilter_Scenario_001(t *testing.T) {
	var filter filters.Filter

	spec := apiMonitoringFilterSpec{}
	args := []interface{}{
		"ApiId: asd123",
		"PathPat: orders/{orderId}",
		"PathPat: orders/{orderId}/order_item/{orderItemId}",
	}

	filter, err := spec.CreateFilter(args)
	assert.Nil(t, err)

	assert.IsType(t, &apiMonitoringFilter{}, filter)
	tf := filter.(*apiMonitoringFilter)

	assert.Equal(t, "asd123", tf.apiId)

	expectedPathPattersKeys := []string{
		"orders/{orderId}",
		"orders/{orderId}/order_item/{orderItemId}",
	}
	assert.Len(t, tf.pathPatterns, len(expectedPathPattersKeys))
	for _, expectedKey := range expectedPathPattersKeys {
		_, ok := tf.pathPatterns[expectedKey]
		if !ok {
			t.Errorf("pathPattern not found for %s", expectedKey)
		}
	}
	assert.NotNil(t, filter)

	t.Run("Handle any call", func(t *testing.T) {
		bodyBuffer := bytes.NewBufferString("abcdefghijklmnopqrstuvwxyzäöüß")
		req, err := http.NewRequest("GET", "https://www.example.org/foo/a/b/c", bodyBuffer)
		if err != nil {
			t.Error(err)
		}
		assertMetricsKey := func(key string) {
			assert.True(t,
				strings.HasPrefix(key, "asd123./foo/a/b/c.GET"),
				"unexpected metrics key prefix: %s", key)
		}
		ctx := &filtertest.Context{
			FRequest: req,
			FResponse: &http.Response{
				StatusCode: 200,
			},
			FStateBag: make(map[string]interface{}),
			FMetrics: &metricstest.MockMetrics{
				IncCounterFn:   func(key string) { assertMetricsKey(key) },
				MeasureSinceFn: func(key string, start time.Time) { assertMetricsKey(key) },
				IncCounterByFn: func(key string, value int64) { assertMetricsKey(key) },
			},
		}
		filter.Request(ctx)
		filter.Response(ctx)
	})
}
