package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

func staticServer(content string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	}))
}

func TestForwardToken(t *testing.T) {
	tokeninfoServer := staticServer(`{"uid": "test", "scope": ["uid"]}`)
	defer tokeninfoServer.Close()

	introspectionServer := staticServer(`{"uid": "test-uid", "sub": "test-sub", "claims": {"email": "test@test.com"}, "active": true}`)
	defer introspectionServer.Close()

	issuerServer := staticServer(`{"claims_supported": ["email"], "introspection_endpoint": "` + introspectionServer.URL + `"}`)
	defer issuerServer.Close()

	for _, ti := range []struct {
		filters        string
		header         http.Header
		expectedHeader http.Header
	}{
		{
			filters: `oauthTokeninfoAnyScope("uid") -> forwardToken("X-Skipper-Tokeninfo")`,
			header:  http.Header{},
			expectedHeader: http.Header{
				"X-Skipper-Tokeninfo": []string{`{"scope":["uid"],"uid":"test"}`},
			},
		},
		{
			filters: `oauthTokeninfoAnyScope("uid") -> forwardToken("X-Skipper-Tokeninfo", "uid")`,
			header:  http.Header{},
			expectedHeader: http.Header{
				"X-Skipper-Tokeninfo": []string{`{"uid":"test"}`},
			},
		},
		{
			filters: `oauthTokeninfoAnyScope("uid") -> forwardToken("X-Skipper-Tokeninfo", "uid", "scope")`,
			header:  http.Header{},
			expectedHeader: http.Header{
				"X-Skipper-Tokeninfo": []string{`{"scope":["uid"],"uid":"test"}`},
			},
		},
		{
			filters: `oauthTokeninfoAnyScope("uid") -> forwardToken("X-Skipper-Tokeninfo", "blah_blah")`,
			header:  http.Header{},
			expectedHeader: http.Header{
				"X-Skipper-Tokeninfo": []string{`{}`},
			},
		},
		{
			filters: `oauthTokenintrospectionAllClaims("` + issuerServer.URL + `", "email") -> forwardToken("X-Skipper-Tokeninfo")`,
			header:  http.Header{},
			expectedHeader: http.Header{
				"X-Skipper-Tokeninfo": []string{`{"active":true,"claims":{"email":"test@test.com"},"sub":"test-sub","uid":"test-uid"}`},
			},
		},
		{
			filters: `oauthTokenintrospectionAllClaims("` + issuerServer.URL + `", "email") -> forwardToken("X-Skipper-Tokeninfo", "uid", "sub")`,
			header:  http.Header{},
			expectedHeader: http.Header{
				"X-Skipper-Tokeninfo": []string{`{"sub":"test-sub","uid":"test-uid"}`},
			},
		},
		{
			filters:        `forwardToken("X-Skipper-Tokeninfo")`, // not tokeninfo or tokenintrospection
			header:         http.Header{},
			expectedHeader: http.Header{},
		},
		{
			filters: `oauthTokeninfoAnyScope("uid") -> forwardToken("X-Skipper-Tokeninfo")`, // overwrites existing
			header: http.Header{
				"X-Skipper-Tokeninfo": []string{`{"already": "exists"}`},
			},
			expectedHeader: http.Header{
				"X-Skipper-Tokeninfo": []string{`{"scope":["uid"],"uid":"test"}`},
			},
		},
		{
			filters: `forwardToken("X-Skipper-Tokeninfo")`, // not tokeninfo or tokenintrospection, passes existing
			header: http.Header{
				"X-Skipper-Tokeninfo": []string{`{"already": "exists"}`},
			},
			expectedHeader: http.Header{
				"X-Skipper-Tokeninfo": []string{`{"already": "exists"}`},
			},
		},
	} {
		t.Run(ti.filters, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				// ignore irrelevant headers
				r.Header.Del("Authorization")
				r.Header.Del("Accept-Encoding")
				r.Header.Del("User-Agent")

				if !reflect.DeepEqual(ti.expectedHeader, r.Header) {
					t.Errorf("header mismatch, expected: %v, got: %v", ti.expectedHeader, r.Header)
				}
			}))
			defer backend.Close()

			fr := make(filters.Registry)

			fr.Register(NewOAuthTokeninfoAnyScope(tokeninfoServer.URL, 10*time.Second))
			fr.Register(NewOAuthTokenintrospectionAllClaims(10 * time.Second))
			fr.Register(NewForwardToken())

			routes := eskip.MustParse(fmt.Sprintf(`* -> %s -> "%s"`, ti.filters, backend.URL))

			proxy := proxytest.New(fr, routes[0])
			defer proxy.Close()

			req, _ := http.NewRequest("GET", proxy.URL, nil)
			req.Header = ti.header
			req.Header.Set("Authorization", "Bearer testtoken")

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
			}
			defer rsp.Body.Close()
			if rsp.StatusCode != http.StatusOK {
				t.Errorf("failed to query backend server: %d", rsp.StatusCode)
			}
		})
	}
}

func TestInvalidHeadername(t *testing.T) {
	ftSpec := NewForwardToken()
	filterArgs := []any{"test-%header\n"}
	_, err := ftSpec.CreateFilter(filterArgs)
	if err == nil {
		t.Fatalf("bad header name")
	}
}
