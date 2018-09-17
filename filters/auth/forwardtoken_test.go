package auth

import (
	"encoding/json"
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"
)

const (
	contentTypeHeader          = "content-type"
	applicationJsonHeaderValue = "application/json"
	oauthTimeout               = 10 * time.Second
	authorizationHeader        = "Authorization"
	authorizationHeaderValue   = "Bearer %s"
	authorizationToken         = "testtoken"
	uidScope                   = "uid"
)

type testTokeninfo struct {
	Uid   string   `json:"uid"`
	Scope []string `json:"scope"`
}

type testTokenIntrospection struct {
	Uid    string            `json:"uid"`
	Claims map[string]string `json:"claims"`
	Active bool              `json:"active"`
	Sub    string            `json:"sub"`
}

func TestForwardTokenInfo(t *testing.T) {
	for _, ti := range []struct {
		msg                string
		headerName         string
		tokenInfo          testTokeninfo
		oauthFilterPresent bool
	}{
		{
			msg:                "Basic Test",
			headerName:         "X-Skipper-Tokeninfo",
			tokenInfo:          testTokeninfo{Uid: "test", Scope: []string{"uid"}},
			oauthFilterPresent: true,
		},
		{
			msg:                "No OAuth Filter Test Test",
			headerName:         "X-Skipper-Tokeninfo",
			tokenInfo:          testTokeninfo{Uid: "test", Scope: []string{"uid"}},
			oauthFilterPresent: false,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			t.Logf("Running test for %v", ti)
			clientServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				if ti.oauthFilterPresent {
					tokenInfo := r.Header.Get(ti.headerName)
					var info testTokeninfo
					err := json.Unmarshal([]byte(tokenInfo), &info)
					if err != nil {
						t.Fatalf("Failed to unmarshall header value %s", tokenInfo)
					}
					if !reflect.DeepEqual(info, ti.tokenInfo) {
						t.Fatalf("Did not receive token info in header %s", ti.headerName)
					}
					t.Logf("tokeninfo present in header %s", ti.headerName)
				} else {
					if _, ok := r.Header[ti.headerName]; ok {
						t.Fatalf("header %s is present even when oauthfilter is disabled", ti.headerName)
					}
				}
			}))

			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				info, err := json.Marshal(ti.tokenInfo)
				if err != nil {
					t.Errorf("failed to marshall %v", ti.tokenInfo)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set(contentTypeHeader, applicationJsonHeaderValue)
				w.Write(info)
				return
			}))

			var routeFilters []*eskip.Filter
			fr := make(filters.Registry)

			if ti.oauthFilterPresent {
				oauthTokenSpec := NewOAuthTokeninfoAnyScope(authServer.URL, oauthTimeout)
				oauthFilterArgs := []interface{}{uidScope}
				oauthFilter, err := oauthTokenSpec.CreateFilter(oauthFilterArgs)
				if err != nil {
					t.Errorf("error creating oauth filter.")
					return
				}
				f1 := oauthFilter.(*tokeninfoFilter)
				defer f1.Close()
				routeFilters = append(routeFilters, &eskip.Filter{Name: oauthTokenSpec.Name(), Args: oauthFilterArgs})
				fr.Register(oauthTokenSpec)
			}

			ftSpec := NewForwardToken()
			_, err := ftSpec.CreateFilter([]interface{}{ti.headerName})
			if err != nil {
				t.Fatalf("error in creating filter")
			}
			fr.Register(ftSpec)
			routeFilters = append(routeFilters, &eskip.Filter{Name: ftSpec.Name(), Args: []interface{}{ti.headerName}})

			r := &eskip.Route{Filters: routeFilters, Backend: clientServer.URL}

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

			if ti.oauthFilterPresent {
				req.Header.Add(authorizationHeader, fmt.Sprintf(authorizationHeaderValue, authorizationToken))
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
			}
			if rsp.StatusCode != http.StatusOK {
				t.Fatalf("failed to query backend server.")
			}
			defer rsp.Body.Close()
		})
	}
}

const (
	emailClaim = "email"
)

func TestForwardTokenIntrospection(t *testing.T) {
	for _, ti := range []struct {
		msg                string
		headerName         string
		tokenIntrospection testTokenIntrospection
		oauthFilterPresent bool
	}{
		{
			msg:                "Basic Test",
			headerName:         "X-Skipper-Tokeninfo",
			tokenIntrospection: testTokenIntrospection{Uid: "test-uid", Claims: map[string]string{"email": "test@test.com"}, Active: true},
			oauthFilterPresent: true,
		},
		{
			msg:                "No OAuth Filter Test Test",
			headerName:         "X-Skipper-Tokeninfo",
			tokenIntrospection: testTokenIntrospection{},
			oauthFilterPresent: false,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			t.Logf("Running test for %v", ti)
			clientServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				if ti.oauthFilterPresent {
					tokenInfo := r.Header.Get(ti.headerName)
					var info testTokenIntrospection
					err := json.Unmarshal([]byte(tokenInfo), &info)
					if err != nil {
						t.Fatalf("Failed to unmarshall header value %s", tokenInfo)
					}
					if !reflect.DeepEqual(info, ti.tokenIntrospection) {
						t.Fatalf("Did not receive token introspection in header %s", ti.headerName)
					}
					t.Logf("tokenintrospection present in header %s", ti.headerName)
				} else {
					if _, ok := r.Header[ti.headerName]; ok {
						t.Fatalf("header %s is present even when oauthfilter is disabled", ti.headerName)
					}
				}
			}))

			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tokenIntro, err := json.Marshal(ti.tokenIntrospection)
				if err != nil {
					t.Errorf("Failed to json encode: %v", err)
				}
				w.Write(tokenIntro)
			}))

			testOidcConfig := &openIDConfig{
				ClaimsSupported: []string{"email"},
			}
			issuerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				e := json.NewEncoder(w)
				err := e.Encode(testOidcConfig)
				if err != nil {
					t.Fatalf("Could not encode testOidcConfig: %v", err)
				}
			}))
			defer issuerServer.Close()
			// patch openIDConfig to the current testservers
			testOidcConfig.Issuer = "http://" + issuerServer.Listener.Addr().String()
			testOidcConfig.IntrospectionEndpoint = "http://" + authServer.Listener.Addr().String() + testAuthPath

			var routeFilters []*eskip.Filter
			fr := make(filters.Registry)

			if ti.oauthFilterPresent {
				oauthTokenSpec := NewOAuthTokenintrospectionAllClaims(oauthTimeout)
				oauthFilterArgs := []interface{}{"http://" + issuerServer.Listener.Addr().String(), emailClaim}
				oauthFilter, err := oauthTokenSpec.CreateFilter(oauthFilterArgs)
				if err != nil {
					t.Errorf("error creating oauth filter. %v", err)
					return
				}
				f1 := oauthFilter.(*tokenintrospectFilter)
				defer f1.Close()
				routeFilters = append(routeFilters, &eskip.Filter{Name: oauthTokenSpec.Name(), Args: oauthFilterArgs})
				fr.Register(oauthTokenSpec)
			}

			ftSpec := NewForwardToken()
			_, err := ftSpec.CreateFilter([]interface{}{ti.headerName})
			if err != nil {
				t.Fatalf("error in creating filter")
			}
			fr.Register(ftSpec)
			routeFilters = append(routeFilters, &eskip.Filter{Name: ftSpec.Name(), Args: []interface{}{ti.headerName}})

			r := &eskip.Route{Filters: routeFilters, Backend: clientServer.URL}

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

			if ti.oauthFilterPresent {
				req.Header.Add(authorizationHeader, fmt.Sprintf(authorizationHeaderValue, authorizationToken))
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
			}
			if rsp.StatusCode != http.StatusOK {
				t.Fatalf("failed to query backend server.")
			}
			defer rsp.Body.Close()
		})
	}
}

func TestInvalidHeadername(t *testing.T) {
	ftSpec := NewForwardToken()
	filterArgs := []interface{}{"test-%header\n"}
	_, err := ftSpec.CreateFilter(filterArgs)
	if err == nil {
		t.Fatalf("bad header name")
	}
}
