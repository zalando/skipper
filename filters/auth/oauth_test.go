package auth

import (
	"encoding/json"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const (
	testToken     = "test-token"
	testUid       = "jdoe"
	testScope     = "test-scope"
	testRealm     = "/immortals"
	testGroup     = "test-group"
	testAuthPath  = "/test-auth"
	testGroupPath = "/test-group"
)

type (
	testAuthDoc struct {
		authDoc
		SomeOtherStuff string
	}

	testGroupDoc struct {
		groupDoc
		SomeOtherStuff string
	}
)

func lastQueryValue(url string) string {
	s := strings.Split(url, "=")
	if len(s) == 0 {
		return ""
	}

	return s[len(s)-1]
}

func Test(t *testing.T) {
	for _, ti := range []struct {
		msg          string
		typ          roleCheckType
		authBaseUrl  string
		groupBaseUrl string
		args         []interface{}
		hasAuth      bool
		auth         string
		statusCode   int
	}{{
		msg:        "uninitialized filter, no authorization header, scope check",
		typ:        checkScope,
		statusCode: http.StatusUnauthorized,
	}, {
		msg:        "uninitialized filter, no authorization header, group check",
		typ:        checkGroup,
		statusCode: http.StatusUnauthorized,
	}, {
		msg:         "no authorization header, scope check",
		typ:         checkScope,
		authBaseUrl: testAuthPath,
		statusCode:  http.StatusUnauthorized,
	}, {
		msg:         "invalid token, scope check",
		typ:         checkScope,
		authBaseUrl: testAuthPath + "?access_token=",
		hasAuth:     true,
		auth:        "invalid-token",
		statusCode:  http.StatusUnauthorized,
	}, {
		msg:         "valid token, auth only, scope check",
		typ:         checkScope,
		authBaseUrl: testAuthPath + "?access_token=",
		hasAuth:     true,
		auth:        testToken,
		statusCode:  http.StatusOK,
	}, {
		msg:          "invalid realm, scope check",
		typ:          checkScope,
		authBaseUrl:  testAuthPath + "?access_token=",
		groupBaseUrl: testGroupPath + "?member=",
		args:         []interface{}{"/not-matching-realm"},
		hasAuth:      true,
		auth:         testToken,
		statusCode:   http.StatusUnauthorized,
	}, {
		msg:         "invalid scope",
		typ:         checkScope,
		authBaseUrl: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, "not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		statusCode:  http.StatusUnauthorized,
	}, {
		msg:         "valid token, valid scope",
		typ:         checkScope,
		authBaseUrl: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, testScope, "other-scope"},
		hasAuth:     true,
		auth:        testToken,
		statusCode:  http.StatusOK,
	}, {
		msg:          "no authorization header, group check",
		typ:          checkGroup,
		authBaseUrl:  testAuthPath,
		groupBaseUrl: testGroupPath,
		statusCode:   http.StatusUnauthorized,
	}, {
		msg:          "invalid token, group check",
		typ:          checkGroup,
		authBaseUrl:  testAuthPath + "?access_token=",
		groupBaseUrl: testGroupPath + "?member=",
		hasAuth:      true,
		auth:         "invalid-token",
		statusCode:   http.StatusUnauthorized,
	}, {
		msg:          "valid token, auth only, group check",
		typ:          checkGroup,
		authBaseUrl:  testAuthPath + "?access_token=",
		groupBaseUrl: testGroupPath + "?member=",
		hasAuth:      true,
		auth:         testToken,
		statusCode:   http.StatusOK,
	}, {
		msg:          "invalid realm, group check",
		typ:          checkGroup,
		authBaseUrl:  testAuthPath + "?access_token=",
		groupBaseUrl: testGroupPath + "?member=",
		args:         []interface{}{"/not-matching-realm"},
		hasAuth:      true,
		auth:         testToken,
		statusCode:   http.StatusUnauthorized,
	}, {
		msg:          "valid token, valid realm, no group check",
		typ:          checkGroup,
		authBaseUrl:  testAuthPath + "?access_token=",
		groupBaseUrl: testGroupPath + "?member=",
		args:         []interface{}{testRealm},
		hasAuth:      true,
		auth:         testToken,
		statusCode:   http.StatusOK,
	}, {
		msg:          "valid token, valid realm, no matching group",
		typ:          checkGroup,
		authBaseUrl:  testAuthPath + "?access_token=",
		groupBaseUrl: testGroupPath + "?member=",
		args:         []interface{}{testRealm, "invalid-group-0", "invalid-group-1"},
		hasAuth:      true,
		auth:         testToken,
		statusCode:   http.StatusUnauthorized,
	}, {
		msg:          "valid token, valid realm, matching group, group",
		typ:          checkGroup,
		authBaseUrl:  testAuthPath + "?access_token=",
		groupBaseUrl: testGroupPath + "?member=",
		args:         []interface{}{testRealm, "invalid-group-0", testGroup},
		hasAuth:      true,
		auth:         testToken,
		statusCode:   http.StatusOK,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {}))

			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != testAuthPath {
					println("not found", r.URL.Path, testAuthPath)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				token, err := getToken(r)
				if err != nil || token != testToken {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				d := testAuthDoc{authDoc{testUid, testRealm, []string{testScope}}, "noise"}
				e := json.NewEncoder(w)
				err = e.Encode(&d)
				if err != nil {
					t.Error(err)
				}
			}))

			groupServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != testGroupPath {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				if token, err := getToken(r); err != nil || token != testToken {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				if lastQueryValue(r.URL.String()) != testUid {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				d := []testGroupDoc{{groupDoc{testGroup}, "noise"}, {groupDoc{"other-group"}, "more noise"}}
				e := json.NewEncoder(w)
				err := e.Encode(&d)
				if err != nil {
					t.Error(err)
				}
			}))

			var s filters.Spec
			args := []interface{}{authServer.URL + ti.authBaseUrl}
			if ti.typ == checkGroup {
				s = NewAuthGroup()
				args = append(args, groupServer.URL+ti.groupBaseUrl)
			} else {
				s = NewAuth()
			}
			args = append(args, ti.args...)
			fr := make(filters.Registry)
			fr.Register(s)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: s.Name(), Args: args}}, Backend: backend.URL}
			proxy := proxytest.New(fr, r)

			req, err := http.NewRequest("GET", proxy.URL, nil)
			if err != nil {
				t.Error(err)
				return
			}

			if ti.hasAuth {
				req.Header.Set(authHeaderName, "Bearer "+url.QueryEscape(ti.auth))
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != ti.statusCode {
				t.Error("auth filter failed", rsp.StatusCode, ti.statusCode)
			}
		})
	}
}
