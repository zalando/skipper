package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestMultiFlagSet(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   string
		values string
	}{
		{
			name:   "single value",
			args:   "foo=bar",
			values: "foo=bar",
		},
		{
			name:   "multiple values",
			args:   "foo=bar foo=baz foo=qux bar=baz",
			values: "foo=bar foo=baz foo=qux bar=baz",
		},
	} {
		t.Run(tc.name+"_valid", func(t *testing.T) {
			multiFlag := &multiFlag{}
			err := multiFlag.Set(tc.args)
			require.NoError(t, err)
			assert.Equal(t, tc.values, multiFlag.String())
		})
	}
}

func TestMultiFlagYamlErr(t *testing.T) {
	m := &multiFlag{}
	err := yaml.Unmarshal([]byte(`-foo=bar`), m)
	require.Error(t, err, "Failed to get error on wrong yaml input")
}
