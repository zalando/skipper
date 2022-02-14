package auth_test

import (
	"encoding/base64"
	"encoding/json"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/auth"
	"net/http"
	"net/http/httptest"
	"testing"
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

func checkDeletedCookie(t *testing.T, rsp *http.Response, cookieName string) {
	for _, c := range rsp.Cookies() {
		if c.Name == cookieName {
			if c.Value != "" {
				t.Fatalf(
					"Unexpected cookie value, got: '%s', expected: ''.",
					c.Value,
				)
			}
			if c.MaxAge != -1 {
				t.Fatalf(
					"Unexpected cookie MaxAge, got: %d, expected: -1.",
					c.MaxAge,
				)
			}
			return
		}
	}

	t.Fatalf("Cookie not found in response: '%s'", cookieName)
}

func TestGrantLogout(t *testing.T) {
	provider := newGrantLogoutTestServer()
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "{\"scope\":[\"match\"], \"uid\":\"foo\"}")
	defer tokeninfo.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)

	client := newGrantHTTPClient()

	proxy, err := newAuthProxy(config, &eskip.Route{
		Filters: []*eskip.Filter{
			{Name: filters.GrantLogoutName},
			{Name: filters.StatusName, Args: []interface{}{http.StatusNoContent}},
		},
		BackendType: eskip.ShuntBackend,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	t.Run("check that logout with both tokens revokes refresh token", func(t *testing.T) {
		cookie, err := auth.NewGrantCookieWithTokens(*config, testRefreshToken, testToken)
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check that logout with no refresh token revokes access token", func(t *testing.T) {
		cookie, err := auth.NewGrantCookieWithTokens(*config, "", testToken)
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check that logout deletes grant token cookie", func(t *testing.T) {
		cookie, err := auth.NewGrantCookieWithTokens(*config, testRefreshToken, testToken)
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

		checkDeletedCookie(t, rsp, config.TokenCookieName)
		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check that logout with no tokens results in a 401", func(t *testing.T) {
		cookie, err := auth.NewGrantCookieWithTokens(*config, "", "")
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

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
		cookie, err := auth.NewGrantCookieWithTokens(*config, "another_refresh_token", testToken)
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

		checkStatus(t, rsp, http.StatusInternalServerError)
	})

	t.Run("check that logout with an access token which fails to revoke on the upstream server results in 500", func(t *testing.T) {
		cookie, err := auth.NewGrantCookieWithTokens(*config, testRefreshToken, "another_access_token")
		if err != nil {
			t.Fatal(err)
		}

		rsp := grantQueryWithCookie(t, client, proxy.URL, cookie)

		checkStatus(t, rsp, http.StatusInternalServerError)
	})
}
