package routestring

import (
	"reflect"
	"testing"

	"github.com/sanity-io/litter"
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

			if !reflect.DeepEqual(r, test.expected) {
				t.Error("invalid routes received")
				t.Log("got:     ", litter.Sdump(r))
				t.Log("expected:", litter.Sdump(test.expected))
			}
		})
	}
}
