package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/proxy/proxytest"
)

const (
	kid = "mykid"
)

var (
	privateKey, _ = rsa.GenerateKey(rand.Reader, 2048)
)

func createToken(t *testing.T, method jwt.SigningMethod) string {
	// Create the Claims
	claims := &jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1000 * time.Second)),
		Issuer:    "test",
		Subject:   "aaa",
	}

	token := jwt.NewWithClaims(method, claims)
	token.Header["kid"] = kid

	s, err := token.SignedString(privateKey)

	t.Logf("Token: %v %v", s, err)

	return s
}

func TestToken(t *testing.T) {
	s := createToken(t, jwt.SigningMethodRS256)

	parsedToken, err := jwt.Parse(s, func(token *jwt.Token) (interface{}, error) {
		return &privateKey.PublicKey, nil
	})

	if err != nil {
		t.Errorf("Failed to json decode: %v", err)
		return
	}

	if !parsedToken.Valid {
		t.Errorf("Failed token: %v", err)
		return
	}

}

func TestJWTValidation(t *testing.T) {
	cli := net.NewClient(net.Options{
		IdleConnTimeout: 2 * time.Second,
	})
	defer cli.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"keys":[{"kty":"RSA", "alg":"RS256", "kid": "%s", "n":"%s","e":"AQAB"}]}`,
			kid, base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()))
	}))
	defer authServer.Close()

	testOidcConfig := getTestOidcConfig()
	issuerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e := json.NewEncoder(w)
		err := e.Encode(testOidcConfig)
		if err != nil {
			t.Fatalf("Could not encode testOidcConfig: %v", err)
		}
	}))
	defer issuerServer.Close()

	// patch openIDConfig to the current testservers
	testOidcConfig.JwksURI = authServer.URL + testAuthPath

	var spec = NewJwtValidationWithOptions(TokenintrospectionOptions{})

	args := []interface{}{issuerServer.URL}

	fr := make(filters.Registry)
	fr.Register(spec)
	r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}

	proxy := proxytest.New(fr, r)
	defer proxy.Close()

	reqURL, _ := url.Parse(proxy.URL)

	for _, ti := range []struct {
		msg      string
		auth     string
		expected int
	}{{
		msg:      "jwtValidation: empty token",
		auth:     authHeaderPrefix,
		expected: http.StatusUnauthorized,
	}, {
		msg:      "jwtValidation: invalid token",
		auth:     authHeaderPrefix + "invalid-token",
		expected: http.StatusUnauthorized,
	}, {
		msg:      "jwtValidation: valid token",
		auth:     authHeaderPrefix + createToken(t, jwt.SigningMethodRS256),
		expected: http.StatusOK,
	}, {
		msg:      "jwtValidation: valid token with algorithm none",
		auth:     authHeaderPrefix + createToken(t, jwt.SigningMethodNone),
		expected: http.StatusUnauthorized,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			req, _ := http.NewRequest("GET", reqURL.String(), nil)

			req.Header.Set(authHeaderName, ti.auth)

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

func createTokenWithKey(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid

	s, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}
	return s
}

func setupJWKSServer(t *testing.T, key *rsa.PrivateKey) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"keys":[{"kty":"RSA", "alg":"RS256", "kid": "%s", "n":"%s","e":"AQAB"}]}`,
			kid, base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()))
	}))
}

func TestJwtValidationKeysSpec(t *testing.T) {
	spec := NewJwtValidationKeys()

	if spec.Name() != filters.JwtValidationKeysName {
		t.Errorf("unexpected name: %s", spec.Name())
	}

	// No arguments
	_, err := spec.CreateFilter([]interface{}{})
	if err == nil {
		t.Error("expected error with no arguments")
	}

	// Too many arguments
	_, err = spec.CreateFilter([]interface{}{"url1", "url2"})
	if err == nil {
		t.Error("expected error with too many arguments")
	}

	// Non-string argument
	_, err = spec.CreateFilter([]interface{}{123})
	if err == nil {
		t.Error("expected error with non-string argument")
	}
}

func TestJwtValidationKeys(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	jwksServer := setupJWKSServer(t, key)
	defer jwksServer.Close()

	cli := net.NewClient(net.Options{
		IdleConnTimeout: 2 * time.Second,
	})
	defer cli.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()

	spec := NewJwtValidationKeys()
	fr := make(filters.Registry)
	fr.Register(spec)

	r := &eskip.Route{
		Filters: []*eskip.Filter{{Name: spec.Name(), Args: []interface{}{jwksServer.URL}}},
		Backend: backend.URL,
	}

	proxy := proxytest.New(fr, r)
	defer proxy.Close()

	for _, ti := range []struct {
		name     string
		auth     string
		expected int
	}{{
		name:     "valid token",
		auth:     authHeaderPrefix + createTokenWithKey(t, key, jwt.MapClaims{"sub": "user1", "exp": jwt.NewNumericDate(time.Now().Add(time.Hour)).Unix()}),
		expected: http.StatusOK,
	}, {
		name:     "expired token",
		auth:     authHeaderPrefix + createTokenWithKey(t, key, jwt.MapClaims{"sub": "user1", "exp": jwt.NewNumericDate(time.Now().Add(-time.Hour)).Unix()}),
		expected: http.StatusUnauthorized,
	}, {
		name:     "missing sub claim accepted",
		auth:     authHeaderPrefix + createTokenWithKey(t, key, jwt.MapClaims{"iss": "test", "exp": jwt.NewNumericDate(time.Now().Add(time.Hour)).Unix()}),
		expected: http.StatusOK,
	}, {
		name:     "no authorization header",
		auth:     "",
		expected: http.StatusUnauthorized,
	}, {
		name:     "empty bearer token",
		auth:     authHeaderPrefix,
		expected: http.StatusUnauthorized,
	}, {
		name:     "invalid token format",
		auth:     authHeaderPrefix + "not-a-jwt",
		expected: http.StatusUnauthorized,
	}, {
		name:     "algorithm none rejected",
		auth:     authHeaderPrefix + createToken(t, jwt.SigningMethodNone),
		expected: http.StatusUnauthorized,
	}} {
		t.Run(ti.name, func(t *testing.T) {
			reqURL, _ := url.Parse(proxy.URL)
			req, _ := http.NewRequest("GET", reqURL.String(), nil)
			if ti.auth != "" {
				req.Header.Set(authHeaderName, ti.auth)
			}

			rsp, err := cli.Do(req)
			if err != nil {
				t.Fatalf("failed to get response: %v", err)
			}
			defer rsp.Body.Close()

			if rsp.StatusCode != ti.expected {
				t.Errorf("unexpected status code: %v != %v", rsp.StatusCode, ti.expected)
			}
		})
	}
}

func TestJWTValidationJwksError(t *testing.T) {
	testOidcConfig := getTestOidcConfig()

	issuerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == TokenIntrospectionConfigPath {
			e := json.NewEncoder(w)
			err := e.Encode(testOidcConfig)
			if err != nil {
				t.Fatalf("Could not encode testOidcConfig: %v", err)
			}
		}
	}))
	defer issuerServer.Close()

	testOidcConfig.JwksURI = issuerServer.URL + "/jwks"

	spec := NewJwtValidationWithOptions(TokenintrospectionOptions{})
	_, err := spec.CreateFilter([]interface{}{issuerServer.URL})

	if err == nil {
		t.Errorf("It should not be possible to create filter")
		return
	}

	/*var state = map[string]interface{}{}
	c := &filtertest.Context{FRequest: &http.Request{}, FStateBag: state}
	c.FRequest.Header = make(http.Header)
	c.FRequest.Header.Add("Authorization", authHeaderPrefix+createToken(t, jwt.SigningMethodRS256))

	f.Request(c)

	if c.FResponse == nil || (c.FResponse != nil && c.FResponse.StatusCode != http.StatusUnauthorized) {
		t.Errorf("Response was not denied as expected")
		return
	}*/

}
