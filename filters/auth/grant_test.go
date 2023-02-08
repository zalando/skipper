package auth_test

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func newGrantTestTokeninfo(validToken string, tokenInfoJSON string) *httptest.Server {
	const prefix = "Bearer "

	if tokenInfoJSON == "" {
		tokenInfoJSON = "{}"
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := func(code int) {
			w.WriteHeader(code)
			w.Write([]byte(tokenInfoJSON))
		}

		token := r.Header.Get("Authorization")
		if !strings.HasPrefix(token, prefix) || token[len(prefix):] != validToken {
			response(http.StatusUnauthorized)
			return
		}

		response(http.StatusOK)
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
		SecretFile:        testSecretFile,
		TokeninfoURL:      tokeninfoURL,
		AuthURL:           providerURL + "/auth",
		TokenURL:          providerURL + "/token",
		RevokeTokenURL:    providerURL + "/revoke",
		TokenCookieName:   testCookieName,
		AuthURLParameters: map[string]string{testQueryParamKey: testQueryParamValue},
	}
}

func newAuthProxy(t *testing.T, config *auth.OAuthConfig, routes []*eskip.Route, hosts ...string) (*proxytest.TestProxy, *http.Client) {
	err := config.Init()
	if err != nil {
		t.Fatal(err)
	}

	fr := builtin.MakeRegistry()
	fr.Register(config.NewGrant())
	fr.Register(config.NewGrantCallback())
	fr.Register(config.NewGrantClaimsQuery())
	fr.Register(config.NewGrantLogout())

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

func newSimpleGrantAuthProxy(t *testing.T, config *auth.OAuthConfig, hosts ...string) (*proxytest.TestProxy, *http.Client) {
	return newAuthProxy(t, config, []*eskip.Route{{
		Filters: []*eskip.Filter{
			{Name: filters.OAuthGrantName},
			{Name: filters.StatusName, Args: []interface{}{http.StatusNoContent}},
		},
		BackendType: eskip.ShuntBackend,
	}}, hosts...)
}

func newGrantCookie(config *auth.OAuthConfig) (*http.Cookie, error) {
	return auth.NewGrantCookieWithExpiration(config, time.Now().Add(testAccessTokenExpiresIn))
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

func findAuthCookie(rsp *http.Response) (*http.Cookie, bool) {
	for _, c := range rsp.Cookies() {
		if c.Name == testCookieName {
			return c, true
		}
	}

	return nil, false
}

func checkCookie(t *testing.T, rsp *http.Response, expectedDomain string) {
	t.Helper()

	c, ok := findAuthCookie(rsp)
	if !ok {
		t.Fatalf("Cookie not found.")
	}

	if c.Value == "" {
		t.Fatalf("Cookie deleted.")
	}

	if !c.Secure {
		t.Fatalf("Cookie not secure")
	}

	if !c.HttpOnly {
		t.Fatalf("Cookie not HTTP only")
	}

	accessTokenExpiryTime := time.Now().Add(testAccessTokenExpiresIn)
	if c.Expires.Before(accessTokenExpiryTime) || c.Expires == accessTokenExpiryTime {
		t.Fatalf("Cookie expires with or before access token.")
	}

	if c.Domain != expectedDomain {
		t.Fatalf("Incorrect cookie domain: %s, expected: %s", c.Domain, expectedDomain)
	}
}

func grantQueryWithCookie(t *testing.T, client *http.Client, url string, cookies ...*http.Cookie) *http.Response {
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

		checkCookie(t, rsp, expectCookieDomain)

		req, err := http.NewRequest("GET", rsp.Header.Get("Location"), nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v.", err)
		}

		c, _ := findAuthCookie(rsp)
		req.Header.Set("Cookie", fmt.Sprintf("%s=%s", c.Name, c.Value))
		rsp, err = client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request to proxy: %v.", err)
		}

		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check login is triggered when access token is invalid", func(t *testing.T) {
		cookie, err := auth.NewGrantCookieWithInvalidAccessToken(config)
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

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

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

		checkRedirect(t, rsp, provider.URL+"/auth")
	})

	t.Run("check handles multiple cookies with same name and uses the first decodable one", func(t *testing.T) {
		badCookie, _ := newGrantCookie(config)
		badCookie.Value = "invalid"
		goodCookie, _ := newGrantCookie(config)
		otherCookie := &http.Cookie{Name: "foo", Value: "bar", Path: "/", Secure: true, HttpOnly: true}

		rsp := grantQueryWithCookie(t, client, proxy.URL, badCookie, goodCookie, otherCookie)

		checkStatus(t, rsp, http.StatusNoContent)

		// Check all cookies are sent to the backend except goodCookie
		cookies := parseCookieHeader(rsp.Header.Get("Backend-Request-Cookie"))
		expected := []*http.Cookie{badCookie, otherCookie}

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
		goodCookie, _ := newGrantCookie(config)

		rsp := grantQueryWithCookie(t, client, proxy.URL, goodCookie)

		_, ok := findAuthCookie(rsp)
		if ok {
			t.Fatalf(
				"The auth cookie should only be added to the response if there was a refresh.",
			)
		}
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
		cookie, err := auth.NewGrantCookieWithExpiration(config, time.Now().Add(time.Duration(-1)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check login is triggered if refresh token is invalid", func(t *testing.T) {
		cookie, err := auth.NewGrantCookieWithInvalidRefreshToken(config)
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

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

	cookie, err := newGrantCookie(config)
	if err != nil {
		t.Fatal(err)
	}

	rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

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

	cookie, err := newGrantCookie(config)
	if err != nil {
		t.Fatal(err)
	}

	rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

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

	checkCookie(t, rsp, expectCookieDomain)
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

	checkCookie(t, rsp, applicationDomain)
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
			cookie, _ := auth.NewGrantCookieWithHost(config, tc.host)

			rsp := grantQueryWithCookie(t, client, proxy.URL+"/test", cookie)

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
			cookie, _ := auth.NewGrantCookieWithHost(config, tc.host)

			rsp := grantQueryWithCookie(t, client, proxy.URL+"/test", cookie)

			if tc.allowed {
				checkStatus(t, rsp, http.StatusNoContent)
			} else {
				checkRedirect(t, rsp, provider.URL+"/auth")
			}
		})
	}
}
