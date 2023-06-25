package authorizewithregopolicy

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
			requestPath:     "allow",
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
			requestPath:     "forbidden",
			expectedStatus:  http.StatusForbidden,
			expectedHeaders: make(http.Header),
			backendHeaders:  make(http.Header),
			removeHeaders:   make(http.Header),
		},
		{
			msg:             "Allow With Structured Rules",
			bundleName:      "somebundle.tar.gz",
			regoQuery:       "envoy/authz/allow_object",
			requestPath:     "allow/structured",
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
			requestPath:     "forbidden",
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
				assert.True(t, isHeadersPresent(ti.backendHeaders, r.Header), "Enriched request header is absent.")
				assert.True(t, isHeadersAbsent(ti.removeHeaders, r.Header), "Unwanted HTTP Headers present.")
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

			var routeFilters []*eskip.Filter
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
			filterArgs := []interface{}{ti.bundleName}
			_, err := ftSpec.CreateFilter(filterArgs)
			if err != nil {
				t.Fatalf("error in creating filter: %v", err)
			}
			fr.Register(ftSpec)
			routeFilters = append(routeFilters, &eskip.Filter{Name: ftSpec.Name(), Args: filterArgs})

			r := &eskip.Route{Filters: routeFilters, Backend: clientServer.URL}

			proxy := proxytest.New(fr, r)
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Fatalf("Failed to parse url %s: %v", proxy.URL, err)
			}
			reqURL.Path = path.Join(reqURL.Path, ti.requestPath)
			req, err := http.NewRequest("GET", reqURL.String(), nil)
			for name, values := range ti.removeHeaders {
				req.Header.Add(name, values[0]) //adding the headers to validate removal.
			}

			if err != nil {
				t.Fatal(err)
				return
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, ti.expectedStatus, rsp.StatusCode, "HTTP status does not match")

			assert.True(t, isHeadersPresent(ti.expectedHeaders, rsp.Header), "HTTP Headers do not match")

			defer rsp.Body.Close()
			body, err := io.ReadAll(rsp.Body)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, ti.expectedBody, string(body), "HTTP Body does not match")
		})
	}
}

func isHeadersPresent(expectedHeaders http.Header, headers http.Header) bool {
	for headerName, expectedValues := range expectedHeaders {
		actualValues, headerFound := headers[headerName]
		if !headerFound {
			return false
		}

		if !areHeaderValuesEqual(expectedValues, actualValues) {
			return false
		}
	}
	return true
}

func areHeaderValuesEqual(expectedValues, actualValues []string) bool {
	if len(expectedValues) != len(actualValues) {
		return false
	}

	actualValueSet := make(map[string]struct{})
	for _, val := range actualValues {
		actualValueSet[val] = struct{}{}
	}

	for _, val := range expectedValues {
		if _, ok := actualValueSet[val]; !ok {
			return false
		}
		delete(actualValueSet, val)
	}

	return len(actualValueSet) == 0
}

func isHeadersAbsent(unwantedHeaders http.Header, headers http.Header) bool {
	for headerName := range unwantedHeaders {
		if _, ok := headers[headerName]; ok {
			return false
		}
	}
	return true
}
