package openpolicyagent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
