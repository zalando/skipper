package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

// TestTokenintrospectionSharedClientCredentials checks that a route introspects
// with the client credentials it was configured with.
//
// issuerAuthClient (filters/auth/tokenintrospection.go:85) caches one authClient
// per issuer URL, and CreateFilter writes the client credentials into that
// shared client (tokenintrospection.go:337), so every route on the same issuer
// ends up using the credentials of whichever route was created last.
func TestTokenintrospectionSharedClientCredentials(t *testing.T) {
	const (
		token = "shared-client-token"
		clock = 2 * time.Second
	)

	var seen []string

	introspection := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, _, ok := r.BasicAuth()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		seen = append(seen, client)

		// The authorization server only lets rs-b introspect this token, and
		// reports it as not active to anybody else. RFC 7662 section 2.2.
		info := tokenIntrospectionInfo{"active": false}
		if client == "rs-b" {
			info = tokenIntrospectionInfo{
				"active": true,
				"sub":    "someone",
				"claims": map[string]any{"uid": "someone"},
			}
		}

		json.NewEncoder(w).Encode(&info)
	}))
	defer introspection.Close()

	cfg := getTestOidcConfig()
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != TokenIntrospectionConfigPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(cfg)
	}))
	defer issuer.Close()

	cfg.IntrospectionEndpoint = introspection.URL
	cfg.ClaimsSupported = append(cfg.ClaimsSupported, "uid")

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	fr := make(filters.Registry)
	fr.Register(NewSecureOAuthTokenintrospectionAnyClaims(clock))

	routes := eskip.MustParse(`
		a: Path("/a") -> secureOauthTokenintrospectionAnyClaims("` + issuer.URL + `", "rs-a", "secret-a", "uid") -> "` + backend.URL + `";
		b: Path("/b") -> secureOauthTokenintrospectionAnyClaims("` + issuer.URL + `", "rs-b", "secret-b", "uid") -> "` + backend.URL + `";
	`)

	proxy := proxytest.New(fr, routes...)
	defer proxy.Close()

	get := func(path string) int {
		req, err := http.NewRequest("GET", proxy.URL+path, nil)
		require.NoError(t, err)
		req.Header.Set(authHeaderName, authHeaderPrefix+token)

		rsp, err := proxy.Client().Do(req)
		require.NoError(t, err)
		rsp.Body.Close()
		return rsp.StatusCode
	}

	seen = nil
	statusA := get("/a")
	clientA := seen
	t.Logf("route a (configured as rs-a): status=%d, introspected as %v", statusA, clientA)

	seen = nil
	statusB := get("/b")
	t.Logf("route b (configured as rs-b): status=%d, introspected as %v", statusB, seen)

	require.Equal(t, http.StatusUnauthorized, statusA,
		"rs-a is not allowed to introspect this token, so route a must reject it")
	require.Equal(t, []string{"rs-a"}, clientA,
		"route a introspected with another route's client credentials")
}
