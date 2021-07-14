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

const (
	testJwtToken = "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIsIng1dCI6Im5PbzNaRHJPRFhFSzFqS1doWHNsSFJfS1hFZyIsImtpZCI6Im5PbzNaRHJPRFhFSzFqS1doWHNsSFJfS1hFZyJ9.eyJhdWQiOiJodHRwczovL3Rlc3QtbWFyaWEuYXBwbGljYXRpb25zLjFjb3JwLm9yZyIsImlzcyI6Imh0dHBzOi8vc3RzLndpbmRvd3MubmV0L2ExZWFjYmQ1LWZiMGUtNDZmMS04MWUzLTQ5NjVlYThlNDViYi8iLCJpYXQiOjE2MjAyMTgxNTksIm5iZiI6MTYyMDIxODE1OSwiZXhwIjoxNjIwMjIyMDU5LCJhaW8iOiJFMlpnWUloTTkyTHJlRDUxU3BHWTdEWW0zM1dyQUE9PSIsImFwcGlkIjoiMmQ2MDA4YjMtNjAxMi00ZjIyLWEwMWUtMzMzMWM2MDk3ZTdhIiwiYXBwaWRhY3IiOiIxIiwiaWRwIjoiaHR0cHM6Ly9zdHMud2luZG93cy5uZXQvYTFlYWNiZDUtZmIwZS00NmYxLTgxZTMtNDk2NWVhOGU0NWJiLyIsIm9pZCI6IjBjNjA5Mzk0LWI4ZmItNDJkNi04MjQ2LWFiMmI0NTcyY2E2NyIsInJoIjoiMC5BU0VBMWN2cW9RNzc4VWFCNDBsbDZvNUZ1N01JWUMwU1lDSlBvQjR6TWNZSmZub2hBQUEuIiwic3ViIjoiMGM2MDkzOTQtYjhmYi00MmQ2LTgyNDYtYWIyYjQ1NzJjYTY3IiwidGlkIjoiYTFlYWNiZDUtZmIwZS00NmYxLTgxZTMtNDk2NWVhOGU0NWJiIiwidXRpIjoiOHZNSDExLUpvVU9xeU81aE1SQW1BZyIsInZlciI6IjEuMCJ9.BL_OL-IWr7w0NMsym_etT_30EpAZYM3zCWlnCynxyQUMfrfDqw2-J35efhKEm44BDAzdrIk-8ksl_FpPfdtCPl-G_Hwx7ye5-tjOeTpPc2mJI67Q2mpvNBA_IvWPpvYVtrcZnNeY9Xykc9Xd9G7YY1-RfuJK1F2Ud0_Sb1YG8y51UQm1UiDz2X6RT6iKotSl9L1iG8UifM7CCSA4N70P9JlgB5l1YYQoKD5ZiDSBKKPOiW7KsK6S_f3Z_MVjtBExoatQrPbrCPOjkNraBMiwDixODeoiyf6GihbZFXVNw2qX8RCtOGMQ9VwHheEAawS-ehaVb-FiLwAJNuWdUEtMhg"
)

/*func introspectionEndpointGetToken(r *http.Request) (string, error) {
	if tok := r.FormValue(tokenKey); tok != "" {
		return tok, nil
	}
	return "", errInvalidToken
}*/

/*func getTestJWTOidcConfig() *openIDConfig {
	return &openIDConfig{
		Issuer:                "https://identity.example.com",
		AuthorizationEndpoint: "https://identity.example.com/oauth2/authorize",
		TokenEndpoint:         "https://identity.example.com/oauth2/token",
		UserinfoEndpoint:      "https://identity.example.com/oauth2/userinfo",
		RevocationEndpoint:    "https://identity.example.com/oauth2/revoke",
		JwksURI:               "https://identity.example.com/.well-known/jwk_uris",
		RegistrationEndpoint:  "https://identity.example.com/oauth2/register",
		//IntrospectionEndpoint:             "https://identity.example.com/oauth2/introspection",
		ResponseTypesSupported:            []string{"code", "token", "code token"},
		SubjectTypesSupported:             []string{"public"},
		IDTokenSigningAlgValuesSupported:  []string{"RS256", "ES512", "PS384"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic"},
		ClaimsSupported:                   []string{"sub", "name", "email", "azp", "iss", "exp", "iat", "https://identity.example.com/token", "https://identity.example.com/realm", "https://identity.example.com/bp", "https://identity.example.com/privileges"},
		ScopesSupported:                   []string{"openid", "email"},
		CodeChallengeMethodsSupported:     []string{"plain", "S256"},
	}
}*/

var (
	/*validClaim1           = "email"
	  validClaim1Value      = "jdoe@example.com"
	  validClaim2           = "name"
	  validClaim2Value      = "Jane Doe"
	  invalidSupportedClaim = "sub"
	  invalidFilterExpected = 999*/
	validClaim3            = "sub"
	invalidSupportedClaim2 = "email"
)

func TestJWTValidation(t *testing.T) {
	cli := net.NewClient(net.Options{
		IdleConnTimeout: 2 * time.Second,
	})
	defer cli.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(489)
			return
		}

		allKeys := map[string][]interface{}{}
		allKeys["keys"] = append(allKeys["keys"], map[string]interface{}{"kid": "nOo3ZDrODXEK1jKWhXslHR_KXEg", "n": "oaLLT9hkcSj2tGfZsjbu7Xz1Krs0qEicXPmEsJKOBQHauZ_kRM1HdEkgOJbUznUspE6xOuOSXjlzErqBxXAu4SCvcvVOCYG2v9G3-uIrLF5dstD0sYHBo1VomtKxzF90Vslrkn6rNQgUGIWgvuQTxm1uRklYFPEcTIRw0LnYknzJ06GC9ljKR617wABVrZNkBuDgQKj37qcyxoaxIGdxEcmVFZXJyrxDgdXh9owRmZn6LIJlGjZ9m59emfuwnBnsIQG7DirJwe9SXrLXnexRQWqyzCdkYaOqkpKrsjuxUj2-MHX31FqsdpJJsOAvYXGOYBKJRjhGrGdONVrZdUdTBQ", "e": "AQAB"})
		if r.URL.Path != testAuthPath {
			w.WriteHeader(488)
			return
		}

		e := json.NewEncoder(w)
		err2 := e.Encode(allKeys)
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
	testOidcConfig.JwksURI = "http://" + authServer.Listener.Addr().String() + testAuthPath

	for _, ti := range []struct {
		msg         string
		authType    string
		authBaseURL string
		args        []interface{}
		hasAuth     bool
		auth        string
		expected    int
	}{{

		msg:      "jwtValidationAnyClaims: uninitialized filter, no authorization header, scope check",
		expected: invalidFilterExpected,
	}, {
		msg:         "jwtValidationAnyClaims: invalid token",
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim1},
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "jwtValidationAnyClaims: unsupported claim",
		authBaseURL: testAuthPath,
		args:        []interface{}{"unsupported-claim"},
		hasAuth:     true,
		auth:        testJwtToken,
		expected:    invalidFilterExpected,
	}, {
		msg:         "jwtValidationAnyClaims: valid claim",
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim3},
		hasAuth:     true,
		auth:        testJwtToken,
		expected:    http.StatusOK,
	}, {
		msg:         "jwtValidationAnyClaims: invalid claim",
		authBaseURL: testAuthPath,
		args:        []interface{}{invalidSupportedClaim2},
		hasAuth:     true,
		auth:        testJwtToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "jwtValidationAnyClaims: valid token, one valid claim",
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim3, validClaim2},
		hasAuth:     true,
		auth:        testJwtToken,
		expected:    http.StatusOK,
	}, {
		msg:         "jwtValidationAnyClaims: valid token, one valid claim, one invalid supported claim",
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim3, invalidSupportedClaim2},
		hasAuth:     true,
		auth:        testJwtToken,
		expected:    http.StatusOK,
	}, {

		msg:         "jwtValidationAnyClaims: invalid token",
		authBaseURL: testAuthPath,
		args:        []interface{}{validClaim1},
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusUnauthorized,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			if ti.msg == "" {
				t.Fatalf("unknown ti: %+v", ti)
			}

			var spec = NewJwtValidation(testAuthTimeout)

			args := []interface{}{testOidcConfig.Issuer}
			args = append(args, ti.args...)
			f, err := spec.CreateFilter(args)
			if err != nil {
				if ti.expected == invalidFilterExpected {
					return
				}
				t.Errorf("error in creating filter for %s: %v", ti.msg, err)
				return
			}

			f2 := f.(*jwtValidationFilter)
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
