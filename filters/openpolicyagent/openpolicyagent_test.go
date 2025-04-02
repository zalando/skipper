package openpolicyagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	interpolatedConfig, err := interpolateConfigTemplate([]byte(`
		token: {{.Env.CONTROL_PLANE_TOKEN }}
		bundle: {{ .bundlename }}
		`),
		"helloBundle")

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
				"url": %q
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

func mockControlPlaneWithResourceBundle() (*opasdktest.Server, []byte) {
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
				"path": "envoy/authz/allow",
				"dry-run": false,
				"skip-request-body-parse": false
			}
		}
	}`, opaControlPlane.URL()))

	return opaControlPlane, config
}

func TestRegistry(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			overridePeriodicTriggers: true,
			expectedTriggerMode:      plugins.TriggerManual,
			discoveryBundle:          "bundles/discovery",
		},
		{
			overridePeriodicTriggers: false,
			expectedTriggerMode:      plugins.DefaultTriggerMode,
			discoveryBundle:          "bundles/discovery",
		},
		{
			overridePeriodicTriggers: true,
			expectedTriggerMode:      plugins.TriggerManual,
			resourceBundle:           true,
		},
		{

			overridePeriodicTriggers: false,
			expectedTriggerMode:      plugins.DefaultTriggerMode,
			resourceBundle:           true,
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

			registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithOverridePeriodPluginTriggers(tc.overridePeriodicTriggers))

			cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
			assert.NoError(t, err)

			inst1, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
			assert.NoError(t, err)

			if tc.discoveryBundle != "" {
				assertTriggerMode(t, tc.expectedTriggerMode, inst1.manager.Plugin("discovery"))
			}
			assertTriggerMode(t, tc.expectedTriggerMode, inst1.manager.Plugin("bundle"))

			registry.markUnused(map[*OpenPolicyAgentInstance]struct{}{})

			inst2, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
			assert.NoError(t, err)
			assert.Equal(t, inst1, inst2, "same instance is reused after release")

			inst3, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
			assert.NoError(t, err)
			assert.Equal(t, inst2, inst3, "same instance is reused multiple times")

			registry.markUnused(map[*OpenPolicyAgentInstance]struct{}{})

			//Allow clean up
			time.Sleep(3 * time.Second)

			inst_different_bundle, err := registry.NewOpenPolicyAgentInstance("anotherbundlename", *cfg, "testfilter")
			assert.NoError(t, err)

			inst4, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
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

			inst5, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
			assert.NoError(t, err)
			assert.NotEqual(t, inst4, inst5, "after cleanup a new instance should be created")

			registry.Close()

			_, err = registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
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

func TestOpaEngineStartFailure(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{overridePeriodicTriggers: true, expectedError: "Bundle name: bundles/non-existing-bundle, Code: bundle_error, HTTPCode: 404, Message: server replied with Not Found"},
		{overridePeriodicTriggers: false, expectedError: "one or more open policy agent plugins failed to start in 1s with error: timed out while starting: context deadline exceeded"},
	}
	runWithTestCases(t, testCases,
		func(t *testing.T, tc opaInstanceStartupTestCase) {
			_, config := mockControlPlaneWithDiscoveryBundle("bundles/discovery-with-wrong-bundle")

			registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithOverridePeriodPluginTriggers(tc.overridePeriodicTriggers))

			cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config), WithStartupTimeout(1*time.Second))
			assert.NoError(t, err)

			engine, err := registry.new(inmem.New(), config, *cfg, "testfilter", "test", DefaultMaxRequestBodySize, DefaultRequestBodyBufferSize)
			assert.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), cfg.startupTimeout)
			defer cancel()

			if tc.overridePeriodicTriggers {
				err = engine.StartAndTriggerPlugins(ctx)
			} else {
				err = engine.Start(ctx, cfg.startupTimeout)
			}

			assert.True(t, engine.stopped)
			assert.Contains(t, err.Error(), tc.expectedError)
		})
}

func TestPluginTriggerIntervalCalculation(t *testing.T) {
	interval := pluginTriggerIntervalWithJitter(10*time.Second, 0*time.Millisecond)
	assert.Equal(t, 10*time.Second, interval)

	interval = pluginTriggerIntervalWithJitter(10*time.Second, 1000*time.Millisecond)
	assert.NotEqual(t, 10*time.Second, interval)

	start := time.Now()
	assert.WithinDuration(t, start.Add(10*time.Second), start.Add(interval), 500*time.Millisecond)
}

func TestBundleDownloadRetry(t *testing.T) {
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
			httpError := download.HTTPError{}
			retryable := asRetryableHttpError(tc.err, &httpError)

			assert.Equal(t, tc.retryable, retryable)
		})
	}
}

func TestOpaActivationSuccessWithDiscovery(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			overridePeriodicTriggers: true,
			discoveryBundle:          "bundles/discovery",
		},
		{
			overridePeriodicTriggers: false,
			discoveryBundle:          "bundles/discovery",
		},
	}
	runWithTestCases(t, testCases,
		func(t *testing.T, tc opaInstanceStartupTestCase) {
			_, config := mockControlPlaneWithDiscoveryBundle(tc.discoveryBundle)

			registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithOverridePeriodPluginTriggers(tc.overridePeriodicTriggers))

			cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
			assert.NoError(t, err)

			instance, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
			assert.NotNil(t, instance)
			assert.NoError(t, err)
			assert.Equal(t, 1, len(registry.instances))
		})
}

func TestOpaLabelsSetInRuntimeWithDiscovery(t *testing.T) {
	_, config := mockControlPlaneWithDiscoveryBundle("bundles/discovery")

	registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	instance, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
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
			overridePeriodicTriggers: true,
			expectedError:            "invalid configuration for discovery",
		},
		{
			overridePeriodicTriggers: false,
			expectedError:            "invalid configuration for discovery",
		},
	}
	runWithTestCases(t, testCases, func(t *testing.T, tc opaInstanceStartupTestCase) {
		configWithUnknownService := []byte(`{
		"discovery": {
			"name": "discovery",
			"resource": "discovery",
			"service": "test"
		}}`)

		registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithOverridePeriodPluginTriggers(tc.overridePeriodicTriggers))

		cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(configWithUnknownService), WithStartupTimeout(1*time.Second))
		assert.NoError(t, err)

		instance, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
		assert.Nil(t, instance)
		assert.Contains(t, err.Error(), tc.expectedError)
		assert.Equal(t, 0, len(registry.instances))
	})
}

func TestOpaActivationFailureWithDiscoveryPointingWrongBundle(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			overridePeriodicTriggers: true,
			expectedError:            "Bundle name: bundles/non-existing-bundle, Code: bundle_error, HTTPCode: 404, Message: server replied with Not Found",
		},
		{
			overridePeriodicTriggers: false,
			expectedError:            "one or more open policy agent plugins failed to start in 1s with error: timed out while starting: context deadline exceeded",
		},
	}
	runWithTestCases(t, testCases,
		func(t *testing.T, tc opaInstanceStartupTestCase) {
			_, config := mockControlPlaneWithDiscoveryBundle("/bundles/discovery-with-wrong-bundle")

			registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithOverridePeriodPluginTriggers(tc.overridePeriodicTriggers))

			cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config), WithStartupTimeout(1*time.Second))
			assert.NoError(t, err)

			instance, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
			assert.Nil(t, instance)
			assert.Equal(t, 0, len(registry.instances))

			assert.Contains(t, err.Error(), tc.expectedError)

		})
}

func TestOpaActivationTimeOutWithDiscoveryParsingError(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			overridePeriodicTriggers: true,
			discoveryBundle:          "/bundles/discovery-with-parsing-error",
			expectedError:            "context cancelled while triggering plugins: context deadline exceeded, last retry returned: server replied with Internal Server Error",
		},
		{
			overridePeriodicTriggers: false,
			discoveryBundle:          "/bundles/discovery-with-parsing-error",
			expectedError:            "one or more open policy agent plugins failed to start in 1s with error: timed out while starting: context deadline exceeded",
		},
	}
	runWithTestCases(t, testCases,
		func(t *testing.T, tc opaInstanceStartupTestCase) {
			_, config := mockControlPlaneWithDiscoveryBundle(tc.discoveryBundle)

			registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithOverridePeriodPluginTriggers(tc.overridePeriodicTriggers))

			cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config), WithStartupTimeout(1*time.Second))
			assert.NoError(t, err)

			instance, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
			assert.Nil(t, instance)
			assert.Contains(t, err.Error(), tc.expectedError)
			assert.Equal(t, 0, len(registry.instances))
		})
}

func TestStartup(t *testing.T) {
	testCases := []opaInstanceStartupTestCase{
		{
			overridePeriodicTriggers: true,
			discoveryBundle:          "bundles/discovery",
		},
		{
			overridePeriodicTriggers: false,
			discoveryBundle:          "bundles/discovery",
		},
		{
			overridePeriodicTriggers: true,
			resourceBundle:           true,
		},
		{
			overridePeriodicTriggers: false,
			resourceBundle:           true,
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

			registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithOverridePeriodPluginTriggers(tc.overridePeriodicTriggers))

			cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
			assert.NoError(t, err)

			inst1, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
			assert.NoError(t, err)

			target := envoy.PluginConfig{Path: "envoy/authz/allow", DryRun: false}
			target.ParseQuery()
			assert.Equal(t, target, inst1.EnvoyPluginConfig())
		})
}

func TestTracing(t *testing.T) {
	_, config := mockControlPlaneWithResourceBundle()

	registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	inst, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
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
			overridePeriodicTriggers: true,
			discoveryBundle:          "bundles/discovery",
		},
		{
			overridePeriodicTriggers: false,
			discoveryBundle:          "bundles/discovery",
		},
		{
			overridePeriodicTriggers: true,
			resourceBundle:           true,
		},
		{
			overridePeriodicTriggers: false,
			resourceBundle:           true,
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

			registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second), WithOverridePeriodPluginTriggers(tc.overridePeriodicTriggers))

			cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
			assert.NoError(t, err)

			inst, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
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

	registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	inst, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
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

			registry := NewOpenPolicyAgentRegistry(WithMaxRequestBodyBytes(ti.maxBodySize),
				WithReadBodyBufferSize(ti.readBodyBuffer))

			cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
			assert.NoError(t, err)

			inst, err := registry.NewOpenPolicyAgentInstance("use_body", *cfg, "testfilter")
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

	registry := NewOpenPolicyAgentRegistry(WithMaxRequestBodyBytes(21),
		WithReadBodyBufferSize(21),
		WithMaxMemoryBodyParsing(40))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	inst, err := registry.NewOpenPolicyAgentInstance("use_body", *cfg, "testfilter")
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

	registry := NewOpenPolicyAgentRegistry(WithMaxRequestBodyBytes(21),
		WithReadBodyBufferSize(21),
		WithMaxMemoryBodyParsing(40))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	inst, err := registry.NewOpenPolicyAgentInstance("use_body", *cfg, "testfilter")
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

	registry := NewOpenPolicyAgentRegistry(WithMaxRequestBodyBytes(21),
		WithReadBodyBufferSize(21),
		WithMaxMemoryBodyParsing(21))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	inst, err := registry.NewOpenPolicyAgentInstance("use_body", *cfg, "testfilter")
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

type opaInstanceStartupTestCase struct {
	overridePeriodicTriggers bool
	expectedError            string
	expectedTriggerMode      plugins.TriggerMode
	discoveryBundle          string
	resourceBundle           bool
}

func runWithTestCases(t *testing.T, cases []opaInstanceStartupTestCase, test func(t *testing.T, tc opaInstanceStartupTestCase)) {
	for _, tc := range cases {
		sb := strings.Builder{}
		sb.WriteString(fmt.Sprintf("override-period-triggers=%v", tc.overridePeriodicTriggers))
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
