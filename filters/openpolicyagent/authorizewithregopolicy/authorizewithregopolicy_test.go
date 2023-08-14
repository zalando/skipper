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
		msg               string
		bundleName        string
		regoQuery         string
		requestPath       string
		contextExtensions string
		expectedBody      string
		expectedHeaders   http.Header
		expectedStatus    int
		backendHeaders    http.Header
		removeHeaders     http.Header
	}{
		{
			msg:               "Allow Requests",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/allow",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Allow Matching Context Extension",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_context_extensions",
			requestPath:       "/allow",
			contextExtensions: "com.mycompany.myprop: myvalue",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Simple Forbidden",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/forbidden",
			contextExtensions: "",
			expectedStatus:    http.StatusForbidden,
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Allow With Structured Rules",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_object",
			requestPath:       "/allow/structured",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    map[string][]string{"X-Consumer": {"x-consumer header value"}},
			removeHeaders:     map[string][]string{"X-Remove-Me": {"Remove me"}},
		},
		{
			msg:               "Forbidden With Body",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_object",
			requestPath:       "/forbidden",
			contextExtensions: "",
			expectedStatus:    http.StatusUnauthorized,
			expectedHeaders:   map[string][]string{"X-Ext-Auth-Allow": {"no"}},
			expectedBody:      "Unauthorized Request",
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Misconfigured Rego Query",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/invalid_path",
			requestPath:       "/allow",
			contextExtensions: "",
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      "",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Wrong Query Data Type",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_wrong_type",
			requestPath:       "/allow",
			contextExtensions: "",
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      "",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Wrong Query Data Type",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_object_invalid_headers_to_remove",
			requestPath:       "/allow",
			contextExtensions: "",
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      "",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Wrong Query Data Type",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_object_invalid_headers",
			requestPath:       "/allow",
			contextExtensions: "",
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      "",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
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

						allow_context_extensions {
							input.attributes.contextExtensions["com.mycompany.myprop"] == "myvalue"
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

						allow_wrong_type := "true"

						allow_object_invalid_headers_to_remove := {
							"allowed": true,
							"request_headers_to_remove": "bogus string instead of object"
						}

						allow_object_invalid_headers := {
							"allowed": true,
							"headers": "bogus string instead of object"
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

			r := eskip.MustParse(fmt.Sprintf(`* -> authorizeWithRegoPolicy("%s", "%s") -> "%s"`, ti.bundleName, ti.contextExtensions, clientServer.URL))

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

func TestCreateFilterArguments(t *testing.T) {
	opaRegistry := openpolicyagent.NewOpenPolicyAgentRegistry()
	ftSpec := NewAuthorizeWithRegoPolicySpec(opaRegistry, openpolicyagent.WithConfigTemplate([]byte("")))

	_, err := ftSpec.CreateFilter([]interface{}{})
	assert.ErrorIs(t, err, filters.ErrInvalidFilterParameters)

	_, err = ftSpec.CreateFilter([]interface{}{"a bundle", "extra: value", "superfluous argument"})
	assert.ErrorIs(t, err, filters.ErrInvalidFilterParameters)
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
