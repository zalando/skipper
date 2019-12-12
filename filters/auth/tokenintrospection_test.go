package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

func introspectionEndpointGetToken(r *http.Request) (string, error) {
	if tok := r.FormValue(tokenKey); tok != "" {
		return tok, nil
	}
	return "", errInvalidToken
}

var testOidcConfig *openIDConfig = &openIDConfig{
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
	validClaim1           = "email"
	validClaim1Value      = "jdoe@example.com"
	validClaim2           = "name"
	validClaim2Value      = "Jane Doe"
	invalidSupportedClaim = "sub"
	invalidFilterExpected = 999
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
		expected: invalidFilterExpected,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: invalid token",
		authType:    OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim1},
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: unsupported claim",
		authType:    OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"unsupported-claim"},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
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
		args:        []interface{}{validClaim1},
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "oauthTokenintrospectionAllClaim: unsupported claim",
		authType:    OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"unsupported-claim"},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
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

		msg:         "anyKV(): invalid KV",
		authType:    OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim1},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
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
		msg:         "anyKV(): valid token, one valid kv, same key multiple times should pass",
		authType:    OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue, testKey, "someValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): invalid KV",
		authType:    OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
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
	}, //secure
		{
			msg:      "secureOauthTokenintrospectionAnyClaims: uninitialized filter, no authorization header, scope check",
			authType: SecureOAuthTokenintrospectionAnyClaimsName,
			expected: invalidFilterExpected,
		}, {
			msg:         "secureOauthTokenintrospectionAnyClaims: invalid token",
			authType:    SecureOAuthTokenintrospectionAnyClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", validClaim1},
			hasAuth:     true,
			auth:        "invalid-token",
			expected:    http.StatusUnauthorized,
		}, {
			msg:         "secureOauthTokenintrospectionAnyClaims: unsupported claim",
			authType:    SecureOAuthTokenintrospectionAnyClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", "unsupported-claim"},
			hasAuth:     true,
			auth:        testToken,
			expected:    invalidFilterExpected,
		}, {
			msg:         "secureOauthTokenintrospectionAnyClaims: valid claim",
			authType:    SecureOAuthTokenintrospectionAnyClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", validClaim1},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusOK,
		}, {
			msg:         "secureOauthTokenintrospectionAnyClaims: invalid claim",
			authType:    SecureOAuthTokenintrospectionAnyClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", invalidSupportedClaim},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusUnauthorized,
		}, {
			msg:         "secureOauthTokenintrospectionAnyClaims: valid token, one valid claim",
			authType:    SecureOAuthTokenintrospectionAnyClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", validClaim1, validClaim2},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusOK,
		}, {
			msg:         "secureOauthTokenintrospectionAnyClaims: valid token, one valid claim, one invalid supported claim",
			authType:    SecureOAuthTokenintrospectionAnyClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", validClaim1, invalidSupportedClaim},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusOK,
		}, {

			msg:         "secureOauthTokenintrospectionAllClaim: invalid token",
			authType:    SecureOAuthTokenintrospectionAllClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", validClaim1},
			hasAuth:     true,
			auth:        "invalid-token",
			expected:    http.StatusUnauthorized,
		}, {
			msg:         "secureOauthTokenintrospectionAllClaim: unsupported claim",
			authType:    SecureOAuthTokenintrospectionAllClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", "unsupported-claim"},
			hasAuth:     true,
			auth:        testToken,
			expected:    invalidFilterExpected,
		}, {
			msg:         "secureOauthTokenintrospectionAllClaim: valid claim",
			authType:    SecureOAuthTokenintrospectionAllClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", validClaim1},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusOK,
		}, {
			msg:         "secureOauthTokenintrospectionAllClaim: invalid claim",
			authType:    SecureOAuthTokenintrospectionAllClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", invalidSupportedClaim},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusUnauthorized,
		}, {
			msg:         "secureOauthTokenintrospectionAllClaim: valid token, one valid claim",
			authType:    SecureOAuthTokenintrospectionAllClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", validClaim1, validClaim2},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusOK,
		}, {
			msg:         "SecureOAuthTokenintrospectionAllClaimsName: valid token, one valid claim, one invalid supported claim",
			authType:    SecureOAuthTokenintrospectionAllClaimsName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", validClaim1, invalidSupportedClaim},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusUnauthorized,
		}, {

			msg:         "secureAnyKV(): invalid KV",
			authType:    SecureOAuthTokenintrospectionAnyKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", validClaim1},
			hasAuth:     true,
			auth:        testToken,
			expected:    invalidFilterExpected,
		}, {
			msg:         "secureAnyKV(): valid token, one valid key, wrong value",
			authType:    SecureOAuthTokenintrospectionAnyKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", testKey, "other-value"},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusUnauthorized,
		}, {
			msg:         "secureAnyKV(): valid token, one valid key value pair",
			authType:    SecureOAuthTokenintrospectionAnyKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", testKey, testValue},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusOK,
		}, {
			msg:         "secureAnyKV(): valid token, one valid kv, multiple key value pairs1",
			authType:    SecureOAuthTokenintrospectionAnyKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", testKey, testValue, "wrongKey", "wrongValue"},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusOK,
		}, {
			msg:         "secureAnyKV(): valid token, one valid kv, multiple key value pairs2",
			authType:    SecureOAuthTokenintrospectionAnyKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", "wrongKey", "wrongValue", testKey, testValue},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusOK,
		}, {
			msg:         "secureAllKV(): invalid KV",
			authType:    SecureOAuthTokenintrospectionAllKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", testKey},
			hasAuth:     true,
			auth:        testToken,
			expected:    invalidFilterExpected,
		}, {
			msg:         "secureAllKV(): valid token, one valid key, wrong value",
			authType:    SecureOAuthTokenintrospectionAllKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", testKey, "other-value"},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusUnauthorized,
		}, {
			msg:         "secureAllKV(): valid token, one valid key value pair",
			authType:    SecureOAuthTokenintrospectionAllKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", testKey, testValue},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusOK,
		}, {
			msg:         "secureAllKV(): valid token, one valid key value pair, check realm",
			authType:    SecureOAuthTokenintrospectionAllKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", testRealmKey, testRealm, testKey, testValue},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusOK,
		}, {
			msg:         "secureAllKV(): valid token, valid key value pairs",
			authType:    SecureOAuthTokenintrospectionAllKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", testKey, testValue, testKey, testValue},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusOK,
		}, {
			msg:         "secureAllKV(): valid token, one valid kv, multiple key value pairs1",
			authType:    SecureOAuthTokenintrospectionAllKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", testKey, testValue, "wrongKey", "wrongValue"},
			hasAuth:     true,
			auth:        testToken,
			expected:    http.StatusUnauthorized,
		}, {
			msg:         "secureAllKV(): valid token, one valid kv, multiple key value pairs2",
			authType:    SecureOAuthTokenintrospectionAllKVName,
			authBaseURL: testAuthPath,
			args:        []interface{}{"client-id", "client-secret", "wrongKey", "wrongValue", testKey, testValue},
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

				token, err := introspectionEndpointGetToken(r)
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

			issuerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != TokenIntrospectionConfigPath {
					w.WriteHeader(486)
					return
				}
				e := json.NewEncoder(w)
				err := e.Encode(testOidcConfig)
				if err != nil {
					t.Fatalf("Could not encode testOidcConfig: %v", err)
				}

			}))
			defer issuerServer.Close()

			// patch openIDConfig to the current testservers
			testOidcConfig.Issuer = "http://" + issuerServer.Listener.Addr().String()
			testOidcConfig.IntrospectionEndpoint = "http://" + authServer.Listener.Addr().String() + testAuthPath

			var spec filters.Spec
			args := []interface{}{
				"http://" + issuerServer.Listener.Addr().String(),
			}

			switch ti.authType {
			case OAuthTokenintrospectionAnyClaimsName:
				spec = NewOAuthTokenintrospectionAnyClaims(time.Second, opentracing.NoopTracer{})
			case OAuthTokenintrospectionAllClaimsName:
				spec = NewOAuthTokenintrospectionAllClaims(time.Second, opentracing.NoopTracer{})
			case OAuthTokenintrospectionAnyKVName:
				spec = NewOAuthTokenintrospectionAnyKV(time.Second, opentracing.NoopTracer{})
			case OAuthTokenintrospectionAllKVName:
				spec = NewOAuthTokenintrospectionAllKV(time.Second, opentracing.NoopTracer{})
			case SecureOAuthTokenintrospectionAnyClaimsName:
				spec = NewSecureOAuthTokenintrospectionAnyClaims(time.Second, opentracing.NoopTracer{})
			case SecureOAuthTokenintrospectionAllClaimsName:
				spec = NewSecureOAuthTokenintrospectionAllClaims(time.Second, opentracing.NoopTracer{})
			case SecureOAuthTokenintrospectionAnyKVName:
				spec = NewSecureOAuthTokenintrospectionAnyKV(time.Second, opentracing.NoopTracer{})
			case SecureOAuthTokenintrospectionAllKVName:
				spec = NewSecureOAuthTokenintrospectionAllKV(time.Second, opentracing.NoopTracer{})
			default:
				t.Fatalf("FATAL: authType '%s' not supported", ti.authType)
			}

			args = append(args, ti.args...)
			f, err := spec.CreateFilter(args)
			if err != nil {
				if ti.expected == invalidFilterExpected {
					return
				}
				t.Errorf("error in creating filter for %s: %v", ti.msg, err)
				return
			}

			f2 := f.(*tokenintrospectFilter)
			defer f2.Close()

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
