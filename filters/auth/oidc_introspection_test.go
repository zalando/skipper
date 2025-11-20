package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/secrets/secrettest"
)

func TestCreateOIDCQueryClaimsFilter(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []interface{}
		want    interface{}
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "no args",
			args:    nil,
			wantErr: assert.Error,
		},
		{
			name:    "bogus formatted",
			args:    []interface{}{"s"},
			wantErr: assert.Error,
		},
		{
			name:    "several bogus formatted",
			args:    []interface{}{"s", "d", "easdf:"},
			wantErr: assert.Error,
		},
		{
			name: "one path query",
			args: []interface{}{"/:[@this].#(sub==\"somesub\")"},
			want: &oidcIntrospectionFilter{
				typ: checkOIDCQueryClaims,
				paths: []pathQuery{
					{
						path:    "/",
						queries: []string{"[@this].#(sub==\"somesub\")"},
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "one path query with whitespace",
			args: []interface{}{`/:[@this].#(sub=="white space")`},
			want: &oidcIntrospectionFilter{
				typ: checkOIDCQueryClaims,
				paths: []pathQuery{
					{
						path:    "/",
						queries: []string{`[@this].#(sub=="white space")`},
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "several path queries",
			args: []interface{}{
				"/some/path:[@this].#(sub==\"somesub\")",
				"/another/path:groups.#(%\"CD-*\")",
				"/asdf/:groups.#(%\"CD-*\") 'groups.#(%\"*-Test-Users\")' groups.#(%\"Purchasing-Department\")",
				"/:groups.#(%\"Purchasing-Department\")",
			},
			want: &oidcIntrospectionFilter{
				typ: checkOIDCQueryClaims,
				paths: []pathQuery{
					{
						path:    "/some/path",
						queries: []string{"[@this].#(sub==\"somesub\")"},
					},
					{
						path:    "/another/path",
						queries: []string{"groups.#(%\"CD-*\")"},
					},
					{
						path: "/asdf/",
						queries: []string{
							"groups.#(%\"CD-*\")",
							"groups.#(%\"*-Test-Users\")",
							"groups.#(%\"Purchasing-Department\")",
						},
					},
					{
						path: "/",
						queries: []string{
							"groups.#(%\"Purchasing-Department\")",
						},
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "several path queries with whitespaces",
			args: []interface{}{
				`/asdf/:groups.#(%"white space") 'groups.#(%"two white spaces")' groups.#(%"consecutive   whitespaces") groups.#(%"nowhitespace")`,
			},
			want: &oidcIntrospectionFilter{
				typ: checkOIDCQueryClaims,
				paths: []pathQuery{
					{
						path: "/asdf/",
						queries: []string{
							`groups.#(%"white space")`,
							`groups.#(%"two white spaces")`,
							`groups.#(%"consecutive   whitespaces")`,
							`groups.#(%"nowhitespace")`,
						},
					},
				},
			},
			wantErr: assert.NoError,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewOIDCQueryClaimsFilter()
			got, err := spec.CreateFilter(tt.args)
			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

}

func TestOIDCQueryClaimsFilter(t *testing.T) {
	for _, tc := range []struct {
		msg          string
		path         string
		expected     int
		expectErr    bool
		args         []interface{}
		removeClaims []string
	}{
		{
			msg: "secure sub/path not permitted",
			args: []interface{}{
				"/login:groups.#[==\"appY-Tester\"]",
				"/:@_:email%\"*@example.org\"",
			},
			path:      "/login/page",
			expected:  401,
			expectErr: false,
		},
		{
			msg: "secure sub/path is permitted",
			args: []interface{}{
				"/login:groups.#[==\"AppX-Test-Users\"]",
				"/:@_:email%\"*@example.org\"",
			},
			path:      "/login/page",
			expected:  200,
			expectErr: false,
		},
		{
			msg: "missing sub claim is not permitted",
			args: []interface{}{
				"/login:groups.#[==\"AppX-Test-Users\"]",
				"/:@_:email%\"*@example.org\"",
			},
			path:         "/login/page",
			expected:     401,
			expectErr:    false,
			removeClaims: []string{"sub"},
		},
		{
			msg: "generic user path permitted",
			args: []interface{}{
				"/login:groups.#[==\"Arbitrary-Group\"]",
				"/:@_:email%\"*@example.org\"",
			},
			path:      "/notsecured",
			expected:  200,
			expectErr: false,
		},
		{
			msg: "using modifier, path matching",
			args: []interface{}{
				`/path:@_:sub=="somesub"`,
			},
			path:      "/path/asdf",
			expected:  200,
			expectErr: false,
		},
		{
			msg: "using escape character",
			args: []interface{}{
				"/path:@_:sub==\"somesub\"",
			},
			path:      "/path/asdf",
			expected:  200,
			expectErr: false,
		},
		{
			msg: "path / permitted for group Purchasing-Department",
			args: []interface{}{
				"/:groups.#(%\"Purchasing-Department\")",
			},
			path:      "/",
			expected:  200,
			expectErr: false,
		},
		{
			msg: "path / permitted for group with whitespace",
			args: []interface{}{
				`/:groups.#(%"white space")`,
			},
			path:      "/",
			expected:  200,
			expectErr: false,
		},
		{
			msg: "path /some/path permitted",
			args: []interface{}{
				"/some/path:groups.#(%\"Purchasing-Department\")",
			},
			path:      "/some/path/down/there",
			expected:  200,
			expectErr: false,
		},
		{
			msg: "path /some/otherpath denied",
			args: []interface{}{
				"/some/path:groups.#(%\"Purchasing-Department\")",
			},
			path:      "/some/otherpath",
			expected:  401,
			expectErr: false,
		},
		{
			msg: "wrong group denied",
			args: []interface{}{
				"/some/path:groups.#(%\"Shipping-Department\")",
			},
			path:      "/some/path",
			expected:  401,
			expectErr: false,
		},
		{
			msg: "several queries, path matching",
			args: []interface{}{
				"/some/path:[@this].#(sub==\"somesub\")",
				"/another/path:groups.#(%\"CD-*\")",
				"/asdf/:groups.#(%\"CD-*\") 'groups.#(%\"*-Test-Users\")' groups.#(%\"Purchasing-Department\")",
				"/:groups.#(%\"Purchasing-Department\")",
			},
			path:      "/asdf/asdf",
			expected:  200,
			expectErr: false,
		},
		{
			msg: "several queries, no path matching",
			args: []interface{}{
				"/some/path:[@this].#(sub==\"somesub\")",
				"/another/path:groups.#(%\"CD-*\")",
				"/asdf/:groups.#(%\"CD-*\") 'groups.#(%\"*-Test-Users\")' groups.#(%\"Purchasing-Department\")",
				"/abc:groups.#(%\"Purchasing-Department\")",
			},
			path:      "/xyz",
			expected:  401,
			expectErr: false,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			t.Parallel()
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Logf("backend got request: %+v", r)
				w.Write([]byte("OK"))
			}))
			defer backend.Close()
			t.Logf("backend URL: %s", backend.URL)

			spec := &tokenOidcSpec{
				typ:             checkOIDCAnyClaims,
				SecretsFile:     "/tmp/foo",
				secretsRegistry: secrettest.NewTestRegistry(),
			}
			fr := make(filters.Registry)
			fr.Register(spec)

			dc := testdataclient.New(nil)
			defer dc.Close()

			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				DataClients: []routing.DataClient{dc},
			})
			defer proxy.Close()

			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
			}
			reqURL.Path = tc.path
			oidcServer := createOIDCServer(proxy.URL+"/redirect", validClient, "mysec", jwt.MapClaims{"groups": []string{"CD-Administrators", "Purchasing-Department", "AppX-Test-Users", "white space"}}, tc.removeClaims)
			defer oidcServer.Close()
			t.Logf("oidc/auth server URL: %s", oidcServer.URL)
			// create filter
			sargs := []interface{}{
				oidcServer.URL,
				validClient,
				"mysec",
				proxy.URL + "/redirect",
				testKey,
				testKey,
			}
			f, err := spec.CreateFilter(sargs)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("Want error but got filter: %v", f)
				}
				return //OK
			} else if err != nil {
				t.Fatalf("Unexpected error while creating filter: %v", err)
			}

			// adding the OIDCQueryClaimsFilter to the route
			querySpec := NewOIDCQueryClaimsFilter()
			fr.Register(querySpec)
			r := &eskip.Route{
				Filters: []*eskip.Filter{
					{
						Name: spec.Name(),
						Args: sargs,
					},
					{
						Name: querySpec.Name(),
						Args: tc.args,
					},
				},
				Backend: backend.URL,
			}

			proxy.Log.Reset()
			dc.Update([]*eskip.Route{r}, nil)
			if err = proxy.Log.WaitFor("route settings applied", 1*time.Second); err != nil {
				t.Fatalf("Failed to get update: %v", err)
			}

			// do request through proxy
			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set(authHeaderName, authHeaderPrefix+testToken)

			// client with cookie handling to support 127.0.0.1 with ports
			client := http.Client{
				Timeout: 1 * time.Second,
				Jar:     newInsecureCookieJar(),
			}

			// trigger OpenID Connect Authorization Code Flow
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("req: %+v: %v", req, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expected {
				t.Logf("response: %+v", resp)
				t.Errorf("auth filter failed got=%d, expected=%d, route=%s", resp.StatusCode, tc.expected, r)
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("Response body: %s", string(b))
			}
		})
	}
}
