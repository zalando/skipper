package auth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/net/dnstest"
)

// newPKCEAuthServer is newIdentityAuthServer with S256 PKCE: it remembers the
// challenge sent with the authorization request and rejects a token request
// whose verifier does not match it.
func newPKCEAuthServer() *httptest.Server {
	var (
		mu         sync.Mutex
		challenges = map[string]string{}
	)

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

			mu.Lock()
			challenges[user+"-code"] = r.URL.Query().Get("code_challenge")
			mu.Unlock()

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

			mu.Lock()
			challenge := challenges[code]
			mu.Unlock()

			if challenge != "" {
				sum := sha256.Sum256([]byte(r.FormValue("code_verifier")))
				if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
			}

			b, _ := json.Marshal(map[string]any{
				"access_token": strings.TrimSuffix(code, "-code") + "-token",
				"expires_in":   3600,
			})
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestGrantAuthorizationCodeInjection checks that an authorization code issued
// for one login flow cannot be redeemed in another one.
//
// The authorization request carries neither a PKCE code challenge nor a nonce
// (filters/auth/grant.go:91, filters/auth/oidc.go:483), and the token request
// carries no code verifier (filters/auth/grantcallback.go:38), so the exchange
// is bound to nothing that only the browser which started the flow holds.
func TestGrantAuthorizationCodeInjection(t *testing.T) {
	const applicationDomain = "foo.skipper.test"

	dnstest.LoopbackNames(t, applicationDomain)

	provider := newPKCEAuthServer()
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

	login := func(t *testing.T, browser *http.Client, as string) *url.URL {
		t.Helper()

		rsp, err := browser.Get(proxy.URL + "/")
		require.NoError(t, err)
		rsp.Body.Close()

		authCodeURL, err := url.Parse(rsp.Header.Get("Location"))
		require.NoError(t, err)

		q := authCodeURL.Query()
		q.Set("login_as", as)
		authCodeURL.RawQuery = q.Encode()

		rsp, err = browser.Get(authCodeURL.String())
		require.NoError(t, err)
		rsp.Body.Close()

		callbackURL, err := url.Parse(rsp.Header.Get("Location"))
		require.NoError(t, err)

		return callbackURL
	}

	// 1. The victim authenticates and their code leaks before they redeem it.
	victimCallback := login(t, newBrowser(t, proxy), "victim")
	victimCode := victimCallback.Query().Get("code")
	require.Equal(t, "victim-code", victimCode)

	// 2. The attacker runs their own login flow and stops at the callback.
	attacker := newBrowser(t, proxy)
	attackerCallback := login(t, attacker, "attacker")

	// 3. The attacker redeems the victim's code in their own flow.
	injected := *attackerCallback
	q := injected.Query()
	q.Set("code", victimCode)
	injected.RawQuery = q.Encode()

	rsp, err := attacker.Get(injected.String())
	require.NoError(t, err)
	rsp.Body.Close()
	t.Logf("attacker callback with the victim's code: %d, session cookies set: %d", rsp.StatusCode, len(rsp.Cookies()))

	// 4. The attacker uses the session it produced.
	rsp, err = attacker.Get(proxy.URL + "/")
	require.NoError(t, err)
	rsp.Body.Close()
	t.Logf("attacker request afterwards: %d, upstream saw %q", rsp.StatusCode, upstreamAuth)

	require.NotEqual(t, "Bearer victim-token", upstreamAuth,
		"the attacker holds a session for the victim's token")
}
