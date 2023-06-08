package openpolicyagent

import (
	"os"
	"testing"

	ext_authz_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	_struct "github.com/golang/protobuf/ptypes/struct"
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

	if err != nil {
		t.Error(err)
	}

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

	if err != nil {
		t.Error(err)
	}

	expected := &ext_authz_v3_core.Metadata{}
	err = protojson.Unmarshal(expectedBytes, expected)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, expected, cfg.envoyMetadata)

}
