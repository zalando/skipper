package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

const (
	testToken       = "test-token"
	testUID         = "jdoe"
	testScope       = "test-scope"
	testScope2      = "test-scope2"
	testScope3      = "test-scope3"
	testRealmKey    = "/realm"
	testRealm       = "/immortals"
	testKey         = "uid"
	testValue       = "jdoe"
	testAuthPath    = "/test-auth"
	testAuthTimeout = 100 * time.Millisecond
)

type testAuthDoc struct {
	authMap map[string]interface{}
}

func lastQueryValue(url string) string {
	s := strings.Split(url, "=")
	if len(s) == 0 {
		return ""
	}

	return s[len(s)-1]
}

func Test_all(t *testing.T) {
	for _, ti := range []struct {
		msg      string
		l        []string
		r        []string
		expected bool
	}{{
		msg:      "l and r nil",
		l:        nil,
		r:        nil,
		expected: true,
	}, {
		msg:      "l is nil and r has one",
		l:        nil,
		r:        []string{"s1"},
		expected: true,
	}, {
		msg:      "l has one and r has one, same",
		l:        []string{"s1"},
		r:        []string{"s1"},
		expected: true,
	}, {
		msg:      "l has one and r has one, different",
		l:        []string{"l"},
		r:        []string{"r"},
		expected: false,
	}, {
		msg:      "l has one and r has two, one same different 1",
		l:        []string{"l"},
		r:        []string{"l", "r"},
		expected: true,
	}, {
		msg:      "l has one and r has two, one same different 2",
		l:        []string{"l"},
		r:        []string{"r", "l"},
		expected: true,
	}, {
		msg:      "l has two and r has two, one different",
		l:        []string{"l", "l2"},
		r:        []string{"r", "l"},
		expected: false,
	}, {
		msg:      "l has two and r has two, both same same 1",
		l:        []string{"l", "r"},
		r:        []string{"r", "l"},
		expected: true,
	}, {
		msg:      "l has two and r has two, both same same 2",
		l:        []string{"r", "l"},
		r:        []string{"r", "l"},
		expected: true,
	}, {
		msg:      "l has N and r has M, r has all of left",
		l:        []string{"r1", "l"},
		r:        []string{"r2", "l", "r1"},
		expected: true,
	}, {
		msg:      "l has N and r has M, l has all of right",
		l:        []string{"l1", "r1", "l2"},
		r:        []string{"r1", "l1"},
		expected: false,
	}, {
		msg:      "l has N and r has M, l is missing one of r",
		l:        []string{"r1", "l1"},
		r:        []string{"r1", "l1", "r2"},
		expected: true,
	}, {
		msg:      "l has N and r has M, r is missing one of l",
		l:        []string{"r1", "l1", "l2"},
		r:        []string{"r1", "l1", "r2"},
		expected: false,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			if all(ti.l, ti.r) != ti.expected {
				t.Errorf("Failed test: %s", ti.msg)
			}
		})

	}
}

func TestOAuth2Tokeninfo(t *testing.T) {
	for _, ti := range []struct {
		msg         string
		authType    string
		authBaseURL string
		args        []interface{}
		hasAuth     bool
		auth        string
		expected    int
	}{{
		msg:      "uninitialized filter, no authorization header, scope check",
		authType: OAuthTokeninfoAnyScopeName,
		expected: http.StatusNotFound,
	}, {
		msg:         "invalid token",
		authType:    OAuthTokeninfoAnyScopeName,
		authBaseURL: testAuthPath,
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusNotFound,
	}, {
		msg:         "invalid scope",
		authType:    OAuthTokeninfoAnyScopeName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
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
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "anyKV(): invalid key",
		authType:    OAuthTokeninfoAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusNotFound,
	}, {
		msg:         "anyKV(): valid token, one valid key, wrong value",
		authType:    OAuthTokeninfoAnyKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
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
		expected:    http.StatusUnauthorized,
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
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "allKV(): valid token, one valid kv, multiple key value pairs2",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath,
		args:        []interface{}{"wrongKey", "wrongValue", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {}))

			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

				if r.URL.Path != testAuthPath {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				token, err := getToken(r)
				if err != nil || token != testToken {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				d := map[string]interface{}{
					"uid":        testUID,
					testRealmKey: testRealm,
					"scope":      []string{testScope, testScope2, testScope3}}

				e := json.NewEncoder(w)
				err = e.Encode(&d)
				if err != nil {
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
			scopes := []string{"read-x"}
			s := make([]interface{}, len(scopes))
			for i, v := range scopes {
				s[i] = v
			}
			f, _ := spec.CreateFilter(s)
			f2 := f.(*filter)
			defer f2.Close()

			args = append(args, ti.args...)
			fr := make(filters.Registry)
			fr.Register(spec)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
			}

			// test accessToken in querystring and header
			for _, name := range []string{"query", "header"} {
				if ti.hasAuth && name == "query" {
					q := reqURL.Query()
					q.Add(accessTokenQueryKey, ti.auth)
					reqURL.RawQuery = q.Encode()
				}

				req, err := http.NewRequest("GET", reqURL.String(), nil)
				if err != nil {
					t.Error(err)
					return
				}

				if ti.hasAuth && name == "header" {
					req.Header.Set(authHeaderName, "Bearer "+url.QueryEscape(ti.auth))
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
			}
		})
	}
}

func TestOAuth2TokenTimeout(t *testing.T) {
	for _, ti := range []struct {
		msg      string
		timeout  time.Duration
		auth     string
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

			handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != testAuthPath {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				token, err := getToken(r)
				if err != nil || token != testToken {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				d := map[string]interface{}{
					"uid":        testUID,
					testRealmKey: testRealm,
					"scope":      []string{testScope},
				}

				time.Sleep(100 * time.Millisecond)
				e := json.NewEncoder(w)
				err = e.Encode(&d)
				if err != nil {
					t.Error(err)
				}
			})
			authServer := httptest.NewServer(http.TimeoutHandler(handlerFunc, ti.timeout, "server unavailable"))

			args := []interface{}{testScope}
			u := authServer.URL + testAuthPath
			spec := NewOAuthTokeninfoAnyScope(u, ti.timeout)
			scopes := []string{"read-x"}
			s := make([]interface{}, len(scopes))
			for i, v := range scopes {
				s[i] = v
			}
			f, _ := spec.CreateFilter(s)
			f2 := f.(*filter)
			defer f2.Close()

			fr := make(filters.Registry)
			fr.Register(spec)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
			}

			q := reqURL.Query()
			q.Add(accessTokenQueryKey, testToken)
			reqURL.RawQuery = q.Encode()

			req, err := http.NewRequest("GET", reqURL.String(), nil)

			if err != nil {
				t.Error(err)
				return
			}

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
