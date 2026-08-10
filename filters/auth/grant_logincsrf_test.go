package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/net/dnstest"
	"github.com/zalando/skipper/proxy/proxytest"
)

// newIdentityAuthServer is newGrantTestAuthServer with two identities: the user
// logged in at the provider decides the authorization code and the access token
// it can be exchanged for. "login_as" stands in for the provider side session of
// the user driving the browser.
func newIdentityAuthServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth":
			rd, err := url.Parse(r.URL.Query().Get("redirect_uri"))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			user := r.URL.Query().Get("login_as")
			if user == "" {
				user = "victim"
			}

			q := rd.Query()
			q.Set("code", user+"-code")
			q.Set("state", r.URL.Query().Get("state"))
			rd.RawQuery = q.Encode()

			http.Redirect(w, r, rd.String(), http.StatusTemporaryRedirect)

		case "/token":
			code := r.FormValue("code")
			if !strings.HasSuffix(code, "-code") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			user := strings.TrimSuffix(code, "-code")

			b, _ := json.Marshal(map[string]any{
				"access_token": user + "-token",
				"expires_in":   3600,
			})
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newIdentityTokeninfo() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !strings.HasSuffix(token, "-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		b, _ := json.Marshal(map[string]any{"uid": strings.TrimSuffix(token, "-token")})
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
}

// newBrowser returns a client that keeps its own cookies and does not follow
// redirects, so that each participant of the flow is a separate user agent.
// proxytest.TestProxy.Client() hands out the same http.Client every time.
func newBrowser(t *testing.T, proxy *proxytest.TestProxy) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	return &http.Client{
		Transport: proxy.Client().Transport,
		Jar:       jar,
		Timeout:   5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// TestGrantLoginCSRF checks that a browser which never started a login flow
// cannot be logged in by a callback URL crafted by somebody else.
//
// The grant flow state (filters/auth/grantflowstate.go) is an encrypted blob
// holding only a validity timestamp, a random nonce and the original request
// URL. Nothing ties it to the user agent it was issued to, and no cookie is set
// alongside it at login redirect time, so grantCallback accepts any state
// Skipper itself minted, in any browser.
func TestGrantLoginCSRF(t *testing.T) {
	const applicationDomain = "foo.skipper.test"

	dnstest.LoopbackNames(t, applicationDomain)

	provider := newIdentityAuthServer()
	defer provider.Close()

	tokeninfo := newIdentityTokeninfo()
	defer tokeninfo.Close()

	var upstreamAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	config := newGrantTestConfig(tokeninfo.URL, provider.URL)
	config.AccessTokenHeaderName = "Authorization"

	routes := eskip.MustParse(`* -> oauthGrant() -> "` + upstream.URL + `"`)

	proxy, _ := newAuthProxy(t, config, routes, applicationDomain)
	defer proxy.Close()

	t.Run("control: a normal login flow works", func(t *testing.T) {
		upstreamAuth = ""

		browser := newBrowser(t, proxy)

		rsp, err := browser.Get(proxy.URL + "/")
		require.NoError(t, err)
		rsp.Body.Close()

		for range 3 {
			rsp, err = browser.Get(rsp.Header.Get("Location"))
			require.NoError(t, err)
			rsp.Body.Close()
		}

		require.Equal(t, http.StatusNoContent, rsp.StatusCode)
		require.Equal(t, "Bearer victim-token", upstreamAuth)
	})

	t.Run("a callback crafted by the attacker must not log the victim in", func(t *testing.T) {
		upstreamAuth = ""

		// 1. The attacker starts a login flow on the victim's Skipper and keeps
		//    the state Skipper minted for them.
		attacker := newBrowser(t, proxy)

		rsp, err := attacker.Get(proxy.URL + "/")
		require.NoError(t, err)
		rsp.Body.Close()

		authCodeURL, err := url.Parse(rsp.Header.Get("Location"))
		require.NoError(t, err)

		state := authCodeURL.Query().Get("state")
		require.NotEmpty(t, state)
		t.Logf("attacker obtained state: %s...", state[:24])

		// 2. The attacker logs in at the provider as themselves and captures the
		//    authorization code from the callback redirect instead of following it.
		q := authCodeURL.Query()
		q.Set("login_as", "attacker")
		authCodeURL.RawQuery = q.Encode()

		rsp, err = attacker.Get(authCodeURL.String())
		require.NoError(t, err)
		rsp.Body.Close()

		callbackURL, err := url.Parse(rsp.Header.Get("Location"))
		require.NoError(t, err)
		require.Equal(t, "attacker-code", callbackURL.Query().Get("code"))
		t.Logf("attacker crafted callback: %s?code=%s&state=%s...",
			callbackURL.Path, callbackURL.Query().Get("code"), state[:24])

		// 3. The victim's browser, which never started a login flow and holds no
		//    Skipper cookie, follows that URL.
		victim := newBrowser(t, proxy)

		rsp, err = victim.Get(callbackURL.String())
		require.NoError(t, err)
		rsp.Body.Close()

		var session []*http.Cookie
		for _, c := range rsp.Cookies() {
			if c.Name == testCookieName && c.Value != "" {
				session = append(session, c)
			}
		}
		t.Logf("victim callback response: %d, session cookies set: %d", rsp.StatusCode, len(session))

		// 4. Whatever session the victim ended up with is now used.
		rsp, err = victim.Get(proxy.URL + "/")
		require.NoError(t, err)
		rsp.Body.Close()
		t.Logf("victim request after the callback: %d, upstream saw %q", rsp.StatusCode, upstreamAuth)

		require.Empty(t, session,
			"grantCallback issued a session cookie to a browser that never started a login flow")
		require.NotEqual(t, "Bearer attacker-token", upstreamAuth,
			"the victim's session carries the attacker's access token")
	})
}
