package auth_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/secrets"
	"golang.org/x/oauth2"
)

const (
	testToken                = "foobarbaz"
	testRefreshToken         = "refreshfoobarbaz"
	testAccessCode           = "quxquuxquz"
	testSecretFile           = "testdata/authsecret"
	testAccessTokenExpiresIn = int(time.Hour / time.Second)
	testCookieName           = "testcookie"
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
				ExpiresIn:    testAccessTokenExpiresIn,
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
		ClientID:        "some-id",
		ClientSecret:    "some-secret",
		Secrets:         secrets.NewRegistry(),
		SecretFile:      testSecretFile,
		TokeninfoURL:    tokeninfoURL,
		AuthURL:         providerURL + "/auth",
		TokenURL:        providerURL + "/token",
		TokenCookieName: testCookieName,
	}
}

func newAuthProxy(config *auth.OAuthConfig, routes ...*eskip.Route) (*proxytest.TestProxy, error) {
	grantSpec, err := config.NewGrant()
	if err != nil {
		return nil, err
	}

	grantCallbackSpec, err := config.NewGrantCallback()
	if err != nil {
		return nil, err
	}

	grantClaimsQuerySpec, err := config.NewGrantClaimsQuery()
	if err != nil {
		return nil, err
	}

	grantPrep, err := config.NewGrantPreprocessor()
	if err != nil {
		return nil, err
	}

	fr := builtin.MakeRegistry()
	fr.Register(grantSpec)
	fr.Register(grantCallbackSpec)
	fr.Register(grantClaimsQuerySpec)

	ro := routing.Options{
		PreProcessors: []routing.PreProcessor{grantPrep},
	}

	return proxytest.WithRoutingOptions(fr, ro, routes...), nil
}

func newSimpleGrantAuthProxy(t *testing.T, config *auth.OAuthConfig) *proxytest.TestProxy {
	proxy, err := newAuthProxy(config, &eskip.Route{
		Filters: []*eskip.Filter{
			{Name: auth.OAuthGrantName},
			{Name: "status", Args: []interface{}{http.StatusNoContent}},
		},
		BackendType: eskip.ShuntBackend,
	})

	if err != nil {
		t.Fatal(err)
	}

	return proxy
}

func newGrantHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func newGrantCookie(config auth.OAuthConfig) (*http.Cookie, error) {
	return newGrantCookieWithExpiration(config, time.Now().Add(time.Second*time.Duration(testAccessTokenExpiresIn)))
}

func newGrantCookieWithExpiration(config auth.OAuthConfig, expiry time.Time) (*http.Cookie, error) {
	token := &oauth2.Token{
		AccessToken:  testToken,
		RefreshToken: testRefreshToken,
		Expiry:       expiry,
	}

	cookie, err := auth.CreateCookie(config, "", token)
	return cookie, err
}

func checkStatus(t *testing.T, rsp *http.Response, expectedStatus int) {
	if rsp.StatusCode != expectedStatus {
		t.Fatalf(
			"Unexpected status code, got: %d, expected: %d.",
			rsp.StatusCode,
			expectedStatus,
		)
	}
}

func checkRedirect(t *testing.T, rsp *http.Response, expectedURL string) {
	checkStatus(t, rsp, http.StatusTemporaryRedirect)
	redirectTo := rsp.Header.Get("Location")
	if !strings.HasPrefix(redirectTo, expectedURL) {
		t.Fatalf(
			"Unexpected redirect location, got: '%s', expected: '%s'.",
			redirectTo,
			expectedURL,
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

func checkCookie(t *testing.T, rsp *http.Response) {
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

	accessTokenExpiryTime := time.Now().Add(time.Second * time.Duration(testAccessTokenExpiresIn))
	if c.Expires.Before(accessTokenExpiryTime) || c.Expires == accessTokenExpiryTime {
		t.Fatalf("Cookie expires with or before access token.")
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

	defer rsp.Body.Close()

	return rsp
}

func TestGrantFlow(t *testing.T) {
	t.Log("create a test provider")
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	t.Log("create a test tokeninfo")
	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	t.Log("create a test config")
	config := newGrantTestConfig(tokeninfo.URL, provider.URL)

	t.Log("create a proxy, returning 204, oauthGrant filter, initially without parameters")
	proxy := newSimpleGrantAuthProxy(t, config)
	defer proxy.Close()

	t.Log("create a client without redirects, to check it manually")
	client := newGrantHTTPClient()

	t.Run("check full grant flow", func(t *testing.T) {
		t.Log("make a request to the proxy without a cookie")
		rsp, err := client.Get(proxy.URL)
		if err != nil {
			t.Fatal(err)
		}

		defer rsp.Body.Close()

		t.Log("get redirected to the auth endpoint")
		checkRedirect(t, rsp, provider.URL+"/auth")

		t.Log("follow the redirect")
		rsp, err = client.Get(rsp.Header.Get("Location"))
		if err != nil {
			t.Fatalf("Failed to make request to provider: %v.", err)
		}

		defer rsp.Body.Close()

		t.Log("get redirected back to the proxy callback URL")
		checkRedirect(t, rsp, proxy.URL+"/.well-known/oauth2-callback")

		t.Log("follow the redirect")
		rsp, err = client.Get(rsp.Header.Get("Location"))
		if err != nil {
			t.Fatalf("Failed to make request to proxy: %v.", err)
		}

		defer rsp.Body.Close()

		t.Log("get redirected back to the proxy")
		checkRedirect(t, rsp, proxy.URL)

		t.Log("check auth cookie was set")
		checkCookie(t, rsp)

		t.Log("follow the redirect, with the cookie")
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

		t.Log("check for successful request")
		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check login is triggered access token is invalid", func(t *testing.T) {
		t.Log("create expired cookie with invalid refresh token")
		token := &oauth2.Token{
			AccessToken:  "invalid",
			RefreshToken: testRefreshToken,
			Expiry:       time.Now().Add(time.Duration(1) * time.Hour),
		}

		cookie, err := auth.CreateCookie(*config, "", token)
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

		t.Log("get redirected to the auth endpoint")
		checkRedirect(t, rsp, provider.URL+"/auth")
	})

	t.Run("check login is triggered when cookie is corrupted", func(t *testing.T) {
		t.Log("create expired cookie with invalid refresh token")
		url, _ := url.Parse(proxy.URL)
		cookie := &http.Cookie{
			Name:     config.TokenCookieName,
			Value:    "corruptedcookievalue",
			Path:     "/",
			Domain:   url.Hostname(),
			Expires:  time.Now().Add(time.Duration(1) * time.Hour),
			Secure:   true,
			HttpOnly: true,
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

		t.Log("get redirected to the auth endpoint")
		checkRedirect(t, rsp, provider.URL+"/auth")
	})

	t.Run("check handles multiple cookies with same name and uses the first decodable one", func(t *testing.T) {
		t.Log("send a request with a bad and good cookie")
		badCookie, _ := newGrantCookie(*config)
		badCookie.Value = "invalid"
		goodCookie, _ := newGrantCookie(*config)

		rsp := grantQueryWithCookie(t, client, proxy.URL, badCookie, goodCookie)

		t.Log("check for successful request")
		checkStatus(t, rsp, http.StatusNoContent)
	})
}

func TestGrantRefresh(t *testing.T) {
	t.Log("create a test provider")
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	t.Log("create a test tokeninfo")
	tokeninfo := newGrantTestTokeninfo(testToken, "")
	defer tokeninfo.Close()

	t.Log("create a test config")
	config := newGrantTestConfig(tokeninfo.URL, provider.URL)

	t.Log("create a client without redirects, to check it manually")
	client := newGrantHTTPClient()

	t.Log("create a proxy, returning 204, oauthGrant filter")
	proxy := newSimpleGrantAuthProxy(t, config)
	defer proxy.Close()

	t.Run("check token is refreshed if it expired", func(t *testing.T) {
		t.Log("create a valid cookie")
		cookie, err := newGrantCookieWithExpiration(*config, time.Now().Add(time.Duration(-1)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

		t.Log("check for successful request")
		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check login is triggered if refresh token is invalid", func(t *testing.T) {
		t.Log("create expired cookie with invalid refresh token")
		token := &oauth2.Token{
			AccessToken:  testToken,
			RefreshToken: "invalid",
			Expiry:       time.Now().Add(time.Duration(-1) * time.Minute),
		}

		cookie, err := auth.CreateCookie(*config, "", token)
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

		t.Log("get redirected to the auth endpoint")
		checkRedirect(t, rsp, provider.URL+"/auth")
	})
}
