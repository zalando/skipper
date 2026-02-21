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

	parsedToken, err := jwt.Parse(s, func(token *jwt.Token) (any, error) {
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

	args := []any{issuerServer.URL}

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
	_, err := spec.CreateFilter([]any{issuerServer.URL})

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
