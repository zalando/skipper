package eskip

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test various strings are parsed to the same value after
// serialization into eskip double-quoted string
func TestRoundtripString(t *testing.T) {
	for i, value := range []string{
		`s`,
		` `,
		`/`,
		`//`,
		`\`,
		`\\`,
		`\x`,
		`\\x`,
		`/x`,
		`//x`,
		`"`,
		`""`,
		`\/`,
		`\//`,
		`/\`,
		`/\\`,
		`//\\`,
		`/\/\`,
		`\/\/`,
		`^\/foo(#.*)?$`,
		`foo\/bar`,
		"\n",
		"\\\n",
		"\\\\\n",
		"\n\\",
		"\n\\\\",
		"\\\n\\",
		"\"\n",
		"\\\"\n",
	} {
		t.Run(fmt.Sprintf("test#%d", i), func(t *testing.T) {
			in := &Route{
				Path: value, // serialized as double-quoted string
				Filters: []*Filter{{
					"afilter",
					[]interface{}{value}, // serialized as double-quoted string
				}},
				BackendType: ShuntBackend,
			}
			t.Logf("%v, %s", value, in)

			outs, err := Parse(in.String())
			require.NoError(t, err)
			require.Len(t, outs, 1)

			out := outs[0]
			t.Logf("%s", out)

			assert.Equal(t, value, out.Path)

			require.Len(t, out.Filters, 1)
			require.Len(t, out.Filters[0].Args, 1)
			assert.Equal(t, value, out.Filters[0].Args[0])
		})
	}
}

// Test various strings are parsed to the same value after
// serialization into eskip regexp string
func TestRoundtripRegexp(t *testing.T) {
	for i, value := range []string{
		`s`,
		` `,
		`/`,
		`//`,
		`\`,
		`\\`,
		`\x`,
		`\\x`,
		`/x`,
		`//x`,
		`"`,
		`""`,
		`\/`,
		`\//`,
		`/\`,
		`/\\`,
		`//\\`,
		`/\/\`,
		`\/\/`,
		`^\/foo(#.*)?$`,
		`foo\/bar`,
		// eskip regexp does not support \n, \t and similar escape sequences
		// "\n",
		`[/]`,
		`[\[]`,
		`[\]]`,
		`[\]`,
		`["]`,
		`[\"]`,
	} {
		t.Run(fmt.Sprintf("test#%d", i), func(t *testing.T) {
			in := &Route{
				PathRegexps: []string{value}, // serialized as eskip regexp
				BackendType: ShuntBackend,
			}
			t.Logf("%v, %s", value, in)

			outs, err := Parse(in.String())
			require.NoError(t, err)
			require.Len(t, outs, 1)

			out := outs[0]
			t.Logf("%s", out)

			require.Len(t, out.PathRegexps, 1)
			assert.Equal(t, value, out.PathRegexps[0])
		})
	}
}
