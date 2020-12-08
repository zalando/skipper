package auth_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestGrantClaimsQuery(t *testing.T) {
	t.Log("create a test provider")
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	t.Log("create a test tokeninfo")
	tokeninfo := newGrantTestTokeninfo(testToken, "{\"scope\":[\"match\"], \"uid\":\"foo\"}")
	defer tokeninfo.Close()

	t.Log("create a test config")
	config := newGrantTestConfig(tokeninfo.URL, provider.URL)

	t.Log("create a client without redirects, to check it manually")
	client := newGrantHTTPClient()

	t.Log("create a valid cookie")
	cookie, err := newGrantCookie(*config)
	if err != nil {
		t.Fatal(err)
	}

	createProxyForQuery := func(config *auth.OAuthConfig, query string) *proxytest.TestProxy {
		t.Log("create a proxy")
		proxy, err := newAuthProxy(config, &eskip.Route{
			Filters: []*eskip.Filter{
				{Name: auth.OAuthGrantName},
				{Name: auth.GrantClaimsQueryName, Args: []interface{}{query}},
				{Name: "status", Args: []interface{}{http.StatusNoContent}},
			},
			BackendType: eskip.ShuntBackend,
		})
		if err != nil {
			t.Fatal(err)
		}
		return proxy
	}

	t.Run("check that matching tokeninfo properties allows the request", func(t *testing.T) {
		proxy := createProxyForQuery(config, "/allowed:scope.#[==\"match\"]")
		defer proxy.Close()

		t.Log("make a request to an allowed endpoint")
		url := fmt.Sprint(proxy.URL, "/allowed")
		rsp := grantQueryWithCookie(t, client, url, cookie)

		t.Log("check for successful request")
		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check that non-matching tokeninfo properties block the request", func(t *testing.T) {
		proxy := createProxyForQuery(config, "/forbidden:scope.#[==\"noMatch\"]")
		defer proxy.Close()

		t.Log("make a request to a forbidden endpoint")
		url := fmt.Sprint(proxy.URL, "/forbidden")
		rsp := grantQueryWithCookie(t, client, url, cookie)

		t.Log("check for unauthorized")
		checkStatus(t, rsp, http.StatusUnauthorized)
	})

	t.Run("check that the subject claim gets initialized from a configurable tokeninfo property and is queriable", func(t *testing.T) {
		newConfig := *config
		newConfig.TokeninfoSubjectKey = "uid"

		proxy := createProxyForQuery(&newConfig, "/allowed:@_:sub%\"foo\"")
		defer proxy.Close()

		t.Log("make a request to the endpoint")
		url := fmt.Sprint(proxy.URL, "/allowed")
		rsp := grantQueryWithCookie(t, client, url, cookie)

		t.Log("check for successful request")
		checkStatus(t, rsp, http.StatusNoContent)
	})
}
