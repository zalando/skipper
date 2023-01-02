package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
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
		args           []interface{}
		auth           string
		expected       int
		expectRequests int32
	}{{
		msg:            "invalid token",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []interface{}{"not-matching-scope"},
		auth:           "invalid-token",
		expected:       http.StatusUnauthorized,
		expectRequests: N,
	}, {
		msg:            "invalid scope",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []interface{}{"not-matching-scope"},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "missing token",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []interface{}{testScope},
		auth:           "",
		expected:       http.StatusUnauthorized,
		expectRequests: 0,
	}, {
		msg:            "oauthTokeninfoAnyScope: valid token, one valid scope",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []interface{}{testScope},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "OAuthTokeninfoAnyScopeName: valid token, one valid scope, one invalid scope",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		args:           []interface{}{testScope, "other-scope"},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllScope(): valid token, valid scopes",
		authType:       filters.OAuthTokeninfoAllScopeName,
		args:           []interface{}{testScope, testScope2, testScope3},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "oauthTokeninfoAllScope(): valid token, one valid scope, one invalid scope",
		authType:       filters.OAuthTokeninfoAllScopeName,
		args:           []interface{}{testScope, "other-scope"},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "anyKV(): valid token, one valid key, wrong value",
		authType:       filters.OAuthTokeninfoAnyKVName,
		args:           []interface{}{testKey, "other-value"},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "anyKV(): valid token, one valid key value pair",
		authType:       filters.OAuthTokeninfoAnyKVName,
		args:           []interface{}{testKey, testValue},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "anyKV(): valid token, one valid kv, multiple key value pairs1",
		authType:       filters.OAuthTokeninfoAnyKVName,
		args:           []interface{}{testKey, testValue, "wrongKey", "wrongValue"},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "anyKV(): valid token, one valid kv, multiple key value pairs2",
		authType:       filters.OAuthTokeninfoAnyKVName,
		args:           []interface{}{"wrongKey", "wrongValue", testKey, testValue},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "anyKV(): valid token, one valid kv, same key multiple times should pass",
		authType:       filters.OAuthTokeninfoAnyKVName,
		args:           []interface{}{testKey, testValue, testKey, "someValue"},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "allKV(): valid token, one valid key, wrong value",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []interface{}{testKey, "other-value"},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "allKV(): valid token, one valid key value pair",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []interface{}{testKey, testValue},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "allKV(): valid token, one valid key value pair, check realm",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []interface{}{testRealmKey, testRealm, testKey, testValue},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "allKV(): valid token, valid key value pairs",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []interface{}{testKey, testValue, testKey, testValue},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: N,
	}, {
		msg:            "allKV(): valid token, one valid kv, multiple key value pairs1",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []interface{}{testKey, testValue, "wrongKey", "wrongValue"},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "allKV(): valid token, one valid kv, multiple key value pairs2",
		authType:       filters.OAuthTokeninfoAllKVName,
		args:           []interface{}{"wrongKey", "wrongValue", testKey, testValue},
		auth:           testToken,
		expected:       http.StatusForbidden,
		expectRequests: N,
	}, {
		msg:            "caches valid token",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		options:        TokeninfoOptions{CacheSize: 1},
		args:           []interface{}{testScope},
		auth:           testToken,
		expected:       http.StatusOK,
		expectRequests: 1,
	}, {
		msg:            "does not cache invalid token",
		authType:       filters.OAuthTokeninfoAnyScopeName,
		options:        TokeninfoOptions{CacheSize: 1},
		args:           []interface{}{testScope},
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

			for i := 0; i < N; i++ {
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
		args     []interface{}
	}{
		{
			msg:      "missing arguments",
			authType: filters.OAuthTokeninfoAnyScopeName,
		}, {
			msg:      "anyKV(): wrong number of arguments",
			authType: filters.OAuthTokeninfoAnyKVName,
			args:     []interface{}{"not-matching-scope"},
		}, {
			msg:      "allKV(): wrong number of arguments",
			authType: filters.OAuthTokeninfoAllKVName,
			args:     []interface{}{"not-matching-scope"},
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

				d := map[string]interface{}{
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

			args := []interface{}{testScope}
			u := authServer.URL + testAuthPath
			spec := NewOAuthTokeninfoAnyScope(u, ti.timeout)

			scopes := []interface{}{"read-x"}
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

func BenchmarkOAuthTokeninfoFilter(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var spec filters.Spec
		args := []interface{}{"uid"}
		spec = NewOAuthTokeninfoAnyScope("https://127.0.0.1:12345/token", 3*time.Second)
		_, err := spec.CreateFilter(args)
		if err != nil {
			b.Logf("error creating filter")
			break
		}
	}
}
