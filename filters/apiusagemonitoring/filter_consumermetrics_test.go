package apiusagemonitoring

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics/metricstest"
	"net/http"
	"testing"
)

type clientMetricsTest struct {
	url                   string
	realmKeyName          string
	clientIdKeyName       string
	clientTrackingPattern string
	header                http.Header

	expectedEndpointMetricPrefix string
	expectedConsumerMetricPrefix string
}

var (
	clientTrackingPatternJustSomeUsers = `users\.(?:joe|sabine)`
	headerUsersJoe                     = http.Header{
		authorizationHeaderName: {
			"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
				"realm":     "users",
				"client-id": "joe",
			}),
		},
	}
)

func Test_Filter_ClientMetrics_NonConfiguredPath(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientIdKeyName:              "client-id",
		clientTrackingPattern:        ".*",
		header:                       headerUsersJoe,
		url:                          "https://www.example.com/non/configured/path/template",
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.<unknown>.<unknown>.*.*.users.joe.",
	})
}

func Test_Filter_ClientMetrics_MatchAll(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientIdKeyName:              "client-id",
		clientTrackingPattern:        ".*",
		header:                       headerUsersJoe,
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.users.joe.",
	})
}

func Test_Filter_ClientMetrics_Realm1User1(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientIdKeyName:              "client-id",
		clientTrackingPattern:        clientTrackingPatternJustSomeUsers,
		header:                       headerUsersJoe,
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.users.joe.",
	})
}

func Test_Filter_ClientMetrics_Realm1User0(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm":     "users",
					"client-id": "berta",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.users.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_Realm0User1(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm":     "nobodies",
					"client-id": "sabine",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.nobodies.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_Realm0User0(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm":     "nobodies",
					"client-id": "david",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.nobodies.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_AuthDoesNotHaveBearerPrefix(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				/* no bearer */ buildFakeJwtWithBody(map[string]interface{}{
					"realm":     "users",
					"client-id": "joe",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_NoAuthHeader(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			/* no Authorization header */
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_JWTIsNot3DotSeparatedString(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + "foo",
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_JWTIsNotBase64Encoded(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + "there&is.no&way.this&is&base64",
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_JWTBodyIsNoJSON(t *testing.T) {
	body := base64.RawURLEncoding.EncodeToString([]byte("I am sadly no JSON :'("))
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + "header." + body + ".signature",
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_JWTBodyHasNoRealm(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					// no realm
					"client-id": "david",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_JWTBodyHasNoClientId_ShouldTrackRealm(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm": "users",
					// no client ID
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.*.*.users.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_NoFlagRealmKeyName(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "", // no realm key name CLI flag
		clientIdKeyName:              "client-id",
		clientTrackingPattern:        clientTrackingPatternJustSomeUsers,
		header:                       headerUsersJoe,
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "", // expecting no metrics
	})
}

func Test_Filter_ClientMetrics_NoFlagClientIdKeyName(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientIdKeyName:              "", // no client ID key name CLI flag
		clientTrackingPattern:        clientTrackingPatternJustSomeUsers,
		header:                       headerUsersJoe,
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedConsumerMetricPrefix: "", // expecting no metrics
	})
}

// todo: Confirm the behaviour in this case
//func Test_Filter_ClientMetrics_NoClientTrackingPatternInRouteFilterJSON(t *testing.T) {
//	testClientMetrics(t, clientMetricsTest{
//		realmKeyName:          "realm",
//		clientIdKeyName:       "client-id",
//		clientTrackingPattern: "", // no client tracking in route's filter configuration
//		header: http.Header{
//			authorizationHeaderName: {
//				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
//					"realm":     "users",
//					"client-id": "joe",
//				}),
//			},
//		},
//		expectingNoClientBasedMetrics: true,
//	})
//}

func testClientMetrics(
	t *testing.T,
	testCase clientMetricsTest,
) {
	conf := testWithFilterConf{
		url:    testCase.url,
		header: testCase.header,
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
		//
		// Assert consumer metrics
		//
		if testCase.expectedConsumerMetricPrefix == "" {
			assertNoMetricsWithSuffixes(t, consumerMetricsSuffix, m)
			return
		} else {
			if assert.Contains(t, m.FloatCounters, testCase.expectedConsumerMetricPrefix+"latency_sum") {
				currentLatencySum := m.FloatCounters[testCase.expectedConsumerMetricPrefix+"latency_sum"]
				assert.Conditionf(t,
					func() (success bool) { return currentLatencySum > previousLatencySum },
					"Current latency sum is not higher than the previous recorded one (%d to %d)",
					previousLatencySum, currentLatencySum)
			}
		}
		//
		// Assert endpoint metrics
		//
		if testCase.expectedEndpointMetricPrefix == "" {
			assertNoMetricsWithSuffixes(t, endpointMetricsSuffix, m)
		} else {
			assert.Equal(t,
				map[string]int64{
					testCase.expectedEndpointMetricPrefix + "http_count":    int64(pass),
					testCase.expectedEndpointMetricPrefix + "http2xx_count": int64(pass),
				},
				m.Counters,
			)
			assert.Contains(t, m.Measures, testCase.expectedEndpointMetricPrefix+"latency")
		}
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
