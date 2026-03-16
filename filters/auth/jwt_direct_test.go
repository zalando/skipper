package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
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

func createDirectToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
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
		auth:     authHeaderPrefix + createDirectToken(t, key, jwt.MapClaims{"sub": "user1", "exp": jwt.NewNumericDate(time.Now().Add(time.Hour)).Unix()}),
		expected: http.StatusOK,
	}, {
		name:     "expired token",
		auth:     authHeaderPrefix + createDirectToken(t, key, jwt.MapClaims{"sub": "user1", "exp": jwt.NewNumericDate(time.Now().Add(-time.Hour)).Unix()}),
		expected: http.StatusUnauthorized,
	}, {
		name:     "missing sub claim",
		auth:     authHeaderPrefix + createDirectToken(t, key, jwt.MapClaims{"iss": "test", "exp": jwt.NewNumericDate(time.Now().Add(time.Hour)).Unix()}),
		expected: http.StatusUnauthorized,
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
