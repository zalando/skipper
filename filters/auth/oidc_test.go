package auth

import (
	"cmp"
	"compress/flate"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"text/template"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gopkg.in/go-jose/go-jose.v2"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net/dnstest"
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
	tokenExp          = 2 * time.Hour

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
	cookieMap := make(map[string]*http.Cookie)
	for _, c := range jar.store[u.Hostname()] {
		cookieMap[c.Name] = c
	}
	for _, c := range cookies {
		cookieMap[c.Name] = c
	}

	cookies = make([]*http.Cookie, 0, len(cookieMap))
	for _, c := range cookieMap {
		cookies = append(cookies, c)
	}

	jar.store[u.Hostname()] = cookies
}
func (jar *insecureCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return jar.store[u.Hostname()]
}

var testOpenIDConfig = `{
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

// returns a localhost instance implementation of an OpenID Connect
// server with configendpoint, tokenendpoint, authenticationserver endpoint, userinfor
// endpoint, jwks endpoint
func createOIDCServer(cb, client, clientsecret string, extraClaims jwt.MapClaims, removeClaims []string) *httptest.Server {
	var oidcServer *httptest.Server
	oidcServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			// dynamic config handling
			// set oidcServer local dynamic listener to us
			st := strings.ReplaceAll(testOpenIDConfig, "https://accounts.google.com", oidcServer.URL)
			st = strings.ReplaceAll(st, "https://oauth2.googleapis.com", oidcServer.URL)
			st = strings.ReplaceAll(st, "https://www.googleapis.com", oidcServer.URL)
			st = strings.ReplaceAll(st, "https://openidconnect.googleapis.com", oidcServer.URL)
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

				claims := jwt.MapClaims{
					testKey: testValue, // claims to check
					"iss":   oidcServer.URL,
					"sub":   testSub,
					"aud":   validClient,
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

				privKey, err := os.ReadFile(keyPath)
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
			certPEM, err := os.ReadFile(certPath)
			if err != nil {
				log.Fatalf("Failed to readfile cert: %v", err)
			}
			pemDecodeCert, _ := pem.Decode(certPEM)
			cert, err := x509.ParseCertificate(pemDecodeCert.Bytes)
			if err != nil {
				log.Fatalf("Failed to parse cert: %v", err)
			}

			privPEM, err := os.ReadFile(keyPath)
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
		case "/v1.0/users/me/transitiveMemberOf":
			if r.Header.Get(authHeaderName) == authHeaderPrefix+validAccessToken &&
				r.URL.Query().Get("$select") == "onPremisesSamAccountName,id" {
				body, err := json.Marshal(azureGraphGroups{
					OdataNextLink: fmt.Sprintf("http://%s/v1.0/users/paginatedresponse", r.Host),
					Value: []struct {
						OnPremisesSamAccountName string `json:"onPremisesSamAccountName"`
						ID                       string `json:"id"`
					}{
						{OnPremisesSamAccountName: "CD-Administrators", ID: "1"},
						{OnPremisesSamAccountName: "Purchasing-Department", ID: "2"},
					}})
				if err != nil {
					log.Fatalf("Failed to marshal to json: %v", err)
				}
				w.Write(body)
			} else {
				w.WriteHeader(401)
			}
		case "/v1.0/users/paginatedresponse":
			if r.Header.Get(authHeaderName) == authHeaderPrefix+validAccessToken {
				body, err := json.Marshal(azureGraphGroups{
					OdataNextLink: "",
					Value: []struct {
						OnPremisesSamAccountName string `json:"onPremisesSamAccountName"`
						ID                       string `json:"id"`
					}{
						{OnPremisesSamAccountName: "AppX-Test-Users", ID: "3"},
						{OnPremisesSamAccountName: "white space", ID: "4"},
						{ID: "5"}, // null value
					}})
				if err != nil {
					log.Fatalf("Failed to marshal to json: %v", err)
				}
				w.Write(body)
			} else {
				w.WriteHeader(401)
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
		given               string
		expected            string
		domainLevelToRemove int
	}{
		{"localhost", "localhost", 1},
		{"localhost.localdomain", "localhost.localdomain", 1},
		{"www.example.local", "example.local", 1},
		{"one.two.three.www.example.local", "two.three.www.example.local", 1},
		{"localhost:9990", "localhost", 1},
		{"www.example.local:9990", "example.local", 1},
		{"127.0.0.1:9090", "127.0.0.1", 1},
		{"www.example.com", "www.example.com", 0},
		{"example.com", "example.com", 1},
		{"test.app.example.com", "example.com", 2},
		{"test.example.com", "test.example.com", 2},
	} {
		t.Run(fmt.Sprintf("test:%s", ht.given), func(t *testing.T) {
			got := extractDomainFromHost(ht.given, ht.domainLevelToRemove)
			assert.Equal(t, ht.expected, got)
		})
	}
}

func TestNewOidc(t *testing.T) {
	reg := secrets.NewRegistry()
	for _, tt := range []struct {
		name    string
		args    string
		f       func(string, secrets.EncrypterCreator, OidcOptions) filters.Spec
		options OidcOptions
		want    *tokenOidcSpec
	}{
		{
			name:    "test UserInfo",
			args:    "/foo",
			f:       NewOAuthOidcUserInfosWithOptions,
			options: OidcOptions{},
			want:    &tokenOidcSpec{typ: checkOIDCUserInfo, SecretsFile: "/foo", secretsRegistry: reg},
		},
		{
			name:    "test AnyClaims",
			args:    "/foo",
			f:       NewOAuthOidcAnyClaimsWithOptions,
			options: OidcOptions{},
			want:    &tokenOidcSpec{typ: checkOIDCAnyClaims, SecretsFile: "/foo", secretsRegistry: reg},
		},
		{
			name:    "test AllClaims",
			args:    "/foo",
			f:       NewOAuthOidcAllClaimsWithOptions,
			options: OidcOptions{},
			want:    &tokenOidcSpec{typ: checkOIDCAllClaims, SecretsFile: "/foo", secretsRegistry: reg},
		},
		{
			name: "test UserInfo with options",
			args: "/foo",
			f:    NewOAuthOidcUserInfosWithOptions,
			options: OidcOptions{
				CookieValidity: 6 * time.Hour,
			},
			want: &tokenOidcSpec{typ: checkOIDCUserInfo, SecretsFile: "/foo", secretsRegistry: reg, options: OidcOptions{CookieValidity: 6 * time.Hour}},
		},
		{
			name: "test AnyClaims with options",
			args: "/foo",
			f:    NewOAuthOidcAnyClaimsWithOptions,
			options: OidcOptions{
				CookieValidity: 6 * time.Hour,
			},
			want: &tokenOidcSpec{typ: checkOIDCAnyClaims, SecretsFile: "/foo", secretsRegistry: reg, options: OidcOptions{CookieValidity: 6 * time.Hour}},
		},
		{
			name: "test AllClaims with options",
			args: "/foo",
			f:    NewOAuthOidcAllClaimsWithOptions,
			options: OidcOptions{
				CookieValidity: 6 * time.Hour,
			},
			want: &tokenOidcSpec{typ: checkOIDCAllClaims, SecretsFile: "/foo", secretsRegistry: reg, options: OidcOptions{CookieValidity: 6 * time.Hour}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f(tt.args, reg, tt.options); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Failed to create object: Want %v, got %v", tt.want, got)
			}
		})
	}

}

func TestCreateFilterOIDC(t *testing.T) {
	oidcServer := createOIDCServer("", "", "", nil, nil)
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
		{
			name: "wrong provider",
			args: []interface{}{
				"invalid url",                  // provider/issuer
				"",                             // client ID
				"",                             // client secret
				"http://skipper.test/redirect", // redirect URL
				"",                             // scopes
				"",                             // claims
			},
			wantErr: true,
		},
		{
			name: "invalid auth code option",
			args: []interface{}{
				oidcServer.URL,               // provider/issuer
				"",                           // client ID
				"",                           // client secret
				oidcServer.URL + "/redirect", // redirect URL
				"",                           // scopes
				"",                           // claims
				"foo",                        // auth code options
			},
			wantErr: true,
		},
		{
			name: "unparsable value of subdomainsToRemove",
			args: []interface{}{
				oidcServer.URL,               // provider/issuer
				"",                           // client ID
				"",                           // client secret
				oidcServer.URL + "/redirect", // redirect URL
				"",                           // scopes
				"",                           // claims
				"",                           // auth code options
				"",                           // upstream headers
				"sdsd",                       // subdomains to remove
			},
			wantErr: true,
		},
		{
			name: "negative value of subdomainsToRemove",
			args: []interface{}{
				oidcServer.URL,               // provider/issuer
				"",                           // client ID
				"",                           // client secret
				oidcServer.URL + "/redirect", // redirect URL
				"",                           // scopes
				"",                           // claims
				"",                           // auth code options
				"",                           // upstream headers
				"-1",                         // subdomains to remove
			},
			wantErr: true,
		},
		{
			name: "missing claims result in error",
			args: []interface{}{
				oidcServer.URL, // provider/issuer
				"cliendId",
				"clientSecret",
				oidcServer.URL + "/redirect", // redirect URL
				"email name",
			},
			wantErr: true,
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
	dnstest.LoopbackNames(t, "skipper.test", "foo.skipper.test", "bar.foo.skipper.test")

	setHostname := func(u *url.URL, name string) {
		if name != "" {
			u.Host = net.JoinHostPort(name, u.Port())
		}
	}

	for _, tc := range []struct {
		msg                string
		hostname           string
		filter             string
		queries            []string
		cookies            []*http.Cookie
		expected           int
		expectRequest      string
		expectNoCookies    bool
		expectCookieDomain string
		filterCookies      []string
		extraClaims        jwt.MapClaims
		expectCookieName   string
	}{{
		msg:             "wrong provider",
		filter:          `oauthOidcAnyClaims("no url", "", "", "{{ .RedirectURL }}", "", "")`,
		expected:        404, // fails to create filter and route due to invalid provider
		expectNoCookies: true,
	}, {
		msg:      "has authType, checkOIDCAnyClaims without claims",
		filter:   `oauthOidcAnyClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "")`,
		expected: 200,
	}, {
		msg:      "has authType, userinfo valid without claims requested",
		filter:   `oauthOidcUserInfo("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "", "")`,
		expected: 200,
	}, {
		msg:      "has authType, userinfo valid with claims requested",
		filter:   `oauthOidcUserInfo("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "uid")`,
		expected: 200,
	}, {
		msg:      "has authType, userinfo with invalid claims requested",
		filter:   `oauthOidcUserInfo("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "uid invalid")`,
		expected: 401,
	}, {
		msg:      "has authType, userinfo with not existed claims requested",
		filter:   `oauthOidcUserInfo("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "does-not-exist")`,
		expected: 401,
	}, {
		msg:      "has authType, any claims 1 valid",
		filter:   `oauthOidcAnyClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "uid")`,
		expected: 200,
	}, {
		msg:      "has authType, any claims valid and invalid",
		filter:   `oauthOidcAnyClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "uid invalid")`,
		expected: 200,
	}, {
		msg:      "has authType, any claims invalid",
		filter:   `oauthOidcAnyClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "invalid")`,
		expected: 401,
	}, {
		msg:      "has authType, all claims valid",
		filter:   `oauthOidcAllClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid", "sub uid")`,
		expected: 200,
	}, {
		msg:             "has authType, all claims: valid claims and invalid scope",
		filter:          `oauthOidcAllClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "invalid", "uid")`,
		expected:        401,
		expectNoCookies: true, // 401 returned by OIDC server due to invalid scope before redirect
	}, {
		msg:      "has authType, all claims valid and invalid",
		filter:   `oauthOidcAllClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "uid invalid")`,
		expected: 401,
	}, {
		msg:      "has authType, all claims invalid",
		filter:   `oauthOidcAllClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "invalid")`,
		expected: 401,
	}, {
		msg: "custom upstream headers",
		filter: `oauthOidcAllClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid", "sub uid", "",
			"x-auth-email:claims.email x-auth-something:claims.sub x-auth-groups:claims.groups.#[%\"*-Users\"]"
		)`,
		extraClaims:   jwt.MapClaims{"groups": []string{"CD-Administrators", "Purchasing-Department", "AppX-Test-Users", "white space"}},
		expected:      200,
		expectRequest: "X-Auth-Email: someone@example.org\r\nX-Auth-Groups: AppX-Test-Users\r\nX-Auth-Something: somesub",
	}, {
		msg: "distributed Azure claims looked up in Microsoft Graph ",
		filter: `oauthOidcAllClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid", "sub uid", "",
		"x-auth-email:claims.email x-auth-something:claims.sub x-auth-groups:claims.groups.#[%\"*-Users\"]")`,
		extraClaims: jwt.MapClaims{
			"oid":            "me",
			"_claim_names":   map[string]string{"groups": "src1"},
			"_claim_sources": map[string]map[string]string{"src1": {"endpoint": "http://graph.windows.net/distributedClaims/getMemberObjects"}},
		},
		expected:      200,
		expectRequest: "X-Auth-Email: someone@example.org\r\nX-Auth-Groups: AppX-Test-Users\r\nX-Auth-Something: somesub\r\n\r\n",
	}, {
		msg: "distributed Azure claims with pagination resolved",
		filter: `oauthOidcAllClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid", "sub uid", "",
		"x-auth-groups:claims.groups")`,
		extraClaims: jwt.MapClaims{
			"oid":            "me",
			"_claim_names":   map[string]string{"groups": "src1"},
			"_claim_sources": map[string]map[string]string{"src1": {"endpoint": "http://graph.windows.net/distributedClaims/getMemberObjects"}},
		},
		expected:      200,
		expectRequest: "X-Auth-Groups: [\"CD-Administrators\",\"Purchasing-Department\",\"AppX-Test-Users\",\"white space\"]\r\n\r\n",
	}, {
		msg: "distributed claims on unsupported IdP no groups claim returned",
		filter: `oauthOidcAllClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid", "sub uid", "",
	"x-auth-email:claims.email x-auth-something:claims.sub x-auth-groups:claims.groups.#[%\"*-Users\"]")`,
		extraClaims: jwt.MapClaims{
			"oid":            "me",
			"_claim_names":   map[string]string{"groups": "src1"},
			"_claim_sources": map[string]map[string]string{"src1": {"endpoint": "http://unknown.com/someendpoint"}},
		},
		expected:        401,
		expectNoCookies: true,
	}, {
		msg:      "auth code with a placeholder and a regular option",
		filter:   `oauthOidcAnyClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "", "", "foo=skipper-request-query bar=baz")`,
		queries:  []string{"foo=bar"},
		expected: 200,
	}, {
		msg:                "cookie domain for skipper.test",
		hostname:           "skipper.test",
		filter:             `oauthOidcUserInfo("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "", "")`,
		expected:           200,
		expectCookieDomain: "skipper.test",
	}, {
		msg:                "cookie domain for foo.skipper.test",
		hostname:           "foo.skipper.test",
		filter:             `oauthOidcUserInfo("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "", "")`,
		expected:           200,
		expectCookieDomain: "skipper.test",
	}, {
		msg:                "cookie domain for bar.foo.skipper.test",
		hostname:           "bar.foo.skipper.test",
		filter:             `oauthOidcUserInfo("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "", "")`,
		expected:           200,
		expectCookieDomain: "foo.skipper.test",
	}, {
		msg:                "do not remove any subdomain",
		hostname:           "foo.skipper.test",
		filter:             `oauthOidcAnyClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "", "", "", "0")`,
		expected:           200,
		expectCookieDomain: "foo.skipper.test",
	}, {
		msg:                "do not remove subdomains if less then 2 levels reimains",
		hostname:           "bar.foo.skipper.test",
		filter:             `oauthOidcAnyClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "", "", "", "3")`,
		expected:           200,
		expectCookieDomain: "bar.foo.skipper.test",
	}, {
		msg:                "remove unverified cookies",
		hostname:           "bar.foo.skipper.test",
		filter:             `oauthOidcAnyClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "uid email", "", "", "", "3")`,
		expected:           200,
		expectCookieDomain: "bar.foo.skipper.test",
		filterCookies:      []string{"badheader", "malformed"},
	}, {
		msg:              "custom cookie name",
		filter:           `oauthOidcUserInfo("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "", "", "", "", "", "custom-cookie")`,
		expected:         200,
		expectCookieName: "custom-cookie",
	}, {
		msg:              "default cookie name when not specified",
		filter:           `oauthOidcUserInfo("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "", "")`,
		expected:         200,
		expectCookieName: "skipperOauthOidc",
	}, {
		msg:                "cookies should be forwarded",
		hostname:           "skipper.test",
		filter:             `oauthOidcUserInfo("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "", "")`,
		cookies:            []*http.Cookie{{Name: "please-forward", Value: "me", Domain: "skipper.test", MaxAge: 7200}},
		expected:           200,
		expectRequest:      "please-forward=me",
		expectCookieDomain: "skipper.test",
	}} {
		t.Run(tc.msg, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestDump, _ := httputil.DumpRequest(r, false)
				assert.Contains(t, string(requestDump), tc.expectRequest, "expected request not fulfilled")
				assert.NotContains(t, string(requestDump), cmp.Or(tc.expectCookieName, oauthOidcCookieName), "oidc cookie should be dropped")
				w.Write([]byte("OK"))
			}))
			defer backend.Close()
			t.Logf("backend URL: %s", backend.URL)

			fd, err := os.CreateTemp("", "testSecrets")
			if err != nil {
				t.Fatal(err)
			}
			secretsFile := fd.Name()
			defer func() { os.Remove(secretsFile) }()

			secretsRegistry := secrettest.NewTestRegistry()

			fr := make(filters.Registry)
			fr.Register(NewOAuthOidcUserInfosWithOptions(secretsFile, secretsRegistry, OidcOptions{}))
			fr.Register(NewOAuthOidcAnyClaimsWithOptions(secretsFile, secretsRegistry, OidcOptions{}))
			fr.Register(NewOAuthOidcAllClaimsWithOptions(secretsFile, secretsRegistry, OidcOptions{}))

			dc := testdataclient.New(nil)
			defer dc.Close()

			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				DataClients: []routing.DataClient{dc},
			})
			defer proxy.Close()

			reqURL, _ := url.Parse(proxy.URL)
			setHostname(reqURL, tc.hostname)

			redirectURL, _ := url.Parse(proxy.URL)
			setHostname(redirectURL, tc.hostname)
			redirectURL.Path = "/redirect"

			t.Logf("redirect URL: %s", redirectURL.String())

			oidcServer := createOIDCServer(redirectURL.String(), "valid-client", "mysec", tc.extraClaims, nil)
			defer oidcServer.Close()
			t.Logf("oidc server URL: %s", oidcServer.URL)

			oidcsrv, err := url.Parse(oidcServer.URL)
			assert.NoError(t, err)
			microsoftGraphHost = oidcsrv.Host

			if tc.queries != nil {
				q := reqURL.Query()
				for _, rq := range tc.queries {
					k, v, _ := strings.Cut(rq, "=")
					q.Add(k, v)
				}
				reqURL.RawQuery = q.Encode()
			}

			filters, err := parseFilter(tc.filter, oidcServer.URL, redirectURL.String())
			if err != nil {
				t.Fatal(err)
			}
			r := &eskip.Route{
				Filters: filters,
				Backend: backend.URL,
			}

			proxy.Log.Reset()
			dc.Update([]*eskip.Route{r}, nil)
			if err = proxy.Log.WaitFor("route settings applied", 10*time.Second); err != nil {
				t.Fatalf("Failed to get update: %v", err)
			}

			// do request through proxy
			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set(authHeaderName, authHeaderPrefix+testToken)

			for _, v := range tc.filterCookies {
				req.Header.Set("Set-Cookie", v)
			}

			// client with cookie handling to support 127.0.0.1 with ports
			client := http.Client{
				Timeout: 1 * time.Second,
				Jar:     newInsecureCookieJar(),
			}
			client.Jar.SetCookies(reqURL, tc.cookies)

			// trigger OpenID Connect Authorization Code Flow
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("req: %+v: %v", req, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expected {
				t.Logf("response: %+v", resp)
				t.Errorf("auth filter failed got=%d, expected=%d, route=%s", resp.StatusCode, tc.expected, r)
				b, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}
				t.Logf("Got body: %s", string(b))
			}

			cookies := client.Jar.Cookies(reqURL)
			if tc.expectNoCookies {
				assert.Empty(t, cookies)
			} else {
				assert.NotEmpty(t, cookies)
				for _, c := range cookies {
					if tc.expectCookieDomain != "" {
						assert.Equal(t, tc.expectCookieDomain, c.Domain)
					}
					// make sure cookies have correct maxAge according to exp claim
					left, right := tokenExp-time.Minute, tokenExp
					maxAge := time.Duration(c.MaxAge) * time.Second
					assert.True(t, left <= maxAge && maxAge <= right, "maxAge has to be within [%v, %v]", left, right)

					for _, v := range tc.filterCookies {
						if v == c.Name {
							assert.True(t, c.Value == "")
						}
					}

					// Check for custom cookie name
					if tc.expectCookieName != "" {
						assert.True(t, strings.HasPrefix(c.Name, tc.expectCookieName),
							"Cookie name should start with %s, but got %s", tc.expectCookieName, c.Name)
					}
				}
			}
		})
	}
}

// Substitutes {{ .OIDCServerURL }} and {{ .RedirectURL }} template variables and parses filter definition string
func parseFilter(def, oidcServerURL, redirectURL string) ([]*eskip.Filter, error) {
	template, err := template.New("test filter def").Parse(def)
	if err != nil {
		return nil, err
	}
	params := struct {
		OIDCServerURL string
		RedirectURL   string
	}{
		OIDCServerURL: oidcServerURL,
		RedirectURL:   redirectURL,
	}
	out := new(strings.Builder)
	if err := template.Execute(out, params); err != nil {
		return nil, err
	}
	return eskip.ParseFilters(out.String())
}

func TestChunkAndMergerCookie(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
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
		largeCookie.Value += string(rune(r.Intn('Z'-'A') + 'A' + i%2*32))
	}
	oneCookie := largeCookie
	oneCookie.Value = oneCookie.Value[:len(oneCookie.Value)-(len(oneCookie.String())-cookieMaxSize)-1]
	twoCookies := largeCookie
	twoCookies.Value = twoCookies.Value[:len(twoCookies.Value)-(len(twoCookies.String())-cookieMaxSize)]

	for _, ht := range []struct {
		name  string
		given http.Cookie
		num   int
	}{
		{"short cookie", tinyCookie, 1},
		{"cookie without content", emptyCookie, 1},
		{"large cookie == 6 chunks", largeCookie, 6},
		{"fits exactly into one cookie", oneCookie, 1},
		{"chunked up cookie", twoCookies, 2},
	} {
		t.Run(fmt.Sprintf("test:%v", ht.name), func(t *testing.T) {
			assert := assert.New(t)
			got := chunkCookie(&ht.given)
			assert.NotNil(t, got, "it should not be empty")
			// shuffle the order of response cookies
			r.Shuffle(len(got), func(i, j int) {
				got[i], got[j] = got[j], got[i]
			})
			assert.Len(got, ht.num, "should result in a different number of chunks")
			mergedCookie := mergerCookies(got)
			assert.NotNil(mergedCookie, "should receive a valid cookie")
			assert.Equal(ht.given, *mergedCookie, "after chunking and remerging the content must be equal")
			// verify no cookie exceeds limits
			for _, ck := range got {
				assert.True(func() bool {
					return len(ck.String()) <= cookieMaxSize
				}(), "its size should not exceed limits cookieMaxSize")
			}
		})
	}
}

var cookieCompressRuns = []struct {
	name       string
	compressor cookieCompression
}{
	{"flate/min", newDeflatePoolCompressor(flate.BestSpeed)},
	{"flate/default", newDeflatePoolCompressor(flate.DefaultCompression)},
	{"flate/max", newDeflatePoolCompressor(flate.BestCompression)},
}

func Test_deflatePoolCompressor(t *testing.T) {
	for _, run := range cookieCompressRuns {
		t.Run(fmt.Sprintf("test:%v", run.name), func(t *testing.T) {
			assert := assert.New(t)
			compressed, err := run.compressor.compress([]byte(testOpenIDConfig))
			assert.NoError(err)
			decomp, err := run.compressor.decompress(compressed)
			assert.NoError(err)
			assert.Equal(decomp, []byte(testOpenIDConfig), "compressed and decompressed should result to equal")
			assert.True(len(compressed) < len([]byte(testOpenIDConfig)), "should be smaller than original")
		})
	}
}

func Benchmark_deflatePoolCompressor(b *testing.B) {
	for _, rw := range []string{"comp", "decomp"} {
		for _, run := range cookieCompressRuns {
			b.Run(fmt.Sprintf("ST/%s/%s", rw, run.name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					compressed, _ := run.compressor.compress([]byte(testOpenIDConfig))
					run.compressor.decompress(compressed)
					b.ReportMetric(float64(len(compressed)), "byte")
					b.ReportMetric(float64(len([]byte(testOpenIDConfig)))/float64(len(compressed)), "/1-ratio")
				}
			})
			b.Run(fmt.Sprintf("MT/%s/%s", rw, run.name), func(b *testing.B) {
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						compressed, _ := run.compressor.compress([]byte(testOpenIDConfig))
						run.compressor.decompress(compressed)
					}
				})
			})
		}
	}
}

func Test_tokenOidcFilter_getMaxAge(t *testing.T) {
	type args struct {
		claimsMap map[string]interface{}
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "Success",
			args: args{
				claimsMap: map[string]interface{}{
					"exp": float64(time.Now().Add(2 * time.Hour).Unix()),
				},
			},
			want: time.Hour * 2,
		},
		{
			name: "No exp set",
			args: args{},
			want: time.Hour,
		},
		{
			name: "Wrong exp type",
			args: args{
				claimsMap: map[string]interface{}{
					"exp": int64(time.Now().Add(2 * time.Hour).Unix()),
				},
			},
			want: time.Hour,
		},
		{
			name: "Exp too early",
			args: args{
				claimsMap: map[string]interface{}{
					"exp": float64(time.Now().Add(10 * time.Second).Unix()),
				},
			},
			want: time.Hour,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &tokenOidcFilter{
				validity: time.Hour,
			}

			got := f.getMaxAge(tt.args.claimsMap)
			assert.True(t, got >= tt.want-time.Minute && got <= 2*tt.want,
				fmt.Sprintf("maxAge has to be within [%s - 1m, %s]", tt.want, tt.want))
		})
	}
}
