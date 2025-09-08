package openpolicyagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/opatestutils"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	pbstruct "google.golang.org/protobuf/types/known/structpb"

	"github.com/open-policy-agent/opa/v1/ast"

	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	opaconf "github.com/open-policy-agent/opa/v1/config"
	"github.com/open-policy-agent/opa/v1/download"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/plugins/bundle"
	"github.com/open-policy-agent/opa/v1/plugins/discovery"
	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/filters/openpolicyagent/internal/envoy"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing/tracingtest"
	"google.golang.org/protobuf/encoding/protojson"
)

type MockOpenPolicyAgentFilter struct {
	opa *OpenPolicyAgentInstance
}

func (f *MockOpenPolicyAgentFilter) OpenPolicyAgent() *OpenPolicyAgentInstance {
	return f.opa
}

func (f *MockOpenPolicyAgentFilter) Request(filters.FilterContext) {}

func (f *MockOpenPolicyAgentFilter) Response(filters.FilterContext) {}

func TestInterpolateTemplate(t *testing.T) {
	os.Setenv("CONTROL_PLANE_TOKEN", "testtoken")

	template := &OpenPolicyAgentInstanceConfig{
		configTemplate: []byte(`
		token: {{.Env.CONTROL_PLANE_TOKEN }}
		bundle: {{ .bundlename }}
		`),
	}

	interpolatedConfig, err := template.interpolateConfigTemplate("helloBundle")

	assert.NoError(t, err)

	assert.Equal(t, `
		token: testtoken
		bundle: helloBundle
		`, string(interpolatedConfig))

}

func TestLoadEnvoyMetadata(t *testing.T) {
	cfg := &OpenPolicyAgentInstanceConfig{}
	_ = WithEnvoyMetadataBytes([]byte(`
	{
		"filter_metadata": {
			"envoy.filters.http.header_to_metadata": {
				"policy_type": "ingress"
			}
		}
	}
	`))(cfg)

	expectedBytes, err := protojson.Marshal(&ext_authz_v3_core.Metadata{
		FilterMetadata: map[string]*pbstruct.Struct{
			"envoy.filters.http.header_to_metadata": {
				Fields: map[string]*pbstruct.Value{
					"policy_type": {
						Kind: &pbstruct.Value_StringValue{StringValue: "ingress"},
					},
				},
			},
		},
	})

	assert.NoError(t, err)

	expected := &ext_authz_v3_core.Metadata{}
	err = protojson.Unmarshal(expectedBytes, expected)
	assert.NoError(t, err)

	assert.Equal(t, expected, cfg.envoyMetadata)
}

func mockControlPlaneWithDiscoveryBundle(discoveryBundle string) (*opasdktest.Server, []byte) {
	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle("/bundles/test", map[string]string{
			"main.rego": `
				package envoy.authz

				default allow = false
			`,
		}),
		opasdktest.MockBundle("/bundles/discovery", map[string]string{
			"data.json": `
				{"discovery":{"bundles":{"bundles/test":{"persist":false,"resource":"bundles/test","service":"test"}}}}
			`,
		}),
		opasdktest.MockBundle("/bundles/discovery-with-wrong-bundle", map[string]string{
			"data.json": `
				{"discovery":{"bundles":{"bundles/non-existing-bundle":{"persist":false,"resource":"bundles/non-existing-bundle","service":"test"}}}}
			`,
		}),
		opasdktest.MockBundle("/bundles/discovery-with-parsing-error", map[string]string{
			"data.json": `
				{unparsable : json}
			`,
		}),
	)

	config := []byte(fmt.Sprintf(`{
		"services": {
			"test": {
				"url": %q,
				"response_header_timeout_seconds": 1
			}
		},
		"labels": {
			"environment": "envValue"
		},
		"discovery": {
			"name": "discovery",
			"resource": %q,
			"service": "test"
		}
	}`, opaControlPlane.URL(), discoveryBundle))

	return opaControlPlane, config
}

type controlPlaneConfig struct {
	enableJwtCaching bool
}
type ControlPlaneOption func(*controlPlaneConfig)

func WithJwtCaching(enabled bool) ControlPlaneOption {
	return func(cfg *controlPlaneConfig) {
		cfg.enableJwtCaching = enabled
	}
}

func mockControlPlaneWithResourceBundle(opts ...ControlPlaneOption) (*opasdktest.Server, []byte) {
	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle("/bundles/test", map[string]string{
			"main.rego": `
				package envoy.authz

				default allow = false
			`,
		}),
		opasdktest.MockBundle("/bundles/use_body", map[string]string{
			"main.rego": `
				package envoy.authz
				
				import rego.v1

				default allow = false

				allow if { input.parsed_body }
			`,
		}),
		opasdktest.MockBundle("/bundles/anotherbundlename", map[string]string{
			"main.rego": `
				package envoy.authz

				default allow = false
			`,
		}),
	)

	cfg := &controlPlaneConfig{
		enableJwtCaching: false,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	jwtCacheConfig := ""
	if cfg.enableJwtCaching {
		jwtCacheConfig = `
			"caching": {
				"inter_query_builtin_value_cache": {
					"named": {
						"io_jwt": {
							"max_num_entries": 5
						}
					}
				}
			},
		`
	}

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
		%s
		"plugins": {
			"envoy_ext_authz_grpc": {
				"path": "envoy/authz/allow",
				"dry-run": false,
				"skip-request-body-parse": false
			}
		}
	}`, opaControlPlane.URL(), jwtCacheConfig))

	return opaControlPlane, config
}

func TestRegistry(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			enableCustomControlLoop: true,
			expectedTriggerMode:     plugins.TriggerManual,
			discoveryBundle:         "bundles/discovery",
		},
		{
			enableCustomControlLoop: false,
			expectedTriggerMode:     plugins.DefaultTriggerMode,
			discoveryBundle:         "bundles/discovery",
		},
		{
			enableCustomControlLoop: true,
			expectedTriggerMode:     plugins.TriggerManual,
			resourceBundle:          true,
		},
		{

			enableCustomControlLoop: false,
			expectedTriggerMode:     plugins.DefaultTriggerMode,
			resourceBundle:          true,
		},
	}
	runWithTestCases(t, testCases,
		func(t *testing.T, tc opaInstanceStartupTestCase) {
			var config []byte
			if tc.discoveryBundle != "" {
				_, config = mockControlPlaneWithDiscoveryBundle(tc.discoveryBundle)
			} else if tc.resourceBundle {
				_, config = mockControlPlaneWithResourceBundle()
			}

			registry, err := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithEnableCustomControlLoop(tc.enableCustomControlLoop), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
			assert.NoError(t, err)

			inst1, err := registry.GetOrStartInstance("test", "testfilter")
			assert.NoError(t, err)

			if tc.discoveryBundle != "" {
				assertTriggerMode(t, tc.expectedTriggerMode, inst1.manager.Plugin("discovery"))
			}
			assertTriggerMode(t, tc.expectedTriggerMode, inst1.manager.Plugin("bundle"))

			registry.markUnused(map[*OpenPolicyAgentInstance]struct{}{})

			inst2, err := registry.GetOrStartInstance("test", "testfilter")
			assert.NoError(t, err)
			assert.Equal(t, inst1, inst2, "same instance is reused after release")

			inst3, err := registry.GetOrStartInstance("test", "testfilter")
			assert.NoError(t, err)
			assert.Equal(t, inst2, inst3, "same instance is reused multiple times")

			registry.markUnused(map[*OpenPolicyAgentInstance]struct{}{})

			//Allow clean up
			time.Sleep(3 * time.Second)

			inst_different_bundle, err := registry.GetOrStartInstance("anotherbundlename", "testfilter")
			assert.NoError(t, err)

			inst4, err := registry.GetOrStartInstance("test", "testfilter")
			assert.NoError(t, err)
			assert.NotEqual(t, inst1, inst4, "after cleanup a new instance should be created")

			//Trigger cleanup via post processor
			registry.Do([]*routing.Route{
				{
					Filters: []*routing.RouteFilter{{Filter: &MockOpenPolicyAgentFilter{opa: inst_different_bundle}}},
				},
			})

			// Allow clean up
			time.Sleep(3 * time.Second)

			inst5, err := registry.GetOrStartInstance("test", "testfilter")
			assert.NoError(t, err)
			assert.NotEqual(t, inst4, inst5, "after cleanup a new instance should be created")

			registry.Close()

			_, err = registry.GetOrStartInstance("test", "testfilter")
			assert.Error(t, err, "should not work after close")
		})
}

func assertTriggerMode(t *testing.T, expectedMode plugins.TriggerMode, plgn plugins.Plugin) {
	if discoveryPlugin, ok := plgn.(*discovery.Discovery); ok {
		assert.Equal(t, expectedMode, *discoveryPlugin.TriggerMode())
	}
	if bundlePlugin, ok := plgn.(*bundle.Plugin); ok {
		for _, bundle := range bundlePlugin.Config().Bundles {
			assert.Equal(t, expectedMode, *bundle.Trigger)
		}
	}
}

func TestWithEnableDataPreProcessingOptimization(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{
			name:    "With pre processing optimization",
			enabled: true,
		},
		{
			name:    "With pre processing optimization disabled",
			enabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, config := mockControlPlaneWithResourceBundle()

			registry, err := NewOpenPolicyAgentRegistry(
				WithReuseDuration(1*time.Second),
				WithCleanInterval(1*time.Second),
				WithEnableDataPreProcessingOptimization(tt.enabled),
				WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)),
			)
			assert.NoError(t, err)

			inst1, err := registry.GetOrStartInstance("test", "testfilter")
			assert.NoError(t, err)

			assert.Equal(t, tt.enabled, registry.enableDataPreProcessingOptimization)
			assert.Equal(t, tt.enabled, inst1.registry.enableDataPreProcessingOptimization)
		})
	}
}

func TestWithJwtCacheConfig(t *testing.T) {
	_, config := mockControlPlaneWithResourceBundle(WithJwtCaching(true))

	registry, err := NewOpenPolicyAgentRegistry(
		WithReuseDuration(1*time.Second),
		WithCleanInterval(1*time.Second),
		WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)),
	)
	assert.NoError(t, err)

	inst1, err := registry.newOpenPolicyAgentInstance("test", "testfilter")
	assert.NoError(t, err)

	expectedJSON := []byte(`{
				"inter_query_builtin_value_cache": {
            		"named": {
                		"io_jwt": {
                    		"max_num_entries": 5
                		}
            		}
        		}
    		}`)

	var expected, actual map[string]interface{}
	err = json.Unmarshal(expectedJSON, &expected)
	if err != nil {
		panic(err)
	}
	assert.NoError(t, err, "unmarshal expected caching json")

	err = json.Unmarshal(inst1.manager.Config.Caching, &actual)
	assert.NoError(t, err, "unmarshal actual caching json")

	assert.Equal(t, expected, actual, "caching config should match expected value")

}

func TestOpaEngineStartFailure(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{enableCustomControlLoop: true, expectedError: "Bundle name: bundles/non-existing-bundle, Code: bundle_error, HTTPCode: 404, Message: server replied with Not Found"},
		{enableCustomControlLoop: false, expectedError: "one or more open policy agent plugins failed to start in 1s with error: timed out while starting: context deadline exceeded"},
	}
	runWithTestCases(t, testCases,
		func(t *testing.T, tc opaInstanceStartupTestCase) {
			_, config := mockControlPlaneWithDiscoveryBundle("bundles/discovery-with-wrong-bundle")

			registry, err := NewOpenPolicyAgentRegistry(WithInstanceStartupTimeout(1*time.Second), WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithEnableCustomControlLoop(tc.enableCustomControlLoop), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
			assert.NoError(t, err)

			engine, err := registry.new(inmem.New(), "testfilter", "test", DefaultMaxRequestBodySize, DefaultRequestBodyBufferSize)
			assert.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), registry.instanceStartupTimeout)
			defer cancel()

			if tc.enableCustomControlLoop {
				err = engine.StartAndTriggerPlugins(ctx)
			} else {
				err = engine.Start(ctx, registry.instanceStartupTimeout)
			}

			assert.True(t, engine.closing)
			assert.Contains(t, err.Error(), tc.expectedError)
		})
}

func TestControlLoopIntervalCalculation(t *testing.T) {
	registry, err := NewOpenPolicyAgentRegistry(WithControlLoopInterval(10*time.Second), WithControlLoopMaxJitter(0*time.Millisecond), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate([]byte(""))))
	require.NoError(t, err)

	interval := registry.controlLoopIntervalWithJitter()
	assert.Equal(t, 10*time.Second, interval)

	registry, err = NewOpenPolicyAgentRegistry(WithControlLoopInterval(10*time.Second), WithControlLoopMaxJitter(1000*time.Millisecond), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate([]byte(""))))
	require.NoError(t, err)

	interval = registry.controlLoopIntervalWithJitter()
	assert.NotEqual(t, 10*time.Second, interval)
	start := time.Now()
	assert.WithinDuration(t, start.Add(10*time.Second), start.Add(interval), 500*time.Millisecond)
}

func TestRetryableErrors(t *testing.T) {
	_, config := mockControlPlaneWithDiscoveryBundle("bundles/discovery")
	registry, err := NewOpenPolicyAgentRegistry(WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
	assert.NoError(t, err)

	instance, _ := registry.GetOrStartInstance("test", "testfilter")

	testCases := []struct {
		err       error
		retryable bool
	}{
		{download.HTTPError{StatusCode: 429}, true},
		{download.HTTPError{StatusCode: 500}, true},
		{download.HTTPError{StatusCode: 404}, false},
		{errors.New("some error"), false},
		{nil, false},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%v:%v", tc.err, tc.retryable), func(t *testing.T) {
			retryable := instance.isRetryable(tc.err)

			assert.Equal(t, tc.retryable, retryable)
		})
	}
}

func TestOpaActivationFailureWithRetry(t *testing.T) {
	slowResponse := 1005 * time.Millisecond
	testCases := []struct {
		status  int
		latency *time.Duration
		error   string
	}{
		{
			status:  503,
			latency: &slowResponse,
			error:   "context cancelled while triggering plugins: context deadline exceeded, last retry returned: request failed: Get \"%v/bundles/discovery\": net/http: timeout awaiting response headers",
		},
		{
			status: 429,
			error:  "context cancelled while triggering plugins: context deadline exceeded, last retry returned: server replied with Too Many Requests",
		},
		{
			status: 500,
			error:  "context cancelled while triggering plugins: context deadline exceeded, last retry returned: server replied with Internal Server Error",
		},
		{
			status: 404,
			error:  "server replied with Not Found",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("status=%v;added-latency:%v", tc.status, tc.latency), func(t *testing.T) {

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.latency != nil {
					time.Sleep(*tc.latency)
				}
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			config := []byte(fmt.Sprintf(`{
		"services": {
			"test": {
				"url": %q,
				"response_header_timeout_seconds": 1
			}
		},
		"labels": {
			"environment": "envValue"
		},
		"discovery": {
			"name": "discovery",
			"resource": %q,
			"service": "test"
		}
	}`, server.URL, "/bundles/discovery"))
			additionalWait := 0 * time.Millisecond
			if tc.latency != nil {
				additionalWait += 2 * *tc.latency
			}

			registry, err := NewOpenPolicyAgentRegistry(WithInstanceStartupTimeout(500*time.Millisecond+additionalWait), WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithEnableCustomControlLoop(true), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
			assert.NoError(t, err)

			instance, err := registry.GetOrStartInstance("test", "testfilter")
			assert.Nil(t, instance)

			if strings.Contains(tc.error, "%") {
				assert.Contains(t, err.Error(), fmt.Sprintf(tc.error, server.URL))
			} else {
				assert.Contains(t, err.Error(), tc.error)
			}
		})
	}
}

func TestOpaActivationSuccessWithDiscovery(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			enableCustomControlLoop: true,
			discoveryBundle:         "bundles/discovery",
		},
		{
			enableCustomControlLoop: false,
			discoveryBundle:         "bundles/discovery",
		},
	}
	runWithTestCases(t, testCases,
		func(t *testing.T, tc opaInstanceStartupTestCase) {
			_, config := mockControlPlaneWithDiscoveryBundle(tc.discoveryBundle)

			registry, err := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithEnableCustomControlLoop(tc.enableCustomControlLoop), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
			assert.NoError(t, err)

			instance, err := registry.GetOrStartInstance("test", "testfilter")
			assert.NotNil(t, instance)
			assert.NoError(t, err)
			assert.Equal(t, 1, len(registry.instances))
		})
}

func TestOpaLabelsSetInRuntimeWithDiscovery(t *testing.T) {
	_, config := mockControlPlaneWithDiscoveryBundle("bundles/discovery")

	registry, err := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
	assert.NoError(t, err)

	instance, err := registry.GetOrStartInstance("test", "testfilter")
	assert.NoError(t, err)
	assert.NotNil(t, instance)
	assert.NotNil(t, instance.Runtime())

	value := instance.Runtime().Value

	j, err := ast.JSON(value)
	assert.NoError(t, err)

	if m, ok := j.(map[string]interface{}); ok {
		configObject := m["config"]
		assert.NotNil(t, configObject)

		jsonData, err := json.Marshal(configObject)
		assert.NoError(t, err)

		var parsed *opaconf.Config
		json.Unmarshal(jsonData, &parsed)

		labels := parsed.Labels
		assert.Equal(t, labels["environment"], "envValue")
	} else {
		t.Fatalf("Failed to process runtime value %v", j)
	}
}

func TestOpaActivationFailureWithWrongServiceConfig(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			enableCustomControlLoop: true,
			expectedError:           "invalid configuration for discovery",
		},
		{
			enableCustomControlLoop: false,
			expectedError:           "invalid configuration for discovery",
		},
	}
	runWithTestCases(t, testCases, func(t *testing.T, tc opaInstanceStartupTestCase) {
		configWithUnknownService := []byte(`{
		"discovery": {
			"name": "discovery",
			"resource": "discovery",
			"service": "test"
		}}`)

		registry, err := NewOpenPolicyAgentRegistry(WithInstanceStartupTimeout(1*time.Second), WithCleanInterval(1*time.Second), WithEnableCustomControlLoop(tc.enableCustomControlLoop), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(configWithUnknownService)))
		assert.NoError(t, err)

		instance, err := registry.GetOrStartInstance("test", "testfilter")
		assert.Nil(t, instance)
		assert.Contains(t, err.Error(), tc.expectedError)
		assert.Equal(t, 0, registry.GetReadyInstanceCount())
		assert.Equal(t, 1, registry.GetFailedInstanceCount())
	})
}

func TestOpaActivationFailureWithDiscoveryPointingWrongBundle(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			enableCustomControlLoop: true,
			expectedError:           "Bundle name: bundles/non-existing-bundle, Code: bundle_error, HTTPCode: 404, Message: server replied with Not Found",
		},
		{
			enableCustomControlLoop: false,
			expectedError:           "one or more open policy agent plugins failed to start in 1s with error: timed out while starting: context deadline exceeded",
		},
	}
	runWithTestCases(t, testCases,
		func(t *testing.T, tc opaInstanceStartupTestCase) {
			_, config := mockControlPlaneWithDiscoveryBundle("/bundles/discovery-with-wrong-bundle")

			registry, err := NewOpenPolicyAgentRegistry(WithInstanceStartupTimeout(1*time.Second), WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithEnableCustomControlLoop(tc.enableCustomControlLoop), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
			assert.NoError(t, err)

			instance, err := registry.GetOrStartInstance("test", "testfilter")
			assert.Nil(t, instance)
			assert.Equal(t, 0, registry.GetReadyInstanceCount())
			assert.Equal(t, 1, registry.GetFailedInstanceCount())

			assert.Contains(t, err.Error(), tc.expectedError)

		})
}

func TestOpaActivationTimeOutWithDiscoveryParsingError(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			enableCustomControlLoop: true,
			discoveryBundle:         "/bundles/discovery-with-parsing-error",
			expectedError:           "context cancelled while triggering plugins: context deadline exceeded, last retry returned: server replied with Internal Server Error",
		},
		{
			enableCustomControlLoop: false,
			discoveryBundle:         "/bundles/discovery-with-parsing-error",
			expectedError:           "one or more open policy agent plugins failed to start in 1s with error: timed out while starting: context deadline exceeded",
		},
	}
	runWithTestCases(t, testCases,
		func(t *testing.T, tc opaInstanceStartupTestCase) {
			_, config := mockControlPlaneWithDiscoveryBundle(tc.discoveryBundle)

			registry, err := NewOpenPolicyAgentRegistry(WithInstanceStartupTimeout(1*time.Second), WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithEnableCustomControlLoop(tc.enableCustomControlLoop), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
			assert.NoError(t, err)

			instance, err := registry.GetOrStartInstance("test", "testfilter")
			assert.Nil(t, instance)
			assert.Contains(t, err.Error(), tc.expectedError)
			assert.Equal(t, 0, registry.GetReadyInstanceCount())
			assert.Equal(t, 1, registry.GetFailedInstanceCount())
		})
}

func TestStartup(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			enableCustomControlLoop: true,
			discoveryBundle:         "bundles/discovery",
		},
		{
			enableCustomControlLoop: false,
			discoveryBundle:         "bundles/discovery",
		},
		{
			enableCustomControlLoop: true,
			resourceBundle:          true,
		},
		{
			enableCustomControlLoop: false,
			resourceBundle:          true,
		},
	}
	runWithTestCases(t, testCases,
		func(t *testing.T, tc opaInstanceStartupTestCase) {
			var config []byte
			if tc.discoveryBundle != "" {
				_, config = mockControlPlaneWithDiscoveryBundle(tc.discoveryBundle)
			} else if tc.resourceBundle {
				_, config = mockControlPlaneWithResourceBundle()
			}

			registry, err := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithEnableCustomControlLoop(tc.enableCustomControlLoop), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
			assert.NoError(t, err)

			inst1, err := registry.GetOrStartInstance("test", "testfilter")
			assert.NoError(t, err)

			target := envoy.PluginConfig{Path: "envoy/authz/allow", DryRun: false}
			target.ParseQuery()
			assert.Equal(t, target, inst1.EnvoyPluginConfig())
		})
}

func TestTracing(t *testing.T) {
	_, config := mockControlPlaneWithResourceBundle()

	registry, err := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
	assert.NoError(t, err)

	inst, err := registry.GetOrStartInstance("test", "testfilter")
	assert.NoError(t, err)

	tracer := tracingtest.NewTracer()
	parent := tracer.StartSpan("start_span")
	ctx := opentracing.ContextWithSpan(context.Background(), parent)
	span, _ := inst.StartSpanFromContext(ctx)
	span.Finish()
	parent.Finish()

	recspan := tracer.FindSpan("open-policy-agent")
	require.NotNil(t, recspan, "No span was created for open policy agent")
	assert.Equal(t, map[string]interface{}{"opa.bundle_name": "test", "opa.label.id": inst.manager.Labels()["id"], "opa.label.version": inst.manager.Labels()["version"]}, recspan.Tags())
}

func TestEval(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			enableCustomControlLoop: true,
			discoveryBundle:         "bundles/discovery",
		},
		{
			enableCustomControlLoop: false,
			discoveryBundle:         "bundles/discovery",
		},
		{
			enableCustomControlLoop: true,
			resourceBundle:          true,
		},
		{
			enableCustomControlLoop: false,
			resourceBundle:          true,
		},
	}
	runWithTestCases(t, testCases,
		func(t *testing.T, tc opaInstanceStartupTestCase) {
			var config []byte
			if tc.discoveryBundle != "" {
				_, config = mockControlPlaneWithDiscoveryBundle(tc.discoveryBundle)
			} else if tc.resourceBundle {
				_, config = mockControlPlaneWithResourceBundle()
			}

			registry, err := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithEnableCustomControlLoop(tc.enableCustomControlLoop), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
			assert.NoError(t, err)

			inst, err := registry.GetOrStartInstance("test", "testfilter")
			assert.NoError(t, err)

			tracer := tracingtest.NewTracer()
			span := tracer.StartSpan("open-policy-agent")
			ctx := opentracing.ContextWithSpan(context.Background(), span)

			result, err := inst.Eval(ctx, &authv3.CheckRequest{
				Attributes: &authv3.AttributeContext{
					Request:           nil,
					ContextExtensions: nil,
					MetadataContext:   nil,
				},
			})
			assert.NoError(t, err)

			allowed, err := result.IsAllowed()
			assert.NoError(t, err)
			assert.False(t, allowed)

			span.Finish()
			testspan := tracer.FindSpan("open-policy-agent")
			require.NotNil(t, testspan)
			assert.Equal(t, result.DecisionID, testspan.Tag("opa.decision_id"))
		})
}

func TestResponses(t *testing.T) {
	_, config := mockControlPlaneWithResourceBundle()

	registry, err := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
	assert.NoError(t, err)

	inst, err := registry.GetOrStartInstance("test", "testfilter")
	assert.NoError(t, err)

	tracer := tracingtest.NewTracer()
	span := tracer.StartSpan("open-policy-agent")
	metrics := &metricstest.MockMetrics{}

	fc := &filtertest.Context{FMetrics: metrics}

	inst.ServeInvalidDecisionError(fc, span, &envoyauth.EvalResult{}, fmt.Errorf("something happened"))
	assert.True(t, fc.FServed)
	assert.Equal(t, fc.FResponse.StatusCode, http.StatusInternalServerError)
	metrics.WithCounters(func(counters map[string]int64) {
		assert.Equal(t, int64(1), counters["decision.err.test"])
	})
	span.Finish()
	testspan := tracer.FindSpan("open-policy-agent")
	require.NotNil(t, testspan, "span not found")
	assert.Equal(t, true, testspan.Tag("error"))

	fc = &filtertest.Context{FMetrics: metrics}
	inst.ServeInvalidDecisionError(fc, span, nil, fmt.Errorf("something happened"))
	assert.True(t, fc.FServed)
	assert.Equal(t, fc.FResponse.StatusCode, http.StatusInternalServerError)
	metrics.WithCounters(func(counters map[string]int64) {
		assert.Equal(t, int64(2), counters["decision.err.test"])
	})

	fc = &filtertest.Context{FMetrics: metrics}
	inst.ServeResponse(fc, span, &envoyauth.EvalResult{
		Decision: map[string]interface{}{
			"http_status": json.Number(strconv.Itoa(http.StatusOK)),
			"headers": map[string]interface{}{
				"someheader": "somevalue",
			},
			"body": "Welcome!",
		},
	})
	assert.True(t, fc.FServed)
	assert.Equal(t, fc.FResponse.StatusCode, http.StatusOK)
	assert.Equal(t, fc.FResponse.Header, http.Header{
		"Someheader": {"somevalue"},
	})
	body, err := io.ReadAll(fc.FResponse.Body)
	assert.NoError(t, err)
	assert.Equal(t, string(body), "Welcome!")

	fc = &filtertest.Context{FMetrics: metrics}
	inst.ServeResponse(fc, span, &envoyauth.EvalResult{
		Decision: map[string]interface{}{
			"headers": "invalid header type",
			"body":    "Welcome!",
		},
	})
	assert.True(t, fc.FServed)
	assert.Equal(t, fc.FResponse.StatusCode, http.StatusInternalServerError)

	fc = &filtertest.Context{FMetrics: metrics}
	inst.ServeResponse(fc, span, &envoyauth.EvalResult{
		Decision: map[string]interface{}{
			"body": map[string]interface{}{
				"invalid": "body type",
			},
		},
	})
	assert.True(t, fc.FServed)
	assert.Equal(t, fc.FResponse.StatusCode, http.StatusInternalServerError)

	fc = &filtertest.Context{FMetrics: metrics}
	inst.ServeResponse(fc, span, &envoyauth.EvalResult{
		Decision: map[string]interface{}{
			"http_status": "invalid status code",
		},
	})
	assert.True(t, fc.FServed)
	assert.Equal(t, fc.FResponse.StatusCode, http.StatusInternalServerError)
}

func TestBodyExtraction(t *testing.T) {

	_, config := mockControlPlaneWithResourceBundle()

	for _, ti := range []struct {
		msg            string
		body           string
		contentLength  int64
		maxBodySize    int64
		readBodyBuffer int64

		bodyInPolicy string
	}{
		{
			msg:            "Read body ",
			body:           `{ "welcome": "world" }`,
			maxBodySize:    1024,
			readBodyBuffer: DefaultRequestBodyBufferSize,
			bodyInPolicy:   `{ "welcome": "world" }`,
		},
		{
			msg:            "Read body in chunks",
			body:           `{ "welcome": "world" }`,
			maxBodySize:    1024,
			readBodyBuffer: 5,
			bodyInPolicy:   `{ "welcome": "world" }`,
		},
		{
			msg:            "Read body with client sending more data than expected",
			body:           `{ "welcome": "world" }`,
			maxBodySize:    1024,
			readBodyBuffer: 5,
			contentLength:  5,
			bodyInPolicy:   `{ "we`,
		},
		{
			msg:            "Read body exhausing max bytes",
			body:           `{ "welcome": "world" }`,
			maxBodySize:    5,
			readBodyBuffer: 5,
			bodyInPolicy:   ``,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			t.Logf("Running test for %v", ti)

			registry, err := NewOpenPolicyAgentRegistry(WithMaxRequestBodyBytes(ti.maxBodySize),
				WithReadBodyBufferSize(ti.readBodyBuffer), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
			assert.NoError(t, err)

			inst, err := registry.GetOrStartInstance("use_body", "testfilter")
			assert.NoError(t, err)

			contentLength := ti.contentLength
			if contentLength == 0 {
				contentLength = int64(len(ti.body))
			}

			req := http.Request{
				ContentLength: contentLength,
				Body:          io.NopCloser(bytes.NewReader([]byte(ti.body))),
			}

			body, peekBody, finalizer, err := inst.ExtractHttpBodyOptionally(&req)
			defer finalizer()
			assert.NoError(t, err)
			defer body.Close()

			fullBody, err := io.ReadAll(body)
			assert.NoError(t, err)
			assert.Equal(t, ti.body, string(fullBody), "Full body must be readable")

			assert.Equal(t, ti.bodyInPolicy, string(peekBody), "Body has been read up till maximum")
		})
	}
}

func TestBodyExtractionExhausingTotalBytes(t *testing.T) {

	_, config := mockControlPlaneWithResourceBundle()

	registry, err := NewOpenPolicyAgentRegistry(WithMaxRequestBodyBytes(21),
		WithReadBodyBufferSize(21),
		WithMaxMemoryBodyParsing(40), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
	assert.NoError(t, err)

	inst, err := registry.GetOrStartInstance("use_body", "testfilter")
	assert.NoError(t, err)

	req1 := http.Request{
		ContentLength: 21,
		Body:          io.NopCloser(bytes.NewReader([]byte(`{ "welcome": "world" }`))),
	}

	_, _, f1, err := inst.ExtractHttpBodyOptionally(&req1)
	assert.NoError(t, err)

	req2 := http.Request{
		ContentLength: 21,
		Body:          io.NopCloser(bytes.NewReader([]byte(`{ "welcome": "world" }`))),
	}

	_, _, f2, err := inst.ExtractHttpBodyOptionally(&req2)
	if assert.Error(t, err) {
		assert.Equal(t, ErrTotalBodyBytesExceeded, err)
	}

	f1()
	f2()

	req3 := http.Request{
		ContentLength: 21,
		Body:          io.NopCloser(bytes.NewReader([]byte(`{ "welcome": "world" }`))),
	}

	_, _, f3, err := inst.ExtractHttpBodyOptionally(&req3)
	f3()
	assert.NoError(t, err)
}

func TestBodyExtractionEmptyBody(t *testing.T) {

	_, config := mockControlPlaneWithResourceBundle()

	registry, err := NewOpenPolicyAgentRegistry(WithMaxRequestBodyBytes(21),
		WithReadBodyBufferSize(21),
		WithMaxMemoryBodyParsing(40), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
	assert.NoError(t, err)

	inst, err := registry.GetOrStartInstance("use_body", "testfilter")
	assert.NoError(t, err)

	req1 := http.Request{
		ContentLength: 0,
		Body:          nil,
	}

	body, bodybytes, f1, err := inst.ExtractHttpBodyOptionally(&req1)
	assert.NoError(t, err)
	assert.Nil(t, body)
	assert.Nil(t, bodybytes)

	f1()
}

func TestBodyExtractionUnknownBody(t *testing.T) {

	_, config := mockControlPlaneWithResourceBundle()

	registry, err := NewOpenPolicyAgentRegistry(WithMaxRequestBodyBytes(21),
		WithReadBodyBufferSize(21),
		WithMaxMemoryBodyParsing(21), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)))
	assert.NoError(t, err)

	inst, err := registry.GetOrStartInstance("use_body", "testfilter")
	assert.NoError(t, err)

	req1 := http.Request{
		ContentLength: -1,
		Body:          io.NopCloser(bytes.NewReader([]byte(`{ "welcome": "world" }`))),
	}

	_, _, f1, err := inst.ExtractHttpBodyOptionally(&req1)
	assert.NoError(t, err)

	req2 := http.Request{
		ContentLength: 3,
		Body:          io.NopCloser(bytes.NewReader([]byte(`{ }`))),
	}

	_, _, f2, err := inst.ExtractHttpBodyOptionally(&req2)
	if assert.Error(t, err) {
		assert.Equal(t, ErrTotalBodyBytesExceeded, err)
	}

	f1()
	f2()
}

/// ------------------ Singleflight Instance Creation tests ------------------

// / Verifies singleflight instance creation behavior when instance creation fails due to timeout
// / and ensures that the following instance creation attempts for the same bundle are allowed and succeed.
func TestSingleflightInstanceCreationForgetErrorAtTimeout(t *testing.T) {
	bundleName := "test_error_forget_bundle"
	cbs := opatestutils.StartControllableBundleServer(bundleName)
	defer cbs.Stop()

	registry := CreateOPARegistry(t, cbs.URL(), bundleName)
	defer registry.Close()

	// Test: PrepareInstanceLoader attempt OPA instance creation.
	loader := registry.PrepareInstanceLoader(bundleName, "opaRequestFilter")
	_, err := loader()

	assert.Error(t, err, "should timeout as bundle server is not available yet")

	// Bundle server recovers
	cbs.SetAvailable(true)

	// Verify instance creation is attempted for the same bundle again and succeeds
	secondLoader := registry.PrepareInstanceLoader(bundleName, "opaRequestFilter")
	instance, err := secondLoader()

	require.NoError(t, err)
	assert.NotNil(t, instance, "instance should be created successfully after bundle server recovers")
	assert.Equal(t, instance.bundleName, bundleName)
}

// / Verifies that concurrent requests for the same bundle result in only one instance creation
func TestSingleflightConcurrentRequests(t *testing.T) {
	bundleName := "test_concurrent_bundle"
	cbs := opatestutils.CreateBundleServers([]string{bundleName})[0]
	defer cbs.Stop()

	registry := CreateOPARegistry(t, cbs.URL(), bundleName)
	defer registry.Close()

	// Record baseline goroutine count for leak detection
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	// Start multiple concurrent requests for the same bundle
	const numRequests = 20
	results := make(chan *OpenPolicyAgentInstance, numRequests)
	errors := make(chan error, numRequests)
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			loader := registry.PrepareInstanceLoader(bundleName, fmt.Sprintf("filter-%d", id))
			instance, err := loader()

			if err != nil {
				errors <- err
			} else {
				results <- instance
			}
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Collect all results
	var instances []*OpenPolicyAgentInstance
	var errs []error

	for instance := range results {
		instances = append(instances, instance)
	}
	for err := range errors {
		errs = append(errs, err)
	}

	// All requests should succeed
	assert.Empty(t, errs, "All concurrent requests should succeed")
	assert.Len(t, instances, numRequests, "Should have results from all requests")

	// All instances should be the same (singleflight ensures only one creation)
	for i := 1; i < len(instances); i++ {
		assert.Equal(t, instances[0], instances[i],
			"All concurrent requests should get the same instance")
	}

	assert.Equal(t, registry.instances[bundleName].instance, instances[0],
		"Instance should be stored in registry")
	assert.Len(t, registry.instances, 1,
		"Only one instance should be created despite concurrent requests")

	// Verify no goroutine leaks
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()
	goroutineDiff := finalGoroutines - initialGoroutines

	assert.LessOrEqual(t, goroutineDiff, 5,
		"Should not have significant goroutine increase (was %d, now %d, diff %d)",
		initialGoroutines, finalGoroutines, goroutineDiff)
}

func TestPrepareInstanceLoader_MultipleBundles(t *testing.T) {
	bundles := []string{"bundle1", "bundle2", "bundle3"}

	// Create bundle servers with proper root configurations
	servers := opatestutils.CreateBundleServers(bundles)
	defer func() {
		for _, server := range servers {
			server.Stop()
		}
	}()

	// Create OPA registry with multi-bundle configuration
	config := opatestutils.CreateMultiBundleConfig(servers)
	registry := CreateRegistryWithConfig(t, config)
	defer registry.Close()

	// Create instances for all bundles concurrently
	instances := CreateInstancesConcurrently(t, registry, bundles)

	// Verify all instances are different and properly stored
	assert.Len(t, instances, len(bundles))
	assert.Len(t, registry.instances, len(bundles))

	for _, bundle := range bundles {
		assert.NotNil(t, registry.instances[bundle])
	}
}

// Verifies that if instance creation fails, the singleflight key is forgotten allowing reattempts
func TestPrepareInstanceLoader_SingleflightCleanupAfterError(t *testing.T) {
	bundleName := "error_cleanup_bundle"

	cbs := opatestutils.StartControllableBundleServer(bundleName)
	defer cbs.Stop()

	registry := CreateOPARegistry(t, cbs.URL(), bundleName)
	defer registry.Close()

	// First attempt should fail and forget singleflight key
	loader1 := registry.PrepareInstanceLoader(bundleName, "opaRequestFilter")
	_, err := loader1()
	assert.Error(t, err)

	// Make bundle server available
	cbs.SetAvailable(true)

	// Second attempt should succeed (not blocked by previous failure)
	loader2 := registry.PrepareInstanceLoader(bundleName, "opaRequestFilter")
	instance, err := loader2()
	assert.NoError(t, err)
	assert.NotNil(t, instance)
	assert.Equal(t, instance.bundleName, bundleName)
}

// / Verifies that after instances are cleaned up, singleflight keys are forgotten
func TestSingleflightForgetOnCleanup(t *testing.T) {
	bundleName := "test_cleanup_bundle"
	cbs := opatestutils.CreateBundleServers([]string{bundleName})[0]
	defer cbs.Stop()

	// Create registry with a very short cleanup interval
	registry := CreateOPARegistry(t, cbs.URL(), bundleName, 1*time.Second)
	defer registry.Close()

	// Create an instance
	loader := registry.PrepareInstanceLoader(bundleName, "opaRequestFilter")
	instance, err := loader()
	require.NoError(t, err)
	require.NotNil(t, registry.instances[bundleName], "should have one instance in registry")

	// Mark all instances as unused to trigger cleanup
	registry.markUnused(map[*OpenPolicyAgentInstance]struct{}{})
	registry.cleanUnusedInstances(time.Now().Add(-1 * time.Second))

	// Wait longer than the cleanup interval
	time.Sleep(2 * time.Second)
	registry.mu.Lock()
	assert.Len(t, registry.instances, 0, "all instances should be cleaned up")
	registry.mu.Unlock()

	// After cleanup, singleflight should have forgotten the key
	// and new instance creation should work (not be blocked by old singleflight)
	newLoader := registry.PrepareInstanceLoader(bundleName, "opaRequestFilter")
	newInstance, err := newLoader()
	require.NoError(t, err)
	require.NotNil(t, newInstance)

	// Should be a different instance after cleanup
	assert.NotEqual(t, instance, newInstance, "should create new instance after cleanup")
	registry.mu.Lock()
	assert.NotNil(t, registry.instances[bundleName], "newly created instance should be in registry")
	registry.mu.Unlock()
}

// Verifies that instance creation respects the coordination timeout and do not hang indefinitely
func TestPrepareInstanceLoader_CoordinationTimeout(t *testing.T) {
	bundleName := "timeout_test_bundle"

	// Create a server that never responds to simulate hanging creation
	hangingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // Hang longer than coordination timeout
	}))
	defer hangingServer.Close()

	cleanupInterval := 1 * time.Second
	startUpTimeOut := 2 * time.Second
	registry := CreateOPARegistry(t, hangingServer.URL, bundleName, cleanupInterval, startUpTimeOut)
	defer registry.Close()

	loader := registry.PrepareInstanceLoader(bundleName, "opaRequestFilter")

	start := time.Now()
	instance, err := loader()
	elapsed := time.Since(start)

	// Should timeout less than coordination timeout (3x startup timeout = 6s)
	assert.Error(t, err)
	assert.Nil(t, instance)
	assert.Contains(t, err.Error(), "timed out while starting")
	assert.Greater(t, elapsed, startUpTimeOut)
	assert.Less(t, elapsed, 3*startUpTimeOut, "should timeout less than coordination timeout")
}

// / Verifies that after the registry is closed, all singleflight keys are forgotten
func TestRegistryClose(t *testing.T) {
	bundleName := "test_close_bundle"
	cbs := opatestutils.CreateBundleServers([]string{bundleName})[0]
	defer cbs.Stop()

	registry := CreateOPARegistry(t, cbs.URL(), bundleName)

	loader := registry.PrepareInstanceLoader(bundleName, "opaRequestFilter")
	loader()

	// Close registry - should forget all singleflight keys
	registry.Close()

	// After close, new instance creation should fail
	newLoader := registry.PrepareInstanceLoader(bundleName, "opaRequestFilter")
	_, err := newLoader()
	assert.Error(t, err, "should fail after registry is closed")
	assert.Equal(t, err.Error(), "failed to get existing OPA instance for bundle \"test_close_bundle\": open policy agent registry is already closed")

	// Create new registry - should allow new instance creation
	registry2 := CreateOPARegistry(t, cbs.URL(), bundleName)
	loader2 := registry2.PrepareInstanceLoader(bundleName, "opaRequestFilter")
	instance, err := loader2()
	assert.NoError(t, err)
	assert.NotNil(t, instance, "should create new instance after registry is recreated")

}

type opaInstanceStartupTestCase struct {
	enableCustomControlLoop bool
	expectedError           string
	expectedTriggerMode     plugins.TriggerMode
	discoveryBundle         string
	resourceBundle          bool
}

func runWithTestCases(t *testing.T, cases []opaInstanceStartupTestCase, test func(t *testing.T, tc opaInstanceStartupTestCase)) {
	for _, tc := range cases {
		sb := strings.Builder{}
		sb.WriteString(fmt.Sprintf("custom-control-loop=%v", tc.enableCustomControlLoop))
		if tc.discoveryBundle != "" {
			sb.WriteString(fmt.Sprintf(";discovery=%v", tc.discoveryBundle))
		}
		if tc.resourceBundle {
			sb.WriteString(";resource-bundle")
		}
		t.Run(sb.String(), func(t *testing.T) {
			test(t, tc)
		})
	}
}

// CreateRegistryWithConfig Helper function to create registry with configuration
func CreateRegistryWithConfig(t *testing.T, config []byte) *OpenPolicyAgentRegistry {
	registry, err := NewOpenPolicyAgentRegistry(
		WithReuseDuration(1*time.Second),
		WithCleanInterval(1*time.Second),
		WithOpenPolicyAgentInstanceConfig(WithConfigTemplate(config)),
	)
	require.NoError(t, err)
	return registry
}

// CreateInstancesConcurrently Helper function to create instances concurrently
func CreateInstancesConcurrently(t *testing.T, registry *OpenPolicyAgentRegistry, bundles []string) map[string]*OpenPolicyAgentInstance {
	var wg sync.WaitGroup
	instances := make(map[string]*OpenPolicyAgentInstance)
	var mu sync.Mutex

	for _, bundle := range bundles {
		wg.Add(1)
		go func(bundleName string) {
			defer wg.Done()
			loader := registry.PrepareInstanceLoader(bundleName, "opaRequestFilter")
			instance, err := loader()
			require.NoError(t, err)

			mu.Lock()
			instances[bundleName] = instance
			mu.Unlock()
		}(bundle)
	}

	wg.Wait()
	return instances
}

func CreateOPARegistry(t *testing.T, url, bundleName string, options ...time.Duration) *OpenPolicyAgentRegistry {
	t.Helper()

	cleanUpInterval := 1 * time.Second // default value
	startupTimeout := 1 * time.Second  // default value
	if len(options) > 0 {
		cleanUpInterval = options[0]
	}

	if len(options) > 1 {
		startupTimeout = options[1]
	}

	config := []byte(fmt.Sprintf(`{
		"services": {
			"test": { "url": %q }
		},
		"bundles": {
			"%s": { "resource": "/bundles/{{ .bundlename }}" }
		}
	}`, url, bundleName))

	opaRegistry, err := NewOpenPolicyAgentRegistry(
		WithTracer(tracingtest.NewTracer()),
		WithPreloadingEnabled(true),
		WithOpenPolicyAgentInstanceConfig(
			WithConfigTemplate(config),
		),
		WithInstanceStartupTimeout(startupTimeout),
		WithCleanInterval(cleanUpInterval),
		WithReuseDuration(1*time.Second))
	require.NoError(t, err)
	return opaRegistry
}
