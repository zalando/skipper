package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

var testOidcConfig *OpenIDConfig = &OpenIDConfig{
	Issuer:                            "https://identity.example.com",
	AuthorizationEndpoint:             "https://identity.example.com/oauth2/authorize",
	TokenEndpoint:                     "https://identity.example.com/oauth2/token",
	UserinfoEndpoint:                  "https://identity.example.com/oauth2/userinfo",
	RevocationEndpoint:                "https://identity.example.com/oauth2/revoke",
	JwksURI:                           "https://identity.example.com/.well-known/jwk_uris",
	RegistrationEndpoint:              "https://identity.example.com/oauth2/register",
	IntrospectionEndpoint:             "https://identity.example.com/oauth2/introspection",
	ResponseTypesSupported:            []string{"code", "token", "code token"},
	SubjectTypesSupported:             []string{"public"},
	IDTokenSigningAlgValuesSupported:  []string{"RS256", "ES512", "PS384"},
	TokenEndpointAuthMethodsSupported: []string{"client_secret_basic"},
	ClaimsSupported:                   []string{"sub", "name", "email", "azp", "iss", "exp", "iat", "https://identity.example.com/token", "https://identity.example.com/realm", "https://identity.example.com/bp", "https://identity.example.com/privileges"},
	ScopesSupported:                   []string{"openid", "email"},
	CodeChallengeMethodsSupported:     []string{"plain", "S256"},
}

var (
	validClaim1                = "email"
	validClaim1Value           = "jdoe@example.com"
	validClaim2                = "name"
	validClaim2Value           = "Jane Doe"
	invalidSupportedClaim      = "sub"
	invalidSupportedClaimValue = "x42"
)

func TestOAuth2Tokenintrospection(t *testing.T) {
	for _, ti := range []struct {
		msg         string
		authType    string
		authBaseURL string
		args        []interface{}
		hasAuth     bool
		auth        string
		expected    int
	}{{
		msg:      "oauthTokenintrospectionAnyClaims: uninitialized filter, no authorization header, scope check",
		authType: OAuthTokenintrospectionAnyClaimsName,
		expected: http.StatusNotFound,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: invalid token",
		authType:    OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusNotFound,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: unsupported claim",
		authType:    OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"unsupported-claim"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusNotFound,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: valid claim",
		authType:    OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim1},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: invalid claim",
		authType:    OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: valid token, one valid claim",
		authType:    OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim1, validClaim2},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: valid token, one valid claim, one invalid supported claim",
		authType:    OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim1, invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {

		msg:         "oauthTokenintrospectionAllClaim: invalid token",
		authType:    OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusNotFound,
	}, {
		msg:         "oauthTokenintrospectionAllClaim: unsupported claim",
		authType:    OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"unsupported-claim"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusNotFound,
	}, {
		msg:         "oauthTokenintrospectionAllClaim: valid claim",
		authType:    OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim1},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "oauthTokenintrospectionAllClaim: invalid claim",
		authType:    OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "oauthTokenintrospectionAllClaim: valid token, one valid claim",
		authType:    OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim1, validClaim2},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "OAuthTokenintrospectionAllClaimsName: valid token, one valid claim, one invalid supported claim",
		authType:    OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim1, invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {

		msg:         "anyKV(): invalid key",
		authType:    OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusNotFound,
	}, {
		msg:         "anyKV(): valid token, one valid key, wrong value",
		authType:    OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "anyKV(): valid token, one valid key value pair",
		authType:    OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "anyKV(): valid token, one valid kv, multiple key value pairs1",
		authType:    OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue, "wrongKey", "wrongValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "anyKV(): valid token, one valid kv, multiple key value pairs2",
		authType:    OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"wrongKey", "wrongValue", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): invalid key",
		authType:    OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusNotFound,
	}, {
		msg:         "allKV(): valid token, one valid key, wrong value",
		authType:    OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "allKV(): valid token, one valid key value pair",
		authType:    OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): valid token, one valid key value pair, check realm",
		authType:    OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testRealmKey, testRealm, testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): valid token, valid key value pairs",
		authType:    OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue, testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): valid token, one valid kv, multiple key value pairs1",
		authType:    OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue, "wrongKey", "wrongValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "allKV(): valid token, one valid kv, multiple key value pairs2",
		authType:    OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"wrongKey", "wrongValue", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			if ti.msg == "" {
				t.Fatalf("unknown ti: %+v", ti)
			}
			backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			defer backend.Close()

			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					w.WriteHeader(489)
					return
				}

				if r.URL.Path != testAuthPath {
					w.WriteHeader(488)
					return
				}

				token, err := getToken(r)
				if err != nil || token != testToken {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				d := tokenIntrospectionInfo{
					"uid":        testUID,
					testRealmKey: testRealm,
					"claims": map[string]string{
						validClaim1: validClaim1Value,
						validClaim2: validClaim2Value,
					},
					"sub":    "testSub",
					"active": true,
				}

				e := json.NewEncoder(w)
				err = e.Encode(&d)
				if err != nil && err != io.EOF {
					t.Errorf("Failed to json encode: %v", err)
				}
			}))
			defer authServer.Close()

			testOidcConfig.IntrospectionEndpoint = "http://" + authServer.Listener.Addr().String() + testAuthPath
			defer authServer.Close()

			var spec filters.Spec
			args := []interface{}{}

			switch ti.authType {
			case OAuthTokenintrospectionAnyClaimsName:
				spec = NewOAuthTokenintrospectionAnyClaims(testOidcConfig, time.Second)
			case OAuthTokenintrospectionAllClaimsName:
				spec = NewOAuthTokenintrospectionAllClaims(testOidcConfig, time.Second)
			case OAuthTokenintrospectionAnyKVName:
				spec = NewOAuthTokenintrospectionAnyKV(testOidcConfig, time.Second)
			case OAuthTokenintrospectionAllKVName:
				spec = NewOAuthTokenintrospectionAllKV(testOidcConfig, time.Second)
			default:
				t.Fatalf("FATAL: authType '%s' not supported", ti.authType)
			}

			args = append(args, ti.args...)
			fr := make(filters.Registry)
			fr.Register(spec)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			defer proxy.Close()

			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
				return
			}

			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Errorf("failed to create request %v", err)
				return
			}

			if ti.hasAuth {
				req.Header.Set(authHeaderName, authHeaderPrefix+ti.auth)
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("failed to get response: %v", err)
				return
			}
			defer rsp.Body.Close()

			if rsp.StatusCode != ti.expected {
				t.Errorf("unexpected status code: %v != %v", rsp.StatusCode, ti.expected)
				return
			}
		})
	}
}

func Test_validateAnyClaims(t *testing.T) {
	claims := []string{"email", "name"}
	claimsPartialOverlapping := []string{"email", "name", "missing"}
	info := tokenIntrospectionInfo{
		"/realm": "/immortals",
		"claims": map[string]interface{}{
			"email": "jdoe@example.com",
			"name":  "Jane Doe",
			"uid":   "jdoe",
		},
	}

	for _, ti := range []struct {
		msg      string
		claims   []string
		info     tokenIntrospectionInfo
		expected bool
	}{{
		msg:      "validate any, noclaims configured, got no claims",
		claims:   []string{},
		info:     tokenIntrospectionInfo{},
		expected: false,
	}, {
		msg:      "validate any, noclaims configured, got claims",
		claims:   []string{},
		info:     info,
		expected: false,
	}, {
		msg:      "validate any, claims configured, got no claims",
		claims:   claims,
		info:     tokenIntrospectionInfo{},
		expected: false,
	}, {
		msg:      "validate any, claims configured, got not enough claims",
		claims:   claimsPartialOverlapping,
		info:     info,
		expected: true,
	}, {
		msg:      "validate any, claims configured, got claims",
		claims:   claims,
		info:     info,
		expected: true,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			if ti.msg == "" {
				t.Fatalf("unknown ti: %+v", ti)
			}

			f := &tokenintrospectFilter{claims: ti.claims}
			if f.validateAnyClaims(ti.info) != ti.expected {
				t.Error("failed to validate any claims")
			}

		})
	}
}

func Test_validateAllClaims(t *testing.T) {
	claims := []string{"email", "name"}
	claimsPartialOverlapping := []string{"email", "name", "missing"}
	info := tokenIntrospectionInfo{
		"/realm": "/immortals",
		"claims": map[string]interface{}{
			"email": "jdoe@example.com",
			"name":  "Jane Doe",
			"uid":   "jdoe",
		},
	}

	for _, ti := range []struct {
		msg      string
		claims   []string
		info     tokenIntrospectionInfo
		expected bool
	}{{
		msg:      "validate all, noclaims configured, got no claims",
		claims:   []string{},
		info:     tokenIntrospectionInfo{},
		expected: true,
	}, {
		msg:      "validate all, noclaims configured, got claims",
		claims:   []string{},
		info:     info,
		expected: true,
	}, {
		msg:      "validate all, claims configured, got no claims",
		claims:   claims,
		info:     tokenIntrospectionInfo{},
		expected: false,
	}, {
		msg:      "validate all, claims configured, got not enough claims",
		claims:   claimsPartialOverlapping,
		info:     info,
		expected: false,
	}, {
		msg:      "validate all, claims configured, got claims",
		claims:   claims,
		info:     info,
		expected: true,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			if ti.msg == "" {
				t.Fatalf("unknown ti: %+v", ti)
			}

			f := &tokenintrospectFilter{claims: ti.claims}
			if f.validateAllClaims(ti.info) != ti.expected {
				t.Error("failed to validate all claims")
			}

		})
	}
}
