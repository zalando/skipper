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

func TestRoundtripLBZone(t *testing.T) {
	t.Run("endpoints have zone", func(t *testing.T) {
		in := &Route{
			Id:          "r1",
			BackendType: LBBackend,
			LBAlgorithm: "roundRobin",
			LBEndpoints: []*LBEndpoint{
				{Address: "http://10.0.0.1:8080", Zone: "eu-central-1a"},
				{Address: "http://10.0.0.2:8080", Zone: "eu-central-1b"},
			},
		}

		serialized := in.String()
		t.Logf("serialized: %s", serialized)

		outs, err := Parse(serialized)
		require.NoError(t, err)
		require.Len(t, outs, 1)

		out := outs[0]
		require.Len(t, out.LBEndpoints, 2)

		assert.Equal(t, "http://10.0.0.1:8080", out.LBEndpoints[0].Address)
		assert.Equal(t, "eu-central-1a", out.LBEndpoints[0].Zone)
		assert.Equal(t, "http://10.0.0.2:8080", out.LBEndpoints[1].Address)
		assert.Equal(t, "eu-central-1b", out.LBEndpoints[1].Zone)

		assert.Equal(t, serialized, out.String(), "round-trip must be stable")
	})

	t.Run("endpoints do not have zone", func(t *testing.T) {
		in := &Route{
			Id:          "r2",
			BackendType: LBBackend,
			LBAlgorithm: "roundRobin",
			LBEndpoints: []*LBEndpoint{
				{Address: "http://10.0.0.1:8080"},
				{Address: "http://10.0.0.2:8080"},
			},
		}

		serialized := in.String()
		t.Logf("serialized: %s", serialized)

		outs, err := Parse(serialized)
		require.NoError(t, err)
		require.Len(t, outs, 1)

		out := outs[0]
		require.Len(t, out.LBEndpoints, 2)

		assert.Equal(t, "http://10.0.0.1:8080", out.LBEndpoints[0].Address)
		assert.Equal(t, "", out.LBEndpoints[0].Zone)
		assert.Equal(t, "http://10.0.0.2:8080", out.LBEndpoints[1].Address)
		assert.Equal(t, "", out.LBEndpoints[1].Zone)

		assert.Equal(t, serialized, out.String(), "round-trip must be stable")
	})
}
