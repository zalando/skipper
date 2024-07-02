package opaauthorizerequest

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	opasdktest "github.com/open-policy-agent/opa/sdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/tracing/tracingtest"

	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/filters/openpolicyagent"
)

func TestAuthorizeRequestFilter(t *testing.T) {
	for _, ti := range []struct {
		msg               string
		filterName        string
		extraeskip        string
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
			extraeskip:      `-> opaAuthorizeRequestWithBody("somebundle.tar.gz")`,
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

						default allow = false

						allow {
							input.parsed_path = [ "allow" ]
						}

						allow_context_extensions {
							input.attributes.contextExtensions["com.mycompany.myprop"] == "myvalue"
						}

						allow_runtime_environment {
							opa.runtime().config.labels.environment == "test"
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

						default allow_body = false

						allow_body {
							input.parsed_body.target_id == "123456"
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

			opaFactory := openpolicyagent.NewOpenPolicyAgentRegistry(openpolicyagent.WithTracer(&tracingtest.Tracer{}))
			ftSpec := NewOpaAuthorizeRequestSpec(opaFactory, openpolicyagent.WithConfigTemplate(config))
			fr.Register(ftSpec)
			ftSpec = NewOpaAuthorizeRequestWithBodySpec(opaFactory, openpolicyagent.WithConfigTemplate(config))
			fr.Register(ftSpec)

			r := eskip.MustParse(fmt.Sprintf(`* -> %s("%s", "%s") %s -> "%s"`, ti.filterName, ti.bundleName, ti.contextExtensions, ti.extraeskip, clientServer.URL))

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

func TestCreateFilterArguments(t *testing.T) {
	opaRegistry := openpolicyagent.NewOpenPolicyAgentRegistry()
	ftSpec := NewOpaAuthorizeRequestSpec(opaRegistry, openpolicyagent.WithConfigTemplate([]byte("")))

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

const (
	tokenExp = 2 * time.Hour

	certPath = "../../../skptesting/cert.pem"
	keyPath  = "../../../skptesting/key.pem"
)

func BenchmarkAuthorizeRequest(b *testing.B) {
	b.Run("authorize-request-minimal", func(b *testing.B) {
		opaControlPlane := opasdktest.MustNewServer(
			opasdktest.MockBundle("/bundles/somebundle.tar.gz", map[string]string{
				"main.rego": `
					package envoy.authz

					default allow = false

					allow {
						input.parsed_path = [ "allow" ]
					}
				`,
			}),
		)

		f, err := createOpaFilter(opaControlPlane)
		assert.NoError(b, err)

		url, err := url.Parse("http://opa-authorized.test/somepath")
		assert.NoError(b, err)

		ctx := &filtertest.Context{
			FStateBag: map[string]interface{}{},
			FResponse: &http.Response{},
			FRequest: &http.Request{
				Header: map[string][]string{
					"Authorization": {"Bearer FOOBAR"},
				},
				URL: url,
			},
			FMetrics: &metricstest.MockMetrics{},
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			f.Request(ctx)
		}
	})

	b.Run("authorize-request-jwt-validation", func(b *testing.B) {

		publicKey, err := os.ReadFile(certPath)
		if err != nil {
			log.Fatalf("Failed to read public key: %v", err)
		}

		opaControlPlane := opasdktest.MustNewServer(
			opasdktest.MockBundle("/bundles/somebundle.tar.gz", map[string]string{
				"main.rego": fmt.Sprintf(`
					package envoy.authz

					import future.keywords.if

					default allow = false

					public_key_cert := %q

					bearer_token := t if {
						v := input.attributes.request.http.headers.authorization
						startswith(v, "Bearer ")
						t := substring(v, count("Bearer "), -1)
					}

					allow if {
						[valid, _, payload] :=  io.jwt.decode_verify(bearer_token, {
							"cert": public_key_cert,
							"aud": "nqz3xhorr5"
						})
					
						valid
						
						payload.sub == "5974934733"
					}				
				`, publicKey),
			}),
		)

		f, err := createOpaFilter(opaControlPlane)
		assert.NoError(b, err)

		url, err := url.Parse("http://opa-authorized.test/somepath")
		assert.NoError(b, err)

		claims := jwt.MapClaims{
			"iss":   "https://some.identity.acme.com",
			"sub":   "5974934733",
			"aud":   "nqz3xhorr5",
			"iat":   time.Now().Add(-time.Minute).UTC().Unix(),
			"exp":   time.Now().Add(tokenExp).UTC().Unix(),
			"email": "someone@example.org",
		}

		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

		privKey, err := os.ReadFile(keyPath)
		if err != nil {
			log.Fatalf("Failed to read priv key: %v", err)
		}

		key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privKey))
		if err != nil {
			log.Fatalf("Failed to parse RSA PEM: %v", err)
		}

		// Sign and get the complete encoded token as a string using the secret
		signedToken, err := token.SignedString(key)
		if err != nil {
			log.Fatalf("Failed to sign token: %v", err)
		}

		ctx := &filtertest.Context{
			FStateBag: map[string]interface{}{},
			FResponse: &http.Response{},
			FRequest: &http.Request{
				Header: map[string][]string{
					"Authorization": {fmt.Sprintf("Bearer %s", signedToken)},
				},
				URL: url,
			},
			FMetrics: &metricstest.MockMetrics{},
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			f.Request(ctx)
			assert.False(b, ctx.FServed)
		}
	})
}

func createOpaFilter(opaControlPlane *opasdktest.Server) (filters.Filter, error) {
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
		}`, opaControlPlane.URL(), "envoy/authz/allow"))

	opaFactory := openpolicyagent.NewOpenPolicyAgentRegistry()
	spec := NewOpaAuthorizeRequestSpec(opaFactory, openpolicyagent.WithConfigTemplate(config))
	f, err := spec.CreateFilter([]interface{}{"somebundle.tar.gz"})
	return f, err
}
