package routestring

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
)

func TestRouteString(t *testing.T) {
	for _, test := range []struct {
		title    string
		text     string
		expected []*eskip.Route
		fail     bool
	}{{
		title: "empty",
	}, {
		title: "invalid",
		text:  "foo",
		fail:  true,
	}, {
		title: "single expression",
		text:  `* -> static("/", "/var/www") -> <shunt>`,
		expected: []*eskip.Route{{
			Filters: []*eskip.Filter{{
				Name: "static",
				Args: []interface{}{
					"/",
					"/var/www",
				},
			}},
			BackendType: eskip.ShuntBackend,
			Shunt:       true,
		}},
	}, {
		title: "single definition",
		text:  `static_content: * -> static("/", "/var/www") -> <shunt>`,
		expected: []*eskip.Route{{
			Id: "static_content",
			Filters: []*eskip.Filter{{
				Name: "static",
				Args: []interface{}{
					"/",
					"/var/www",
				},
			}},
			BackendType: eskip.ShuntBackend,
			Shunt:       true,
		}},
	}, {
		title: "multiple definitions",
		text: `static_content: * -> static("/", "/var/www") -> <shunt>;
			register: Method("POST") && Path("/register")
				-> setPath("/")
				-> "https://register.example.org"`,
		expected: []*eskip.Route{{
			Id: "static_content",
			Filters: []*eskip.Filter{{
				Name: "static",
				Args: []interface{}{
					"/",
					"/var/www",
				},
			}},
			BackendType: eskip.ShuntBackend,
			Shunt:       true,
		}, {
			Id:     "register",
			Method: "POST",
			Path:   "/register",
			Filters: []*eskip.Filter{{
				Name: "setPath",
				Args: []interface{}{
					"/",
				},
			}},
			Backend: "https://register.example.org",
		}},
	}} {
		t.Run(test.title, func(t *testing.T) {
			dc, err := New(test.text)
			if err == nil && test.fail {
				t.Error("failed to fail")
				return
			} else if err != nil && !test.fail {
				t.Error(err)
				return
			} else if test.fail {
				return
			}

			r, err := dc.LoadAll()
			if err != nil {
				t.Error(err)
				return
			}

			if len(r) == 0 {
				r = nil
			}

			if !cmp.Equal(r, test.expected) {
				t.Errorf("invalid routes received\n %s", cmp.Diff(r, test.expected))
			}
		})
	}
}

func TestNewList(t *testing.T) {
	for _, tc := range []struct {
		defs     []string
		expected []*eskip.Route
	}{
		{
			defs:     nil,
			expected: nil,
		},
		{
			defs:     []string{},
			expected: nil,
		},
		{
			defs:     []string{""},
			expected: nil,
		},
		{
			defs:     []string{`* -> static("/", "/var/www") -> <shunt>`},
			expected: eskip.MustParse(`* -> static("/", "/var/www") -> <shunt>`),
		},
		{
			defs: []string{
				`* -> static("/", "/var/www") -> <shunt>`,
				`Path("/foo") -> status(404) -> <shunt>`,
			},
			// multiple routes require route ids so use append instead of single eskip.MustParse
			expected: append(
				eskip.MustParse(`* -> static("/", "/var/www") -> <shunt>`),
				eskip.MustParse(`Path("/foo") -> status(404) -> <shunt>`)...,
			),
		},
		{
			defs: []string{
				`* -> static("/", "/var/www") -> <shunt>`,
				`r1: Path("/foo") -> status(404) -> <shunt>;
					r2: Path("/bar") -> status(404) -> <shunt>;`,
			},
			// multiple routes require route ids so use append instead of single eskip.MustParse
			expected: append(
				eskip.MustParse(`* -> static("/", "/var/www") -> <shunt>`),
				eskip.MustParse(`r1: Path("/foo") -> status(404) -> <shunt>;
					r2: Path("/bar") -> status(404) -> <shunt>;`)...,
			),
		},
	} {
		t.Run(strings.Join(tc.defs, ";"), func(t *testing.T) {
			dc, err := NewList(tc.defs)
			require.NoError(t, err)

			routes, err := dc.LoadAll()
			require.NoError(t, err)

			if !cmp.Equal(tc.expected, routes) {
				t.Errorf("invalid routes received\n %s", cmp.Diff(tc.expected, routes))
			}
		})
	}
}

func TestNewListError(t *testing.T) {
	for _, tc := range []struct {
		defs     []string
		expected string
	}{
		{
			defs:     []string{`* ->`},
			expected: "#0: parse failed after token ->, position 4: syntax error",
		},
		{
			defs: []string{
				`* -> <shunt>`,
				`Path("/") ->`,
			},
			expected: "#1: parse failed after token ->, position 12: syntax error",
		},
		{
			// multiple routes require route ids
			defs: []string{
				`Path("/foo") -> status(404) -> <shunt>;
					Path("/bar") -> status(404) -> <shunt>;`,
			},
			expected: "#0: parse failed after token ;, position 39: syntax error",
		},
	} {
		t.Run(strings.Join(tc.defs, ";"), func(t *testing.T) {
			_, err := NewList(tc.defs)
			assert.EqualError(t, err, tc.expected)
		})
	}
}
