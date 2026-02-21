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
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/proxy/proxytest"
)

func introspectionEndpointGetToken(r *http.Request) (string, error) {
	if tok := r.FormValue(tokenKey); tok != "" {
		return tok, nil
	}
	return "", errInvalidToken
}

func getTestOidcConfig() *openIDConfig {
	return &openIDConfig{
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
	cli := net.NewClient(net.Options{
		IdleConnTimeout: 2 * time.Second,
	})
	defer cli.Close()

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

		token, err2 := introspectionEndpointGetToken(r)
		if err2 != nil || token != testToken {
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
		err2 = e.Encode(&d)
		if err2 != nil && err2 != io.EOF {
			t.Errorf("Failed to json encode: %v", err2)
		}
	}))
	defer authServer.Close()

	testOidcConfig := getTestOidcConfig()
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

	for _, ti := range []struct {
		msg         string
		authType    string
		authBaseURL string
		args        []any
		hasAuth     bool
		auth        string
		expected    int
	}{{

		msg:      "oauthTokenintrospectionAnyClaims: uninitialized filter, no authorization header, scope check",
		authType: filters.OAuthTokenintrospectionAnyClaimsName,
		expected: invalidFilterExpected,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: invalid token",
		authType:    filters.OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{validClaim1},
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: unsupported claim",
		authType:    filters.OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"unsupported-claim"},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: valid claim",
		authType:    filters.OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{validClaim1},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: invalid claim",
		authType:    filters.OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: valid token, one valid claim",
		authType:    filters.OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{validClaim1, validClaim2},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "oauthTokenintrospectionAnyClaims: valid token, one valid claim, one invalid supported claim",
		authType:    filters.OAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{validClaim1, invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {

		msg:         "oauthTokenintrospectionAllClaim: invalid token",
		authType:    filters.OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{validClaim1},
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "oauthTokenintrospectionAllClaim: unsupported claim",
		authType:    filters.OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"unsupported-claim"},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
	}, {
		msg:         "oauthTokenintrospectionAllClaim: valid claim",
		authType:    filters.OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{validClaim1},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "oauthTokenintrospectionAllClaim: invalid claim",
		authType:    filters.OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "oauthTokenintrospectionAllClaim: valid token, one valid claim",
		authType:    filters.OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{validClaim1, validClaim2},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "OAuthTokenintrospectionAllClaimsName: valid token, one valid claim, one invalid supported claim",
		authType:    filters.OAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{validClaim1, invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {

		msg:         "anyKV(): invalid KV",
		authType:    filters.OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []any{validClaim1},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
	}, {
		msg:         "anyKV(): valid token, one valid key, wrong value",
		authType:    filters.OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []any{testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "anyKV(): valid token, one valid key value pair",
		authType:    filters.OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []any{testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "anyKV(): valid token, one valid kv, multiple key value pairs1",
		authType:    filters.OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []any{testKey, testValue, "wrongKey", "wrongValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "anyKV(): valid token, one valid kv, multiple key value pairs2",
		authType:    filters.OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []any{"wrongKey", "wrongValue", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "anyKV(): valid token, one valid kv, same key multiple times should pass",
		authType:    filters.OAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []any{testKey, testValue, testKey, "someValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): invalid KV",
		authType:    filters.OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{testKey},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
	}, {
		msg:         "allKV(): valid token, one valid key, wrong value",
		authType:    filters.OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "allKV(): valid token, one valid key value pair",
		authType:    filters.OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): valid token, one valid key value pair, check realm",
		authType:    filters.OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{testRealmKey, testRealm, testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): valid token, valid key value pairs",
		authType:    filters.OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{testKey, testValue, testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): valid token, one valid kv, multiple key value pairs1",
		authType:    filters.OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{testKey, testValue, "wrongKey", "wrongValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "allKV(): valid token, one valid kv, multiple key value pairs2",
		authType:    filters.OAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{"wrongKey", "wrongValue", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:      "secureOauthTokenintrospectionAnyClaims: uninitialized filter, no authorization header, scope check",
		authType: filters.SecureOAuthTokenintrospectionAnyClaimsName,
		expected: invalidFilterExpected,
	}, {
		msg:         "secureOauthTokenintrospectionAnyClaims: invalid token",
		authType:    filters.SecureOAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", validClaim1},
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "secureOauthTokenintrospectionAnyClaims: unsupported claim",
		authType:    filters.SecureOAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", "unsupported-claim"},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
	}, {
		msg:         "secureOauthTokenintrospectionAnyClaims: valid claim",
		authType:    filters.SecureOAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", validClaim1},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "secureOauthTokenintrospectionAnyClaims: invalid claim",
		authType:    filters.SecureOAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "secureOauthTokenintrospectionAnyClaims: valid token, one valid claim",
		authType:    filters.SecureOAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", validClaim1, validClaim2},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "secureOauthTokenintrospectionAnyClaims: valid token, one valid claim, one invalid supported claim",
		authType:    filters.SecureOAuthTokenintrospectionAnyClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", validClaim1, invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "secureOauthTokenintrospectionAllClaim: invalid token",
		authType:    filters.SecureOAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", validClaim1},
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "secureOauthTokenintrospectionAllClaim: unsupported claim",
		authType:    filters.SecureOAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", "unsupported-claim"},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
	}, {
		msg:         "secureOauthTokenintrospectionAllClaim: valid claim",
		authType:    filters.SecureOAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", validClaim1},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "secureOauthTokenintrospectionAllClaim: invalid claim",
		authType:    filters.SecureOAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "secureOauthTokenintrospectionAllClaim: valid token, one valid claim",
		authType:    filters.SecureOAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", validClaim1, validClaim2},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "SecureOAuthTokenintrospectionAllClaimsName: valid token, one valid claim, one invalid supported claim",
		authType:    filters.SecureOAuthTokenintrospectionAllClaimsName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", validClaim1, invalidSupportedClaim},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {

		msg:         "secureAnyKV(): invalid KV",
		authType:    filters.SecureOAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", validClaim1},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
	}, {
		msg:         "secureAnyKV(): valid token, one valid key, wrong value",
		authType:    filters.SecureOAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "secureAnyKV(): valid token, one valid key value pair",
		authType:    filters.SecureOAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "secureAnyKV(): valid token, one valid kv, multiple key value pairs1",
		authType:    filters.SecureOAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", testKey, testValue, "wrongKey", "wrongValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "secureAnyKV(): valid token, one valid kv, multiple key value pairs2",
		authType:    filters.SecureOAuthTokenintrospectionAnyKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", "wrongKey", "wrongValue", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "secureAllKV(): invalid KV",
		authType:    filters.SecureOAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", testKey},
		hasAuth:     true,
		auth:        testToken,
		expected:    invalidFilterExpected,
	}, {
		msg:         "secureAllKV(): valid token, one valid key, wrong value",
		authType:    filters.SecureOAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "secureAllKV(): valid token, one valid key value pair",
		authType:    filters.SecureOAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "secureAllKV(): valid token, one valid key value pair, check realm",
		authType:    filters.SecureOAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", testRealmKey, testRealm, testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "secureAllKV(): valid token, valid key value pairs",
		authType:    filters.SecureOAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", testKey, testValue, testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "secureAllKV(): valid token, one valid kv, multiple key value pairs1",
		authType:    filters.SecureOAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", testKey, testValue, "wrongKey", "wrongValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "secureAllKV(): valid token, one valid kv, multiple key value pairs2",
		authType:    filters.SecureOAuthTokenintrospectionAllKVName,
		authBaseURL: testAuthPath,
		args:        []any{"client-id", "client-secret", "wrongKey", "wrongValue", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			if ti.msg == "" {
				t.Fatalf("unknown ti: %+v", ti)
			}

			var spec filters.Spec
			switch ti.authType {
			case filters.OAuthTokenintrospectionAnyClaimsName:
				spec = NewOAuthTokenintrospectionAnyClaims(time.Second)
			case filters.OAuthTokenintrospectionAllClaimsName:
				spec = NewOAuthTokenintrospectionAllClaims(time.Second)
			case filters.OAuthTokenintrospectionAnyKVName:
				spec = NewOAuthTokenintrospectionAnyKV(time.Second)
			case filters.OAuthTokenintrospectionAllKVName:
				spec = NewOAuthTokenintrospectionAllKV(time.Second)
			case filters.SecureOAuthTokenintrospectionAnyClaimsName:
				spec = NewSecureOAuthTokenintrospectionAnyClaims(time.Second)
			case filters.SecureOAuthTokenintrospectionAllClaimsName:
				spec = NewSecureOAuthTokenintrospectionAllClaims(time.Second)
			case filters.SecureOAuthTokenintrospectionAnyKVName:
				spec = NewSecureOAuthTokenintrospectionAnyKV(time.Second)
			case filters.SecureOAuthTokenintrospectionAllKVName:
				spec = NewSecureOAuthTokenintrospectionAllKV(time.Second)
			default:
				t.Fatalf("FATAL: authType '%s' not supported", ti.authType)
			}

			args := []any{testOidcConfig.Issuer}
			args = append(args, ti.args...)
			_, err := spec.CreateFilter(args)
			if err != nil {
				if ti.expected == invalidFilterExpected {
					return
				}
				t.Errorf("error creating filter for %s: %v", ti.msg, err)
				return
			}

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

			rsp, err := cli.Do(req)
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
		"claims": map[string]any{
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
		"claims": map[string]any{
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
