package apiusagemonitoring

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
	"net/http"
	"testing"
	"time"
)

var defaultArgs = []interface{}{`{
		"application_id": "my_app",
		"api_id": "my_api",
	  	"path_templates": [
			"foo/orders",
			"foo/orders/:order-id",
			"foo/orders/:order-id/order-items/{order-item-id}",
			"/foo/customers/",
			"/foo/customers/{customer-id}/"
		]
	}`}

func createFilterForTest() (filters.Filter, error) {
	spec := NewApiUsageMonitoring(true, "", "")
	return spec.CreateFilter(defaultArgs)
}

func testWithFilter(
	t *testing.T,
	filterCreate func() (filters.Filter, error),
	method string,
	url string,
	resStatus int,
	expect func(t *testing.T, pass int, m *metricstest.MockMetrics),
) {
	filter, err := filterCreate()
	assert.NoError(t, err)
	assert.NotNil(t, filter)

	metricsMock := &metricstest.MockMetrics{
		Prefix: "apiUsageMonitoring.custom.",
	}

	// performing multiple passes to make sure that the caching of metrics keys
	// does not fail.
	for pass := 1; pass <= 3; pass++ {
		t.Run(fmt.Sprintf("pass %d", pass), func(t *testing.T) {
			req, err := http.NewRequest(method, url, nil)
			if method == "" {
				req.Method = ""
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx := &filtertest.Context{
				FRequest: req,
				FResponse: &http.Response{
					StatusCode: resStatus,
				},
				FStateBag: make(map[string]interface{}),
				FMetrics:  metricsMock,
			}
			filter.Request(ctx)
			filter.Response(ctx)

			expect(
				t,
				pass,
				metricsMock,
			)
		})
	}
}

type testWithFilterConf struct {
	passCount    *int
	filterCreate func() (filters.Filter, error)
	method       *string
	url          string
	resStatus    *int
	header       http.Header
}

func testWithFilterC(
	t *testing.T,
	conf testWithFilterConf,
	expect func(t *testing.T, pass int, m *metricstest.MockMetrics),
) {
	var (
		passCount    int
		filterCreate func() (filters.Filter, error)
		method       string
		url          string
		resStatus    int
	)
	if conf.passCount == nil {
		passCount = 3
	} else {
		passCount = *conf.passCount
	}
	if conf.filterCreate == nil {
		filterCreate = createFilterForTest
	} else {
		filterCreate = conf.filterCreate
	}
	if conf.method == nil {
		method = http.MethodGet
	} else {
		method = *conf.method
	}
	if conf.url == "" {
		url = "https://www.example.com/foo/orders"
	} else {
		url = conf.url
	}
	if conf.resStatus == nil {
		resStatus = http.StatusOK
	} else {
		resStatus = *conf.resStatus
	}

	// Create Filter
	filter, err := filterCreate()
	assert.NoError(t, err)
	assert.NotNil(t, filter)

	// Create Metrics Mock
	metricsMock := &metricstest.MockMetrics{
		Prefix: "apiUsageMonitoring.custom.",
	}

	// Performing multiple passes to make sure that the caching of metrics keys
	// does not fail.
	for pass := 1; pass <= passCount; pass++ {
		t.Run(fmt.Sprintf("pass %d", pass), func(t *testing.T) {
			req, err := http.NewRequest(method, url, nil)
			if !assert.NoError(t, err) {
				return
			}
			if method == "" {
				req.Method = ""
			}
			req.Header = conf.header
			ctx := &filtertest.Context{
				FRequest: req,
				FResponse: &http.Response{
					StatusCode: resStatus,
				},
				FStateBag: make(map[string]interface{}),
				FMetrics:  metricsMock,
			}
			filter.Request(ctx)
			filter.Response(ctx)

			expect(
				t,
				pass,
				metricsMock,
			)
		})
		if t.Failed() {
			return
		}
	}
}

func testClientMetrics(
	t *testing.T,
	testCase clientMetricsTest,
) {
	conf := testWithFilterConf{
		url:       testCase.url,
		header:    testCase.header,
		filterCreate: func() (filters.Filter, error) {
			filterConf := map[string]interface{}{
				"application_id": "my_app",
				"api_id":         "my_api",
				"path_templates": []string{
					"foo/orders",
				},
			}
			if testCase.clientTrackingPattern != "" {
				filterConf["client_tracking_pattern"] = testCase.clientTrackingPattern
			}
			js, err := json.Marshal(filterConf)
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			args := []interface{}{string(js)}
			spec := NewApiUsageMonitoring(true, testCase.realmKeyName, testCase.clientIdKeyName)
			return spec.CreateFilter(args)
		},
	}
	previousLatencySum := float64(0)

	testWithFilterC(t, conf, func(t *testing.T, pass int, m *metricstest.MockMetrics) {

		expectedCounters := make([]string, 0)
		expectedFloatCounters := make([]string, 0)
		expectedMeasures := make([]string, 0)

		//
		// Assert client metrics
		//
		if testCase.expectedClientMetricPrefix != "" {

			m.WithCounters(func(counters map[string]int64) {
				httpCountKey := testCase.expectedClientMetricPrefix + "http_count"
				expectedCounters = append(expectedCounters, httpCountKey)
				if assert.Contains(t, counters, httpCountKey) {
					v := counters[httpCountKey]
					assert.Equal(t, int64(pass), v)
				}

				httpClassCountKey := testCase.expectedClientMetricPrefix + "http2xx_count"
				expectedCounters = append(expectedCounters, httpClassCountKey)
				if assert.Contains(t, counters, httpClassCountKey) {
					v := counters[httpClassCountKey]
					assert.Equal(t, int64(pass), v)
				}
			})

			m.WithFloatCounters(func(floatCounters map[string]float64) {
				latencySumKey := testCase.expectedClientMetricPrefix + "latency_sum"
				expectedFloatCounters = append(expectedFloatCounters, latencySumKey)
				if assert.Contains(t, floatCounters, latencySumKey) {
					v := floatCounters[latencySumKey]
					assert.Conditionf(t,
						func() bool {
							return v > previousLatencySum
						}, "current client latency sum is not higher than the previous recorded one (%f to %f)",
						previousLatencySum, v)
				}
			})

		}

		//
		// Assert endpoint metrics
		//
		if testCase.expectedEndpointMetricPrefix != "" {

			m.WithCounters(func(counters map[string]int64) {
				httpCountKey := testCase.expectedEndpointMetricPrefix + "http_count"
				expectedCounters = append(expectedCounters, httpCountKey)
				if assert.Contains(t, counters, httpCountKey) {
					v := counters[httpCountKey]
					assert.Equal(t, int64(pass), v)
				}

				httpCountClassKey := testCase.expectedEndpointMetricPrefix + "http2xx_count"
				expectedCounters = append(expectedCounters, httpCountClassKey)
				if assert.Contains(t, counters, httpCountClassKey) {
					v := counters[httpCountClassKey]
					assert.Equal(t, int64(pass), v)
				}
			})

			m.WithMeasures(func(measures map[string][]time.Duration) {
				latencyKey := testCase.expectedEndpointMetricPrefix + "latency"
				expectedMeasures = append(expectedMeasures, latencyKey)
				assert.Contains(t, measures, latencyKey)
			})
		}

		m.WithCounters(func(counters map[string]int64) {
			actualCounters := make([]string, 0, len(counters))
			for k := range counters {
				actualCounters = append(actualCounters, k)
			}
			assert.ElementsMatch(t, expectedCounters, actualCounters)
		})

		m.WithFloatCounters(func(floatCounters map[string]float64) {
			actualFloatCounters := make([]string, 0, len(floatCounters))
			for k := range floatCounters {
				actualFloatCounters = append(actualFloatCounters, k)
			}
			assert.ElementsMatch(t, expectedFloatCounters, actualFloatCounters)
		})

		m.WithMeasures(func(measures map[string][]time.Duration) {
			actualMeasures := make([]string, 0, len(measures))
			for k := range measures {
				actualMeasures = append(actualMeasures, k)
			}
			assert.ElementsMatch(t, expectedMeasures, actualMeasures)
		})
	})
}

func buildFakeJwtWithBody(jwtBodyJson map[string]interface{}) string {
	jwtBodyBytes, err := json.Marshal(jwtBodyJson)
	if err != nil {
		panic(err)
	}
	jwtBody := base64.RawURLEncoding.EncodeToString(jwtBodyBytes)
	jwt := fmt.Sprintf("<No Header>.%s.< No Verify Signature>", jwtBody)
	return jwt
}
