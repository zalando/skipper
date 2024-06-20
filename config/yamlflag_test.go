package config

import (
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type yamlFlagTestConfig struct {
	Foo string
	Bar []string
	Baz int
	Qux *yamlFlagTestConfig
}

func TestYamlFlag(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		cfg := struct {
			Field *yamlFlagTestConfig
		}{}

		f := newYamlFlag(&cfg.Field)
		v := `{foo: hello, bar: [world, "1"], baz: 2, qux: {baz: 3}}`
		err := f.Set(v)

		require.NoError(t, err)
		assert.Equal(t, "hello", cfg.Field.Foo)
		assert.Equal(t, []string{"world", "1"}, cfg.Field.Bar)
		assert.Equal(t, 2, cfg.Field.Baz)
		assert.Equal(t, 3, cfg.Field.Qux.Baz)

		assert.Equal(t, v, f.String())
	})

	t.Run("set empty", func(t *testing.T) {
		cfg := struct {
			Field *yamlFlagTestConfig
		}{}

		f := newYamlFlag(&cfg.Field)
		err := f.Set("")

		require.NoError(t, err)
		assert.Equal(t, &yamlFlagTestConfig{}, cfg.Field)
		assert.Equal(t, "", f.String())
	})

	t.Run("set empty object", func(t *testing.T) {
		cfg := struct {
			Field *yamlFlagTestConfig
		}{}

		f := newYamlFlag(&cfg.Field)
		err := f.Set("{}")

		require.NoError(t, err)
		assert.Equal(t, &yamlFlagTestConfig{}, cfg.Field)
		assert.Equal(t, "{}", f.String())
	})

	t.Run("set invalid yaml", func(t *testing.T) {
		cfg := struct {
			Field *yamlFlagTestConfig
		}{}

		f := newYamlFlag(&cfg.Field)
		v := `This is not a valid YAML`
		err := f.Set(v)

		assert.Error(t, err)
	})

	t.Run("unmarshal YAML", func(t *testing.T) {
		cfg := struct {
			Field *yamlFlagTestConfig
		}{}

		err := yaml.Unmarshal([]byte(`
field:
  foo: hello
  bar:
    - world
    - "1"
  baz: 2
  qux:
    baz: 3
`), &cfg)

		require.NoError(t, err)
		assert.Equal(t, "hello", cfg.Field.Foo)
		assert.Equal(t, []string{"world", "1"}, cfg.Field.Bar)
		assert.Equal(t, 2, cfg.Field.Baz)
		assert.Equal(t, 3, cfg.Field.Qux.Baz)
	})

	t.Run("unmarshal invalid YAML", func(t *testing.T) {
		cfg := struct {
			Field *yamlFlagTestConfig
		}{}

		err := yaml.Unmarshal([]byte(`This is not a valid YAML`), &cfg)

		assert.Error(t, err)
	})
}
