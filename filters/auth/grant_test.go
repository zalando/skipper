package auth_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/net/dnstest"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/secrets"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testToken                = "test-token"
	testRefreshToken         = "refreshfoobarbaz"
	testAccessTokenExpiresIn = time.Hour
	testClientID             = "some-id"
	testClientSecret         = "some-secret"
	testAccessCode           = "quxquuxquz"
	testSecretFile           = "testdata/authsecret"
	testCookieName           = "testcookie"
	testQueryParamKey        = "param_key"
	testQueryParamValue      = "param_value"
)

type loggingRoundTripper struct {
	http.RoundTripper
	t *testing.T
}

func (rt *loggingRoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	rt.t.Logf("\n%v", rt.requestString(req))

	resp, err = rt.RoundTripper.RoundTrip(req)

	if err == nil {
		rt.t.Logf("\n%v", rt.responseString(resp))
	} else {
		rt.t.Logf("response err: %v", err)
	}
	return
}

func (rt *loggingRoundTripper) requestString(req *http.Request) string {
	tmp := req.Clone(context.Background())
	tmp.Body = nil

	var b strings.Builder
	_ = tmp.Write(&b)
	return b.String()
}

func (rt *loggingRoundTripper) responseString(resp *http.Response) string {
	tmp := *resp
	tmp.Body = nil

	var b strings.Builder
	_ = tmp.Write(&b)
	return b.String()
}

func newGrantTestTokeninfo(validToken string, tokenInfoJSON string) *httptest.Server {
	if tokenInfoJSON == "" {
		tokenInfoJSON = "{}"
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+validToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(tokenInfoJSON))
	}))
}

func newGrantTestAuthServer(testToken, testAccessCode string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := func(w http.ResponseWriter, r *http.Request) {
			rq := r.URL.Query()
			redirect := rq.Get("redirect_uri")
			rd, err := url.Parse(redirect)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			q := rd.Query()
			q.Set("code", testAccessCode)
			q.Set("state", r.URL.Query().Get("state"))
			rd.RawQuery = q.Encode()

			http.Redirect(
				w,
				r,
				rd.String(),
				http.StatusTemporaryRedirect,
			)
		}

		token := func(w http.ResponseWriter, r *http.Request) {
			var code, refreshToken string

			grantType := r.FormValue("grant_type")

			switch grantType {
			case "authorization_code":
				code = r.FormValue("code")
				if code != testAccessCode {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
			case "refresh_token":
				refreshToken = r.FormValue("refresh_token")
				if refreshToken != testRefreshToken {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
			}

			type tokenJSON struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
				ExpiresIn    int    `json:"expires_in"`
			}

			token := tokenJSON{
				AccessToken:  testToken,
				RefreshToken: testRefreshToken,
				ExpiresIn:    int(testAccessTokenExpiresIn / time.Second),
			}

			b, err := json.Marshal(token)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
		}

		switch r.URL.Path {
		case "/auth":
			auth(w, r)
		case "/token":
			token(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newGrantTestConfig(tokeninfoURL, providerURL string) *auth.OAuthConfig {
	return &auth.OAuthConfig{
		ClientID:          testClientID,
		ClientSecret:      testClientSecret,
		Secrets:           secrets.NewRegistry(),
		SecretsProvider:   secrets.NewSecretPaths(1 * time.Hour),
		SecretFile:        testSecretFile,
		TokeninfoURL:      tokeninfoURL,
		AuthURL:           providerURL + "/auth",
		TokenURL:          providerURL + "/token",
		RevokeTokenURL:    providerURL + "/revoke",
		TokenCookieName:   testCookieName,
		AuthURLParameters: map[string]string{testQueryParamKey: testQueryParamValue},
	}
}

func newAuthProxy(t *testing.T, config *auth.OAuthConfig, routes []*eskip.Route, hosts ...string) (*proxytest.TestProxy, *proxytest.TestClient) {
	err := config.Init()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if config.Secrets != nil {
			config.Secrets.Close()
		}
		if config.SecretsProvider != nil {
			config.SecretsProvider.Close()
		}
		if config.TokeninfoClient != nil {
			config.TokeninfoClient.Close()
		}
		if config.AuthClient != nil {
			config.AuthClient.Close()
		}
	})

	fr := builtin.MakeRegistry()
	fr.Register(config.NewGrant())
	fr.Register(config.NewGrantCallback())
	fr.Register(config.NewGrantClaimsQuery())
	fr.Register(config.NewGrantLogout())
	fr.Register(auth.NewOIDCQueryClaimsFilter())

	pc := proxytest.Config{
		RoutingOptions: routing.Options{
			FilterRegistry: fr,
			PreProcessors:  []routing.PreProcessor{config.NewGrantPreprocessor()},
		},
		Routes: routes,
	}

	if len(hosts) > 0 {
		pc.Certificates = []tls.Certificate{proxytest.NewCertificate(hosts...)}
	}

	proxy := pc.Create()

	if len(hosts) > 0 {
		proxy.URL = "https://" + net.JoinHostPort(hosts[0], proxy.Port)
	}

	client := proxy.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return proxy, client
}

func newSimpleGrantAuthProxy(t *testing.T, config *auth.OAuthConfig, hosts ...string) (*proxytest.TestProxy, *proxytest.TestClient) {
	return newAuthProxy(t, config, []*eskip.Route{{
		Filters: []*eskip.Filter{
			{Name: filters.OAuthGrantName},
			{Name: filters.StatusName, Args: []any{http.StatusNoContent}},
		},
		BackendType: eskip.ShuntBackend,
	}}, hosts...)
}

func checkStatus(t *testing.T, rsp *http.Response, expectedStatus int) {
	t.Helper()
	if rsp.StatusCode != expectedStatus {
		t.Fatalf(
			"Unexpected status code, got: %d, expected: %d.",
			rsp.StatusCode,
			expectedStatus,
		)
	}
}

func checkRedirect(t *testing.T, rsp *http.Response, expectedLocationWithoutQuery string) {
	t.Helper()

	checkStatus(t, rsp, http.StatusTemporaryRedirect)

	location, err := url.Parse(rsp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("Invalid location url: %v", err)
	}
	location.RawQuery = ""

	if location.String() != expectedLocationWithoutQuery {
		t.Fatalf(
			"Unexpected redirect location, got: '%s', expected: '%s'.",
			location,
			expectedLocationWithoutQuery,
		)
	}
}

func checkCookies(t *testing.T, rsp *http.Response, expectedDomain string) {
	t.Helper()

	require.NotEmpty(t, rsp.Cookies(), "No cookies found in the response.")
	for _, c := range rsp.Cookies() {
		require.NotEmpty(t, c.Value, "Cookie deleted.")
		require.True(t, c.Secure, "Cookie not secure.")
		require.True(t, c.HttpOnly, "Cookie not HTTP only.")
		require.True(t, c.Expires.After(time.Now().Add(testAccessTokenExpiresIn)), "Cookie expires with or before access token.")
		require.Equal(t, expectedDomain, c.Domain, "Incorrect cookie domain.")
	}
}

func grantQueryWithCookies(t *testing.T, client *proxytest.TestClient, url string, cookies ...*http.Cookie) *http.Response {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	rsp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rsp.Body.Close()

	return rsp
}

func parseCookieHeader(value string) []*http.Cookie {
	return (&http.Request{Header: http.Header{"Cookie": []string{value}}}).Cookies()
}

func TestGrantFlow(t *testing.T) {
	const (
		applicationDomain  = "foo.skipper.test"
		expectCookieDomain = "skipper.test"
	)

	dnstest.LoopbackNames(t, applicationDomain)

	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)

	routes := eskip.MustParse(`* -> oauthGrant()
		-> oidcClaimsQuery("/:sub")
		-> status(204)
		-> setResponseHeader("Backend-Request-Cookie", "${request.header.Cookie}")
		-> <shunt>
	`)

	proxy, client := newAuthProxy(t, config, routes, applicationDomain)
	defer proxy.Close()

	t.Run("check full grant flow", func(t *testing.T) {
		rsp, err := client.Get(proxy.URL + "/test")
		if err != nil {
			t.Fatal(err)
		}

		defer rsp.Body.Close()

		checkRedirect(t, rsp, provider.URL+"/auth")

		rsp, err = client.Get(rsp.Header.Get("Location"))
		if err != nil {
			t.Fatalf("Failed to make request to provider: %v.", err)
		}

		defer rsp.Body.Close()

		checkRedirect(t, rsp, proxy.URL+"/.well-known/oauth2-callback")

		rsp, err = client.Get(rsp.Header.Get("Location"))
		if err != nil {
			t.Fatalf("Failed to make request to proxy: %v.", err)
		}

		defer rsp.Body.Close()

		checkRedirect(t, rsp, proxy.URL+"/test")

		checkCookies(t, rsp, expectCookieDomain)

		rsp = grantQueryWithCookies(t, client, rsp.Header.Get("Location"), rsp.Cookies()...)

		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check login is triggered when access token is invalid", func(t *testing.T) {
		cookies := auth.NewGrantCookiesWithInvalidAccessToken(t, config)

		rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

		checkRedirect(t, rsp, provider.URL+"/auth")
	})

	t.Run("check login is triggered when cookie is corrupted", func(t *testing.T) {
		cookie := &http.Cookie{
			Name:     config.TokenCookieName,
			Value:    "corruptedcookievalue",
			Path:     "/",
			Expires:  time.Now().Add(time.Duration(1) * time.Hour),
			Secure:   true,
			HttpOnly: true,
		}

		rsp := grantQueryWithCookies(t, client, proxy.URL, cookie)

		checkRedirect(t, rsp, provider.URL+"/auth")
	})

	t.Run("check handles multiple cookies with same name and uses the first decodable one", func(t *testing.T) {
		badCookies := auth.NewGrantCookies(t, config)
		for _, c := range badCookies {
			c.Value = "invalid"
		}
		goodCookies := auth.NewGrantCookies(t, config)
		otherCookies := []*http.Cookie{{Name: "foo", Value: "bar", Path: "/", Secure: true, HttpOnly: true}}

		rsp := grantQueryWithCookies(t, client, proxy.URL, slices.Concat(badCookies, goodCookies, otherCookies)...)

		checkStatus(t, rsp, http.StatusNoContent)

		// Check all cookies are sent to the backend except goodCookies
		cookies := parseCookieHeader(rsp.Header.Get("Backend-Request-Cookie"))
		expected := slices.Concat(badCookies, otherCookies)

		if len(cookies) != len(expected) {
			t.Fatalf("Expected %v, got: %v", expected, cookies)
		}
		for i, expected := range expected {
			got := cookies[i]
			if got.Name != expected.Name || got.Value != expected.Value {
				t.Errorf("Unexpected cookie, expected: %v, got: %v", expected, got)
			}
		}
	})

	t.Run("check does not send cookie again if token was not refreshed", func(t *testing.T) {
		goodCookies := auth.NewGrantCookies(t, config)

		rsp := grantQueryWithCookies(t, client, proxy.URL, goodCookies...)

		checkStatus(t, rsp, http.StatusNoContent)

		assert.Empty(t, rsp.Cookies(), "The auth cookie should only be added to the response if there was a refresh.")
	})
}

func TestGrantRefresh(t *testing.T) {
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)

	proxy, client := newSimpleGrantAuthProxy(t, config)
	defer proxy.Close()

	t.Run("check token is refreshed if it expired", func(t *testing.T) {
		expiredCookies := auth.NewGrantCookiesWithExpiration(t, config, time.Now().Add(time.Duration(-1)*time.Minute))

		rsp := grantQueryWithCookies(t, client, proxy.URL, expiredCookies...)

		checkStatus(t, rsp, http.StatusNoContent)

		rsp = grantQueryWithCookies(t, client, proxy.URL, rsp.Cookies()...)

		checkStatus(t, rsp, http.StatusNoContent)

		assert.Empty(t, rsp.Cookies(), "The auth cookie should only be added to the response if there was a refresh.")
	})

	t.Run("check login is triggered if refresh token is invalid", func(t *testing.T) {
		cookies := auth.NewGrantCookiesWithInvalidRefreshToken(t, config)

		rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

		checkRedirect(t, rsp, provider.URL+"/auth")
	})
}

func TestGrantTokeninfoSubjectPresent(t *testing.T) {
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, `{"uid": "whatever"}`)
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.TokeninfoSubjectKey = "uid"

	proxy, client := newSimpleGrantAuthProxy(t, config)
	defer proxy.Close()

	cookies := auth.NewGrantCookies(t, config)

	rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

	checkStatus(t, rsp, http.StatusNoContent)
}

func TestGrantTokeninfoSubjectMissing(t *testing.T) {
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, `{"sub": "whatever"}`)
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.TokeninfoSubjectKey = "uid"

	proxy, client := newSimpleGrantAuthProxy(t, config)
	defer proxy.Close()

	cookies := auth.NewGrantCookies(t, config)

	rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

	checkRedirect(t, rsp, provider.URL+"/auth")
}

func TestGrantAuthParameterRedirectURI(t *testing.T) {
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)

	// Configure fixed redirect_uri parameter
	const redirectUriParamValue = "https://auth.test/a-callback"
	config.AuthURLParameters["redirect_uri"] = redirectUriParamValue

	proxy, client := newSimpleGrantAuthProxy(t, config)
	defer proxy.Close()

	rsp, err := client.Get(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	checkRedirect(t, rsp, provider.URL+"/auth")

	rsp, err = client.Get(rsp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("Failed to make request to provider: %v.", err)
	}
	defer rsp.Body.Close()

	checkRedirect(t, rsp, redirectUriParamValue)
}

func TestGrantTokenCookieRemoveSubDomains(t *testing.T) {
	const (
		applicationDomain  = "foo.skipper.test"
		expectCookieDomain = applicationDomain
	)

	dnstest.LoopbackNames(t, applicationDomain)

	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	zero := 0
	config.TokenCookieRemoveSubdomains = &zero

	proxy, client := newSimpleGrantAuthProxy(t, config, applicationDomain)
	defer proxy.Close()

	rsp, err := client.Get(proxy.URL + "/test")
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	rsp, err = client.Get(rsp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("Failed to make request to provider: %v.", err)
	}
	defer rsp.Body.Close()

	rsp, err = client.Get(rsp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("Failed to make request to proxy: %v.", err)
	}
	defer rsp.Body.Close()

	checkCookies(t, rsp, expectCookieDomain)
}

func TestGrantCallbackRedirectsToTheInitialRequestDomain(t *testing.T) {
	const (
		applicationDomain = "foo.skipper.test"
		callbackDomain    = "callback.skipper.test"
		callbackPath      = "/a-callback"
	)

	dnstest.LoopbackNames(t, applicationDomain, callbackDomain)

	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	zero := 0
	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.TokenCookieRemoveSubdomains = &zero
	config.CallbackPath = callbackPath

	proxy, client := newSimpleGrantAuthProxy(t, config, applicationDomain, callbackDomain)
	defer proxy.Close()

	callbackUri := "https://" + net.JoinHostPort(callbackDomain, proxy.Port) + callbackPath

	// note: there is a chicken & egg problem:
	// this updates AuthURLParameters after proxy and filter specs were created
	// because callbackUri needs to have proxy port number.
	// This update works because grant filter specs receive pointer to the config and
	// evaluate AuthURLParameters in runtime during request
	config.AuthURLParameters["redirect_uri"] = callbackUri

	httpGet := func(url string) *http.Response {
		t.Helper()
		rsp, err := client.Get(url)
		if err != nil {
			t.Fatalf("failed to GET %s: %v", url, err)
		}
		rsp.Body.Close()
		return rsp
	}

	rsp := httpGet(proxy.URL + "/test")

	checkRedirect(t, rsp, provider.URL+"/auth")

	rsp = httpGet(rsp.Header.Get("Location"))

	checkRedirect(t, rsp, callbackUri)

	rsp = httpGet(rsp.Header.Get("Location"))

	checkRedirect(t, rsp, proxy.URL+callbackPath)

	if len(rsp.Cookies()) > 0 {
		t.Error("expected no cookies from redirect to the callback")
	}

	rsp = httpGet(rsp.Header.Get("Location"))

	checkRedirect(t, rsp, proxy.URL+"/test")

	checkCookies(t, rsp, applicationDomain)
}

func TestGrantTokenCookieDomainZeroRemovedSubdomains(t *testing.T) {
	const applicationDomain = "foo.skipper.test"

	dnstest.LoopbackNames(t, applicationDomain)

	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	zero := 0
	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.TokenCookieRemoveSubdomains = &zero

	proxy, client := newSimpleGrantAuthProxy(t, config, applicationDomain)
	defer proxy.Close()

	for _, tc := range []struct {
		name    string
		host    string
		allowed bool
	}{
		{"application domain", "foo.skipper.test", true},
		{"parent domain", "skipper.test", true},
		//
		{"neighbor domain", "bar.skipper.test", false},
		{"another domain", "foo.other.test", false},
		{"another parent domain", "other.test", false},
		{"application subdomain", "baz.foo.skipper.test", false},
		{"neighbor subdomain", "baz.bar.skipper.test", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cookies := auth.NewGrantCookiesWithHost(t, config, tc.host)

			rsp := grantQueryWithCookies(t, client, proxy.URL+"/test", cookies...)

			if tc.allowed {
				checkStatus(t, rsp, http.StatusNoContent)
			} else {
				checkRedirect(t, rsp, provider.URL+"/auth")
			}

		})
	}
}

func TestGrantTokenCookieDomainOneRemovedSubdomains(t *testing.T) {
	const applicationDomain = "foo.skipper.test"

	dnstest.LoopbackNames(t, applicationDomain)

	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	one := 1
	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.TokenCookieRemoveSubdomains = &one

	proxy, client := newSimpleGrantAuthProxy(t, config, applicationDomain)
	defer proxy.Close()

	for _, tc := range []struct {
		name    string
		host    string
		allowed bool
	}{
		{"application domain", "foo.skipper.test", true},
		{"parent domain", "skipper.test", true},
		{"neighbor domain", "bar.skipper.test", true},
		{"application subdomain", "baz.foo.skipper.test", true},
		//
		{"another domain", "foo.other.test", false},
		{"another parent domain", "other.test", false},
		{"neighbor subdomain", "baz.bar.skipper.test", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cookies := auth.NewGrantCookiesWithHost(t, config, tc.host)

			rsp := grantQueryWithCookies(t, client, proxy.URL+"/test", cookies...)

			if tc.allowed {
				checkStatus(t, rsp, http.StatusNoContent)
			} else {
				checkRedirect(t, rsp, provider.URL+"/auth")
			}
		})
	}
}

func TestGrantAccessTokenHeaderName(t *testing.T) {
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.AccessTokenHeaderName = "X-Access-Token"

	routes := eskip.MustParse(`* -> oauthGrant()
		-> status(204)
		-> setResponseHeader("Backend-X-Access-Token", "${request.header.X-Access-Token}")
		-> <shunt>
	`)

	proxy, client := newAuthProxy(t, config, routes)
	defer proxy.Close()

	cookies := auth.NewGrantCookies(t, config)

	rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

	checkStatus(t, rsp, http.StatusNoContent)

	assert.Equal(t, "Bearer "+testToken, rsp.Header.Get("Backend-X-Access-Token"))
}

func TestGrantForwardToken(t *testing.T) {
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, `{"token_type":"Bearer", "access_token":"foo", "uid":"bar", "scope":["baz"], "expires_in":1234}`)
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.TokeninfoSubjectKey = "uid"

	routes := eskip.MustParse(`* -> oauthGrant()
		-> status(204)
		-> forwardToken("X-Tokeninfo-Forward")
		-> setResponseHeader("Backend-X-Tokeninfo-Forward", "${request.header.X-Tokeninfo-Forward}")
		-> <shunt>
	`)

	proxy, client := newAuthProxy(t, config, routes)
	defer proxy.Close()

	cookies := auth.NewGrantCookies(t, config)

	rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

	checkStatus(t, rsp, http.StatusNoContent)

	assert.JSONEq(t, `{"token_type":"Bearer", "access_token":"foo", "uid":"bar", "sub":"bar", "scope":["baz"], "expires_in":1234}`, rsp.Header.Get("Backend-X-Tokeninfo-Forward"))
}

func TestGrantTokeninfoKeys(t *testing.T) {
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	const tokenInfoJson = `{"token_type":"Bearer", "access_token":"foo", "uid":"bar", "scope":["baz"], "expires_in":1234}`

	tokeninfo := newGrantTestTokeninfo(testToken, tokenInfoJson)
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.GrantTokeninfoKeys = []string{"uid", "scope"}

	routes := eskip.MustParse(`* -> oauthGrant()
		-> status(204)
		-> forwardToken("X-Tokeninfo-Forward")
		-> setResponseHeader("Backend-X-Tokeninfo-Forward", "${request.header.X-Tokeninfo-Forward}")
		-> <shunt>
	`)

	proxy, client := newAuthProxy(t, config, routes)
	defer proxy.Close()

	cookies := auth.NewGrantCookies(t, config)

	rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

	checkStatus(t, rsp, http.StatusNoContent)

	assert.JSONEq(t, `{"uid":"bar", "scope":["baz"]}`, rsp.Header.Get("Backend-X-Tokeninfo-Forward"))
}

func TestGrantCredentialsFile(t *testing.T) {
	const (
		fooDomain = "foo.skipper.test"
		barDomain = "bar.skipper.test"
	)

	dnstest.LoopbackNames(t, fooDomain, barDomain)

	secretsDir := t.TempDir()

	clientIdFile := secretsDir + "/test-client-id"
	clientSecretFile := secretsDir + "/test-client-secret"

	require.NoError(t, os.WriteFile(clientIdFile, []byte(testClientID), 0644))
	require.NoError(t, os.WriteFile(clientSecretFile, []byte(testClientSecret), 0644))

	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	zero := 0
	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.TokenCookieRemoveSubdomains = &zero
	config.ClientID = ""
	config.ClientSecret = ""
	config.ClientIDFile = clientIdFile
	config.ClientSecretFile = clientSecretFile

	routes := eskip.MustParse(`* -> oauthGrant() -> status(204) -> <shunt>`)

	proxy, client := newAuthProxy(t, config, routes, fooDomain, barDomain)
	defer proxy.Close()

	// Follow redirects as store cookies
	client.CheckRedirect = nil
	client.Jar, _ = cookiejar.New(nil)
	httpLogger := &loggingRoundTripper{client.Transport, t}
	client.Transport = httpLogger

	resetClient := func(t *testing.T) {
		client.Jar, _ = cookiejar.New(nil)
		httpLogger.t = t
	}

	t.Run("request to "+fooDomain+" succeeds", func(t *testing.T) {
		resetClient(t)

		rsp, err := client.Get(proxy.URL + "/test")
		require.NoError(t, err)
		rsp.Body.Close()

		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("request to "+barDomain+" succeeds", func(t *testing.T) {
		resetClient(t)

		barUrl := "https://" + net.JoinHostPort(barDomain, proxy.Port)

		rsp, err := client.Get(barUrl + "/test")
		require.NoError(t, err)
		rsp.Body.Close()

		checkStatus(t, rsp, http.StatusNoContent)
	})
}

func TestGrantCredentialsPlaceholder(t *testing.T) {
	const (
		fooDomain = "foo.skipper.test"
		barDomain = "bar.skipper.test"
	)

	dnstest.LoopbackNames(t, fooDomain, barDomain)

	secretsDir := t.TempDir()

	require.NoError(t, os.WriteFile(secretsDir+"/"+fooDomain+"-client-id", []byte(testClientID), 0644))
	require.NoError(t, os.WriteFile(secretsDir+"/"+fooDomain+"-client-secret", []byte(testClientSecret), 0644))

	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	zero := 0
	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.TokenCookieRemoveSubdomains = &zero
	config.ClientID = ""
	config.ClientSecret = ""
	config.ClientIDFile = secretsDir + "/{host}-client-id"
	config.ClientSecretFile = secretsDir + "/{host}-client-secret"

	routes := eskip.MustParse(`* -> oauthGrant() -> status(204) -> <shunt>`)

	proxy, client := newAuthProxy(t, config, routes, fooDomain, barDomain)
	defer proxy.Close()

	// Follow redirects as store cookies
	client.CheckRedirect = nil
	client.Jar, _ = cookiejar.New(nil)
	httpLogger := &loggingRoundTripper{client.Transport, t}
	client.Transport = httpLogger

	resetClient := func(t *testing.T) {
		client.Jar, _ = cookiejar.New(nil)
		httpLogger.t = t
	}

	t.Run("request to the hostname with existing client credentials succeeds", func(t *testing.T) {
		resetClient(t)

		rsp, err := client.Get(proxy.URL + "/test")
		require.NoError(t, err)
		rsp.Body.Close()

		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("request to the hostname without existing client credentials is forbidden", func(t *testing.T) {
		resetClient(t)

		barUrl := "https://" + net.JoinHostPort(barDomain, proxy.Port)

		rsp, err := client.Get(barUrl + "/test")
		require.NoError(t, err)
		rsp.Body.Close()

		checkStatus(t, rsp, http.StatusForbidden)
	})
}

func TestGrantInsecure(t *testing.T) {
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.Insecure = true

	proxy, client := newSimpleGrantAuthProxy(t, config)
	defer proxy.Close()

	rsp, err := client.Get(proxy.URL + "/test")
	require.NoError(t, err)
	defer rsp.Body.Close()

	rsp, err = client.Get(rsp.Header.Get("Location"))
	require.NoError(t, err, "Failed to make request to provider")
	defer rsp.Body.Close()

	callbackUrl := rsp.Header.Get("Location")

	assert.True(t, strings.HasPrefix(callbackUrl, "http://"), "Callback URL should be insecure")

	rsp, err = client.Get(callbackUrl)
	require.NoError(t, err, "Failed to make callback request to proxy")
	defer rsp.Body.Close()

	if assert.NotEmpty(t, rsp.Cookies(), "Cookies not found") {
		for _, c := range rsp.Cookies() {
			assert.False(t, c.Secure, "Cookie should be insecure")
		}
	}
}

func TestGrantLoginRedirectStub(t *testing.T) {
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)

	const stubContent = "foo {{authCodeURL}} bar {authCodeURL} baz"

	routes := eskip.MustParse(fmt.Sprintf(`*
		-> annotate("oauthGrant.loginRedirectStub", "%s")
		-> oauthGrant()
		-> status(204)
		-> <shunt>
	`, stubContent))

	proxy, client := newAuthProxy(t, config, routes)
	defer proxy.Close()

	rsp, body, err := client.GetBody(proxy.URL + "/test")
	require.NoError(t, err)

	assert.Equal(t, rsp.StatusCode, http.StatusOK)

	authCodeUrl := rsp.Header.Get("X-Auth-Code-Url")
	assert.True(t, strings.HasPrefix(authCodeUrl, provider.URL))

	expectedContent := fmt.Sprintf("foo %s bar %s baz", authCodeUrl, authCodeUrl)

	assert.Equal(t, int64(len(expectedContent)), rsp.ContentLength)
	assert.Equal(t, expectedContent, string(body))
}

func TestGrantLoginRedirectForRequestWithTrailingQuestionMark(t *testing.T) {
	const (
		applicationDomain = "foo.skipper.test"
		callbackPath      = "/a-callback"
	)

	dnstest.LoopbackNames(t, applicationDomain)

	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.CallbackPath = callbackPath

	routes := eskip.MustParse(`* -> oauthGrant() -> status(204) -> <shunt>`)

	proxy, client := newAuthProxy(t, config, routes, applicationDomain)
	defer proxy.Close()

	// When requested with trailing question mark
	rsp, err := client.Get(proxy.URL + "?")
	require.NoError(t, err)
	defer rsp.Body.Close()

	require.Equal(t, rsp.StatusCode, http.StatusTemporaryRedirect)

	location, err := url.Parse(rsp.Header.Get("Location"))
	require.NoError(t, err)

	// Then redirect_uri does not contain a trailing question mark
	assert.Equal(t, proxy.URL+callbackPath, location.Query().Get("redirect_uri"))
}
