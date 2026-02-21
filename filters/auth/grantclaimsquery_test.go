package auth_test

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestGrantClaimsQuery(t *testing.T) {
	provider := newGrantTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newGrantTestTokeninfo(testToken, "{\"scope\":[\"match\"], \"uid\":\"foo\"}")
	defer tokeninfo.Close()

	newAuthProxyForQuery := func(t *testing.T, config *auth.OAuthConfig, query string) (*proxytest.TestProxy, *proxytest.TestClient) {
		return newAuthProxy(t, config, []*eskip.Route{{
			Filters: []*eskip.Filter{
				{Name: filters.OAuthGrantName},
				{Name: filters.GrantClaimsQueryName, Args: []any{query}},
				{Name: filters.StatusName, Args: []any{http.StatusNoContent}},
			},
			BackendType: eskip.ShuntBackend,
		}})
	}

	t.Run("check that matching tokeninfo properties allows the request", func(t *testing.T) {
		config := newGrantTestConfig(tokeninfo.URL, provider.URL)

		proxy, client := newAuthProxyForQuery(t, config, "/allowed:scope.#[==\"match\"]")
		defer proxy.Close()

		cookies := auth.NewGrantCookies(t, config)

		rsp := grantQueryWithCookies(t, client, proxy.URL+"/allowed", cookies...)

		checkStatus(t, rsp, http.StatusNoContent)
	})

	t.Run("check that non-matching tokeninfo properties block the request", func(t *testing.T) {
		config := newGrantTestConfig(tokeninfo.URL, provider.URL)

		proxy, client := newAuthProxyForQuery(t, config, "/forbidden:scope.#[==\"noMatch\"]")
		defer proxy.Close()

		cookies := auth.NewGrantCookies(t, config)

		rsp := grantQueryWithCookies(t, client, proxy.URL+"/forbidden", cookies...)

		checkStatus(t, rsp, http.StatusUnauthorized)
	})

	t.Run("check that the subject claim gets initialized from a configurable tokeninfo property and is queryable", func(t *testing.T) {
		config := newGrantTestConfig(tokeninfo.URL, provider.URL)
		config.TokeninfoSubjectKey = "uid"

		proxy, client := newAuthProxyForQuery(t, config, "/allowed:@_:sub%\"foo\"")
		defer proxy.Close()

		cookies := auth.NewGrantCookies(t, config)

		rsp := grantQueryWithCookies(t, client, proxy.URL+"/allowed", cookies...)

		checkStatus(t, rsp, http.StatusNoContent)
	})
}
