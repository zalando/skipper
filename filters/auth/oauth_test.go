package auth

import (
	"encoding/json"
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
	testRealm    = "/immortals"
	testAuthPath = "/test-auth"
)

type testAuthDoc struct {
	authDoc
	SomeOtherStuff string
}

func lastQueryValue(url string) string {
	s := strings.Split(url, "=")
	if len(s) == 0 {
		return ""
	}

	return s[len(s)-1]
}

func Test(t *testing.T) {
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
		authType: AuthAnyScopeName,
		expected: http.StatusNotFound,
	}, {
		msg:         "invalid token, without realm",
		authType:    AuthAnyScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		hasAuth:     true,
		auth:        "invalid-token",
		expected:    http.StatusNotFound,
	}, {
		msg:         "invalid realm",
		authType:    AuthAnyScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{"/not-matching-realm"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "invalid realm, valid token, one valid scope",
		authType:    AuthAnyScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{"/invalid", testScope},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "invalid scope",
		authType:    AuthAnyScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, "not-matching-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
	}, {
		msg:         "valid token, one valid scope",
		authType:    AuthAnyScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, testScope},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "valid token, one valid scope, one invalid scope",
		authType:    AuthAnyScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, testScope, "other-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "authAllScope(): valid token, one valid scope",
		authType:    AuthAllScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, testScope},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusOK,
	}, {
		msg:         "authAllScope(): valid token, one valid scope, one invalid scope",
		authType:    AuthAllScopeName,
		authBaseURL: testAuthPath + "?access_token=",
		args:        []interface{}{testRealm, testScope, "other-scope"},
		hasAuth:     true,
		auth:        testToken,
		expected:    http.StatusUnauthorized,
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
				log.Infof("auth server got call %s", token)

				d := testAuthDoc{
					authDoc{
						UID:    testUID,
						Realm:  testRealm,
						Scopes: []string{testScope}},
					"noise",
				}
				e := json.NewEncoder(w)
				err = e.Encode(&d)
				if err != nil {
					t.Error(err)
				}
			}))

			var s filters.Spec
			args := []interface{}{} //{authServer.URL + ti.authBaseURL}
			s = NewAuth(Options{
				TokenURL: authServer.URL + ti.authBaseURL,
				AuthType: ti.authType,
			})

			args = append(args, ti.args...)
			log.Infof("args: %v", args)
			fr := make(filters.Registry)
			fr.Register(s)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: s.Name(), Args: args}}, Backend: backend.URL}
			log.Infof("r: %s", r)
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

			if rsp.StatusCode != ti.expected {
				t.Errorf("auth filter failed got=%d, expected=%d", rsp.StatusCode, ti.expected)
			}
		})
	}
}
