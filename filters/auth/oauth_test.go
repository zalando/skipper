package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

const (
	testToken    = "test-token"
	testUID      = "jdoe"
	testScope    = "test-scope"
	testScope2   = "test-scope2"
	testScope3   = "test-scope3"
	testRealm    = "/immortals"
	testKey      = "uid"
	testValue    = "jdoe"
	testAuthPath = "/test-auth"
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
		msg:         "invalid token, without realm",
		authType:    OAuthTokeninfoAnyScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusNotFound,
	}, {
		msg:         "invalid scope",
		authType:    OAuthTokeninfoAnyScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{"not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		// FIXME
		// 	msg:         "oauthTokeninfoAnyScope: valid token, one valid scope",
		// 	authType:    OAuthTokeninfoAnyScopeName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testScope},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		// 	msg:         "OAuthTokeninfoAnyScopeName: valid token, one valid scope, one invalid scope",
		// 	authType:    OAuthTokeninfoAnyScopeName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testScope, "other-scope"},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		// 	msg:         "oauthTokeninfoAllScope(): valid token, valid scopes",
		// 	authType:    OAuthTokeninfoAllScopeName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testScope, testScope2, testScope3},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		msg:         "oauthTokeninfoAllScope(): valid token, one valid scope, one invalid scope",
		authType:    OAuthTokeninfoAllScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testScope, "other-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "anyKV(): invalid key",
		authType:    OAuthTokeninfoAnyKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{"not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusNotFound,
	}, {
		msg:         "anyKV(): valid token, one valid key, wrong value",
		authType:    OAuthTokeninfoAnyKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		// 	msg:         "anyKV(): valid token, one valid key value pair",
		// 	authType:    OAuthTokeninfoAnyKVName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testKey, testValue},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		// 	msg:         "anyKV(): valid token, one valid kv, multiple key value pairs1",
		// 	authType:    OAuthTokeninfoAnyKVName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testKey, testValue, "wrongKey", "wrongValue"},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		// 	msg:         "anyKV(): valid token, one valid kv, multiple key value pairs2",
		// 	authType:    OAuthTokeninfoAnyKVName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{"wrongKey", "wrongValue", testKey, testValue},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		msg:         "allKV(): invalid key",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{"not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusNotFound,
	}, {
		msg:         "allKV(): valid token, one valid key, wrong value",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		// 	msg:         "allKV(): valid token, one valid key value pair",
		// 	authType:    OAuthTokeninfoAllKVName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testKey, testValue},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		// 	msg:         "allKV(): valid token, valid key value pairs",
		// 	authType:    OAuthTokeninfoAllKVName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testKey, testValue, testKey, testValue},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		msg:         "allKV(): valid token, one valid kv, multiple key value pairs1",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testKey, testValue, "wrongKey", "wrongValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "allKV(): valid token, one valid kv, multiple key value pairs2",
		authType:    OAuthTokeninfoAllKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{"wrongKey", "wrongValue", testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {

		// TODO Realm checks

		msg:         "invalid realm, realm check",
		authType:    OAuthTokeninfoRealmAnyScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{"/not-matching-realm"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "invalid realm, realm check, valid token, one valid scope",
		authType:    OAuthTokeninfoRealmAnyScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{"/invalid", testScope},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "invalid scope",
		authType:    OAuthTokeninfoRealmAnyScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, "not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		// 	msg:         "OAuthTokeninfoRealmAnyScopeName: valid token, one valid scope",
		// 	authType:    OAuthTokeninfoRealmAnyScopeName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testRealm, testScope},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		// 	msg:         "valid token, one valid scope, one invalid scope",
		// 	authType:    OAuthTokeninfoRealmAnyScopeName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testRealm, testScope, "other-scope"},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		// 	msg:         "oauthTokeninfoRealmAllScope(): valid token, valid scopes",
		// 	authType:    OAuthTokeninfoRealmAllScopeName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testRealm, testScope, testScope2, testScope3},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		msg:         "oauthTokeninfoRealmAllScope(): valid token, one valid scope, one invalid scope",
		authType:    OAuthTokeninfoRealmAllScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, testScope, "other-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "anyKV(): invalid key",
		authType:    OAuthTokeninfoRealmAnyKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, "not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusNotFound,
	}, {
		msg:         "anyKV(): valid token, one valid key, wrong value",
		authType:    OAuthTokeninfoRealmAnyKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
		// }, {
		// 	msg:         "anyKV(): valid token, one valid key value pair",
		// 	authType:    OAuthTokeninfoRealmAnyKVName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testRealm, testKey, testValue},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		// 	msg:         "anyKV(): valid token, one valid kv, multiple key value pairs1",
		// 	authType:    OAuthTokeninfoRealmAnyKVName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testRealm, testKey, testValue, "wrongKey", "wrongValue"},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
		// }, {
		// 	msg:         "anyKV(): valid token, one valid kv, multiple key value pairs2",
		// 	authType:    OAuthTokeninfoRealmAnyKVName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testRealm, "wrongKey", "wrongValue", testKey, testValue},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
	}, {
		msg:         "allKV(): invalid key",
		authType:    OAuthTokeninfoRealmAllKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, "not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusNotFound,
	}, {
		msg:         "allKV(): valid token, one valid key, wrong value",
		authType:    OAuthTokeninfoRealmAllKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, testKey, "other-value"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
		// }, {
		// 	msg:         "allKV(): valid token, one valid key value pair",
		// 	authType:    OAuthTokeninfoRealmAllKVName,
		// 	authBaseURL: testAuthPath + "?access_token=",
		// 	args:        []interface{}{testRealm, testKey, testValue},
		// 	hasAuth:     true,
		// 	auth:        testToken,
		// 	expected:    http.StatusOK,
	}, {
		msg:         "oauthTokeninfoRealmAllKV: valid token, valid key value pairs",
		authType:    OAuthTokeninfoRealmAllKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, testKey, testValue}, //, testKey, testValue},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "allKV(): valid token, one valid kv, multiple key value pairs1",
		authType:    OAuthTokeninfoRealmAllKVName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, testKey, testValue, "wrongKey", "wrongValue"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
		// }, {
		// msg:         "allKV(): valid token, one valid kv, multiple key value pairs2",
		// authType:    OAuthTokeninfoAllKVName,
		// authBaseURL: testAuthPath + "?access_token=",
		// args:        []interface{}{testRealm, "wrongKey", "wrongValue", testKey, testValue},
		// hasAuth:     true,
		// auth:        testToken,
		// expected:    http.StatusUnauthorized,
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
					w.Write([]byte(fmt.Sprintf("Failed to getToken token=%s: %v", token, err)))
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				d := map[string]interface{}{
					"uid":   testUID,
					"realm": testRealm,
					"scope": []string{testScope, testScope2, testScope3}}

				e := json.NewEncoder(w)
				err = e.Encode(&d)
				if err != nil {
					t.Error(err)
				}
			}))

			var s filters.Spec
			args := []interface{}{}
			u := authServer.URL + ti.authBaseURL

			switch ti.authType {
			case OAuthTokeninfoAnyScopeName:
				s = NewOAuthTokeninfoAnyScope(u)
			case OAuthTokeninfoAllScopeName:
				s = NewOAuthTokeninfoAllScope(u)
			case OAuthTokeninfoAnyKVName:
				s = NewOAuthTokeninfoAnyKV(u)
			case OAuthTokeninfoAllKVName:
				s = NewOAuthTokeninfoAllKV(u)
			case OAuthTokeninfoRealmAnyScopeName:
				s = NewOAuthTokeninfoRealmAnyScope(u)
			case OAuthTokeninfoRealmAllScopeName:
				s = NewOAuthTokeninfoRealmAllScope(u)
			case OAuthTokeninfoRealmAnyKVName:
				s = NewOAuthTokeninfoRealmAnyKV(u)
			case OAuthTokeninfoRealmAllKVName:
				s = NewOAuthTokeninfoRealmAllKV(u)
			}

			args = append(args, ti.args...)
			fr := make(filters.Registry)
			fr.Register(s)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: s.Name(), Args: args}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
			}

			if ti.hasAuth {
				q := reqURL.Query()
				q.Add(accessTokenQueryKey, ti.auth)
				reqURL.RawQuery = q.Encode()
			}

			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Error(err)
				return
			}

			// if ti.hasAuth {
			// 	req.Header.Set(authHeaderName, "Bearer "+url.QueryEscape(ti.auth))
			// }

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != ti.expected {
				t.Errorf("auth filter failed got=%d, expected=%d, route=%s", rsp.StatusCode, ti.expected, r)
				buf := make([]byte, rsp.ContentLength)
				rsp.Body.Read(buf)
				log.Infof("buf: %s", string(buf))
			}
		})
	}
}
