package apiusagemonitoring

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics/metricstest"
	"net/http"
	"strings"
	"testing"
)

type clientMetricsTest struct {
	realmKeyName          string
	clientIdKeyName       string
	clientTrackingPattern string
	header                http.Header
	expectingNoMetrics    bool
	expectedRealm         string
	expectedClientId      string
}

var clientTrackingPatternJustSomeUsers = strings.Replace(
	`^users\.(?:joe|sabine)$`,
	`\`, `\\`, -1)

func Test_Filter_ClientMetrics_Realm1User1(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientIdKeyName:       "client-id",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm":     "users",
					"client-id": "joe",
				}),
			},
		},
		expectedRealm:    "users",
		expectedClientId: "joe",
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

// todo: No Authorization header ==> <unknown>.<unknown>
// todo: JWT is not parsable ==> <unknown>.<unknown>
// todo: JWT has no realm ==> <unknown>.<unknown>
// todo: JWT has no user_id ==> realm.<unknown>
// todo: 1st CLI flag not provided ==> No tracking
// todo: 2nd CLI flag not provided ==> No tracking
// todo: No client_tracking_pattern set (or empty) in filter param JSON ==> No tracking

func testClientMetrics(
	t *testing.T,
	testCase clientMetricsTest,
) {
	conf := testWithFilterConf{
		header: testCase.header,
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
		if testCase.expectingNoMetrics {
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
