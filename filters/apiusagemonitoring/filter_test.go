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
	"strings"
	"testing"
)

func Test_Filter_NoPathTemplate(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodGet,
		"https://www.example.org/a/b/c",
		299,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.*.*."
			// no path matching: tracked as unknown
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_NoConfiguration(t *testing.T) {
	testWithFilter(
		t,
		func() (filters.Filter, error) {
			spec := NewApiUsageMonitoring(true, "", "")
			return spec.CreateFilter([]interface{}{})
		},
		http.MethodGet,
		"https://www.example.org/a/b/c",
		200,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.*.*."
			// no path matching: tracked as unknown
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_PathTemplateNoVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		400,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http4xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_PathTemplateWithVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders/1234",
		204,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_PathTemplateWithMultipleVariablePart(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders/1234/order-items/123",
		301,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id/order-items/:order-item-id.*.*."
			if !assert.NotContains(
				t, m.Counters,
				"apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id.http_count",
				"Matched `foo/orders/:order-id` instead of `foo/orders/:order-id`/order-items/:order-item-id") {
			}
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http3xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_PathTemplateFromSecondConfiguredApi(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/customers/loremipsum",
		502,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/customers/:customer-id.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http5xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_StatusCodes1xxAreMonitored(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		100,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http1xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_StatusCodeOver599IsMonitored(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		600,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count": int64(pass),
					//pre + "http*xx_count" <--- no code group tracked
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_StatusCodeUnder100IsMonitoredWithoutHttpStatusCount(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders",
		99,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count": int64(pass),
					//pre + "http*xx_count" <--- no code group tracked
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_NonConfiguredPathTrackedUnderUnknown(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodGet,
		"https://www.example.org/lapin/malin",
		200,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_AllHttpMethodsAreSupported(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		method                 string
		expectedMethodInMetric string
	}{
		{http.MethodGet, "GET"},
		{http.MethodHead, "HEAD"},
		{http.MethodPost, "POST"},
		{http.MethodPut, "PUT"},
		{http.MethodPatch, "PATCH"},
		{http.MethodDelete, "DELETE"},
		{http.MethodConnect, "CONNECT"},
		{http.MethodOptions, "OPTIONS"},
		{http.MethodTrace, "TRACE"},
		{"", "<unknown>"},
		{"foo", "<unknown>"},
	} {
		t.Run(testCase.method, func(t *testing.T) {
			testWithFilter(
				t,
				createFilterForTest,
				testCase.method,
				"https://www.example.org/lapin/malin",
				200,
				func(t *testing.T, pass int, m *metricstest.MockMetrics) {
					pre := fmt.Sprintf(
						"apiUsageMonitoring.custom.<unknown>.<unknown>.%s.<unknown>.*.*.",
						testCase.expectedMethodInMetric)
					assert.Equal(t,
						map[string]int64{
							pre + "http_count":    int64(pass),
							pre + "http2xx_count": int64(pass),
						},
						m.Counters,
					)
					assert.Contains(t, m.Measures, pre+"latency")
				})
		})
	}
}

func Test_Filter_PathTemplateMatchesInternalSlashes(t *testing.T) {
	testWithFilter(
		t,
		createFilterForTest,
		http.MethodPost,
		"https://www.example.org/foo/orders/1/2/3/order-items/123",
		204,
		func(t *testing.T, pass int, m *metricstest.MockMetrics) {
			pre := "apiUsageMonitoring.custom.my_app.my_api.POST.foo/orders/:order-id/order-items/:order-item-id.*.*."
			assert.Equal(t,
				map[string]int64{
					pre + "http_count":    int64(pass),
					pre + "http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, pre+"latency")
		})
}

func Test_Filter_PathTemplateMatchesInternalSlashesTooFollowingVarPart(t *testing.T) {
	filterCreate := func() (filters.Filter, error) {
		args := []interface{}{`{
				"application_id": "my_app",
				"api_id": "my_api",
				"path_templates": [
					"foo/:a",
					"foo/:a/:b",
					"foo/:a/:b/:c"
				]
			}`}
		spec := NewApiUsageMonitoring(true, "", "")
		return spec.CreateFilter(args)
	}
	for _, c := range []struct {
		requestPath                 string
		expectedMatchedPathTemplate string
	}{
		{"foo/1", "foo/:a"},
		{"foo/1/2", "foo/:a/:b"},
		{"foo/1/2/3", "foo/:a/:b/:c"},
		{"foo/1/2/3/4", "foo/:a/:b/:c"},
		{"foo/1/2/3/4/5", "foo/:a/:b/:c"},
	} {
		subTestName := strings.Replace(c.requestPath, "/", "_", -1)
		t.Run(subTestName, func(t *testing.T) {
			testWithFilter(
				t,
				filterCreate,
				http.MethodGet,
				fmt.Sprintf("https://www.example.org/%s", c.requestPath),
				204,
				func(t *testing.T, pass int, m *metricstest.MockMetrics) {
					pre := "apiUsageMonitoring.custom.my_app.my_api.GET." + c.expectedMatchedPathTemplate + ".*.*."
					assert.Equal(t,
						map[string]int64{
							pre + "http_count":    int64(pass),
							pre + "http2xx_count": int64(pass),
						},
						m.Counters,
					)
					assert.Contains(t, m.Measures, pre+"latency")
				})
		})
	}
}

func Test_Filter_ClientMetrics(t *testing.T) {
	for testName, testCase := range map[string]struct {
		realmKeyName          string
		clientIdKeyName       string
		clientTrackingPattern string
		jwtBodyJson           map[string]interface{}
		expectedRealm         string // "" means not expecting client metrics
		expectedClientId      string
	}{
		"Realm and user matches": {
			realmKeyName:          "realm",
			clientIdKeyName:       "client-id",
			clientTrackingPattern: ".*",
			jwtBodyJson:           map[string]interface{}{"realm": "users", "client-id": "joe"},
			expectedRealm:         "users",
			expectedClientId:      "joe",
		},
	} {
		t.Run(testName, func(t *testing.T) {
			jwt, ok := buildFakeJwtWithBody(t, testCase.jwtBodyJson)
			if !ok {
				return
			}
			conf := testWithFilterConf{
				header: map[string][]string{
					authorizationHeaderName: {"Bearer " + jwt},
				},
				filterCreate: func() (filters.Filter, error) {
					args := []interface{}{`{
					"application_id": "my_app",
					"api_id": "my_api",
				  	"path_templates": [
						"foo/orders"
					],
					"client_tracking_pattern": "` + testCase.clientTrackingPattern + `"
				}`}
					spec := NewApiUsageMonitoring(true, testCase.realmKeyName, testCase.clientIdKeyName)
					return spec.CreateFilter(args)
				},
			}
			previousLatencySum := float64(0)
			testWithFilterC(t, conf, func(t *testing.T, pass int, m *metricstest.MockMetrics) {
				pre := fmt.Sprintf(
					"apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.%s.%s.",
					testCase.expectedRealm,
					testCase.expectedClientId)
				if testCase.expectedRealm == "" {
					assertNoMetricsWithSuffixes(t, clientMetricsSuffix, m)
					return
				}
				if assert.Contains(t, m.FloatCounters, pre+"latency_sum") {
					currentLatencySum := m.FloatCounters[pre+"latency_sum"]
					assert.Conditionf(t,
						func() (success bool) { return currentLatencySum > previousLatencySum },
						"Current latency sum is not higher than the previous recorded one (%d to %d)",
						previousLatencySum, currentLatencySum)
				}
			})
		})
	}
}

func buildFakeJwtWithBody(t *testing.T, jwtBodyJson map[string]interface{}) (string, bool) {
	jwtBodyBytes, err := json.Marshal(jwtBodyJson)
	if !assert.NoError(t, err) {
		return "", false
	}
	jwtBody := base64.RawURLEncoding.EncodeToString(jwtBodyBytes)
	jwt := fmt.Sprintf("<No Header>.%s.< No Verify Signature>", jwtBody)
	return jwt, true
}

// todo: Fix test or remove
//func Test_getRealmAndClientFromContext(t *testing.T) {
//	for _, testCase := range []struct {
//		jwt              string
//		expectedRealm    string
//		expectedClientId string
//	}{
//		// use https://jwt.io/ to decode/encore JWT base64 strings
//		{
//			// {	"foo": "abc",
//			// 		"bar": "xyz"	}
//			jwt:              "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJhYmMiLCJiYXIiOiJ4eXoifQ.mvySdTnLnbBAL__hDrk9Q7t9l1vCwrc1U5wttyqu1Ng",
//			expectedRealm:    "",
//			expectedClientId: "",
//		},
//		{
//			// {	"realm": "abc",
//			// 		"bar":   "xyz"	}
//			jwt:              "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJyZWFsbSI6ImFiYyIsImJhciI6Inh5eiJ9.8IkCEEFeJ3SqOwEQ27ru5uwck6GjWttbI7RSCiu_T2E",
//			expectedRealm:    "abc",
//			expectedClientId: "",
//		},
//		{
//			// {	"realm":   "abc",
//			// 		"user_id": "me/myself/I"	}
//			jwt:              "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJyZWFsbSI6ImFiYyIsInVzZXJfaWQiOiJtZS9teXNlbGYvSSJ9.jA2HojPdk5etOskayRmmI-GRw_Rqge_unoW6lpUqHBE",
//			expectedRealm:    "abc",
//			expectedClientId: "me/myself/I",
//		},
//		{
//			// {	"foo":     "abc",
//			// 		"user_id": "me/myself/I"	}
//			jwt:              "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJhYmMiLCJ1c2VyX2lkIjoibWUvbXlzZWxmL0kifQ.Ii8cpBP8l3evKEtbRl_RRsi23aF7l1MjfahPGTOh81I",
//			expectedRealm:    "",
//			expectedClientId: "me/myself/I",
//		},
//		{
//			// {	"realm":   42,
//			// 		"user_id": true	}
//			jwt:              "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJyZWFsbSI6NDIsInVzZXJfaWQiOnRydWV9.SM2Ym9kD9j7dMkc-hOj90bnAtGQaRKz-2GDDKva4O5A",
//			expectedRealm:    "",
//			expectedClientId: "",
//		},
//	} {
//		path := &pathInfo{
//			ClientTracking: &clientTrackingInfo{
//				RealmKey:    "realm",
//				ClientIdKey: "user_id",
//			},
//		}
//		request, err := http.NewRequest(http.MethodGet, "http://example.com/", nil)
//		if !assert.NoError(t, err) {
//			return
//		}
//		request.Header.Add(authorizationHeaderName, authorizationHeaderPrefix+testCase.jwt)
//
//		filterContext := &filtertest.Context{
//			FRequest: request,
//		}
//		realm, clientId := getRealmAndClientFromContext(filterContext, path)
//
//		assert.Equal(t, testCase.expectedRealm, realm)
//		assert.Equal(t, testCase.expectedClientId, clientId)
//	}
//}

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
	url          *string
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
	if conf.url == nil {
		url = "https://www.example.com/foo/orders"
	} else {
		url = *conf.url
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
			//t.Parallel() // todo: Try this (potentially increment the default pass count to test parallelism)
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

var clientMetricsSuffix = []string{metricLatencySum}

func assertNoMetricsWithSuffixes(t *testing.T, unexpectedSuffixes []string, metrics *metricstest.MockMetrics) bool {
	success := true
	for _, suffix := range unexpectedSuffixes {
		for key := range metrics.Counters {
			if strings.HasSuffix(key, "."+suffix) {
				assert.Failf(t, "Counter with key %q is not expected", key)
				success = false
			}
		}
		for key := range metrics.FloatCounters {
			if strings.HasSuffix(key, "."+suffix) {
				assert.Failf(t, "FloatCounter with key %q is not expected", key)
				success = false
			}
		}
		for key := range metrics.Measures {
			if strings.HasSuffix(key, "."+suffix) {
				assert.Failf(t, "Measure with key %q is not expected", key)
				success = false
			}
		}
	}
	return success
}
