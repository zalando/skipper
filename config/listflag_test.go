package config

import (
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/google/go-cmp/cmp"
)

func TestListFlag(t *testing.T) {
	const yamlList = `- foo
- bar
- baz`

	t.Run("custom separator", func(t *testing.T) {
		var (
			expected = []string{"foo", "bar", "baz"}
			current  = newListFlag(":")
		)

		if err := current.Set("foo:bar:baz"); err != nil {
			t.Fatal(err)
		}

		if cmp.Equal(expected, current.values) == false {
			t.Error("failed to parse flags", current.values)
		}

		if err := yaml.Unmarshal([]byte(yamlList), current); err != nil {
			t.Fatal(err)
		}

		if cmp.Equal(expected, current.values) == false {
			t.Error("failed to parse yaml", current.values)
		}

		if current.value != "foo:bar:baz" {
			t.Error("invalid value composed by yaml parser")
		}
	})

	t.Run("comma separator", func(t *testing.T) {
		f := commaListFlag()
		if err := f.Set("foo,bar,baz"); err != nil {
			t.Fatal(err)
		}

		if cmp.Equal([]string{"foo", "bar", "baz"}, f.values) == false {
			t.Error("failed to parse flags", f.values)
		}
	})

	t.Run("restricted values", func(t *testing.T) {
		t.Run("good", func(t *testing.T) {
			var (
				expected = []string{"foo", "bar", "baz"}
				current  = commaListFlag("foo", "bar", "baz", "qux")
			)

			if err := current.Set("foo,bar,baz"); err != nil {
				t.Fatal(err)
			}

			if cmp.Equal(expected, current.values) == false {
				t.Error("failed to parse flags", current.values)
			}

			if err := yaml.Unmarshal([]byte(yamlList), current); err != nil {
				t.Fatal(err)
			}

			if cmp.Equal(expected, current.values) == false {
				t.Error("failed to parse yaml", current.values)
			}
		})

		t.Run("bad", func(t *testing.T) {
			current := commaListFlag("foo", "bar")
			if err := current.Set("foo,bar,baz"); err == nil {
				t.Error("failed to fail")
			}

			if err := yaml.Unmarshal([]byte(yamlList), current); err == nil {
				t.Error("failed to fail")
			}
		})
	})

	t.Run("string representation", func(t *testing.T) {
		const input = "foo,bar,baz"

		current := commaListFlag()
		if err := current.Set(input); err != nil {
			t.Error(err)
		}

		output := current.String()
		if output != input {
			t.Error("unexpected string representation", output, input)
		}

		if err := yaml.Unmarshal([]byte(yamlList), current); err != nil {
			t.Fatal(err)
		}
		if output != input {
			t.Error("unexpected string representation from yaml", output, input)
		}
	})
}
