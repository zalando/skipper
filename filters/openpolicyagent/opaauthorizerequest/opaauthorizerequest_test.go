package opaauthorizerequest

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/tracing/tracingtest"

	"github.com/zalando/skipper/filters/openpolicyagent"
)

func TestAuthorizeRequestFilter(t *testing.T) {
	for _, ti := range []struct {
		msg               string
		filterName        string
		extraeskipBefore  string
		extraeskipAfter   string
		bundleName        string
		regoQuery         string
		requestPath       string
		requestMethod     string
		requestHeaders    http.Header
		body              string
		contextExtensions string
		expectedBody      string
		expectedHeaders   http.Header
		expectedStatus    int
		backendHeaders    http.Header
		removeHeaders     http.Header
	}{
		{
			msg:               "Allow Requests",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/allow",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Allow Requests with spaces in path",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_with_space_in_path",
			requestPath:       "/my%20path",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Allow Requests with request path overridden by the setPath filter",
			filterName:        "opaAuthorizeRequest",
			extraeskipBefore:  `setPath("/allow") ->`,
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/some-random-path-that-would-fail",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Allow Request based on http path",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_with_http_path",
			requestPath:       "/some/api/path?q1=v1&msg=help%20me",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Allow Requests with query parameters",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_with_query",
			requestPath:       "/allow-with-query?pass=yes&id=1&id=2&msg=help%20me",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Allow Matching Context Extension",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_context_extensions",
			requestPath:       "/allow",
			requestMethod:     "GET",
			contextExtensions: "com.mycompany.myprop: myvalue",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Allow Requests with an empty query string",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_with_path_having_empty_query",
			requestPath:       "/path-with-empty-query?",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:             "Allow Matching Environment",
			filterName:      "opaAuthorizeRequest",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_runtime_environment",
			requestPath:     "/allow",
			expectedStatus:  http.StatusOK,
			expectedBody:    "Welcome!",
			expectedHeaders: make(http.Header),
			backendHeaders:  make(http.Header),
			removeHeaders:   make(http.Header),
		},
		{
			msg:               "Simple Forbidden",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/forbidden",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusForbidden,
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Simple Forbidden with Query Parameters",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/deny_with_query",
			requestPath:       "/allow-me?tofail=true",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusForbidden,
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Allow With Structured Rules",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_object",
			requestPath:       "/allow/structured",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   map[string][]string{"X-Response-Header": {"a response header value"}, "Server": {"Skipper", "server header"}},
			backendHeaders:    map[string][]string{"X-Consumer": {"x-consumer header value"}},
			removeHeaders:     map[string][]string{"X-Remove-Me": {"Remove me"}},
		},
		{
			msg:               "Forbidden With Body",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_object",
			requestPath:       "/forbidden",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusUnauthorized,
			expectedHeaders:   map[string][]string{"X-Ext-Auth-Allow": {"no"}},
			expectedBody:      "Unauthorized Request",
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Misconfigured Rego Query",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/invalid_path",
			requestPath:       "/allow",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      "",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Wrong Query Data Type",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_wrong_type",
			requestPath:       "/allow",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      "",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Wrong Query Data Type",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_object_invalid_headers_to_remove",
			requestPath:       "/allow",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      "",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Wrong Query Data Type",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_object_invalid_headers",
			requestPath:       "/allow",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      "",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:             "Allow With Body",
			filterName:      "opaAuthorizeRequestWithBody",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_body",
			requestMethod:   "POST",
			body:            `{ "target_id" : "123456" }`,
			requestHeaders:  map[string][]string{"content-type": {"application/json"}},
			requestPath:     "/allow_body",
			expectedStatus:  http.StatusOK,
			expectedBody:    "Welcome!",
			expectedHeaders: make(http.Header),
			backendHeaders:  make(http.Header),
			removeHeaders:   make(http.Header),
		},
		{
			msg:             "Forbidden With Body",
			filterName:      "opaAuthorizeRequestWithBody",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_body",
			requestMethod:   "POST",
			body:            `{ "target_id" : "wrong id" }`,
			requestHeaders:  map[string][]string{"content-type": {"application/json"}},
			requestPath:     "/allow_body",
			expectedStatus:  http.StatusForbidden,
			expectedBody:    "",
			expectedHeaders: make(http.Header),
			backendHeaders:  make(http.Header),
			removeHeaders:   make(http.Header),
		},
		{
			msg:             "GET against body protected endpoint",
			filterName:      "opaAuthorizeRequestWithBody",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_body",
			requestMethod:   "GET",
			requestHeaders:  map[string][]string{"content-type": {"application/json"}},
			requestPath:     "/allow_body",
			expectedStatus:  http.StatusForbidden,
			expectedBody:    "",
			expectedHeaders: make(http.Header),
			backendHeaders:  make(http.Header),
			removeHeaders:   make(http.Header),
		},
		{
			msg:             "Broken Body",
			filterName:      "opaAuthorizeRequestWithBody",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_body",
			requestMethod:   "POST",
			body:            `{ "target_id" / "wrong id" }`,
			requestHeaders:  map[string][]string{"content-type": {"application/json"}},
			requestPath:     "/allow_body",
			expectedStatus:  http.StatusBadRequest,
			expectedBody:    "",
			expectedHeaders: make(http.Header),
			backendHeaders:  make(http.Header),
			removeHeaders:   make(http.Header),
		},
		{
			msg:             "Chained OPA filter with body",
			filterName:      "opaAuthorizeRequestWithBody",
			extraeskipAfter: `-> opaAuthorizeRequestWithBody("somebundle.tar.gz")`,
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_body",
			requestMethod:   "POST",
			body:            `{ "target_id" : "123456" }`,
			requestHeaders:  map[string][]string{"content-type": {"application/json"}},
			requestPath:     "/allow_body",
			expectedStatus:  http.StatusOK,
			expectedBody:    "Welcome!",
			expectedHeaders: make(http.Header),
			backendHeaders:  make(http.Header),
			removeHeaders:   make(http.Header),
		},
		{
			msg:             "Decision id in request header",
			filterName:      "opaAuthorizeRequest",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_object_decision_id_in_header",
			requestMethod:   "POST",
			body:            `{ "target_id" : "123456" }`,
			requestHeaders:  map[string][]string{"content-type": {"application/json"}},
			requestPath:     "/allow/structured",
			expectedStatus:  http.StatusOK,
			expectedBody:    "Welcome!",
			expectedHeaders: map[string][]string{"Decision-Id": {"some-random-decision-id-generated-during-evaluation"}},
			backendHeaders:  make(http.Header),
			removeHeaders:   make(http.Header),
		},
		{
			msg:               "Invalid UTF-8 in Path",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/allow/%c0%ae%c0%ae",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusBadRequest,
			expectedBody:      "",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Invalid UTF-8 in Query",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/allow?%c0%ae=%c0%ae%c0%ae",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusBadRequest,
			expectedBody:      "",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
		},
		{
			msg:               "Allow Requests ignoring fragment",
			filterName:        "opaAuthorizeRequest",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow_with_path_having_fragment",
			requestPath:       "/path-with-empty-query#fragment?",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
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

				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, ti.body, string(body))
			}))
			defer clientServer.Close()

			opaControlPlane := opasdktest.MustNewServer(
				opasdktest.MockBundle("/bundles/"+ti.bundleName, map[string]string{
					"main.rego": `
						package envoy.authz

						import rego.v1

						default allow := false
						default deny_with_query := false

						allow if {
							input.parsed_path == [ "allow" ]
							input.parsed_query == {}
						}

						allow_with_http_path if {
							input.attributes.request.http.path == "/some/api/path?q1=v1&msg=help%20me"
						}

						allow_with_space_in_path if{
							input.parsed_path == [ "my path" ]
						}

						allow_with_path_having_empty_query if {
							input.parsed_path == [ "path-with-empty-query" ]
							input.parsed_query == {}
						}

						allow_with_query if {
							input.parsed_path == [ "allow-with-query" ]
							input.parsed_query.pass == ["yes"]
							input.parsed_query.id == ["1", "2"]
							input.parsed_query.msg == ["help me"]
						}

						deny_with_query if {
							input.attributes.request.http.path == "/allow-me?tofail=true"
							not input.parsed_query.tofail == ["true"]
						}

						allow_with_path_having_fragment if {
							input.parsed_path == [ "path-with-empty-query" ]
							input.attributes.request.http.path == "/path-with-empty-query"
						}

						allow_context_extensions if {
							input.attributes.contextExtensions["com.mycompany.myprop"] == "myvalue"
						}

						allow_runtime_environment if {
							opa.runtime().config.labels.environment == "test"
						}

						default allow_object := {
							"allowed": false,
							"headers": {"x-ext-auth-allow": "no"},
							"body": "Unauthorized Request",
							"http_status": 401
						}

						allow_object := response if {
							input.parsed_path == [ "allow", "structured" ]
							response := {
								"allowed": true,
								"headers": {
									"x-consumer": "x-consumer header value"
								},
								"request_headers_to_remove" : [
									"x-remove-me",
									"absent-header"
								],
								"response_headers_to_add": {
									"x-response-header": "a response header value",
									"server": "server header"
								}
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

						default allow_body := false

						allow_body if {
							input.parsed_body.target_id == "123456"
						}

						decision_id := input.attributes.metadataContext.filterMetadata.open_policy_agent.decision_id

						allow_object_decision_id_in_header := response if {
						    input.parsed_path = ["allow", "structured"]
						    decision_id
						    response := {
						        "allowed": true,
						        "response_headers_to_add": {
						            "decision-id": decision_id
						        }
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
				"labels": {
					"environment": "test"
				},
				"plugins": {
					"envoy_ext_authz_grpc": {
						"path": %q,
						"dry-run": false
					}
				}
			}`, opaControlPlane.URL(), ti.regoQuery))

			envoyMetaDataConfig := []byte(`{
				"filter_metadata": {
					"envoy.filters.http.header_to_metadata": {
						"policy_type": "ingress"
					}
				}
			}`)

			opts := make([]func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error, 0)
			opts = append(opts,
				openpolicyagent.WithConfigTemplate(config),
				openpolicyagent.WithEnvoyMetadataBytes(envoyMetaDataConfig),
			)

			opaFactory, err := openpolicyagent.NewOpenPolicyAgentRegistry(openpolicyagent.WithTracer(tracingtest.NewTracer()), openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...))
			assert.NoError(t, err)

			ftSpec := NewOpaAuthorizeRequestSpec(opaFactory)
			fr.Register(ftSpec)
			ftSpec = NewOpaAuthorizeRequestWithBodySpec(opaFactory)
			fr.Register(ftSpec)
			fr.Register(builtin.NewSetPath())

			r := eskip.MustParse(fmt.Sprintf(`* -> %s %s("%s", "%s") %s -> "%s"`, ti.extraeskipBefore, ti.filterName, ti.bundleName, ti.contextExtensions, ti.extraeskipAfter, clientServer.URL))

			proxy := proxytest.New(fr, r...)

			var bodyReader io.Reader
			if ti.body != "" {
				bodyReader = strings.NewReader(ti.body)
			}

			req, err := http.NewRequest(ti.requestMethod, proxy.URL+ti.requestPath, bodyReader)
			for name, values := range ti.removeHeaders {
				for _, value := range values {
					req.Header.Add(name, value) //adding the headers to validate removal.
				}
			}
			for name, values := range ti.requestHeaders {
				for _, value := range values {
					req.Header.Add(name, value)
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

func TestAuthorizeRequestFilterWithS3DecisionLogPlugin(t *testing.T) {
	for _, ti := range []struct {
		msg               string
		filterName        string
		extraeskipBefore  string
		extraeskipAfter   string
		regoQuery         string
		requestPath       string
		requestMethod     string
		body              string
		contextExtensions string
		expectedBody      string
		expectedHeaders   http.Header
		expectedStatus    int
		backendHeaders    http.Header
		removeHeaders     http.Header
		discoveryPath     string
		discoveryBundle   string
	}{
		{
			msg:               "Allow Requests for log upload success",
			filterName:        "opaAuthorizeRequest",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/allow",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
			discoveryPath:     "logs-success",
		},
		{
			msg:               "Deny Requests for log upload success",
			filterName:        "opaAuthorizeRequest",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/not-allow",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusForbidden,
			expectedBody:      "",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
			discoveryPath:     "logs-success",
		},
		{
			msg:               "Allow Requests for log upload timeout",
			filterName:        "opaAuthorizeRequest",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/allow",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
			discoveryPath:     "logs-timeout",
		},
		{
			msg:               "Allow Requests for log upload forbidden",
			filterName:        "opaAuthorizeRequest",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/allow",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
			discoveryPath:     "logs-forbidden",
		},
		{
			msg:               "Allow Requests for log upload path not found",
			filterName:        "opaAuthorizeRequest",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/allow",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
			discoveryPath:     "logs-notfound",
		},
		{
			msg:               "Allow Requests for log upload throws 5xx",
			filterName:        "opaAuthorizeRequest",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/allow",
			requestMethod:     "GET",
			contextExtensions: "",
			expectedStatus:    http.StatusOK,
			expectedBody:      "Welcome!",
			expectedHeaders:   make(http.Header),
			backendHeaders:    make(http.Header),
			removeHeaders:     make(http.Header),
			discoveryPath:     "logs-5xx",
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			t.Logf("Running test for %v", ti)
			clientServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("Welcome!"))
				assert.True(t, isHeadersPresent(t, ti.backendHeaders, r.Header), "Enriched request header is absent.")
				assert.True(t, isHeadersAbsent(t, ti.removeHeaders, r.Header), "Unwanted HTTP Headers present.")

				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, ti.body, string(body))
			}))
			defer clientServer.Close()

			var logUploadCount int32
			s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "logs-success") {
					atomic.AddInt32(&logUploadCount, 1)
					w.WriteHeader(http.StatusOK)
				} else if strings.Contains(r.URL.Path, "logs-forbidden") {
					atomic.AddInt32(&logUploadCount, 1)
					w.WriteHeader(http.StatusForbidden)
				} else if strings.Contains(r.URL.Path, "logs-timeout") {
					atomic.AddInt32(&logUploadCount, 1)
					time.Sleep(5 * time.Second)
					w.WriteHeader(http.StatusOK)
				} else if strings.Contains(r.URL.Path, "logs-5xx") {
					atomic.AddInt32(&logUploadCount, 1)
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					atomic.AddInt32(&logUploadCount, 1)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer s3Server.Close()

			opaControlPlane := opasdktest.MustNewServer(
				opasdktest.MockBundle("/bundles/test", map[string]string{
					"main.rego": `
						package envoy.authz
		
						default allow = false
						
						allow if {
							input.parsed_path == [ "allow" ]
							input.parsed_query == {}
						}
					`,
				}),
				opasdktest.MockBundle("/bundles/discovery", map[string]string{
					"data.json": `{
					  "discovery": {
						"bundles": {
						  "bundles/test": {
							"persist": false,
							"resource": "bundles/test",
							"service": "test"
						  }
				}}
				}`,
				}),
			)

			config := []byte(fmt.Sprintf(`{
				"services": {
					"test": {
						"url": %q
					}
				},
				"discovery": {
					"name": "discovery",
					"resource": "/bundles/discovery",
					"service": "test"
				},
				"labels": {
					"environment": "test"
				},
				"decision_logs": {
						"plugin": "eopa_dl"
				},
				"plugins": {
					"envoy_ext_authz_grpc": {
						"path": %q,
						"dry-run": false
					},
					"eopa_dl": {
						"buffer": {
							"type": "memory",
							"max_bytes": 50000000
						},
						"output": {
							"type": "s3",
							"bucket": %q,
							"endpoint": %q,
							"force_path": true,
							"region": "eu-central-1",
							"access_key_id": "myid",
							"access_secret": "mysecret",
							"timeout": "2s",	
							"batching": {
							  "at_bytes": 10000000,
							  "at_period": "1s"
							}
						}
					}
			}
			}`, opaControlPlane.URL(), ti.regoQuery, ti.discoveryPath, s3Server.URL))

			fr := make(filters.Registry)

			envoyMetaDataConfig := []byte(`{
				"filter_metadata": {
					"envoy.filters.http.header_to_metadata": {
						"policy_type": "ingress"
					}
				}
			}`)

			opts := make([]func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error, 0)
			opts = append(opts,
				openpolicyagent.WithConfigTemplate(config),
				openpolicyagent.WithEnvoyMetadataBytes(envoyMetaDataConfig),
			)
			opaFactory, err := openpolicyagent.NewOpenPolicyAgentRegistry(openpolicyagent.WithTracer(tracingtest.NewTracer()),
				openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...))
			assert.NoError(t, err)

			ftSpec := NewOpaAuthorizeRequestSpec(opaFactory)
			fr.Register(ftSpec)
			ftSpec = NewOpaAuthorizeRequestWithBodySpec(opaFactory)
			fr.Register(ftSpec)
			fr.Register(builtin.NewSetPath())

			r := eskip.MustParse(fmt.Sprintf(`* -> %s %s("%s", "%s") %s -> "%s"`, ti.extraeskipBefore, ti.filterName, "test", ti.contextExtensions, ti.extraeskipAfter, clientServer.URL))

			proxy := proxytest.New(fr, r...)

			var bodyReader io.Reader
			if ti.body != "" {
				bodyReader = strings.NewReader(ti.body)
			}

			req, err := http.NewRequest(ti.requestMethod, proxy.URL+ti.requestPath, bodyReader)
			assert.NoError(t, err)

			rsp, err := proxy.Client().Do(req)
			assert.NoError(t, err)

			assert.Equal(t, ti.expectedStatus, rsp.StatusCode, "HTTP status does not match")

			assert.True(t, isHeadersPresent(t, ti.expectedHeaders, rsp.Header), "HTTP Headers do not match")
			defer rsp.Body.Close()
			body, err := io.ReadAll(rsp.Body)
			assert.NoError(t, err)
			assert.Equal(t, ti.expectedBody, string(body), "HTTP Body does not match")

			time.Sleep(2 * time.Second) // wait for async decision log to be sent
			assert.True(t, atomic.LoadInt32(&logUploadCount) >= 1, "Decision log upload was not attempted")

			// Simulate a second request while decision log batching/upload is in progress
			proxy.Client().Timeout = 20 * time.Millisecond
			rsp, err = proxy.Client().Do(req)
			assert.NoError(t, err)
			assert.Equal(t, ti.expectedStatus, rsp.StatusCode, "HTTP status does not match")
		})
	}
}

func TestCreateFilterArguments(t *testing.T) {
	opaRegistry, err := openpolicyagent.NewOpenPolicyAgentRegistry(
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(openpolicyagent.WithConfigTemplate([]byte(""))))
	assert.NoError(t, err)

	ftSpec := NewOpaAuthorizeRequestSpec(opaRegistry)

	_, err = ftSpec.CreateFilter([]interface{}{})
	assert.ErrorIs(t, err, filters.ErrInvalidFilterParameters)

	_, err = ftSpec.CreateFilter([]interface{}{"a bundle", "extra: value", "superfluous argument"})
	assert.ErrorIs(t, err, filters.ErrInvalidFilterParameters)
}

func TestAuthorizeRequestInputContract(t *testing.T) {
	for _, ti := range []struct {
		msg               string
		filterName        string
		bundleName        string
		regoQuery         string
		requestPath       string
		requestMethod     string
		requestHeaders    http.Header
		body              string
		contextExtensions string
		expectedBody      string
		expectedHeaders   http.Header
		expectedStatus    int
		backendHeaders    http.Header
		removeHeaders     http.Header
	}{
		{
			msg:               "Input contract validation",
			filterName:        "opaAuthorizeRequestWithBody",
			bundleName:        "somebundle.tar.gz",
			regoQuery:         "envoy/authz/allow",
			requestPath:       "/users/profile/amal?param=1",
			requestMethod:     "GET",
			contextExtensions: "",
			body:              `{ "key" : "value" }`,
			requestHeaders: http.Header{
				"accept":       []string{"*/*"},
				"user-agent":   []string{"curl/7.68.0-DEV"},
				"x-request-id": []string{"1455bbb0-0623-4810-a2c6-df73ffd8863a"},
				"content-type": {"application/json"},
			},
			expectedStatus:  http.StatusOK,
			expectedBody:    "Welcome!",
			expectedHeaders: map[string][]string{"user-agent": {"curl/7.68.0-DEV"}},
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			t.Logf("Running test for %v", ti)
			clientServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("Welcome!"))

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.Equal(t, ti.body, string(body))
			}))
			defer clientServer.Close()

			opaControlPlane := opasdktest.MustNewServer(
				opasdktest.MockBundle("/bundles/"+ti.bundleName, map[string]string{
					"main.rego": `
						package envoy.authz

						import rego.v1

						default allow = false

						allow if {
							input.attributes.request.http.path == "/users/profile/amal?param=1"
							input.parsed_path == ["users", "profile", "amal"]
							input.parsed_query == {"param": ["1"]}
							input.attributes.request.http.headers["accept"] == "*/*"
							input.attributes.request.http.headers["user-agent"] == "curl/7.68.0-DEV"
							input.attributes.request.http.headers["x-request-id"] == "1455bbb0-0623-4810-a2c6-df73ffd8863a"
							input.attributes.request.http.headers["content-type"] == "application/json"
							input.attributes.metadataContext.filterMetadata["envoy.filters.http.header_to_metadata"].policy_type == "ingress"
							opa.runtime().config.labels.environment == "test"
							input.parsed_body.key == "value"
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
				"labels": {
					"environment": "test"
				},
				"plugins": {
					"envoy_ext_authz_grpc": {    
						"path": %q,
						"dry-run": false    
					}
				}
			}`, opaControlPlane.URL(), ti.regoQuery))

			envoyMetaDataConfig := []byte(`{
				"filter_metadata": {
					"envoy.filters.http.header_to_metadata": {
						"policy_type": "ingress"
					}
				}
			}`)

			opts := make([]func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error, 0)
			opts = append(opts,
				openpolicyagent.WithConfigTemplate(config),
				openpolicyagent.WithEnvoyMetadataBytes(envoyMetaDataConfig))

			opaFactory, err := openpolicyagent.NewOpenPolicyAgentRegistry(openpolicyagent.WithTracer(tracingtest.NewTracer()),
				openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...))
			assert.NoError(t, err)

			ftSpec := NewOpaAuthorizeRequestSpec(opaFactory)
			fr.Register(ftSpec)
			ftSpec = NewOpaAuthorizeRequestWithBodySpec(opaFactory)
			fr.Register(ftSpec)
			fr.Register(builtin.NewSetPath())

			r := eskip.MustParse(fmt.Sprintf(`* -> %s("%s", "%s") -> "%s"`, ti.filterName, ti.bundleName, ti.contextExtensions, clientServer.URL))

			proxy := proxytest.New(fr, r...)

			var bodyReader io.Reader
			if ti.body != "" {
				bodyReader = strings.NewReader(ti.body)
			}

			req, err := http.NewRequest(ti.requestMethod, proxy.URL+ti.requestPath, bodyReader)

			require.NoError(t, err)

			for name, values := range ti.requestHeaders {
				for _, value := range values {
					req.Header.Add(name, value)
				}
			}

			rsp, err := proxy.Client().Do(req)
			require.NoError(t, err)

			assert.Equal(t, ti.expectedStatus, rsp.StatusCode, "HTTP status does not match")

			defer rsp.Body.Close()
			body, err := io.ReadAll(rsp.Body)
			require.NoError(t, err)
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

		// since decision id is randomly generated we are just checking for not nil
		if headerName == "Decision-Id" {
			assert.NotNil(t, actualValues)
		} else {
			assert.ElementsMatch(t, expectedValues, actualValues)
		}
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
