package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net/dnstest"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/secrets/secrettest"
)

// TestOidcLoginCSRF is TestGrantLoginCSRF for the oauthOidc* filters: the state
// created in doOauthRedirect (filters/auth/oidc.go) is an encrypted blob with a
// validity and a random nonce, and nothing is stored in the browser that the
// callback could compare it against, so callbackEndpoint accepts a state in a
// user agent that never started the flow.
func TestOidcLoginCSRF(t *testing.T) {
	dnstest.LoopbackNames(t, "skipper.test")

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	fd, err := os.CreateTemp("", "testSecrets")
	require.NoError(t, err)
	secretsFile := fd.Name()
	defer func() { os.Remove(secretsFile) }()

	fr := make(filters.Registry)
	fr.Register(NewOAuthOidcAnyClaimsWithOptions(secretsFile, secrettest.NewTestRegistry(), OidcOptions{}))

	dc := testdataclient.New(nil)
	defer dc.Close()

	proxy := proxytest.WithRoutingOptions(fr, routing.Options{
		DataClients: []routing.DataClient{dc},
	})
	defer proxy.Close()

	redirectURL, _ := url.Parse(proxy.URL)
	redirectURL.Path = "/redirect"

	oidcServer := createOIDCServer(redirectURL.String(), "valid-client", "mysec", nil, nil)
	defer oidcServer.Close()

	f, err := parseFilter(
		`oauthOidcAnyClaims("{{ .OIDCServerURL }}", "valid-client", "mysec", "{{ .RedirectURL }}", "", "")`,
		oidcServer.URL, redirectURL.String())
	require.NoError(t, err)

	proxy.Log.Reset()
	dc.Update([]*eskip.Route{{Filters: f, Backend: backend.URL}}, nil)
	require.NoError(t, proxy.Log.WaitFor("route settings applied", 10*time.Second))

	noRedirects := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// 1. The attacker starts the flow on the victim's Skipper and follows it to
	//    the provider, where they authenticate as themselves.
	attacker := &http.Client{Timeout: 5 * time.Second, CheckRedirect: noRedirects}

	rsp, err := attacker.Get(proxy.URL)
	require.NoError(t, err)
	rsp.Body.Close()
	require.Equal(t, http.StatusTemporaryRedirect, rsp.StatusCode)

	rsp, err = attacker.Get(rsp.Header.Get("Location"))
	require.NoError(t, err)
	rsp.Body.Close()

	// 2. The provider hands back code + state. The attacker captures the URL
	//    instead of following it.
	callbackURL := rsp.Header.Get("Location")
	parsed, err := url.Parse(callbackURL)
	require.NoError(t, err)
	require.NotEmpty(t, parsed.Query().Get("code"))
	require.NotEmpty(t, parsed.Query().Get("state"))
	t.Logf("attacker captured callback: %s?code=%s&state=%s...",
		parsed.Path, parsed.Query().Get("code"), parsed.Query().Get("state")[:24])

	// 3. The victim's browser, which never started a flow and holds no Skipper
	//    cookie, follows that URL.
	victim := &http.Client{Timeout: 5 * time.Second, CheckRedirect: noRedirects}

	rsp, err = victim.Get(callbackURL)
	require.NoError(t, err)
	rsp.Body.Close()

	t.Logf("victim callback response: %d, cookies set: %d", rsp.StatusCode, len(rsp.Cookies()))
	for _, c := range rsp.Cookies() {
		t.Logf("  cookie %s (len %d)", c.Name, len(c.Value))
	}

	require.Empty(t, rsp.Cookies(),
		"callbackEndpoint issued a session cookie to a browser that never started the flow")
}
