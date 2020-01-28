package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gopkg.in/square/go-jose.v2"

	"github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/secrets"
	"github.com/zalando/skipper/secrets/secrettest"
)

const (
	testRedirectUrl   = "http://redirect-somewhere.com/some-path?arg=param"
	validClient       = "valid-client"
	validCode         = "valid-code"
	validRefreshToken = "valid-refresh-token"
	validAccessToken  = "valid-access-token"

	certPath = "../../skptesting/cert.pem"
	keyPath  = "../../skptesting/key.pem"
)

type insecureCookieJar struct {
	store map[string][]*http.Cookie
}

func newInsecureCookieJar() *insecureCookieJar {
	store := make(map[string][]*http.Cookie)
	return &insecureCookieJar{
		store: store,
	}
}

func (jar *insecureCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	jar.store[u.Hostname()] = cookies
}
func (jar *insecureCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return jar.store[u.Hostname()]
}

// returns a localhost instance implementation of an OpenID Connect
// server with configendpoint, tokenendpoint, authenticationserver endpoint, userinfor
// endpoint, jwks endpoint
func createOIDCServer(cb, client, clientsecret string) *httptest.Server {
	s := `{
 "issuer": "https://accounts.google.com",
 "authorization_endpoint": "https://accounts.google.com/o/oauth2/v2/auth",
 "token_endpoint": "https://oauth2.googleapis.com/token",
 "userinfo_endpoint": "https://openidconnect.googleapis.com/v1/userinfo",
 "revocation_endpoint": "https://oauth2.googleapis.com/revoke",
 "jwks_uri": "https://www.googleapis.com/oauth2/v3/certs",
 "response_types_supported": [
  "code",
  "token",
  "id_token",
  "code token",
  "code id_token",
  "token id_token",
  "code token id_token",
  "none"
 ],
 "subject_types_supported": [
  "public"
 ],
 "id_token_signing_alg_values_supported": [
  "HS256",
  "RS256"
 ],
 "scopes_supported": [
  "openid",
  "email",
  "uid",
  "profile"
 ],
 "token_endpoint_auth_methods_supported": [
  "client_secret_post",
  "client_secret_basic"
 ],
 "claims_supported": [
  "aud",
  "email",
  "email_verified",
  "uid",
  "exp",
  "family_name",
  "given_name",
  "iat",
  "iss",
  "locale",
  "name",
  "picture",
  "sub"
 ],
 "code_challenge_methods_supported": [
  "plain",
  "S256",
  "HS256"
 ]
}`
	var oidcServer *httptest.Server
	oidcServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			// dynamic config handling
			// set oidcServer local dynamic listener to us
			st := strings.Replace(s, "https://accounts.google.com", oidcServer.URL, -1)
			st = strings.Replace(st, "https://oauth2.googleapis.com", oidcServer.URL, -1)
			st = strings.Replace(st, "https://www.googleapis.com", oidcServer.URL, -1)
			st = strings.Replace(st, "https://openidconnect.googleapis.com", oidcServer.URL, -1)
			_, _ = w.Write([]byte(st))
		case "/o/oauth2/v2/auth":
			// https://openid.net/specs/openid-connect-core-1_0.html#CodeFlowSteps
			// 3. Authorization Server Authenticates the End-User.
			// 4. Authorization Server obtains End-User Consent/Authorization.
			// 5. Authorization Server sends the End-User back to the Client with an Authorization Code.
			clientID := r.URL.Query().Get("client_id")
			if clientID != validClient {
				w.WriteHeader(401)
				return
			}
			redirectURI := r.URL.Query().Get("redirect_uri")
			if redirectURI == "" {
				w.WriteHeader(401)
				return
			}
			responseType := r.URL.Query().Get("response_type")
			if responseType != "code" {
				w.WriteHeader(500) // not implemented
				return
			}

			scopesString := r.URL.Query().Get("scope")
			scopes := strings.Fields(scopesString)
			supportedScopes := []string{"openid", "sub", "email", "uid", "profile"}
			allScopesSupported := all(scopes, supportedScopes)
			if !allScopesSupported {
				w.WriteHeader(401)
				return
			}

			// endpoint: https://openid.net/specs/openid-connect-core-1_0.html#AuthorizationEndpoint
			// auth based on
			// https://openid.net/specs/openid-connect-core-1_0.html#AuthRequest
			// https://openid.net/specs/openid-connect-core-1_0.html#AuthRequestValidation
			//
			// redirect if we have a callback
			if cb != "" {
				state := r.URL.Query().Get("state")
				u, err := url.Parse(cb + "?state=" + state + "&code=" + validCode)
				if err != nil {
					log.Fatalf("Failed to parse cb: %v", err)
				}

				// response: https://openid.net/specs/openid-connect-core-1_0.html#AuthResponse
				w.Header().Set("Location", u.String())
				w.WriteHeader(302)
			} else {
				// error response https://openid.net/specs/openid-connect-core-1_0.html#AuthError
				w.WriteHeader(401)
			}
		case "/token":
			// exchange https://openid.net/specs/openid-connect-core-1_0.html#TokenEndpoint
			// req validation https://openid.net/specs/openid-connect-core-1_0.html#ImplicitValidation
			// - Authenticate the Client if it was issued Client Credentials or if it uses another Client Authentication method, per Section 9.
			// - Ensure the Authorization Code was issued to the authenticated Client.
			// - Verify that the Authorization Code is valid.
			// - If possible, verify that the Authorization Code has not been previously used.
			// - Ensure that the redirect_uri parameter value is identical to the redirect_uri parameter value that was included in the initial Authorization Request. If the redirect_uri parameter value is not present when there is only one registered redirect_uri value, the Authorization Server MAY return an error (since the Client should have included the parameter) or MAY proceed without an error (since OAuth 2.0 permits the parameter to be omitted in this case).
			// - Verify that the Authorization Code used was issued in response to an OpenID Connect Authentication Request (so that an ID Token will be returned from the Token Endpoint).

			user, password, ok := r.BasicAuth()
			if ok && user == client && password == clientsecret {
				// https://openid.net/specs/openid-connect-core-1_0.html#TokenResponse

				if err := r.ParseForm(); err != nil {
					log.Fatalf("Failed to parse form: %v", err)
				}

				code := r.Form.Get("code")
				if code != validCode {
					w.WriteHeader(401)
					return
				}
				redirectURI := r.Form.Get("redirect_uri")
				if redirectURI != cb {
					w.WriteHeader(401)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Pragma", "no-cache")

				token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
					testKey: testValue, // claims to check
					"iss":   oidcServer.URL,
					"sub":   testSub,
					"aud":   validClient,
					"iat":   time.Now().Add(-time.Minute).UTC().Unix(),
					"exp":   time.Now().Add(time.Hour).UTC().Unix(),
				})

				privKey, err := ioutil.ReadFile(keyPath)
				if err != nil {
					log.Fatalf("Failed to read priv key: %v", err)
				}

				key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privKey))
				if err != nil {
					log.Fatalf("Failed to parse RSA PEM: %v", err)
				}

				// Sign and get the complete encoded token as a string using the secret
				validIDToken, err := token.SignedString(key)
				if err != nil {
					log.Fatalf("Failed to sign token: %v", err)
				}

				body := fmt.Sprintf(`{"access_token": "%s", "token_type": "Bearer", "refresh_token": "%s", "expires_in": 3600, "id_token": "%s"}`, validAccessToken, validRefreshToken, validIDToken)
				w.Write([]byte(body))
				return

			} else {
				// https://openid.net/specs/openid-connect-core-1_0.html#TokenErrorResponse
				w.WriteHeader(401)
			}
		case "/v1/userinfo":
			token := r.Header.Get(authHeaderName)
			if token == authHeaderPrefix+validAccessToken {
				body := fmt.Sprintf(`{"sub": "%s", "email": "%s", "email_verified": true}`, testSub, fmt.Sprintf("%s@example.org", testUID))
				w.Write([]byte(body))
			} else {
				w.WriteHeader(401)
			}
		case "/revoke":
			log.Fatalln("oidcServer /revoke - not implemented")
		case "/oauth2/v3/certs":
			certPEM, err := ioutil.ReadFile(certPath)
			if err != nil {
				log.Fatalf("Failed to readfile cert: %v", err)
			}
			pemDecodeCert, _ := pem.Decode(certPEM)
			cert, err := x509.ParseCertificate(pemDecodeCert.Bytes)
			if err != nil {
				log.Fatalf("Failed to parse cert: %v", err)
			}

			privPEM, err := ioutil.ReadFile(keyPath)
			if err != nil {
				log.Fatalf("Failed to readfile key: %v", err)
			}
			pemDecodePriv, _ := pem.Decode(privPEM)
			privKey, err := x509.ParsePKCS8PrivateKey(pemDecodePriv.Bytes)
			if err != nil {
				log.Fatalf("Failed to parse PKCS8 privkey: %v", err)
			}
			rsaPrivKey, ok := privKey.(*rsa.PrivateKey)
			if !ok {
				log.Fatal("Failed to convert privkey to rsa.PrivateKey")
			}

			j := jose.JSONWebKeySet{
				Keys: []jose.JSONWebKey{
					{
						Key:          &rsaPrivKey.PublicKey,
						Certificates: []*x509.Certificate{cert},
						KeyID:        "",
						Algorithm:    "RS256",
						Use:          "sig",
					},
				},
			}
			b, err := json.Marshal(j)
			if err != nil {
				log.Fatalf("Failed to marshal to json: %v", err)
			}

			_, err = w.Write(b)
			if err != nil {
				log.Fatalf("Failed to write: %v", err)
			}

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return oidcServer
}

func makeTestingFilter(claims []string) (*tokenOidcFilter, error) {
	r := secrettest.NewTestRegistry()
	encrypter, err := r.GetEncrypter(5*time.Second, "key")
	if err != nil {
		return nil, err
	}

	f := &tokenOidcFilter{
		typ:    checkOIDCAnyClaims,
		claims: claims,
		config: &oauth2.Config{
			ClientID: "test",
			Endpoint: google.Endpoint,
		},
		encrypter: encrypter,
	}
	return f, err
}

func TestEncryptDecryptState(t *testing.T) {
	f, err := makeTestingFilter([]string{})
	assert.NoError(t, err, "could not refresh ciphers")

	nonce, err := f.encrypter.CreateNonce()
	if err != nil {
		t.Errorf("Failed to create nonce: %v", err)
	}

	// enc
	state, err := createState(nonce, testRedirectUrl)
	assert.NoError(t, err, "failed to create state")
	stateEnc, err := f.encrypter.Encrypt(state)
	if err != nil {
		t.Errorf("Failed to encrypt data block: %v", err)
	}
	stateEncHex := fmt.Sprintf("%x", stateEnc)

	// dec
	stateQueryEnc := make([]byte, len(stateEncHex))
	if _, err = fmt.Sscanf(stateEncHex, "%x", &stateQueryEnc); err != nil && err != io.EOF {
		t.Errorf("Failed to read hex string: %v", err)
	}
	stateQueryPlain, err := f.encrypter.Decrypt(stateQueryEnc)
	if err != nil {
		t.Errorf("token from state query is invalid: %v", err)
	}

	// test same
	if len(stateQueryPlain) != len(state) {
		t.Errorf("encoded and decoded states do no match")
	}
	for i, b := range stateQueryPlain {
		if b != state[i] {
			t.Errorf("encoded and decoded states do no match")
			break
		}
	}
	decOauthState, err := extractState(stateQueryPlain)
	if err != nil {
		t.Errorf("failed to recreate state from decrypted byte array.")
	}
	ts := time.Unix(decOauthState.Validity, 0)
	if time.Now().After(ts) {
		t.Errorf("now is after time from state but should be before: %s", ts)
	}

	if decOauthState.RedirectUrl != testRedirectUrl {
		t.Errorf("Decrypted Redirect Url %s does not match input %s", decOauthState.RedirectUrl, testRedirectUrl)
	}
}

func TestOidcValidateAllClaims(t *testing.T) {
	oidcFilter, err := makeTestingFilter([]string{"uid", "email"})
	assert.NoError(t, err, "error creating test filter")
	assert.True(t, oidcFilter.validateAllClaims(
		map[string]interface{}{"uid": "test", "email": "test@example.org"}),
		"claims should be valid but filter returned false.")
	assert.False(t, oidcFilter.validateAllClaims(
		map[string]interface{}{}), "claims are invalid but filter returned true.")
	assert.False(t, oidcFilter.validateAllClaims(
		map[string]interface{}{"uid": "test"}),
		"claims are not enough but filter returned true.")
	assert.False(t, oidcFilter.validateAllClaims(
		map[string]interface{}{}),
		"no claims but filter returned true.")
}

func TestOidcValidateAnyClaims(t *testing.T) {
	oidcFilter, err := makeTestingFilter([]string{"uid", "test", "email"})
	assert.NoError(t, err, "error creating test filter")
	assert.True(t, oidcFilter.validateAnyClaims(
		map[string]interface{}{"uid": "test", "email": "test@example.org"}),
		"claims should be valid but filter returned false.")
	assert.False(t, oidcFilter.validateAnyClaims(
		map[string]interface{}{}), "claims are invalid but filter returned true.")
	assert.True(t, oidcFilter.validateAnyClaims(
		map[string]interface{}{"foo": "test", "email": "test@example.org"}),
		"claims are valid but filter returned false.")
	assert.True(t, oidcFilter.validateAnyClaims(
		map[string]interface{}{"uid": "test", "email": "test@example.org", "hd": "something.com", "empty": ""}),
		"claims are valid but filter returned false.")
}

func TestExtractDomainFromHost(t *testing.T) {

	for _, ht := range []struct {
		given    string
		expected string
	}{
		{"localhost", "localhost"},
		{"localhost.localdomain", "localhost.localdomain"},
		{"www.example.local", "example.local"},
		{"one.two.three.www.example.local", "two.three.www.example.local"},
		{"localhost:9990", "localhost"},
		{"www.example.local:9990", "example.local"},
		{"127.0.0.1:9090", "127.0.0.1"},
	} {
		t.Run(fmt.Sprintf("test:%s", ht.given), func(t *testing.T) {
			got := extractDomainFromHost(ht.given)
			assert.Equal(t, ht.expected, got)
		})
	}
}

func TestNewOidc(t *testing.T) {
	reg := secrets.NewRegistry()
	for _, tt := range []struct {
		name string
		args string
		f    func(string, *secrets.Registry) filters.Spec
		want *tokenOidcSpec
	}{
		{
			name: "test UserInfo",
			args: "/foo",
			f:    NewOAuthOidcUserInfos,
			want: &tokenOidcSpec{typ: checkOIDCUserInfo, SecretsFile: "/foo", secretsRegistry: reg},
		},
		{
			name: "test AnyClaims",
			args: "/foo",
			f:    NewOAuthOidcAnyClaims,
			want: &tokenOidcSpec{typ: checkOIDCAnyClaims, SecretsFile: "/foo", secretsRegistry: reg},
		},
		{
			name: "test AllClaims",
			args: "/foo",
			f:    NewOAuthOidcAllClaims,
			want: &tokenOidcSpec{typ: checkOIDCAllClaims, SecretsFile: "/foo", secretsRegistry: reg},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f(tt.args, reg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Failed to create object: Want %v, got %v", tt.want, got)
			}
		})
	}

}

func TestCreateFilterOIDC(t *testing.T) {
	oidcServer := createOIDCServer("", "", "")
	defer oidcServer.Close()

	for _, tt := range []struct {
		name    string
		args    []interface{}
		wantErr bool
	}{
		{
			name:    "test no args",
			args:    nil,
			wantErr: true,
		},
		{
			name:    "test wrong number of args",
			args:    []interface{}{"s"},
			wantErr: true,
		},
		{
			name:    "test wrong number of args",
			args:    []interface{}{"s", "d"},
			wantErr: true,
		},
		{
			name:    "test wrong number of args",
			args:    []interface{}{"s", "d", "a"},
			wantErr: true,
		},
		{
			name:    "test wrong args",
			args:    []interface{}{"s", "d", "a", "f"},
			wantErr: true,
		},
		{
			name: "test minimal args",
			args: []interface{}{
				oidcServer.URL,               // provider/issuer
				"",                           // client ID
				"",                           // client secret
				oidcServer.URL + "/redirect", // redirect URL
				"",                           // scopes
				"",                           // claims
			},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := &tokenOidcSpec{
				typ:             checkOIDCAllClaims,
				SecretsFile:     "/foo",
				secretsRegistry: secrettest.NewTestRegistry(),
			}

			got, err := spec.CreateFilter(tt.args)
			if tt.wantErr && err == nil {
				t.Errorf("Failed to get error but wanted, got: %v", got)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Failed to get no error: %v", err)
			}

			if got == nil && !tt.wantErr {
				t.Errorf("Failed to create filter: got %v", got)
			}
		})
	}

}

func TestOIDCSetup(t *testing.T) {
	for _, tc := range []struct {
		msg          string
		provider     string
		client       string
		clientsecret string
		scopes       []string
		claims       []string
		authType     roleCheckType
		expected     int
		expectErr    bool
	}{{
		msg:       "wrong provider",
		provider:  "no url",
		expectErr: true,
	}, {
		msg:          "has authType, checkOIDCAnyClaims without claims",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCAnyClaims,
		scopes:       []string{testKey, "email"},
		expected:     200,
	}, {
		msg:          "has authType, userinfo valid without claims requested",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCUserInfo,
		expected:     200,
		expectErr:    false,
	}, {
		msg:          "has authType, userinfo valid with claims requested",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCUserInfo,
		expected:     200,
		expectErr:    false,
		scopes:       []string{testKey, "email"},
		claims:       []string{testKey},
	}, {
		msg:          "has authType, userinfo with invalid claims requested",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCUserInfo,
		expected:     401,
		expectErr:    false,
		scopes:       []string{testKey, "email"},
		claims:       []string{testKey, "invalid"},
	}, {
		msg:          "has authType, userinfo with not existed claims requested",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCUserInfo,
		expected:     401,
		expectErr:    false,
		scopes:       []string{testKey, "email"},
		claims:       []string{"does-not-exist"},
	}, {
		msg:          "has authType, any claims 1 valid",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCAnyClaims,
		expected:     200,
		expectErr:    false,
		scopes:       []string{testKey, "email"},
		claims:       []string{testKey},
	}, {
		msg:          "has authType, any claims valid and invalid",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCAnyClaims,
		expected:     200,
		expectErr:    false,
		scopes:       []string{testKey, "email"},
		claims:       []string{testKey, "testKey"},
	}, {
		msg:          "has authType, any claims invalid",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCAnyClaims,
		expected:     401,
		expectErr:    false,
		scopes:       []string{testKey, "email"},
		claims:       []string{"testKey"},
	}, {
		msg:          "has authType, all claims valid",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCAllClaims,
		expected:     200,
		expectErr:    false,
		scopes:       []string{"uid"},
		claims:       []string{"sub", "uid"},
	}, {
		msg:          "has authType, all claims valid and scopes",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCAllClaims,
		expected:     401,
		expectErr:    false,
		scopes:       []string{"invalid"},
		claims:       []string{testKey},
	}, {
		msg:          "has authType, all claims valid and invalid",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCAllClaims,
		expected:     401,
		expectErr:    false,
		scopes:       []string{testKey, "email"},
		claims:       []string{testKey, "testKey"},
	}, {
		msg:          "has authType, all claims invalid",
		client:       validClient,
		clientsecret: "mysec",
		authType:     checkOIDCAllClaims,
		expected:     401,
		expectErr:    false,
		scopes:       []string{testKey, "email"},
		claims:       []string{"testKey"},
	}} {
		t.Run(tc.msg, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Logf("backend got request: %+v", r)
				w.Write([]byte("OK"))
			}))
			defer backend.Close()
			t.Logf("backend URL: %s", backend.URL)

			spec := &tokenOidcSpec{
				typ:             tc.authType,
				SecretsFile:     "/tmp/foo", // TODO(sszuecs): random
				secretsRegistry: secrettest.NewTestRegistry(),
			}
			fr := make(filters.Registry)
			fr.Register(spec)
			dc := testdataclient.New(nil)
			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				DataClients: []routing.DataClient{dc},
				Log:         loggingtest.New(),
			})
			defer proxy.Close()
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
			}

			oidcServer := createOIDCServer(proxy.URL+"/redirect", tc.client, tc.clientsecret)
			defer oidcServer.Close()
			t.Logf("oidc/auth server URL: %s", oidcServer.URL)

			// create filter
			sargs := []interface{}{
				tc.client,
				tc.clientsecret,
				proxy.URL + "/redirect",
			}

			// test that, we get an error if provider is no url
			if tc.provider != "" {
				sargs = append([]interface{}{tc.provider}, sargs...)
			} else {
				sargs = append([]interface{}{oidcServer.URL}, sargs...)
			}

			sargs = append(sargs, strings.Join(tc.scopes, " "))
			sargs = append(sargs, strings.Join(tc.claims, " "))

			f, err := spec.CreateFilter(sargs)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("Want error but got filter: %v", f)
				}
				return //OK
			} else if err != nil {
				t.Fatalf("Unexpected error while creating filter: %v", err)
			}
			fOIDC := f.(*tokenOidcFilter)
			defer fOIDC.Close()

			r := &eskip.Route{
				Filters: []*eskip.Filter{{
					Name: spec.Name(),
					Args: sargs,
				}},
				Backend: backend.URL,
			}

			proxy.Log.Reset()
			dc.Update([]*eskip.Route{r}, nil)
			if err = proxy.Log.WaitFor("route settings applied", 1*time.Second); err != nil {
				t.Fatalf("Failed to get update: %v", err)
			}

			// do request through proxy
			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set(authHeaderName, authHeaderPrefix+testToken)

			// client with cookie handling to support 127.0.0.1 with ports
			client := http.Client{
				Timeout: 1 * time.Second,
				Jar:     newInsecureCookieJar(),
			}

			// trigger OpenID Connect Authorization Code Flow
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("req: %+v: %v", req, err)
				return
			}
			defer resp.Body.Close()

			var b []byte
			if resp.StatusCode != tc.expected {
				t.Logf("response: %+v", resp)
				t.Errorf("auth filter failed got=%d, expected=%d, route=%s", resp.StatusCode, tc.expected, r)
				b, err = ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}
			}
			bs := string(b)
			t.Logf("Got body: %s", bs)
		})
	}
}

func TestChunkAndMergerCookie(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	tinyCookie := http.Cookie{
		Name:     "skipperOauthOidcHASHHASH-",
		Value:    "eyJ0eXAiOiJKV1QiLCJhbGciO",
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		MaxAge:   3600,
		Domain:   "www.example.com",
	}
	emptyCookie := tinyCookie
	emptyCookie.Value = ""
	largeCookie := tinyCookie
	for i := 0; i < 5*cookieMaxSize; i++ {
		largeCookie.Value += string(rand.Intn('Z'-'A') + 'A' + i%2*32)
	}
	oneCookie := largeCookie
	oneCookie.Value = oneCookie.Value[:len(oneCookie.Value)-(len(oneCookie.String())-cookieMaxSize)-1]
	twoCookies := largeCookie
	twoCookies.Value = twoCookies.Value[:len(twoCookies.Value)-(len(twoCookies.String())-cookieMaxSize)]

	for _, ht := range []struct {
		name   string
		given  http.Cookie
		num int
	}{
		{"short cookie", tinyCookie, 1},
		{"cookie without content", emptyCookie, 1},
		{"large cookie == 6 chunks", largeCookie, 6},
		{"fits exactly into one cookie", oneCookie, 1},
		{"chunked up cookie", twoCookies, 2},
	} {
		t.Run(fmt.Sprintf("test:%v", ht.name), func(t *testing.T) {
			assert := assert.New(t)
			got := chunkCookie(ht.given)
			assert.NotNil(t, got, "it should not be empty")
			// shuffle the order of response cookies
			rand.Shuffle(len(got), func(i, j int) {
				got[i], got[j] = got[j], got[i]
			})
			assert.Len(got, ht.num, "should result in a different number of chunks")
			ck := mergerCookies(got)
			assert.NotNil(ck, "should receive a valid cookie")
			// verify no cookie exceeds limits
			for _, ck := range got {
				assert.True(func() bool {
					return len(ck.String()) <= cookieMaxSize
				}(), "its size should not exceed limits cookieMaxSize")
			}
			assert.Equal(ht.given, ck, "after chunking and remerging the content must be equal")
		})
	}
}
