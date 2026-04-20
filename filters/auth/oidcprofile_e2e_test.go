package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/go-jose/go-jose.v2"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/annotate"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/secrets/secrettest"
)

// createFlexOIDCServer creates an OIDC test server that does not require a
// pre-configured callback URL. It stores the redirect_uri from each auth
// request and validates it during the token exchange — matching the behavior
// of a real OIDC server. This allows the callback URL to be determined at
// request time (e.g. via {{.Request.Host}} in a profile template) rather than
// at server start-up time.
func createFlexOIDCServer(client, clientsecret string, extraClaims jwt.MapClaims, removeClaims []string) *httptest.Server {
	var (
		mu              sync.Mutex
		lastRedirectURI string
	)

	var oidcServer *httptest.Server
	oidcServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			st := strings.ReplaceAll(testOpenIDConfig, "https://accounts.google.com", oidcServer.URL)
			st = strings.ReplaceAll(st, "https://oauth2.googleapis.com", oidcServer.URL)
			st = strings.ReplaceAll(st, "https://www.googleapis.com", oidcServer.URL)
			st = strings.ReplaceAll(st, "https://openidconnect.googleapis.com", oidcServer.URL)
			_, _ = w.Write([]byte(st))

		case "/o/oauth2/v2/auth":
			clientID := r.URL.Query().Get("client_id")
			if clientID != client {
				w.WriteHeader(401)
				return
			}
			redirectURI := r.URL.Query().Get("redirect_uri")
			if redirectURI == "" {
				w.WriteHeader(401)
				return
			}
			if r.URL.Query().Get("response_type") != "code" {
				w.WriteHeader(500)
				return
			}
			scopesString := r.URL.Query().Get("scope")
			scopes := strings.Fields(scopesString)
			supportedScopes := []string{"openid", "sub", "email", "uid", "profile"}
			if !all(scopes, supportedScopes) {
				w.WriteHeader(401)
				return
			}

			mu.Lock()
			lastRedirectURI = redirectURI
			mu.Unlock()

			state := r.URL.Query().Get("state")
			u, err := url.Parse(redirectURI + "?state=" + state + "&code=" + validCode)
			if err != nil {
				log.Fatalf("createFlexOIDCServer: failed to parse redirect URI: %v", err)
			}
			w.Header().Set("Location", u.String())
			w.WriteHeader(302)

		case "/token":
			user, password, ok := r.BasicAuth()
			if !ok || user != client || password != clientsecret {
				w.WriteHeader(401)
				return
			}
			if err := r.ParseForm(); err != nil {
				log.Fatalf("createFlexOIDCServer: failed to parse form: %v", err)
			}
			if r.Form.Get("code") != validCode {
				w.WriteHeader(401)
				return
			}
			redirectURI := r.Form.Get("redirect_uri")
			mu.Lock()
			expected := lastRedirectURI
			mu.Unlock()
			if redirectURI != expected {
				w.WriteHeader(401)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Pragma", "no-cache")

			claims := jwt.MapClaims{
				testKey: testValue,
				"iss":   oidcServer.URL,
				"sub":   testSub,
				"aud":   client,
				"iat":   time.Now().Add(-time.Minute).UTC().Unix(),
				"exp":   time.Now().Add(tokenExp).UTC().Unix(),
				"email": "someone@example.org",
			}
			for k, v := range extraClaims {
				claims[k] = v
			}
			for _, k := range removeClaims {
				delete(claims, k)
			}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			privPEM, err := os.ReadFile(keyPath)
			if err != nil {
				log.Fatalf("createFlexOIDCServer: failed to read key: %v", err)
			}
			key, err := jwt.ParseRSAPrivateKeyFromPEM(privPEM)
			if err != nil {
				log.Fatalf("createFlexOIDCServer: failed to parse RSA key: %v", err)
			}
			validIDToken, err := token.SignedString(key)
			if err != nil {
				log.Fatalf("createFlexOIDCServer: failed to sign token: %v", err)
			}
			body := fmt.Sprintf(
				`{"access_token": "%s", "token_type": "Bearer", "refresh_token": "%s", "expires_in": 3600, "id_token": "%s"}`,
				validAccessToken, validRefreshToken, validIDToken,
			)
			_, _ = w.Write([]byte(body))

		case "/v1/userinfo":
			if r.Header.Get(authHeaderName) == authHeaderPrefix+validAccessToken {
				body := fmt.Sprintf(`{"sub": "%s", "email": "%s@example.org", "email_verified": true}`, testSub, testUID)
				_, _ = w.Write([]byte(body))
			} else {
				w.WriteHeader(401)
			}

		case "/oauth2/v3/certs":
			certPEM, err := os.ReadFile(certPath)
			if err != nil {
				log.Fatalf("createFlexOIDCServer: failed to read cert: %v", err)
			}
			pemDecodeCert, _ := pem.Decode(certPEM)
			cert, err := x509.ParseCertificate(pemDecodeCert.Bytes)
			if err != nil {
				log.Fatalf("createFlexOIDCServer: failed to parse cert: %v", err)
			}
			privPEM, err := os.ReadFile(keyPath)
			if err != nil {
				log.Fatalf("createFlexOIDCServer: failed to read key: %v", err)
			}
			pemDecodePriv, _ := pem.Decode(privPEM)
			privKey, err := x509.ParsePKCS8PrivateKey(pemDecodePriv.Bytes)
			if err != nil {
				log.Fatalf("createFlexOIDCServer: failed to parse PKCS8 key: %v", err)
			}
			rsaPrivKey, ok := privKey.(*rsa.PrivateKey)
			if !ok {
				log.Fatal("createFlexOIDCServer: private key is not RSA")
			}
			j := jose.JSONWebKeySet{
				Keys: []jose.JSONWebKey{{
					Key:          &rsaPrivKey.PublicKey,
					Certificates: []*x509.Certificate{cert},
					Algorithm:    "RS256",
					Use:          "sig",
				}},
			}
			b, err := json.Marshal(j)
			if err != nil {
				log.Fatalf("createFlexOIDCServer: failed to marshal JWKS: %v", err)
			}
			_, _ = w.Write(b)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return oidcServer
}

// TestOIDCProfileSetup runs end-to-end tests for the OIDC profile feature.
//
// Each sub-test spins up a fresh OIDC server, proxy, and backend. The proxy's
// filter registry is configured with named OIDC profiles (OidcOptions.Profiles).
// The IdpURL in each profile is set to the test OIDC server URL after server
// creation to avoid the chicken-and-egg dependency. The CallbackURL uses
// {{.Request.Host}} so it resolves to the proxy address at request time — no
// hard-coded port is needed.
func TestOIDCProfileSetup(t *testing.T) {
	// callbackTemplate is used as the CallbackURL in all test profiles. The
	// template is resolved to http://<proxy-host>/redirect at request time.
	const callbackTemplate = `http://{{.Request.Host}}/redirect`

	for _, tc := range []struct {
		msg string
		// profiles holds the profiles to register WITHOUT IdpURL (injected below).
		profiles     map[string]OidcProfile
		routeFilters string // eskip filter chain for the injected route
		expected     int    // expected HTTP status after the full OIDC flow
	}{
		{
			msg: "static profile credentials — full auth code flow succeeds",
			profiles: map[string]OidcProfile{
				"myprofile": {
					ClientID:     validClient,
					ClientSecret: "mysec",
					CallbackURL:  callbackTemplate,
				},
			},
			routeFilters: `oauthOidcAnyClaims("profile:myprofile", "uid")`,
			expected:     200,
		},
		{
			msg: "annotation-injected client-id — filter chain annotate() -> profile filter succeeds",
			profiles: map[string]OidcProfile{
				"myprofile": {
					ClientID:     `{{index .Annotations "client-id"}}`,
					ClientSecret: "mysec",
					CallbackURL:  callbackTemplate,
				},
			},
			routeFilters: `annotate("client-id", "valid-client") -> oauthOidcAnyClaims("profile:myprofile", "uid")`,
			expected:     200,
		},
		{
			msg: "wrong client-id from annotation — OIDC server rejects, returns 401",
			profiles: map[string]OidcProfile{
				"myprofile": {
					ClientID:     `{{index .Annotations "client-id"}}`,
					ClientSecret: "mysec",
					CallbackURL:  callbackTemplate,
				},
			},
			routeFilters: `annotate("client-id", "wrong-client") -> oauthOidcAnyClaims("profile:myprofile", "uid")`,
			expected:     401,
		},
		{
			msg: "unknown profile name — CreateFilter error, route not loaded, returns 404",
			profiles: map[string]OidcProfile{
				"myprofile": {
					ClientID:     validClient,
					ClientSecret: "mysec",
					CallbackURL:  callbackTemplate,
				},
			},
			// Route references a profile that does not exist → CreateFilter returns
			// ErrInvalidFilterParameters → routing engine drops the route → 404.
			routeFilters: `oauthOidcAnyClaims("profile:nonexistent", "uid")`,
			expected:     404,
		},
		{
			msg: "templated IdpURL — CreateFilter rejects, route not loaded, returns 404",
			profiles: map[string]OidcProfile{
				"bad-idp": {
					// IdpURL must be static; template expressions are rejected in CreateFilter.
					// Note: IdpURL is set at test runtime below; the prefix test here relies on
					// the special marker that triggers the static-check branch.
					ClientID:    validClient,
					CallbackURL: callbackTemplate,
					// IdpURL contains "{{" — injected below after the oidcServer URL is known.
					// We store a marker so the injection below can recognise this case.
				},
			},
			// The route uses this bad profile → CreateFilter returns error → 404.
			routeFilters: `oauthOidcAnyClaims("profile:bad-idp", "uid")`,
			expected:     404,
		},
		{
			msg: "profile with allClaims check — all required claims present, returns 200",
			profiles: map[string]OidcProfile{
				"myprofile": {
					ClientID:     validClient,
					ClientSecret: "mysec",
					CallbackURL:  callbackTemplate,
					Scopes:       "uid",
				},
			},
			routeFilters: `oauthOidcAllClaims("profile:myprofile", "sub uid")`,
			expected:     200,
		},
		{
			msg: "profile with userInfo check — userinfo endpoint consulted, returns 200",
			profiles: map[string]OidcProfile{
				"myprofile": {
					ClientID:     validClient,
					ClientSecret: "mysec",
					CallbackURL:  callbackTemplate,
					Scopes:       "uid",
				},
			},
			routeFilters: `oauthOidcUserInfo("profile:myprofile", "uid")`,
			expected:     200,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			oidcServer := createFlexOIDCServer(validClient, "mysec", nil, nil)
			defer oidcServer.Close()

			// Inject IdpURL into each profile now that the server URL is known.
			// For the "templated IdpURL" case, set it to a value that contains "{{".
			profiles := make(map[string]OidcProfile, len(tc.profiles))
			for name, p := range tc.profiles {
				if name == "bad-idp" {
					// IdpURL with template expression — must be rejected at CreateFilter time.
					p.IdpURL = "http://{{.Request.Host}}/oidc"
				} else {
					p.IdpURL = oidcServer.URL
				}
				profiles[name] = p
			}

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("OK"))
			}))
			defer backend.Close()

			fd, err := os.CreateTemp("", "testSecrets")
			require.NoError(t, err)
			secretsFile := fd.Name()
			fd.Close()
			defer os.Remove(secretsFile)

			secretsRegistry := secrettest.NewTestRegistry()

			fr := make(filters.Registry)
			fr.Register(annotate.New())
			opts := OidcOptions{Profiles: profiles}
			fr.Register(NewOAuthOidcUserInfosWithOptions(secretsFile, secretsRegistry, opts))
			fr.Register(NewOAuthOidcAnyClaimsWithOptions(secretsFile, secretsRegistry, opts))
			fr.Register(NewOAuthOidcAllClaimsWithOptions(secretsFile, secretsRegistry, opts))

			dc := testdataclient.New(nil)
			defer dc.Close()

			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				DataClients: []routing.DataClient{dc},
			})
			defer proxy.Close()

			parsedFilters, err := eskip.ParseFilters(tc.routeFilters)
			require.NoError(t, err)

			r := &eskip.Route{
				Filters: parsedFilters,
				Backend: backend.URL,
			}

			proxy.Log.Reset()
			dc.Update([]*eskip.Route{r}, nil)
			err = proxy.Log.WaitFor("route settings applied", 10*time.Second)
			require.NoError(t, err)

			req, err := http.NewRequest("GET", proxy.URL, nil)
			require.NoError(t, err)

			client := http.Client{
				Timeout: 2 * time.Second,
				Jar:     newInsecureCookieJar(),
			}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tc.expected, resp.StatusCode)

			// For successful cases, verify that a session cookie was set.
			if tc.expected == 200 {
				cookies := client.Jar.Cookies(&url.URL{Host: req.URL.Host})
				assert.NotEmpty(t, cookies, "expected a session cookie after successful OIDC login")
			}
		})
	}
}
