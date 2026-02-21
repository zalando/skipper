package annotate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/annotate"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestAnnotate(t *testing.T) {
	spec := annotate.New()

	for _, tc := range []struct {
		name     string
		def      string
		expected map[string]string
	}{
		{
			name:     "key and value",
			def:      `annotate("akey", "avalue")`,
			expected: map[string]string{"akey": "avalue"},
		},
		{
			name:     "multiple annotations",
			def:      `annotate("akey1", "avalue1") -> annotate("akey2", "avalue2")`,
			expected: map[string]string{"akey1": "avalue1", "akey2": "avalue2"},
		},
		{
			name:     "overwrite annotation",
			def:      `annotate("akey1", "avalue1") -> annotate("akey1", "avalue2")`,
			expected: map[string]string{"akey1": "avalue2"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &filtertest.Context{
				FStateBag: make(map[string]any),
			}

			for _, f := range eskip.MustParseFilters(tc.def) {
				filter, err := spec.CreateFilter(f.Args)
				require.NoError(t, err)

				filter.Request(ctx)
			}

			assert.Equal(t, tc.expected, annotate.GetAnnotations(ctx))
		})
	}
}

func TestAnnotateArgs(t *testing.T) {
	spec := annotate.New()

	t.Run("valid", func(t *testing.T) {
		for _, def := range []string{
			`annotate("akey", "avalue")`,
		} {
			t.Run(def, func(t *testing.T) {
				args := eskip.MustParseFilters(def)[0].Args

				_, err := spec.CreateFilter(args)
				assert.NoError(t, err)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		for _, def := range []string{
			`annotate()`,
			`annotate("akey")`,
			`annotate(1)`,
			`annotate("akey", 1)`,
			`annotate("akey", "avalue", "anextra")`,
			`annotate("akey", "avalue", 1)`,
		} {
			t.Run(def, func(t *testing.T) {
				args := eskip.MustParseFilters(def)[0].Args

				_, err := spec.CreateFilter(args)
				assert.Error(t, err)
			})
		}
	})
}
