package opaserveresponse

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
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
		expectedBody      string
		contextExtensions string
		expectedHeaders   http.Header
		expectedStatus    int
	}{
		{
			msg:             "Allow Requests",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow",
			requestPath:     "allow",
			expectedStatus:  http.StatusInternalServerError,
			expectedBody:    "",
			expectedHeaders: make(http.Header),
		},
		{
			msg:             "Simple Forbidden",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow",
			requestPath:     "forbidden",
			expectedStatus:  http.StatusForbidden,
			expectedHeaders: make(http.Header),
		},
		{
			msg:             "Misconfigured Rego Query",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/invalid_path",
			requestPath:     "allow",
			expectedStatus:  http.StatusInternalServerError,
			expectedBody:    "",
			expectedHeaders: make(http.Header),
		},
		{
			msg:             "Allow With Structured Rules",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_object",
			requestPath:     "allow/structured",
			expectedStatus:  http.StatusOK,
			expectedBody:    "Welcome from policy!",
			expectedHeaders: map[string][]string{"X-Ext-Auth-Allow": {"yes"}},
		},
		{
			msg:             "Allow With Structured Body",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_object_structured_body",
			requestPath:     "allow/structured",
			expectedStatus:  http.StatusInternalServerError,
			expectedBody:    "",
			expectedHeaders: map[string][]string{},
		},
		{
			msg:               "Allow with context extensions",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_object_contextextensions",
			requestPath:       "allow/structured",
			contextExtensions: "hello: world",
			expectedStatus:    http.StatusOK,
			expectedHeaders:   map[string][]string{"X-Ext-Auth-Allow": {"yes"}},
			expectedBody:      `{"hello":"world"}`,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			t.Logf("Running test for %v", ti)
			clientServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("Welcome!"))
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
								"headers": {"x-ext-auth-allow": "yes"},
								"body": "Welcome from policy!",
								"http_status": 200
							}
						}

						allow_object_structured_body = response {
							input.parsed_path = [ "allow", "structured" ]
							response := {
								"allowed": true,
								"headers": {"x-ext-auth-allow": "yes"},
								"body": {"hello": "world"},
								"http_status": 200
							}
						}


						allow_object_contextextensions = response {
							input.parsed_path = [ "allow", "structured" ]
							response := {
								"allowed": true,
								"headers": {"x-ext-auth-allow": "yes"},
								"body": json.marshal(input.attributes.contextExtensions),
								"http_status": 200
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
			ftSpec := NewOpaServeResponseSpec(opaFactory, openpolicyagent.WithConfigTemplate(config))

			filterArgs := []interface{}{ti.bundleName}
			if ti.contextExtensions != "" {
				filterArgs = append(filterArgs, ti.contextExtensions)
			}

			_, err := ftSpec.CreateFilter(filterArgs)
			assert.NoErrorf(t, err, "error in creating filter: %v", err)

			fr.Register(ftSpec)

			r := eskip.MustParse(fmt.Sprintf(`* -> opaServeResponse("%s", "%s") -> "%s"`, ti.bundleName, ti.contextExtensions, clientServer.URL))

			proxy := proxytest.New(fr, r...)
			reqURL, err := url.Parse(proxy.URL)
			assert.NoErrorf(t, err, "Failed to parse url %s: %v", proxy.URL, err)

			reqURL.Path = path.Join(reqURL.Path, ti.requestPath)
			req, err := http.NewRequest("GET", reqURL.String(), nil)
			assert.NoError(t, err)

			rsp, err := proxy.Client().Do(req)
			assert.NoError(t, err)

			assert.Equal(t, ti.expectedStatus, rsp.StatusCode, "HTTP status does not match")

			sanitizedHeaders := rsp.Header
			sanitizedHeaders.Del("Date")
			sanitizedHeaders.Del("Server")
			sanitizedHeaders.Del("Content-Length")
			sanitizedHeaders.Del("Content-Type")
			assert.Equal(t, ti.expectedHeaders, sanitizedHeaders, "HTTP Headers do not match")

			defer rsp.Body.Close()
			body, err := io.ReadAll(rsp.Body)
			assert.NoError(t, err)
			assert.Equal(t, ti.expectedBody, string(body), "HTTP Headers do not match")
		})
	}
}

func TestCreateFilterArguments(t *testing.T) {
	opaRegistry := openpolicyagent.NewOpenPolicyAgentRegistry()
	ftSpec := NewOpaServeResponseSpec(opaRegistry, openpolicyagent.WithConfigTemplate([]byte("")))

	_, err := ftSpec.CreateFilter([]interface{}{})
	assert.ErrorIs(t, err, filters.ErrInvalidFilterParameters)

	_, err = ftSpec.CreateFilter([]interface{}{42})
	assert.ErrorIs(t, err, filters.ErrInvalidFilterParameters)

	_, err = ftSpec.CreateFilter([]interface{}{"a bundle", 42})
	assert.ErrorIs(t, err, filters.ErrInvalidFilterParameters)

	_, err = ftSpec.CreateFilter([]interface{}{"a bundle", "invalid; context extensions"})
	assert.Error(t, err)

	_, err = ftSpec.CreateFilter([]interface{}{"a bundle", "extra: value", "superfluous argument"})
	assert.ErrorIs(t, err, filters.ErrInvalidFilterParameters)
}
