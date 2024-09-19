package auth

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	Name  string
	Value int
	Error bool

	initialized bool
}

func (tc *testConfig) initialize() error {
	if tc.Error {
		return fmt.Errorf("error initializing %s", tc.Name)
	}
	tc.initialized = true
	return nil
}

func TestYamlConfig_parse(t *testing.T) {
	const (
		fooConfig = `{name: foo, value: 42}`
		barConfig = `{name: bar, value: 1984}`
		bazConfig = `{name: baz, value: 3024}`
	)

	yc := newYamlConfigParser[testConfig](2)

	foo1, err := yc.parse(fooConfig)
	require.NoError(t, err)
	assert.Equal(t, "foo", foo1.Name)
	assert.Equal(t, 42, foo1.Value)
	assert.True(t, foo1.initialized)

	foo2, err := yc.parse(fooConfig)
	require.NoError(t, err)
	assert.True(t, foo1 == foo2, "expected cached instance")

	bar1, err := yc.parse(barConfig)
	require.NoError(t, err)
	assert.Equal(t, "bar", bar1.Name)
	assert.Equal(t, 1984, bar1.Value)
	assert.True(t, bar1.initialized)

	baz1, err := yc.parse(bazConfig)
	require.NoError(t, err)
	assert.Equal(t, "baz", baz1.Name)
	assert.Equal(t, 3024, baz1.Value)
	assert.True(t, baz1.initialized)

	// check either foo or bar was evicted
	assert.Len(t, yc.cache, 2)
	assert.Contains(t, yc.cache, bazConfig)
	assert.Subset(t, map[string]*testConfig{
		fooConfig: foo1,
		barConfig: bar1,
		bazConfig: baz1,
	}, yc.cache)
}

func TestYamlConfig_parse_errors(t *testing.T) {
	t.Run("invalid yaml", func(t *testing.T) {
		yc := newYamlConfigParser[testConfig](1)

		config, err := yc.parse(`invalid yaml`)
		assert.Error(t, err)
		assert.Nil(t, config)
	})

	t.Run("initialize error", func(t *testing.T) {
		yc := newYamlConfigParser[testConfig](1)

		config, err := yc.parse(`{name: foo, error: true}`)
		assert.EqualError(t, err, "error initializing foo")
		assert.Nil(t, config)
	})
}

func TestYamlConfig_parseSingleArg(t *testing.T) {
	yc := newYamlConfigParser[testConfig](1)

	t.Run("single string arg", func(t *testing.T) {
		config, err := yc.parseSingleArg([]any{`{name: foo, value: 42}`})
		require.NoError(t, err)
		assert.Equal(t, "foo", config.Name)
		assert.Equal(t, 42, config.Value)
		assert.True(t, config.initialized)
	})

	t.Run("single non-string arg", func(t *testing.T) {
		config, err := yc.parseSingleArg([]any{42})
		assert.EqualError(t, err, "requires single string argument")
		assert.Nil(t, config)
	})

	t.Run("empty args", func(t *testing.T) {
		config, err := yc.parseSingleArg([]any{})
		assert.EqualError(t, err, "requires single string argument")
		assert.Nil(t, config)
	})

	t.Run("too many args", func(t *testing.T) {
		config, err := yc.parseSingleArg([]any{`{name: foo, value: 42}`, `{name: bar, value: 1984}`})
		assert.EqualError(t, err, "requires single string argument")
		assert.Nil(t, config)
	})
}
