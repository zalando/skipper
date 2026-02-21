package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/annotate"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/proxy/proxytest"
)

const testAuthTimeout = 100 * time.Millisecond

func newOAuthTokeninfoSpec(authType string, options TokeninfoOptions) filters.Spec {
	switch authType {
	case filters.OAuthTokeninfoAnyScopeName:
		return NewOAuthTokeninfoAnyScopeWithOptions(options)
	case filters.OAuthTokeninfoAllScopeName:
		return NewOAuthTokeninfoAllScopeWithOptions(options)
	case filters.OAuthTokeninfoAnyKVName:
		return NewOAuthTokeninfoAnyKVWithOptions(options)
	case filters.OAuthTokeninfoAllKVName:
		return NewOAuthTokeninfoAllKVWithOptions(options)
	default:
		panic("unsupported auth type: " + authType)
	}
}

func TestOAuth2Tokeninfo(t *testing.T) {
	const N = 5

	for _, ti := range []struct {
		msg            string
		authType       string
		options        TokeninfoOptions
		args           []any
		auth           string
		expected       int
		expectRequests int32
	}{{
		msg:            "oauthTokeninfoAnyScope: invalid token",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []any{"not-matching-scope"},
		auth:           "invalid-token",
		expected:       http.StatusUnauthorized,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAnyScope: one invalid scope",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []any{"not-matching-scope"},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAnyScope: two invalid scopes",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []any{"not-matching-scope1", "not-matching-scope2"},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAnyScope: missing token",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []any{testScope},
		auth:           "",
		expected:       http.StatusUnauthorized,
		expectRequests: 0,
	}, {
		msg:            "oauthTokeninfoAnyScope: valid token, one valid scope",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []any{testScope},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAnyScope: valid token, one valid scope, one invalid scope",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []any{testScope, "other-scope"},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAnyScope: valid token, one invalid scope, one valid scope",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []any{"other-scope", testScope},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllScope: valid token, all valid scopes",
		authType:       filters.OAuthTokeninfoAllScopeName,
		args:           []any{testScope, testScope2, testScope3},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllScope: valid token, two valid scopes",
		authType:       filters.OAuthTokeninfoAllScopeName,
		args:           []any{testScope, testScope2},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllScope: valid token, one valid scope",
		authType:       filters.OAuthTokeninfoAllScopeName,
		args:           []any{testScope},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllScope: valid token, one valid scope, one invalid scope",
		authType:       filters.OAuthTokeninfoAllScopeName,
		args:           []any{testScope, "other-scope"},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAnyKV: valid token, one valid key, wrong value",
		authType:       filters.OAuthTokeninfoAnyKVName,
		args:           []any{testKey, "other-value"},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAnyKV: valid token, one valid key value pair",
		authType:       filters.OAuthTokeninfoAnyKVName,
		args:           []any{testKey, testValue},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAnyKV: valid token, one valid kv, multiple key value pairs1",
		authType:       filters.OAuthTokeninfoAnyKVName,
		args:           []any{testKey, testValue, "wrongKey", "wrongValue"},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAnyKV: valid token, one valid kv, multiple key value pairs2",
		authType:       filters.OAuthTokeninfoAnyKVName,
		args:           []any{"wrongKey", "wrongValue", testKey, testValue},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAnyKV: valid token, one valid kv, same key multiple times should pass",
		authType:       filters.OAuthTokeninfoAnyKVName,
		args:           []any{testKey, testValue, testKey, "someValue"},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllKV: valid token, one valid key, wrong value",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []any{testKey, "other-value"},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllKV: valid token, one valid key value pair",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []any{testKey, testValue},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllKV: valid token, one valid key value pair, check realm",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []any{testRealmKey, testRealm, testKey, testValue},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllKV: valid token, valid key value pairs",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []any{testKey, testValue, testKey, testValue},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllKV: valid token, one valid kv, multiple key value pairs1",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []any{testKey, testValue, "wrongKey", "wrongValue"},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllKV: valid token, one valid kv, multiple key value pairs2",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []any{"wrongKey", "wrongValue", testKey, testValue},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "caches valid token",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		options:        TokeninfoOptions{CacheSize: 1},
		args:           []any{testScope},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: 1,
	}, {
		msg:            "does not cache invalid token",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		options:        TokeninfoOptions{CacheSize: 1},
		args:           []any{testScope},
		auth:           "invalid-token",
		expected:       http.StatusUnauthorized,
		expectRequests: N,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {}))
			defer backend.Close()

			var authRequests int32
			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&authRequests, 1)

				if r.URL.Path != testAuthPath {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				token, ok := getToken(r)
				if !ok || token != testToken {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				fmt.Fprintf(w, `{"uid": "%s", "%s": "%s", "scope":["%s", "%s", "%s"], "expires_in": 600}`,
					testUID, testRealmKey, testRealm, testScope, testScope2, testScope3)
			}))
			defer authServer.Close()

			ti.options.URL = authServer.URL + testAuthPath
			ti.options.Timeout = testAuthTimeout

			t.Logf("ti.options: %#v", ti.options)

			spec := newOAuthTokeninfoSpec(ti.authType, ti.options)

			_, err := spec.CreateFilter(ti.args)
			if err != nil {
				t.Fatalf("error creating filter: %v", err)
			}

			fr := make(filters.Registry)
			fr.Register(spec)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: ti.args}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			defer proxy.Close()

			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
			}

			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Error(err)
				return
			}

			if ti.auth != "" {
				req.Header.Set(authHeaderName, authHeaderPrefix+ti.auth)
			}

			for range N {
				rsp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				rsp.Body.Close()

				if rsp.StatusCode != ti.expected {
					t.Errorf("auth filter failed got=%d, expected=%d, route=%s", rsp.StatusCode, ti.expected, r)
				}
			}

			if ti.expectRequests != authRequests {
				t.Errorf("expected %d auth requests, got: %d", ti.expectRequests, authRequests)
			}
		})
	}
}

func TestOAuth2TokeninfoInvalidArguments(t *testing.T) {
	for _, ti := range []struct {
		msg      string
		authType string
		args     []any
	}{
		{
			msg:      "missing arguments",
			authType: filters.OAuthTokeninfoAnyScopeName,
		}, {
			msg:      "anyKV(): wrong number of arguments",
			authType: filters.OAuthTokeninfoAnyKVName,
			args:     []any{"not-matching-scope"},
		}, {
			msg:      "allKV(): wrong number of arguments",
			authType: filters.OAuthTokeninfoAllKVName,
			args:     []any{"not-matching-scope"},
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			tio := TokeninfoOptions{
				URL:     "https://authserver.test/info",
				Timeout: testAuthTimeout,
			}
			spec := newOAuthTokeninfoSpec(ti.authType, tio)

			f, err := spec.CreateFilter(ti.args)
			if err == nil {
				t.Fatalf("expected error, got filter: %v", f)
			}
		})
	}
}

func TestOAuth2TokenTimeout(t *testing.T) {
	for _, ti := range []struct {
		msg      string
		timeout  time.Duration
		authType string
		expected int
	}{{
		msg:      "get token within specified timeout",
		timeout:  2 * testAuthTimeout,
		authType: filters.OAuthTokeninfoAnyScopeName,
		expected: http.StatusOK,
	}, {
		msg:      "get token request timeout",
		timeout:  50 * time.Millisecond,
		authType: filters.OAuthTokeninfoAnyScopeName,
		expected: http.StatusUnauthorized,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {}))
			defer backend.Close()

			handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != testAuthPath {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				token, ok := getToken(r)
				if !ok || token != testToken {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				d := map[string]any{
					"uid":        testUID,
					testRealmKey: testRealm,
					"scope":      []string{testScope},
				}

				time.Sleep(100 * time.Millisecond)

				b, err := json.Marshal(d)
				if err != nil {
					t.Fatal(err)
				}

				// we ignore the potential write error because the client may not wait for it
				w.Write(b)
			})
			authServer := httptest.NewServer(http.TimeoutHandler(handlerFunc, ti.timeout, "server unavailable"))
			defer authServer.Close()

			args := []any{testScope}
			u := authServer.URL + testAuthPath
			spec := NewOAuthTokeninfoAnyScope(u, ti.timeout)

			scopes := []any{"read-x"}
			_, err := spec.CreateFilter(scopes)
			if err != nil {
				t.Error(err)
				return
			}

			fr := make(filters.Registry)
			fr.Register(spec)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			defer proxy.Close()
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
				return
			}

			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set(authHeaderName, authHeaderPrefix+testToken)

			resp, err := http.DefaultClient.Do(req)

			if err != nil {
				t.Error(err)
				return
			}

			defer resp.Body.Close()

			if resp.StatusCode != ti.expected {
				t.Errorf("auth filter failed got=%d, expected=%d, route=%s", resp.StatusCode, ti.expected, r)
				buf := make([]byte, resp.ContentLength)
				resp.Body.Read(buf)
			}
		})
	}
}

func TestOAuth2Tokeninfo5xx(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer authServer.Close()

	opts := TokeninfoOptions{
		URL:     authServer.URL + testAuthPath,
		Timeout: testAuthTimeout,
	}
	t.Logf("ti.options: %#v", opts)

	spec := newOAuthTokeninfoSpec(filters.OAuthTokeninfoAnyScopeName, opts)

	fr := make(filters.Registry)
	fr.Register(spec)
	r := eskip.MustParse(fmt.Sprintf(`* -> oauthTokeninfoAnyScope("%s") -> "%s"`, testScope, backend.URL))

	proxy := proxytest.New(fr, r...)
	defer proxy.Close()

	req, err := http.NewRequest("GET", proxy.URL, nil)
	require.NoError(t, err)

	req.Header.Set(authHeaderName, authHeaderPrefix+testToken)

	rsp, err := proxy.Client().Do(req)
	require.NoError(t, err)
	rsp.Body.Close()

	// We return 401 for 5xx responses from the auth server, but log the error for visibility.
	require.Equal(t, http.StatusUnauthorized, rsp.StatusCode, "auth filter failed got=%d, expected=%d, route=%s", rsp.StatusCode, http.StatusUnauthorized, r)
}

func BenchmarkOAuthTokeninfoCreateFilter(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var spec filters.Spec
		args := []any{"uid"}
		spec = NewOAuthTokeninfoAnyScope("https://127.0.0.1:12345/token", 3*time.Second)
		_, err := spec.CreateFilter(args)
		if err != nil {
			b.Logf("error creating filter")
			break
		}
	}
}

func BenchmarkOAuthTokeninfoRequest(b *testing.B) {
	b.Run("oauthTokeninfoAllScope", func(b *testing.B) {
		spec := NewOAuthTokeninfoAllScope("https://127.0.0.1:12345/token", 3*time.Second)
		f, err := spec.CreateFilter([]any{"foobar.read", "foobar.write"})
		require.NoError(b, err)

		ctx := &filtertest.Context{
			FStateBag: map[string]any{
				tokeninfoCacheKey: map[string]any{
					scopeKey: []any{"uid", "foobar.read", "foobar.write"},
				},
			},
			FResponse: &http.Response{},
		}

		f.Request(ctx)
		require.False(b, ctx.FServed)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			f.Request(ctx)
		}
	})

	b.Run("oauthTokeninfoAnyScope", func(b *testing.B) {
		spec := NewOAuthTokeninfoAnyScope("https://127.0.0.1:12345/token", 3*time.Second)
		f, err := spec.CreateFilter([]any{"foobar.read", "foobar.write"})
		require.NoError(b, err)

		ctx := &filtertest.Context{
			FStateBag: map[string]any{
				tokeninfoCacheKey: map[string]any{
					scopeKey: []any{"uid", "foobar.write", "foobar.exec"},
				},
			},
			FResponse: &http.Response{},
		}

		f.Request(ctx)
		require.False(b, ctx.FServed)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			f.Request(ctx)
		}
	})
}

func TestOAuthTokeninfoAllocs(t *testing.T) {
	tio := TokeninfoOptions{
		URL:     "https://127.0.0.1:12345/token",
		Timeout: 3 * time.Second,
	}

	fr := make(filters.Registry)
	fr.Register(NewOAuthTokeninfoAllScopeWithOptions(tio))
	fr.Register(NewOAuthTokeninfoAnyScopeWithOptions(tio))
	fr.Register(NewOAuthTokeninfoAllKVWithOptions(tio))
	fr.Register(NewOAuthTokeninfoAnyKVWithOptions(tio))

	var filters []filters.Filter
	for _, def := range eskip.MustParseFilters(`
		oauthTokeninfoAnyScope("foobar.read", "foobar.write") ->
		oauthTokeninfoAllScope("foobar.read", "foobar.write") ->
		oauthTokeninfoAnyKV("k1", "v1", "k2", "v2") ->
		oauthTokeninfoAllKV("k1", "v1", "k2", "v2")
	`) {
		f, err := fr[def.Name].CreateFilter(def.Args)
		require.NoError(t, err)

		filters = append(filters, f)
	}

	ctx := &filtertest.Context{
		FStateBag: map[string]any{
			tokeninfoCacheKey: map[string]any{
				scopeKey: []any{"uid", "foobar.read", "foobar.write", "foobar.exec"},
				"k1":     "v1",
				"k2":     "v2",
			},
		},
		FResponse: &http.Response{},
	}

	allocs := testing.AllocsPerRun(100, func() {
		for _, f := range filters {
			f.Request(ctx)
		}
		require.False(t, ctx.FServed)
	})
	if allocs != 0.0 {
		t.Errorf("Expected zero allocations, got %f", allocs)
	}
}

func TestOAuthTokeninfoValidate(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	const validAuthHeader = "Bearer foobarbaz"

	var authRequestsTotal atomic.Int32
	testAuthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authRequestsTotal.Add(1)
		if r.Header.Get("Authorization") != validAuthHeader {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.Write([]byte(`{"uid": "foobar", "scope":["foo", "bar"]}`))
		}
	}))
	defer testAuthServer.Close()

	tio := TokeninfoOptions{
		URL:     testAuthServer.URL,
		Timeout: testAuthTimeout,
	}

	const (
		unauthorizedResponse = `Authentication required, see https://auth.test/foo`

		oauthTokeninfoValidateDef = `oauthTokeninfoValidate("{optOutAnnotations: [oauth.disabled], optOutHosts: [ '^.+[.]domain[.]test$', '^exact[.]test$' ], unauthorizedResponse: '` + unauthorizedResponse + `'}")`
	)

	for _, tc := range []struct {
		name               string
		precedingFilters   string
		host               string
		authHeader         string
		expectStatus       int
		expectAuthRequests int32
	}{
		{
			name:               "reject missing token",
			authHeader:         "",
			expectStatus:       http.StatusUnauthorized,
			expectAuthRequests: 0,
		},
		{
			name:               "reject invalid token",
			authHeader:         "Bearer invalid",
			expectStatus:       http.StatusUnauthorized,
			expectAuthRequests: 1,
		},
		{
			name:               "reject invalid token type",
			authHeader:         "Basic foobarbaz",
			expectStatus:       http.StatusUnauthorized,
			expectAuthRequests: 0,
		},
		{
			name:               "reject missing token when opt-out is invalid",
			precedingFilters:   `annotate("oauth.invalid", "invalid opt-out")`,
			authHeader:         "",
			expectStatus:       http.StatusUnauthorized,
			expectAuthRequests: 0,
		},
		{
			name:               "allow valid token",
			authHeader:         validAuthHeader,
			expectStatus:       http.StatusOK,
			expectAuthRequests: 1,
		},
		{
			name:               "allow already validated by a preceding filter",
			precedingFilters:   `oauthTokeninfoAllScope("foo", "bar")`,
			authHeader:         validAuthHeader,
			expectStatus:       http.StatusOK,
			expectAuthRequests: 1, // called once by oauthTokeninfoAllScope
		},
		{
			name:               "allow missing token when opted-out",
			precedingFilters:   `annotate("oauth.disabled", "this endpoint is public")`,
			authHeader:         "",
			expectStatus:       http.StatusOK,
			expectAuthRequests: 0,
		},
		{
			name:               "allow invalid token when opted-out",
			precedingFilters:   `annotate("oauth.disabled", "this endpoint is public")`,
			authHeader:         "Bearer invalid",
			expectStatus:       http.StatusOK,
			expectAuthRequests: 0,
		},
		{
			name:               "allow invalid token type when opted-out",
			precedingFilters:   `annotate("oauth.disabled", "this endpoint is public")`,
			authHeader:         "Basic foobarbaz",
			expectStatus:       http.StatusOK,
			expectAuthRequests: 0,
		},
		{
			name:               "allow missing token when request host matches opted-out domain",
			host:               "foo.domain.test",
			authHeader:         "",
			expectStatus:       http.StatusOK,
			expectAuthRequests: 0,
		},
		{
			name:               "allow missing token when request second level host matches opted-out domain",
			host:               "foo.bar.domain.test",
			authHeader:         "",
			expectStatus:       http.StatusOK,
			expectAuthRequests: 0,
		},
		{
			name:               "allow missing token when request host matches opted-out host exactly",
			host:               "exact.test",
			authHeader:         "",
			expectStatus:       http.StatusOK,
			expectAuthRequests: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fr := make(filters.Registry)
			fr.Register(annotate.New())
			fr.Register(NewOAuthTokeninfoAllScopeWithOptions(tio))
			fr.Register(NewOAuthTokeninfoValidate(tio))

			filters := oauthTokeninfoValidateDef
			if tc.precedingFilters != "" {
				filters = tc.precedingFilters + " -> " + filters
			}

			p := proxytest.New(fr, eskip.MustParse(fmt.Sprintf(`* -> %s -> "%s"`, filters, backend.URL))...)
			defer p.Close()

			authRequestsBefore := authRequestsTotal.Load()

			req, err := http.NewRequest("GET", p.URL, nil)
			require.NoError(t, err)

			if tc.host != "" {
				req.Host = tc.host
			}

			if tc.authHeader != "" {
				req.Header.Set(authHeaderName, tc.authHeader)
			}

			resp, err := p.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectStatus, resp.StatusCode)
			assert.Equal(t, tc.expectAuthRequests, authRequestsTotal.Load()-authRequestsBefore)

			if resp.StatusCode == http.StatusUnauthorized {
				assert.Equal(t, resp.ContentLength, int64(len(unauthorizedResponse)))

				b, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				assert.Equal(t, unauthorizedResponse, string(b))
			}
		})
	}
}

func TestOAuthTokeninfoValidateArgs(t *testing.T) {
	tio := TokeninfoOptions{
		URL:     "https://auth.test",
		Timeout: testAuthTimeout,
	}
	spec := NewOAuthTokeninfoValidate(tio)

	t.Run("valid", func(t *testing.T) {
		for _, def := range []string{
			`oauthTokeninfoValidate("{}")`,
			`oauthTokeninfoValidate("{optOutAnnotations: [oauth.disabled], optOutHosts: [ '^.+[.]domain[.]test$', '^exact[.]test$' ]}")`,
			`oauthTokeninfoValidate("{unauthorizedResponse: 'Authentication required, see https://auth.test/foo'}")`,
			`oauthTokeninfoValidate("{optOutAnnotations: [oauth.disabled], optOutHosts: [ '^.+[.]domain[.]test$', '^exact[.]test$' ], unauthorizedResponse: 'Authentication required, see https://auth.test/foo'}")`,
		} {
			t.Run(def, func(t *testing.T) {
				f := eskip.MustParseFilters(def)[0]
				require.Equal(t, spec.Name(), f.Name)

				_, err := spec.CreateFilter(f.Args)
				assert.NoError(t, err)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		for _, def := range []string{
			`oauthTokeninfoValidate()`,
			`oauthTokeninfoValidate("iss")`,
			`oauthTokeninfoValidate(1)`,
			`oauthTokeninfoValidate("{optOutAnnotations: [oauth.disabled]}", "extra arg")`,
			`oauthTokeninfoValidate("{optOutHosts: [ '[' ]}")`, // invalid regexp
		} {
			t.Run(def, func(t *testing.T) {
				f := eskip.MustParseFilters(def)[0]
				require.Equal(t, spec.Name(), f.Name)

				_, err := spec.CreateFilter(f.Args)
				t.Logf("%v", err)
				assert.Error(t, err)
			})
		}
	})
}
