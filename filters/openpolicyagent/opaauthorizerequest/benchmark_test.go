package opaauthorizerequest

import (
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	opasdktest "github.com/open-policy-agent/opa/sdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/metrics/metricstest"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	tokenExp = 2 * time.Hour

	certPath = "../../../skptesting/cert.pem"
	keyPath  = "../../../skptesting/key.pem"

	testDecisionPath = "envoy/authz/allow"
	testBundleName   = "somebundle.tar.gz"
)

var testBundleEndpoint = fmt.Sprintf("/bundles/%s", testBundleName)

func BenchmarkMinimalPolicy(b *testing.B) {
	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle(testBundleEndpoint, map[string]string{
			"main.rego": `
					package envoy.authz

					default allow = false

					allow {
						input.parsed_path = [ "allow" ]
					}
				`,
		}),
	)

	filterOpts := NewFilterOptionsWithDefaults(opaControlPlane.URL())
	f, err := createOpaFilter(filterOpts)
	require.NoError(b, err)

	reqUrl, err := url.Parse("http://opa-authorized.test/allow")
	require.NoError(b, err)

	ctx := &filtertest.Context{
		FStateBag: map[string]interface{}{},
		FResponse: &http.Response{},
		FRequest: &http.Request{
			URL: reqUrl,
		},
		FMetrics: &metricstest.MockMetrics{},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f.Request(ctx)
			assert.False(b, ctx.FServed)
		}
	})
}

func BenchmarkMinimalPolicyWithDecisionLogs(b *testing.B) {
	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle(testBundleEndpoint, map[string]string{
			"main.rego": `
					package envoy.authz

					default allow = false

					allow {
						input.parsed_path = [ "allow" ]
					}
				`,
		}),
	)

	decisionLogsConsumer := newDecisionConsumer()
	filterOpts := FilterOptions{
		OpaControlPlaneUrl:  opaControlPlane.URL(),
		DecisionConsumerUrl: decisionLogsConsumer.URL,
		DecisionPath:        testDecisionPath,
		BundleName:          testBundleName,
		DecisionLogging:     true,
		ContextExtensions:   "",
	}
	f, err := createOpaFilter(filterOpts)
	require.NoError(b, err)

	reqUrl, err := url.Parse("http://opa-authorized.test/allow")
	require.NoError(b, err)

	ctx := &filtertest.Context{
		FStateBag: map[string]interface{}{},
		FResponse: &http.Response{},
		FRequest: &http.Request{
			URL: reqUrl,
		},
		FMetrics: &metricstest.MockMetrics{},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f.Request(ctx)
		}
	})
}

func BenchmarkAllowWithReqBody(b *testing.B) {
	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle(testBundleEndpoint, map[string]string{
			"main.rego": `
					package envoy.authz

					import rego.v1

					default allow = false

					allow if {
						endswith(input.parsed_body.email, "@zalando.de")
					}
				`,
		}),
	)

	filterOpts := NewFilterOptionsWithDefaults(opaControlPlane.URL())
	f, err := createBodyBasedOpaFilter(filterOpts)
	require.NoError(b, err)

	reqUrl, err := url.Parse("http://opa-authorized.test/allow")
	require.NoError(b, err)

	body := `{"email": "bench-test@zalando.de"}`
	ctx := &filtertest.Context{
		FStateBag: map[string]interface{}{},
		FResponse: &http.Response{},
		FRequest: &http.Request{
			Method: "POST",
			Header: map[string][]string{
				"Content-Type": {"application/json"},
			},
			URL:           reqUrl,
			Body:          io.NopCloser(strings.NewReader(body)),
			ContentLength: int64(len(body)),
		},
		FMetrics: &metricstest.MockMetrics{},
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f.Request(ctx)
		}
	})

}

func BenchmarkJwtValidation(b *testing.B) {
	publicKey, err := os.ReadFile(certPath)
	if err != nil {
		b.Fatalf("Failed to read public key: %v", err)
	}

	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle(testBundleEndpoint, map[string]string{
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

	filterOpts := NewFilterOptionsWithDefaults(opaControlPlane.URL())
	f, err := createOpaFilter(filterOpts)
	require.NoError(b, err)

	reqUrl, err := url.Parse("http://opa-authorized.test/somepath")
	require.NoError(b, err)

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
	require.NoError(b, err, "Failed to read priv key from path %q", keyPath)

	key, err := jwt.ParseRSAPrivateKeyFromPEM(privKey)
	require.NoError(b, err, "Failed to parse RSA PEM")

	// Sign and get the complete encoded token as a string using the secret
	signedToken, err := token.SignedString(key)
	require.NoError(b, err, "Failed to sign token")

	ctx := &filtertest.Context{
		FStateBag: map[string]interface{}{},
		FResponse: &http.Response{},
		FRequest: &http.Request{
			Header: map[string][]string{
				"Authorization": {fmt.Sprintf("Bearer %s", signedToken)},
			},
			URL: reqUrl,
		},
		FMetrics: &metricstest.MockMetrics{},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f.Request(ctx)
			assert.False(b, ctx.FServed)
		}
	})
}

// BenchmarkPolicyBundle measures the performance of the OPA authorization filter
// with a policy bundle loaded from a .tar.gz file.

// This benchmark, serving as a reference test case, evaluates the
// filter's overhead when using a pre-compiled policy bundle to make authorization decisions.

// To generate a bundle in .tar.gz format, use the following command:
// ./opa build -b <bundle_directory> -o <output_file.tar.gz>
// For example:
// ./opa build -b simple-opa-bundle -o simple-opa-bundle.tar.gz
func BenchmarkMinimalPolicyBundle(b *testing.B) {
	bundleName := "simple-opa-bundle.tar.gz"
	bundlePath := fmt.Sprintf("../../../skptesting/%s", bundleName)

	opaControlPlane := newOpaControlPlaneServingBundle(bundlePath, bundleName, b)

	filterOpts := FilterOptions{
		OpaControlPlaneUrl:  opaControlPlane.URL,
		DecisionConsumerUrl: opaControlPlane.URL,
		DecisionPath:        "envoy/authz/allow",
		BundleName:          bundleName,
		DecisionLogging:     false,
	}
	f, err := createOpaFilter(filterOpts)
	require.NoError(b, err)

	requestUrl, err := url.Parse("http://opa-authorized.test/allow")
	assert.NoError(b, err)

	ctx := &filtertest.Context{
		FStateBag: map[string]interface{}{},
		FResponse: &http.Response{},
		FRequest: &http.Request{
			URL:    requestUrl,
			Method: "GET",
		},
		FMetrics: &metricstest.MockMetrics{},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f.Request(ctx)
			assert.False(b, ctx.FServed)
		}
	})
}

func newDecisionConsumer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func newOpaControlPlaneServingBundle(bundlePath, bundleName string, b *testing.B) *httptest.Server {
	if !strings.HasSuffix(bundlePath, ".tar.gz") {
		b.Fatalf("bundle file %q does not have .tar.gz extension", bundlePath)
	}

	fileData, err := os.ReadFile(bundlePath)
	if err != nil {
		b.Fatalf("failed to read bundle file from path %q: %v", bundlePath, err)
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bundles/"+bundleName {
			w.Header().Set("Content-Type", "application/gzip")
			w.Header().Set("Content-Disposition", "attachment; filename="+bundleName)
			_, err := w.Write(fileData)
			if err != nil {
				fmt.Printf("failed to write bundle file: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}

		// Decision logs consumer endpoint
		if r.URL.Path == "/logs" {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

func createOpaFilter(opts FilterOptions) (filters.Filter, error) {
	config := generateConfig(opts.OpaControlPlaneUrl, opts.DecisionConsumerUrl, opts.DecisionPath, opts.DecisionLogging)
	opaFactory := openpolicyagent.NewOpenPolicyAgentRegistry()
	spec := NewOpaAuthorizeRequestSpec(opaFactory, openpolicyagent.WithConfigTemplate(config))
	return spec.CreateFilter([]interface{}{opts.BundleName, opts.ContextExtensions})
}

func createBodyBasedOpaFilter(opts FilterOptions) (filters.Filter, error) {
	config := generateConfig(opts.OpaControlPlaneUrl, opts.DecisionConsumerUrl, opts.DecisionPath, opts.DecisionLogging)
	opaFactory := openpolicyagent.NewOpenPolicyAgentRegistry()
	spec := NewOpaAuthorizeRequestWithBodySpec(opaFactory, openpolicyagent.WithConfigTemplate(config))
	return spec.CreateFilter([]interface{}{opts.BundleName, opts.ContextExtensions})
}

func generateConfig(opaControlPlane string, decisionLogConsumer string, decisionPath string, decisionLogging bool) []byte {
	var decisionPlugin string
	if decisionLogging {
		decisionPlugin = `
			"decision_logs": {
				"console": false,
				"service": "decision_svc",
  				"reporting": {
					"min_delay_seconds": 300,
					"max_delay_seconds": 600				
				}
			},
		`
	}

	return []byte(fmt.Sprintf(`{
		"services": {
			"bundle_svc": {
				"url": %q
			},
			"decision_svc": {
				"url": %q
			}
		},
		"bundles": {
			"test": {
				"service": "bundle_svc",
				"resource": "/bundles/{{ .bundlename }}",
				"polling": {
					"min_delay_seconds": 600,
					"max_delay_seconds": 1200
				}
			}
		},
		"labels": {
			"environment": "test"
		},
		%s
		"plugins": {
			"envoy_ext_authz_grpc": {    
				"path": %q,
				"dry-run": false    
			}
		}
	}`, opaControlPlane, decisionLogConsumer, decisionPlugin, decisionPath))
}

type FilterOptions struct {
	OpaControlPlaneUrl  string
	DecisionConsumerUrl string
	DecisionPath        string
	BundleName          string
	DecisionLogging     bool
	ContextExtensions   string
}

func NewFilterOptionsWithDefaults(opaControlPlaneURL string) FilterOptions {
	return FilterOptions{
		OpaControlPlaneUrl:  opaControlPlaneURL,
		DecisionConsumerUrl: "",
		DecisionPath:        testDecisionPath,
		BundleName:          testBundleName,
		DecisionLogging:     false,
		ContextExtensions:   "",
	}
}
