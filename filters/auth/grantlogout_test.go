package auth_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/net/dnstest"
)

type testRevokeError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func newGrantLogoutTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/revoke" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		expectedCredentials := base64.StdEncoding.EncodeToString([]byte(testClientID + ":" + testClientSecret))
		expectedAuthorization := "Basic " + expectedCredentials
		if expectedAuthorization != r.Header.Get("Authorization") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		paramValue := r.URL.Query().Get(testQueryParamKey)
		if testQueryParamValue != paramValue {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err := r.ParseForm()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		token := r.Form.Get("token")
		tokenType := r.Form.Get("token_type_hint")

		if token == "" || tokenType == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if tokenType == "access_token" && token == testToken {
			// Simulate a provider that only supports revoking refresh tokens
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")

			errorResponse := testRevokeError{
				Error:            "unsupported_token_type",
				ErrorDescription: "simulate unsupported access token revocation",
			}

			b, _ := json.Marshal(errorResponse)
			w.Write(b)
		} else if tokenType == "refresh_token" && token == testRefreshToken {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
}

func checkDeletedCookie(t *testing.T, rsp *http.Response, cookieName, expectedDomain string) {
	t.Helper()
	for _, c := range rsp.Cookies() {
		if c.Name == cookieName {
			if c.Value != "" {
				t.Errorf("Unexpected cookie value, got: '%s', expected: ''.", c.Value)
			}
			if c.MaxAge != -1 {
				t.Errorf("Unexpected cookie MaxAge, got: %d, expected: -1.", c.MaxAge)
			}
			if c.Domain != expectedDomain {
				t.Fatalf("Unexpected cookie domain, got: %s, expected: %s", c.Domain, expectedDomain)
			}
			return
		}
	}
	t.Fatalf("Cookie not found in response: '%s'", cookieName)
}

func TestGrantLogout(t *testing.T) {
	const (
		applicationDomain  = "foo.skipper.test"
		expectCookieDomain = "skipper.test"
	)

	dnstest.LoopbackNames(t, applicationDomain)

	provider := newGrantLogoutTestServer()
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "{\"scope\":[\"match\"], \"uid\":\"foo\"}")
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)

	proxy, client := newAuthProxy(t, config, []*eskip.Route{{
		Filters: []*eskip.Filter{
			{Name: filters.GrantLogoutName},
			{Name: filters.StatusName, Args: []any{http.StatusNoContent}},
		},
		BackendType: eskip.ShuntBackend,
	}}, applicationDomain)
	defer proxy.Close()

	t.Run("check that logout with both tokens revokes refresh token", func(t *testing.T) {
		cookies := auth.NewGrantCookiesWithTokens(t, config, testRefreshToken, testToken)

		rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check that logout with no refresh token revokes access token", func(t *testing.T) {
		cookies := auth.NewGrantCookiesWithTokens(t, config, "", testToken)

		rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check that logout deletes grant token cookie", func(t *testing.T) {
		cookies := auth.NewGrantCookiesWithTokens(t, config, testRefreshToken, testToken)

		rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

		checkDeletedCookie(t, rsp, config.TokenCookieName, expectCookieDomain)
		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check that logout with no tokens results in a 401", func(t *testing.T) {
		cookies := auth.NewGrantCookiesWithTokens(t, config, "", "")

		rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

		checkStatus(t, rsp, http.StatusUnauthorized)
	})

	t.Run("check that logout with no cookie results in 401", func(t *testing.T) {
		req, err := http.NewRequest("GET", proxy.URL, nil)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()

		checkStatus(t, rsp, http.StatusUnauthorized)
	})

	t.Run("check that logout with a refresh token which fails to revoke on the upstream server results in 500", func(t *testing.T) {
		cookies := auth.NewGrantCookiesWithTokens(t, config, "another_refresh_token", testToken)

		rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

		checkStatus(t, rsp, http.StatusInternalServerError)
	})

	t.Run("check that logout with an access token which fails to revoke on the upstream server results in 500", func(t *testing.T) {
		cookies := auth.NewGrantCookiesWithTokens(t, config, testRefreshToken, "another_access_token")

		rsp := grantQueryWithCookies(t, client, proxy.URL, cookies...)

		checkStatus(t, rsp, http.StatusInternalServerError)
	})
}

func TestGrantLogoutNoRevokeTokenURL(t *testing.T) {
	const applicationDomain = "foo.skipper.test"

	dnstest.LoopbackNames(t, applicationDomain)

	zero := 0
	config := newGrantTestConfig("http://invalid.test", "http://invalid.test")
	config.TokenCookieRemoveSubdomains = &zero
	config.RevokeTokenURL = ""

	routes := eskip.MustParse(`Path("/logout") -> grantLogout() -> redirectTo(302) -> <shunt>`)

	proxy, client := newAuthProxy(t, config, routes, applicationDomain)
	defer proxy.Close()

	rsp, err := client.Get(proxy.URL + "/logout")
	if err != nil {
		t.Fatal(err)
	}
	defer rsp.Body.Close()

	checkDeletedCookie(t, rsp, config.TokenCookieName, applicationDomain)
	checkStatus(t, rsp, http.StatusFound)
}
