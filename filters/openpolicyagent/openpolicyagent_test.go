package openpolicyagent

import (
	"fmt"
	"os"
	"testing"
	"time"

	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	_struct "github.com/golang/protobuf/ptypes/struct"
	opasdktest "github.com/open-policy-agent/opa/sdk/test"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"
)

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

func TestRegistry(t *testing.T) {
	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle("/bundles/test", map[string]string{
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

	registry := NewOpenPolicyAgentRegistry(WithReuseDuration(2 * time.Second))

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
	time.Sleep(15 * time.Second)

	inst4, err := registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")

	assert.NoError(t, err)

	assert.NotEqual(t, inst1, inst4, "after cleanup a new instance should be created")

	registry.Close()

	_, err = registry.NewOpenPolicyAgentInstance("test", *cfg, "testfilter")

	assert.Error(t, err, "should not work after close")
}
