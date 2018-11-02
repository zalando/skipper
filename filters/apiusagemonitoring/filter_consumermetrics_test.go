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
	url                           string
	realmKeyName                  string
	clientIdKeyName               string
	clientTrackingPattern         string
	header                        http.Header
	expectingNoClientBasedMetrics bool

	expectedEndpointMetricPrefix string
	expectedConsumerMetricPrefix string

	expectedRealm                 string
	expectedClientId              string
	expectedPathTemplate          string
	expectedApplicationId         string
	expectedApiId                 string
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
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: ".*",
		header:                headerUsersJoe,
		url:                   "https://www.example.com/non/configured/path/template",
		expectedApplicationId: unknownElementPlaceholder,
		expectedApiId:         unknownElementPlaceholder,
		expectedPathTemplate:  unknownElementPlaceholder,
		expectedRealm:         "users",
		expectedClientId:      "joe",
	})
}

func Test_Filter_ClientMetrics_MatchAll(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: ".*",
		header:                headerUsersJoe,
		expectedRealm:         "users",
		expectedClientId:      "joe",
	})
}

func Test_Filter_ClientMetrics_Realm1User1(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header:                headerUsersJoe,
		expectedRealm:         "users",
		expectedClientId:      "joe",
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
		expectedRealm:    "users",
		expectedClientId: unknownElementPlaceholder,
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
		expectedRealm:    "nobodies",
		expectedClientId: unknownElementPlaceholder,
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
		expectedRealm:    "nobodies",
		expectedClientId: unknownElementPlaceholder,
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
		expectedRealm:    unknownElementPlaceholder,
		expectedClientId: unknownElementPlaceholder,
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
		expectedRealm:    unknownElementPlaceholder,
		expectedClientId: unknownElementPlaceholder,
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
		expectedRealm:    unknownElementPlaceholder,
		expectedClientId: unknownElementPlaceholder,
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
		expectedRealm:    unknownElementPlaceholder,
		expectedClientId: unknownElementPlaceholder,
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
		expectedRealm:    unknownElementPlaceholder,
		expectedClientId: unknownElementPlaceholder,
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
		expectedRealm:    unknownElementPlaceholder,
		expectedClientId: unknownElementPlaceholder,
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
		expectedRealm:    "users",
		expectedClientId: unknownElementPlaceholder,
	})
}

func Test_Filter_ClientMetrics_NoFlagRealmKeyName(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                  "", // no realm key name CLI flag
		clientIdKeyName:               "client-id",
		clientTrackingPattern:         clientTrackingPatternJustSomeUsers,
		header:                        headerUsersJoe,
		expectingNoClientBasedMetrics: true,
	})
}

func Test_Filter_ClientMetrics_NoFlagClientIdKeyName(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                  "realm",
		clientIdKeyName:               "", // no client ID key name CLI flag
		clientTrackingPattern:         clientTrackingPatternJustSomeUsers,
		header:                        headerUsersJoe,
		expectingNoClientBasedMetrics: true,
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
		if testCase.expectedPathTemplate == "" {
			testCase.expectedPathTemplate = `foo/orders`
		}
		if testCase.expectedApplicationId == "" {
			testCase.expectedApplicationId = "my_app"
		}
		if testCase.expectedApiId == "" {
			testCase.expectedApiId = "my_api"
		}

		//
		// Assert consumer metrics
		//

		consumerMetricsPrefix := fmt.Sprintf(
			"apiUsageMonitoring.custom.%s.%s.*.*.%s.%s.",
			testCase.expectedApplicationId,
			testCase.expectedApiId,
			testCase.expectedRealm,
			testCase.expectedClientId,
		)
		if testCase.expectingNoClientBasedMetrics {
			assertNoMetricsWithSuffixes(t, clientMetricsSuffix, m)
			return
		} else {
			if assert.Contains(t, m.FloatCounters, consumerMetricsPrefix+"latency_sum") {
				currentLatencySum := m.FloatCounters[consumerMetricsPrefix+"latency_sum"]
				assert.Conditionf(t,
					func() (success bool) { return currentLatencySum > previousLatencySum },
					"Current latency sum is not higher than the previous recorded one (%d to %d)",
					previousLatencySum, currentLatencySum)
			}
		}

		//
		// Assert endpoint metrics
		//

		endpointMetricsPrefix := fmt.Sprintf(
			"apiUsageMonitoring.custom.%s.%s.GET.%s.*.*.",
			testCase.expectedApplicationId,
			testCase.expectedApiId,
			testCase.expectedPathTemplate,
		)
		assert.Equal(t,
			map[string]int64{
				endpointMetricsPrefix + "http_count":    int64(pass),
				endpointMetricsPrefix + "http2xx_count": int64(pass),
			},
			m.Counters,
		)
		assert.Contains(t, m.Measures, endpointMetricsPrefix+"latency")
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
