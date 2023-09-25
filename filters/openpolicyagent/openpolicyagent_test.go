package openpolicyagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	opasdktest "github.com/open-policy-agent/opa/sdk/test"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
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
	WithEnvoyMetadataBytes([]byte(`
	{
		"filter_metadata": {
			"envoy.filters.http.header_to_metadata": {
				"policy_type": "ingress"
			}	
		}
	}
	`))(cfg)

	expectedBytes, err := protojson.Marshal(&ext_authz_v3_core.Metadata{
		FilterMetadata: map[string]*_struct.Struct{
			"envoy.filters.http.header_to_metadata": {
				Fields: map[string]*_struct.Value{
					"policy_type": {
						Kind: &_struct.Value_StringValue{StringValue: "ingress"},
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
				{"discovery":{"bundles":{"bundles/test":{"persist":false,"resource":"bundles/test","service":"test"}},"plugins":{"envoy_ext_authz_grpc":{"addr":"unix:///run/opa/sockets/auth.sock","dry-run":false,"enable-reflection":true,"path":"main/main"}},"status":{"console": true}}}
			`,
		}),
		opasdktest.MockBundle("/bundles/discovery-with-error", map[string]string{
			"data.json": `
				{"discovery":{"bundles":{"bundles/non-existing-bundle":{"persist":false,"resource":"bundles/non-existing-bundle","service":"test"}},"plugins":{"envoy_ext_authz_grpc":{"addr":"unix:///run/opa/sockets/auth.sock","dry-run":false,"enable-reflection":true,"path":"main/main"}},"status":{"console": true}}}
			`,
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
				"path": "/envoy/authz/allow",
				"dry-run": false    
			}
		}
	}`, opaControlPlane.URL()))

	return opaControlPlane, config
}

func TestRegistry(t *testing.T) {
	_, config := mockControlPlaneWithResourceBundle()

	registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	inst1, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
	assert.NoError(t, err)

	registry.markUnused(map[*OpenPolicyAgentInstance]struct{}{})

	inst2, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
	assert.NoError(t, err)
	assert.Equal(t, inst1, inst2, "same instance is reused after release")

	inst3, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
	assert.NoError(t, err)
	assert.Equal(t, inst2, inst3, "same instance is reused multiple times")

	registry.markUnused(map[*OpenPolicyAgentInstance]struct{}{})

	//Allow clean up
	time.Sleep(2 * time.Second)

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
	time.Sleep(2 * time.Second)

	inst5, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
	assert.NoError(t, err)
	assert.NotEqual(t, inst4, inst5, "after cleanup a new instance should be created")

	registry.Close()

	_, err = registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
	assert.Error(t, err, "should not work after close")
}

func TestBundleActivationSuccessWithDiscovery(t *testing.T) {
	_, config := mockControlPlaneWithDiscoveryBundle("bundles/discovery")

	registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	instance, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
	assert.NotNil(t, instance)
	assert.NoError(t, err)
}

func TestBundleActivationFailureWithDiscovery(t *testing.T) {
	_, config := mockControlPlaneWithDiscoveryBundle("/bundles/discovery-with-error")

	registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	instance, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
	assert.Nil(t, instance)
	assert.Error(t, err)
}

func TestStartup(t *testing.T) {
	_, config := mockControlPlaneWithResourceBundle()

	registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	inst1, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
	assert.NoError(t, err)

	target := envoy.PluginConfig{Path: "/envoy/authz/allow", DryRun: false}
	target.ParseQuery()
	assert.Equal(t, target, inst1.EnvoyPluginConfig())
}

func TestTracing(t *testing.T) {
	_, config := mockControlPlaneWithResourceBundle()

	registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	inst, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
	assert.NoError(t, err)

	tracer := &tracingtest.Tracer{}
	parent := tracer.StartSpan("start_span")
	ctx := opentracing.ContextWithSpan(context.Background(), parent)
	span, _ := inst.StartSpanFromContext(ctx)
	span.Finish()
	parent.Finish()

	recspan, ok := tracer.FindSpan("open-policy-agent")
	assert.True(t, ok, "No span was created for open policy agent")
	assert.Equal(t, map[string]interface{}{"opa.bundle_name": "test", "opa.label.id": inst.manager.Labels()["id"], "opa.label.version": inst.manager.Labels()["version"]}, recspan.Tags)
}

func TestEval(t *testing.T) {
	_, config := mockControlPlaneWithResourceBundle()

	registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	inst, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
	assert.NoError(t, err)

	tracer := &tracingtest.Tracer{}
	span := tracer.StartSpan("open-policy-agent")
	ctx := opentracing.ContextWithSpan(context.Background(), span)

	result, err := inst.Eval(ctx, &authv3.CheckRequest{})
	assert.NoError(t, err)

	allowed, err := result.IsAllowed()
	assert.NoError(t, err)
	assert.False(t, allowed)

	span.Finish()
	testspan, ok := tracer.FindSpan("open-policy-agent")
	assert.True(t, ok)
	assert.Equal(t, result.DecisionID, testspan.Tags["opa.decision_id"])
}

func TestResponses(t *testing.T) {
	_, config := mockControlPlaneWithResourceBundle()

	registry := NewOpenPolicyAgentRegistry(WithReuseDuration(1*time.Second), WithCleanInterval(1*time.Second))

	cfg, err := NewOpenPolicyAgentConfig(WithConfigTemplate(config))
	assert.NoError(t, err)

	inst, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")
	assert.NoError(t, err)

	tracer := &tracingtest.Tracer{}
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
	testspan, ok := tracer.FindSpan("open-policy-agent")
	assert.True(t, ok, "span not found")
	assert.Contains(t, testspan.Tags, "error")

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
