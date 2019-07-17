package main

import "testing"

func TestListFlag(t *testing.T) {
	t.Run("custom separator", func(t *testing.T) {
		f := newListFlag(":")
		if err := f.Set("foo:bar:baz"); err != nil {
			t.Fatal(err)
		}

		if len(f.values) != 3 ||
			f.values[0] != "foo" || f.values[1] != "bar" || f.values[2] != "baz" {
			t.Error("failed to parse flags", f.values)
		}
	})

	t.Run("comma separator", func(t *testing.T) {
		f := commaListFlag()
		if err := f.Set("foo,bar,baz"); err != nil {
			t.Fatal(err)
		}

		if len(f.values) != 3 ||
			f.values[0] != "foo" || f.values[1] != "bar" || f.values[2] != "baz" {
			t.Error("failed to parse flags", f.values)
		}
	})

	t.Run("restricted values", func(t *testing.T) {
		t.Run("good", func(t *testing.T) {
			f := commaListFlag("foo", "bar", "baz", "qux")
			if err := f.Set("foo,bar,baz"); err != nil {
				t.Fatal(err)
			}

			if len(f.values) != 3 ||
				f.values[0] != "foo" || f.values[1] != "bar" || f.values[2] != "baz" {
				t.Error("failed to parse flags", f.values)
			}
		})

		t.Run("bad", func(t *testing.T) {
			f := commaListFlag("foo", "bar")
			if err := f.Set("foo,bar,baz"); err == nil {
				t.Error("failed to fail")
			}
		})
	})

	t.Run("string representation", func(t *testing.T) {
		const input = "foo,bar,baz"
		f := commaListFlag()
		if err := f.Set(input); err != nil {
			t.Error(err)
		}

		output := f.String()
		if output != input {
			t.Error("unexpected string representation", output, input)
		}
	})
}
