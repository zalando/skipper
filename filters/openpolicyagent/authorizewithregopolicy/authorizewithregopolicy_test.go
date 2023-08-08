package authorizewithregopolicy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	opasdktest "github.com/open-policy-agent/opa/sdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"

	"github.com/zalando/skipper/filters/openpolicyagent"
)

func TestAuthorizeRequestFilter(t *testing.T) {
	for _, ti := range []struct {
		msg             string
		bundleName      string
		regoQuery       string
		requestPath     string
		expectedBody    string
		expectedHeaders http.Header
		expectedStatus  int
		backendHeaders  http.Header
		removeHeaders   http.Header
	}{
		{
			msg:             "Allow Requests",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow",
			requestPath:     "/allow",
			expectedStatus:  http.StatusOK,
			expectedBody:    "Welcome!",
			expectedHeaders: make(http.Header),
			backendHeaders:  make(http.Header),
			removeHeaders:   make(http.Header),
		},
		{
			msg:             "Simple Forbidden",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow",
			requestPath:     "/forbidden",
			expectedStatus:  http.StatusForbidden,
			expectedHeaders: make(http.Header),
			backendHeaders:  make(http.Header),
			removeHeaders:   make(http.Header),
		},
		{
			msg:             "Allow With Structured Rules",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_object",
			requestPath:     "/allow/structured",
			expectedStatus:  http.StatusOK,
			expectedBody:    "Welcome!",
			expectedHeaders: make(http.Header),
			backendHeaders:  map[string][]string{"X-Consumer": {"x-consumer header value"}},
			removeHeaders:   map[string][]string{"X-Remove-Me": {"Remove me"}},
		},
		{
			msg:             "Forbidden With Body",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_object",
			requestPath:     "/forbidden",
			expectedStatus:  http.StatusUnauthorized,
			expectedHeaders: map[string][]string{"X-Ext-Auth-Allow": {"no"}},
			expectedBody:    "Unauthorized Request",
			backendHeaders:  make(http.Header),
			removeHeaders:   make(http.Header),
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			t.Logf("Running test for %v", ti)
			clientServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("Welcome!"))
				assert.True(t, isHeadersPresent(t, ti.backendHeaders, r.Header), "Enriched request header is absent.")
				assert.True(t, isHeadersAbsent(t, ti.removeHeaders, r.Header), "Unwanted HTTP Headers present.")
			}))

			opaControlPlane := opasdktest.MustNewServer(
				opasdktest.MockBundle("/bundles/"+ti.bundleName, map[string]string{
					"main.rego": `
						package envoy.authz

						default allow = false

						allow {
							input.parsed_path = [ "allow" ]
						}
	
						default allow_object = {
							"allowed": false,
							"headers": {"x-ext-auth-allow": "no"},
							"body": "Unauthorized Request",
							"http_status": 401
						}
						  
						allow_object = response {
							input.parsed_path = [ "allow", "structured" ]
							response := {
								"allowed": true,
								"headers": {
									"x-consumer": "x-consumer header value"
								},
								"request_headers_to_remove" : [
									"x-remove-me",
									"absent-header"
								]
							}
						}
					`,
				}),
			)

			fr := make(filters.Registry)

			config := []byte(fmt.Sprintf(`{
				"services": {
					"test": {
						"url": %q
					}
				},
				"bundles": {
					"test": {
						"resource": "/bundles/{{ .bundlename }}"
					}
				},
				"plugins": {
					"envoy_ext_authz_grpc": {    
						"path": %q,
						"dry-run": false    
					}
				}
			}`, opaControlPlane.URL(), ti.regoQuery))

			opaFactory := openpolicyagent.NewOpenPolicyAgentRegistry()
			ftSpec := NewAuthorizeWithRegoPolicySpec(opaFactory, openpolicyagent.WithConfigTemplate(config))
			fr.Register(ftSpec)

			r := eskip.MustParse(fmt.Sprintf(`* -> authorizeWithRegoPolicy("%s") -> "%s"`, ti.bundleName, clientServer.URL))

			proxy := proxytest.New(fr, r...)

			req, err := http.NewRequest("GET", proxy.URL+ti.requestPath, nil)
			for name, values := range ti.removeHeaders {
				for _, value := range values {
					req.Header.Add(name, value) //adding the headers to validate removal.
				}
			}

			assert.NoError(t, err)

			rsp, err := proxy.Client().Do(req)
			assert.NoError(t, err)

			assert.Equal(t, ti.expectedStatus, rsp.StatusCode, "HTTP status does not match")

			assert.True(t, isHeadersPresent(t, ti.expectedHeaders, rsp.Header), "HTTP Headers do not match")

			defer rsp.Body.Close()
			body, err := io.ReadAll(rsp.Body)
			assert.NoError(t, err)
			assert.Equal(t, ti.expectedBody, string(body), "HTTP Body does not match")
		})
	}
}

func isHeadersPresent(t *testing.T, expectedHeaders http.Header, headers http.Header) bool {
	for headerName, expectedValues := range expectedHeaders {
		actualValues, headerFound := headers[headerName]
		if !headerFound {
			return false
		}

		assert.ElementsMatch(t, expectedValues, actualValues)
	}
	return true
}

func isHeadersAbsent(t *testing.T, unwantedHeaders http.Header, headers http.Header) bool {
	for headerName := range unwantedHeaders {
		if _, ok := headers[headerName]; ok {
			return false
		}
	}
	return true
}
