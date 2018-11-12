package apiusagemonitoring

import (
	"encoding/base64"
	"net/http"
	"testing"
)

type clientMetricsTest struct {
	url                   string
	realmKeyName          string
	clientKeyName         string
	clientTrackingPattern *string
	header                http.Header

	expectedEndpointMetricPrefix string
	expectedClientMetricPrefix   string
	defaultClientTrackingPattern string
}

var (
	clientTrackingPatternJustSomeUsers = s(`users\.(joe|sabine)`)
	headerUsersJoe                     = http.Header{
		authorizationHeaderName: {
			"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
				"realm":  "users",
				"client": "joe",
			}),
		},
	}
)

func s(str string) *string {
	return &str
}

func Test_Filter_ClientMetrics_ClientTrackingPatternDoesNotCompile(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientKeyName:                "client",
		clientTrackingPattern:        s("(["),
		header:                       headerUsersJoe,
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "", // expecting no metrics
	})
}

func Test_Filter_ClientMetrics_NoMatchingPath_RealmIsKnown(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientKeyName:                "client",
		clientTrackingPattern:        s(".*"),
		header:                       headerUsersJoe,
		url:                          "https://www.example.com/non/configured/path/template",
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.<unknown>.<unknown>.*.*.users.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_NoMatchingPath_RealmIsUnknown(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "will not match",
		clientKeyName:                "client",
		clientTrackingPattern:        s(".*"),
		header:                       headerUsersJoe,
		url:                          "https://www.example.com/non/configured/path/template",
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.<unknown>.<unknown>.GET.<unknown>.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.<unknown>.<unknown>.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_MatchAll(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientKeyName:                "client",
		clientTrackingPattern:        s(".*"),
		header:                       headerUsersJoe,
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.users.joe.",
	})
}

func Test_Filter_ClientMetrics_MatchOneOfClientKeyName(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client,sub",
		clientTrackingPattern: s(".*"),
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm": "services",
					"sub":   "payments",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.services.payments.",
	})
}

func Test_Filter_ClientMetrics_MatchOneOfClientKeyName_UseFirstMatching(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client,sub",
		clientTrackingPattern: s(".*"),
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm":  "services",
					"sub":    "payments",
					"client": "but_I_should_come_first",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.services.but_I_should_come_first.",
	})
}

func Test_Filter_ClientMetrics_Realm1User1(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientKeyName:                "client",
		clientTrackingPattern:        clientTrackingPatternJustSomeUsers,
		header:                       headerUsersJoe,
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.users.joe.",
	})
}

func Test_Filter_ClientMetrics_Realm1User0(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm":  "users",
					"client": "berta",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.users.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_Realm0User1(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm":  "nobodies",
					"client": "sabine",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.nobodies.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_Realm0User0(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm":  "nobodies",
					"client": "david",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.nobodies.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_AuthDoesNotHaveBearerPrefix(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				/* no bearer */ buildFakeJwtWithBody(map[string]interface{}{
					"realm":  "users",
					"client": "joe",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_NoAuthHeader(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header:                http.Header{
			/* no Authorization header */
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_JWTIsNot3DotSeparatedString(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + "foo",
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_JWTIsNotBase64Encoded(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + "there&is.no&way.this&is&base64",
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_JWTBodyIsNoJSON(t *testing.T) {
	body := base64.RawURLEncoding.EncodeToString([]byte("I am sadly no JSON :'("))
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + "header." + body + ".signature",
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_JWTBodyHasNoRealm(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client",
		clientTrackingPattern: clientTrackingPatternJustSomeUsers,
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					// no realm
					"client": "david",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.<unknown>.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_JWTBodyHasNoClient_ShouldTrackRealm(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:          "realm",
		clientKeyName:         "client",
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
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.users.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_NoFlagRealmKeyName(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "", // no realm key name CLI flag
		clientKeyName:                "client",
		clientTrackingPattern:        clientTrackingPatternJustSomeUsers,
		header:                       headerUsersJoe,
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "", // expecting no metrics
	})
}

func Test_Filter_ClientMetrics_NoFlagClientKeyName(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientKeyName:                "", // no client ID key name CLI flag
		clientTrackingPattern:        clientTrackingPatternJustSomeUsers,
		header:                       headerUsersJoe,
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "", // expecting no metrics
	})
}

func Test_Filter_ClientMetrics_DefaultClientTrackingPattern_NoClientTrackingPatternInRouteFilterJSON_User(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientKeyName:                "client",
		defaultClientTrackingPattern: "services\\..*",
		clientTrackingPattern:        nil, // no client tracking in route's filter configuration
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm":  "users",
					"client": "joe",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.users.<unknown>.",
	})
}

func Test_Filter_ClientMetrics_DefaultClientTrackingPattern_NoClientTrackingPatternInRouteFilterJSON_Service(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientKeyName:                "client",
		defaultClientTrackingPattern: "services\\..*",
		clientTrackingPattern:        nil, // no client tracking in route's filter configuration
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm":  "services",
					"client": "my_app",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "apiUsageMonitoring.custom.my_app.my_api.*.*.services.my_app.",
	})
}

func Test_Filter_ClientMetrics_EmptyClientTrackingPatternInRouteFilterJSON(t *testing.T) {
	testClientMetrics(t, clientMetricsTest{
		realmKeyName:                 "realm",
		clientKeyName:                "client",
		defaultClientTrackingPattern: "services\\..*",
		clientTrackingPattern:        s(""), // no client tracking in route's filter configuration
		header: http.Header{
			authorizationHeaderName: {
				"Bearer " + buildFakeJwtWithBody(map[string]interface{}{
					"realm":  "users",
					"client": "joe",
				}),
			},
		},
		expectedEndpointMetricPrefix: "apiUsageMonitoring.custom.my_app.my_api.GET.foo/orders.*.*.",
		expectedClientMetricPrefix:   "",
	})
}
