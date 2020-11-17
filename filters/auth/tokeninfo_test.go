package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/log"
	"github.com/zalando/skipper/filters/tracing"
	"github.com/zalando/skipper/proxy/proxytest"
)

const testAuthTimeout = 100 * time.Millisecond

func TestOAuth2Tokeninfo(t *testing.T) {
	for _, ti := range []struct {
		msg           string
		authType      string
		authBaseURL   string
		args          []interface{}
		maskOauthUser string
		hasAuth       bool
		auth          string
		expected      int
		authUser      string
	}{{
		msg:      "uninitialized filter, no authorization header, scope check",
		authType: OAuthTokeninfoAnyScopeName,
		expected: http.StatusNotFound,
	}, {
		msg:         "invalid token",
		authType:    OAuthTokeninfoAnyScopeName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"not-matching-scope"},
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "invalid scope",
		authType:    OAuthTokeninfoAnyScopeName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusForbidden,
	}, {
		msg:         "oauthTokeninfoAnyScope: valid token, one valid scope",
		authType:    OAuthTokeninfoAnyScopeName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testScope},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "OAuthTokeninfoAnyScopeName: valid token, one valid scope, one invalid scope",
		authType:    OAuthTokeninfoAnyScopeName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testScope, "other-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "oauthTokeninfoAllScope(): valid token, valid scopes",
		authType:    OAuthTokeninfoAllScopeName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testScope, testScope2, testScope3},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "oauthTokeninfoAllScope(): valid token, one valid scope, one invalid scope",
		authType:    OAuthTokeninfoAllScopeName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testScope, "other-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusForbidden,
	}, {
		msg:         "anyKV(): invalid key",
		authType:    OAuthTokeninfoAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "anyKV(): valid token, one valid key, wrong value",
		authType:    OAuthTokeninfoAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusForbidden,
	}, {
		msg:         "anyKV(): valid token, one valid key value pair",
		authType:    OAuthTokeninfoAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "anyKV(): valid token, one valid kv, multiple key value pairs1",
		authType:    OAuthTokeninfoAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue, "wrongKey", "wrongValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "anyKV(): valid token, one valid kv, multiple key value pairs2",
		authType:    OAuthTokeninfoAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"wrongKey", "wrongValue", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "anyKV(): valid token, one valid kv, same key multiple times should pass",
		authType:    OAuthTokeninfoAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue, testKey, "someValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): invalid key",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusNotFound,
	}, {
		msg:         "allKV(): valid token, one valid key, wrong value",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusForbidden,
	}, {
		msg:         "allKV(): valid token, one valid key value pair",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): valid token, one valid key value pair, check realm",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testRealmKey, testRealm, testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): valid token, valid key value pairs",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue, testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): valid token, one valid kv, multiple key value pairs1",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, testValue, "wrongKey", "wrongValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusForbidden,
	}, {
		msg:         "allKV(): valid token, one valid kv, multiple key value pairs2",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"wrongKey", "wrongValue", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusForbidden,
	}, {
		msg:           "create tag for auth-user",
		authType:      OAuthTokeninfoAllKVName,
		authBaseURL:   testAuthPath,
		args:          []interface{}{testKey, testValue},
		hasAuth:       true,
		auth:          testToken,
		expected:      http.StatusOK,
		maskOauthUser: "immortals:/realm=foo",
		authUser:      "jdoe",
	}, {
		msg:           "create tag for auth-user (masked)",
		authType:      OAuthTokeninfoAllKVName,
		authBaseURL:   testAuthPath,
		args:          []interface{}{testKey, testValue},
		hasAuth:       true,
		auth:          testToken,
		expected:      http.StatusOK,
		maskOauthUser: "immortals:/realm=/immortals",
		authUser:      "immortals",
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {}))

			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

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
					"scope":      []string{testScope, testScope2, testScope3}}

				e := json.NewEncoder(w)
				if err := e.Encode(&d); err != nil {
					t.Error(err)
				}
			}))

			var spec filters.Spec
			args := []interface{}{}
			u := authServer.URL + ti.authBaseURL
			switch ti.authType {
			case OAuthTokeninfoAnyScopeName:
				spec = NewOAuthTokeninfoAnyScope(u, testAuthTimeout)
			case OAuthTokeninfoAllScopeName:
				spec = NewOAuthTokeninfoAllScope(u, testAuthTimeout)
			case OAuthTokeninfoAnyKVName:
				spec = NewOAuthTokeninfoAnyKV(u, testAuthTimeout)
			case OAuthTokeninfoAllKVName:
				spec = NewOAuthTokeninfoAllKV(u, testAuthTimeout)
			}

			args = append(args, ti.args...)
			f, err := spec.CreateFilter(args)
			if err != nil {
				t.Logf("error in creating filter")
				return
			}
			f2 := f.(*tokeninfoFilter)
			defer f2.Close()

			fr := make(filters.Registry)
			fr.Register(spec)
			filterDef := []*eskip.Filter{{Name: spec.Name(), Args: args}}
			filterDef = addMaskOAuthUser(t, ti.maskOauthUser, fr, filterDef)

			r := &eskip.Route{Filters: filterDef, Backend: backend.URL}

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

			if ti.hasAuth {
				req.Header.Set(authHeaderName, authHeaderPrefix+ti.auth)
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != ti.expected {
				t.Errorf("auth filter failed got=%d, expected=%d, route=%s", rsp.StatusCode, ti.expected, r)
				buf := make([]byte, rsp.ContentLength)
				rsp.Body.Read(buf)
			}

			assertAuthUser(t, proxy, ti.authUser)
		})
	}
}

func assertAuthUser(t *testing.T, proxy *proxytest.TestProxy, expected string) {
	if expected == "" {
		return
	}

	//spans are finished in goroutine
	assert.Eventually(t, func() bool {
		authUser := ""
		spans := proxy.Tracer.FinishedSpans()
		for _, span := range spans {
			if span.OperationName == "ingress" {
				tag := span.Tag("client_id")
				if tag != nil {
					authUser = tag.(string)
				}
			}
		}

		return expected == authUser
	}, time.Second, 10*time.Millisecond)
}

func addMaskOAuthUser(t *testing.T, maskOauthUser string, fr filters.Registry, filterDef []*eskip.Filter) []*eskip.Filter {
	if maskOauthUser != "" {
		user, err := ParseMaskOAuthUser(maskOauthUser)
		require.NoError(t, err)
		fr.Register(tracing.NewStateBagToTag(user))
		filterDef = append(filterDef, &eskip.Filter{
			Name: tracing.StateBagToTagFilterName, Args: []interface{}{log.AuthUserKey, "client_id"},
		})
	}
	return filterDef
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
		authType: OAuthTokeninfoAnyScopeName,
		expected: http.StatusOK,
	}, {
		msg:      "get token request timeout",
		timeout:  50 * time.Millisecond,
		authType: OAuthTokeninfoAnyScopeName,
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
			f, err := spec.CreateFilter(scopes)
			if err != nil {
				t.Error(err)
				return
			}
			f2 := f.(*tokeninfoFilter)
			defer f2.Close()

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
	allF := make([]*tokeninfoFilter, 0)
	for i := 0; i < b.N; i++ {
		var spec filters.Spec
		args := []interface{}{"uid"}
		spec = NewOAuthTokeninfoAnyScope("https://127.0.0.1:12345/token", 3*time.Second)
		f, err := spec.CreateFilter(args)
		if err != nil {
			b.Logf("error in creating filter")
			break
		}
		f2 := f.(*tokeninfoFilter)
		allF = append(allF, f2)
	}

	for i := range allF {
		allF[i].Close()
	}
}
