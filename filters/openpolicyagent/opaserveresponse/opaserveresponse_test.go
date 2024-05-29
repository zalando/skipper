package opaserveresponse

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	opasdktest "github.com/open-policy-agent/opa/sdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/tracing/tracingtest"

	"github.com/zalando/skipper/filters/openpolicyagent"
)

func TestAuthorizeRequestFilter(t *testing.T) {
	for _, ti := range []struct {
		msg               string
		filterName        string
		bundleName        string
		regoQuery         string
		requestPath       string
		requestHeaders    http.Header
		body              string
		expectedBody      string
		contextExtensions string
		expectedHeaders   http.Header
		expectedStatus    int
	}{
		{
			msg:             "Allow Requests",
			filterName:      "opaServeResponse",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow",
			requestPath:     "/allow",
			expectedStatus:  http.StatusInternalServerError,
			expectedBody:    "",
			expectedHeaders: make(http.Header),
		},
		{
			msg:             "Simple Forbidden",
			filterName:      "opaServeResponse",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow",
			requestPath:     "/forbidden",
			expectedStatus:  http.StatusForbidden,
			expectedHeaders: make(http.Header),
		},
		{
			msg:             "Misconfigured Rego Query",
			filterName:      "opaServeResponse",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/invalid_path",
			requestPath:     "/allow",
			expectedStatus:  http.StatusInternalServerError,
			expectedBody:    "",
			expectedHeaders: make(http.Header),
		},
		{
			msg:             "Allow With Structured Rules",
			filterName:      "opaServeResponse",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_object",
			requestPath:     "/allow/structured",
			expectedStatus:  http.StatusOK,
			expectedBody:    "Welcome from policy!",
			expectedHeaders: map[string][]string{"X-Ext-Auth-Allow": {"yes"}},
		},
		{
			msg:             "Allow With opa.runtime execution",
			filterName:      "opaServeResponse",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_object",
			requestPath:     "/allow/production",
			expectedStatus:  http.StatusOK,
			expectedBody:    "Welcome to production evaluation!",
			expectedHeaders: map[string][]string{"X-Ext-Auth-Allow": {"yes"}},
		},
		{
			msg:             "Deny With opa.runtime execution",
			filterName:      "opaServeResponse",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_object",
			requestPath:     "/allow/test",
			expectedStatus:  http.StatusForbidden,
			expectedBody:    "Unauthorized Request",
			expectedHeaders: map[string][]string{"X-Ext-Auth-Allow": {"no"}},
		},
		{
			msg:             "Allow With Structured Body",
			filterName:      "opaServeResponse",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_object_structured_body",
			requestPath:     "/allow/structured",
			expectedStatus:  http.StatusInternalServerError,
			expectedBody:    "",
			expectedHeaders: map[string][]string{},
		},
		{
			msg:               "Allow with context extensions",
			filterName:        "opaServeResponse",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_object_contextextensions",
			requestPath:       "/allow/structured",
			contextExtensions: "hello: world",
			expectedStatus:    http.StatusOK,
			expectedHeaders:   map[string][]string{"X-Ext-Auth-Allow": {"yes"}},
			expectedBody:      `{"hello":"world"}`,
		},
		{
			msg:             "Use request body",
			filterName:      "opaServeResponseWithReqBody",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_object_req_body",
			requestPath:     "/allow/allow_object_req_body",
			requestHeaders:  map[string][]string{"content-type": {"application/json"}},
			body:            `{"hello":"world"}`,
			expectedStatus:  http.StatusOK,
			expectedBody:    `{"hello":"world"}`,
			expectedHeaders: map[string][]string{},
		},
		{
			msg:             "Invalid UTF-8 in Path",
			filterName:      "opaServeResponse",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow",
			requestPath:     "/allow/%c0%ae%c0%ae",
			expectedStatus:  http.StatusBadRequest,
			expectedBody:    "",
			expectedHeaders: make(http.Header),
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			t.Logf("Running test for %v", ti)

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
							"http_status": 403
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
						
						allow_object = response {
							input.parsed_path = [ "allow", "production" ]
							opa.runtime().config.labels.environment == "production"
							response := {
								"allowed": true,
								"headers": {"x-ext-auth-allow": "yes"},
								"body": "Welcome to production evaluation!",
								"http_status": 200
							}
						}

						allow_object = response {
							input.parsed_path = [ "allow", "test" ]
							opa.runtime().config.labels.environment == "test"
							response := {
								"allowed": true,
								"headers": {"x-ext-auth-allow": "yes"},
								"body": "Welcome to test evaluation!",
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

						allow_object_req_body = response {
							response := {
								"allowed": true,
								"headers": {},
								"body": json.marshal(input.parsed_body),
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
				"labels": {
					"environment" : "production"
				},
				"bundles": {
					"test": {
						"resource": "/bundles/{{ .bundlename }}"
					}
				},
				"plugins": {
					"envoy_ext_authz_grpc": {    
						"path": %q,
						"dry-run": false,
						"skip-request-body-parse": false
					}
				},
				"decision_logs": {
					"console": true
				}
			}`, opaControlPlane.URL(), ti.regoQuery))

			opaFactory := openpolicyagent.NewOpenPolicyAgentRegistry(openpolicyagent.WithTracer(&tracingtest.Tracer{}))
			ftSpec := NewOpaServeResponseSpec(opaFactory, openpolicyagent.WithConfigTemplate(config))
			fr.Register(ftSpec)
			ftSpec = NewOpaServeResponseWithReqBodySpec(opaFactory, openpolicyagent.WithConfigTemplate(config))
			fr.Register(ftSpec)

			filterArgs := []interface{}{ti.bundleName}
			if ti.contextExtensions != "" {
				filterArgs = append(filterArgs, ti.contextExtensions)
			}

			_, err := ftSpec.CreateFilter(filterArgs)
			assert.NoErrorf(t, err, "error in creating filter: %v", err)

			r := eskip.MustParse(fmt.Sprintf(`* -> %s("%s", "%s") -> <shunt>`, ti.filterName, ti.bundleName, ti.contextExtensions))

			proxy := proxytest.New(fr, r...)

			req, err := http.NewRequest("GET", proxy.URL+ti.requestPath, strings.NewReader(ti.body))
			assert.NoError(t, err)
			for name, values := range ti.requestHeaders {
				for _, value := range values {
					req.Header.Add(name, value)
				}
			}

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
			assert.Equal(t, ti.expectedBody, string(body), "HTTP Body does not match")
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
