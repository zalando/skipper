package opaauthorizerequest

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/metrics/metricstest"
)

const (
	tokenExp         = 2 * time.Hour
	testDecisionPath = "envoy/authz/allow"
	testBundleName   = "somebundle.tar.gz"
)

var (
	//go:embed testResources/cert.pem
	publicKey []byte
	//go:embed testResources/key.pem
	privateKey []byte

	testBundleEndpoint = fmt.Sprintf("/bundles/%s", testBundleName)
)

// BenchmarkMinimalPolicy measures the performance of the OPA authorization filter with a minimal policy.
//
// Run this benchmark with varying parallelism:
// go test -bench=^BenchmarkMinimalPolicy$ -cpu=1,2,4 -benchmem ./filters/openpolicyagent/opaauthorizerequest
//
// Run all benchmarks with varying parallelism:
// go test -bench=. -cpu=1,2,4 -benchmem ./filters/openpolicyagent/opaauthorizerequest
//
// `-cpu` controls parallelism by setting the maximum number of CPUs that can be used to run the benchmark.
// Values should be less than or equal to the number of logical CPUs available on your system.  If the `-cpu`
// flag is omitted, the benchmark will default to using GOMAXPROCS, which is typically set to the number
// of logical CPUs.  This means the benchmark will run with maximum available parallelism by default.
//
// `-benchmem` adds memory allocation information to the benchmark results, showing how much memory
// is being used during the benchmark. This is useful for identifying memory leaks or inefficiencies.
//
// `b.RunParallel` is used internally to execute the benchmark in parallel.  The `-cpu` flag (or GOMAXPROCS)
// controls the number of goroutines created by `b.RunParallel`, thus determining the degree of parallelism.
func BenchmarkMinimalPolicy(b *testing.B) {
	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle(testBundleEndpoint, map[string]string{
			"main.rego": `
					package envoy.authz

					import rego.v1

					default allow = false

					allow if {
						input.parsed_path = [ "allow" ]
					}
				`,
		}),
	)
	defer opaControlPlane.Stop()

	filterOpts := NewFilterOptionsWithDefaults(opaControlPlane.URL())
	f, err := createOpaFilter(filterOpts)
	require.NoError(b, err)

	reqUrl, err := url.Parse("http://opa-authorized.test/allow")
	require.NoError(b, err)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &filtertest.Context{
				FStateBag: map[string]interface{}{},
				FResponse: &http.Response{},
				FRequest: &http.Request{
					URL: reqUrl,
				},
				FMetrics: &metricstest.MockMetrics{},
			}
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

					import rego.v1

					default allow = false

					allow if {
						input.parsed_path = [ "allow" ]
					}
				`,
		}),
	)
	defer opaControlPlane.Stop()

	decisionLogsConsumer := newDecisionConsumer()
	defer decisionLogsConsumer.Close()

	filterOpts := FilterOptions{
		OpaControlPlaneUrl:  opaControlPlane.URL(),
		DecisionConsumerUrl: decisionLogsConsumer.URL,
		DecisionPath:        testDecisionPath,
		BundleNames:         []string{testBundleName},
		DecisionLogging:     true,
		ContextExtensions:   "",
	}
	f, err := createOpaFilter(filterOpts)
	require.NoError(b, err)

	reqUrl, err := url.Parse("http://opa-authorized.test/allow")
	require.NoError(b, err)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &filtertest.Context{
				FStateBag: map[string]interface{}{},
				FResponse: &http.Response{},
				FRequest: &http.Request{
					URL: reqUrl,
				},
				FMetrics: &metricstest.MockMetrics{},
			}
			f.Request(ctx)
			assert.False(b, ctx.FServed)
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
	defer opaControlPlane.Stop()

	filterOpts := NewFilterOptionsWithDefaults(opaControlPlane.URL())
	f, err := createBodyBasedOpaFilter(filterOpts)
	require.NoError(b, err)

	reqUrl, err := url.Parse("http://opa-authorized.test/allow")
	require.NoError(b, err)

	body := `{"email": "bench-test@zalando.de"}`
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
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
			f.Request(ctx)
			assert.False(b, ctx.FServed)
		}
	})

}

func BenchmarkJwtValidation(b *testing.B) {
	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle(testBundleEndpoint, map[string]string{
			"main.rego": fmt.Sprintf(`
					package envoy.authz

					import rego.v1

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
	defer opaControlPlane.Stop()

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

	key, err := jwt.ParseRSAPrivateKeyFromPEM(privateKey)
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
// when using a pre-compiled policy bundle loaded from a .tar.gz file.

// This benchmark evaluates the filter's authorization decision overhead with a
// pre-compiled bundle, serving as a representative use case.

// A sample bundle for this benchmark is located at testResources/simple-opa-bundle.
// To generate a .tar.gz bundle, use the following command:
//
//   opa build -b <bundle_directory> -o <output_file.tar.gz>
//
// For example:
//
//   cd testResources
//   opa build -b simple-opa-bundle -o simple-opa-bundle.tar.gz

// You can also use your own bundle.  If you do so, ensure that the bundleName,
// bundlePath, and filterOpts variables are correctly configured to match your bundle.
func BenchmarkMinimalPolicyBundle(b *testing.B) {
	bundleName := "simple-opa-bundle.tar.gz"

	bundleFiles := map[string]string{
		bundleName: fmt.Sprintf("testResources/%s", bundleName),
	}

	opaControlPlane := newOpaControlPlaneServingDataAndPolicyBundles(b, bundleFiles)
	defer opaControlPlane.Close()

	filterOpts := FilterOptions{
		OpaControlPlaneUrl:  opaControlPlane.URL,
		DecisionConsumerUrl: opaControlPlane.URL,
		DecisionPath:        "envoy/authz/allow",
		BundleNames:         []string{bundleName},
		DecisionLogging:     false,
	}
	f, err := createOpaFilter(filterOpts)
	require.NoError(b, err)

	requestUrl, err := url.Parse("http://opa-authorized.test/allow")
	assert.NoError(b, err)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &filtertest.Context{
				FStateBag: map[string]interface{}{},
				FResponse: &http.Response{},
				FRequest: &http.Request{
					URL:    requestUrl,
					Method: "GET",
				},
				FMetrics: &metricstest.MockMetrics{},
			}
			f.Request(ctx)
			assert.False(b, ctx.FServed)
		}
	})
}

// BenchmarkSplitPolicyAndDataBundles measures the performance of the OPA authorization filter
// when using a pre-compiled split policy and data bundles loaded from a .tar.gz file.

// This benchmark evaluates the filter's authorization decision overhead with a
// pre-compiled bundles, serving as a representative use case.

// A sample bundles for this benchmark are located at testResources/split-bundles.
// To evaluate the performance of data-preprocessing optimization, use the resources at
// testResources/data-pre-processing-visible-impact.
// To generate a .tgz bundle, use the following command:
//
//   opa build -b <bundle_directory> -o <output_file.tgz>
//
// For example:
//
//   cd testResources/split-bundles
//   opa build -b data -o context-data.tgz
//   opa build -b policy -o policy.tgz

// You can also use your own bundles.  If you do so, ensure that the bundlePaths,
// and filterOpts variables are correctly configured to match your bundle and
// the roots in .manifest files in your bundles do not overlap.
func BenchmarkSplitPolicyAndDataBundles(b *testing.B) {
	type benchmarkCase struct {
		name       string
		createFunc func(FilterOptions) (filters.Filter, error)
	}

	cases := []benchmarkCase{
		{
			name: "default_filter",
			createFunc: func(opts FilterOptions) (filters.Filter, error) {
				return createOpaFilterForMultipleBundles(opts)
			},
		},
		{
			name: "with_data_preprocessing_optimization",
			createFunc: func(opts FilterOptions) (filters.Filter, error) {
				return createOpaFilterWithDataProcessingOptimization(opts)
			},
		},
	}

	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			bundleFiles := map[string]string{
				"policy":       "testResources/split-bundles/policy.tgz",
				"context-data": "testResources/split-bundles/context-data.tgz",
			}

			opaControlPlane := newOpaControlPlaneServingDataAndPolicyBundles(b, bundleFiles)
			defer opaControlPlane.Close()

			filterOpts := FilterOptions{
				OpaControlPlaneUrl:  opaControlPlane.URL,
				DecisionConsumerUrl: opaControlPlane.URL,
				DecisionPath:        "policy/allow",
				BundleNames:         []string{"policy", "context-data"},
				DecisionLogging:     false,
			}

			f, err := bc.createFunc(filterOpts)
			require.NoError(b, err)

			requestURL, err := url.Parse("http://opa-authorized.test/allow/alice")
			require.NoError(b, err)

			b.ReportAllocs()
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					ctx := &filtertest.Context{
						FStateBag: map[string]interface{}{},
						FResponse: &http.Response{},
						FRequest: &http.Request{
							URL:    requestURL,
							Method: "GET",
						},
						FMetrics: &metricstest.MockMetrics{},
					}

					f.Request(ctx)
					assert.False(b, ctx.FServed)
					assert.NotEqual(b, 403, ctx.FResponse.StatusCode, "Expected 403 Forbidden response")
				}
			})
		})
	}
}

func newOpaControlPlaneServingDataAndPolicyBundles(b *testing.B, bundleFiles map[string]string) *httptest.Server {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for bundleName, bundlePath := range bundleFiles {
			if r.URL.Path == "/bundles/"+bundleName {
				fileData, err := os.ReadFile(bundlePath)
				if err != nil {
					b.Fatalf("failed to read bundle file from path %q: %v", bundlePath, err)
				}
				w.Header().Set("Content-Type", "application/gzip")
				w.Header().Set("Content-Disposition", "attachment; filename="+bundleName)
				_, err = w.Write(fileData)
				if err != nil {
					fmt.Printf("failed to write bundle file: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
				}
				return
			}
		}

		if r.URL.Path == "/logs" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))

	return server
}

func newDecisionConsumer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func createOpaFilter(opts FilterOptions) (filters.Filter, error) {
	config := generateConfig(opts.OpaControlPlaneUrl, opts.DecisionConsumerUrl, opts.DecisionPath, opts.DecisionLogging)

	opaFactory := openpolicyagent.NewOpenPolicyAgentRegistry(openpolicyagent.WithOpenPolicyAgentInstanceConfig(openpolicyagent.WithConfigTemplate(config)))
	spec := NewOpaAuthorizeRequestSpec(opaFactory)
	return spec.CreateFilter([]interface{}{opts.BundleNames[0], opts.ContextExtensions})
}

func createBodyBasedOpaFilter(opts FilterOptions) (filters.Filter, error) {
	config := generateConfig(opts.OpaControlPlaneUrl, opts.DecisionConsumerUrl, opts.DecisionPath, opts.DecisionLogging)

	opaFactory := openpolicyagent.NewOpenPolicyAgentRegistry(openpolicyagent.WithOpenPolicyAgentInstanceConfig(openpolicyagent.WithConfigTemplate(config)))
	spec := NewOpaAuthorizeRequestWithBodySpec(opaFactory)
	return spec.CreateFilter([]interface{}{opts.BundleNames[0], opts.ContextExtensions})
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

func createOpaFilterForMultipleBundles(opts FilterOptions) (filters.Filter, error) {
	config := generateConfigForMultipleBundles(opts.BundleNames, opts.OpaControlPlaneUrl, opts.DecisionConsumerUrl, opts.DecisionPath, opts.DecisionLogging)
	opaFactory := openpolicyagent.NewOpenPolicyAgentRegistry(openpolicyagent.WithOpenPolicyAgentInstanceConfig(openpolicyagent.WithConfigTemplate(config)))
	// Enable data pre-processing optimization by default for multiple bundles
	spec := NewOpaAuthorizeRequestSpec(opaFactory)
	return spec.CreateFilter([]interface{}{opts.BundleNames[0], opts.ContextExtensions})
}

func createOpaFilterWithDataProcessingOptimization(opts FilterOptions) (filters.Filter, error) {
	config := generateConfigForMultipleBundles(opts.BundleNames, opts.OpaControlPlaneUrl, opts.DecisionConsumerUrl, opts.DecisionPath, opts.DecisionLogging)
	registry := openpolicyagent.NewOpenPolicyAgentRegistry(openpolicyagent.WithEnableDataPreProcessingOptimization(true), openpolicyagent.WithOpenPolicyAgentInstanceConfig(openpolicyagent.WithConfigTemplate(config)))
	spec := NewOpaAuthorizeRequestSpec(registry)
	return spec.CreateFilter([]interface{}{opts.BundleNames[0], opts.ContextExtensions})
}

func generateConfigForMultipleBundles(bundlesNames []string, opaControlPlane string, decisionLogConsumer string, decisionPath string, decisionLogging bool) []byte {
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
			"policy": {
      			"service": "bundle_svc",
      			"resource": "/bundles/%s",
      			"polling": {
        			"min_delay_seconds": 600,
					"max_delay_seconds": 1200
      			}
    		},
    		"context-data": {
      			"service": "bundle_svc",
      			"resource": "/bundles/%s",
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
	}`, opaControlPlane, decisionLogConsumer, bundlesNames[0], bundlesNames[1], decisionPlugin, decisionPath))
}

type FilterOptions struct {
	OpaControlPlaneUrl  string
	DecisionConsumerUrl string
	DecisionPath        string
	BundleNames         []string
	DecisionLogging     bool
	ContextExtensions   string
}

func NewFilterOptionsWithDefaults(opaControlPlaneURL string) FilterOptions {
	return FilterOptions{
		OpaControlPlaneUrl:  opaControlPlaneURL,
		DecisionConsumerUrl: "",
		DecisionPath:        testDecisionPath,
		BundleNames:         []string{testBundleName},
		DecisionLogging:     false,
		ContextExtensions:   "",
	}
}
