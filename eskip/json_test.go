package eskip

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/sanity-io/litter"
)

func TestJSON(t *testing.T) {
	for _, test := range []struct {
		msg   string
		route string
		json  string
	}{{
		msg:   "minimal route expression",
		route: `* -> <shunt>`,
		json:  `{"id":"","backend":"<shunt>","predicates":[],"filters":[]}`,
	}, {
		msg:   "typical route expression",
		route: `Method("GET") && Path("/foo") -> setPath("/bar") -> "https://www.example.org"`,
		json: `{"id":"","backend":"https://www.example.org","predicates":[{"name":"Method",` +
			`"args":["GET"]},{"name":"Path","args":["/foo"]}],"filters":[{"name":"setPath",` +
			`"args":["/bar"]}]}`,
	}, {
		msg:   "typical route definition",
		route: `route1: Method("GET") && Path("/foo") -> setPath("/bar") -> "https://www.example.org"`,
		json: `{"id":"route1","backend":"https://www.example.org","predicates":[{"name":"Method",` +
			`"args":["GET"]},{"name":"Path","args":["/foo"]}],"filters":[{"name":"setPath",` +
			`"args":["/bar"]}]}`,
	}, {
		msg:   "shunt route",
		route: `teapot: Path("/foo") -> status(418) -> <shunt>`,
		json: `{"id":"teapot","backend":"<shunt>","predicates":[{"name":"Path",` +
			`"args":["/foo"]}],"filters":[{"name":"status","args":[418]}]}`,
	}, {
		msg:   "loopback route",
		route: `loop: Path("/foo") -> setPath("/bar") -> <loopback>`,
		json: `{"id":"loop","backend":"<loopback>","predicates":[{"name":"Path",` +
			`"args":["/foo"]}],"filters":[{"name":"setPath","args":["/bar"]}]}`,
	}} {
		t.Run(test.msg, func(t *testing.T) {
			r, err := Parse(test.route)
			if err != nil {
				t.Error(err)
				return
			}

			// Using Route.MarshalJSON directly because json.Marshall
			// forces encoding of HTML entities like <>
			b, err := r[0].MarshalJSON()
			if err != nil {
				t.Error(err)
				return
			}

			if strings.TrimSpace(string(b)) != test.json {
				t.Error("invalid json serialization result")
				t.Log("got:     ", string(b))
				t.Log("expected:", test.json)
				return
			}

			var routeBack Route
			if err := json.Unmarshal(b, &routeBack); err != nil {
				t.Error(err)
				return
			}

			if !reflect.DeepEqual(&routeBack, r[0]) {
				t.Error("invalid json parse result")
				t.Log("got:     ", litter.Sdump(&routeBack))
				t.Log("expected:", litter.Sdump(r[0]))
				return
			}
		})
	}
}

func TestParseJSON(t *testing.T) {
	const doc = `
		route1: Path("/foo") -> setPath("/bar") -> "https://bar.example.org";
		route2: Path("/baz") -> setPath("/qux") -> "https://qux.example.org";
		route3: * -> "https://www.example.org";
	`

	routes, err := Parse(doc)
	if err != nil {
		t.Error(err)
		return
	}

	jsn, err := PrintJSON(false, routes...)
	if err != nil {
		t.Error(err)
		return
	}

	routesBack, err := ParseJSON(jsn)
	if err != nil {
		t.Error(err)
		return
	}

	if !reflect.DeepEqual(routesBack, routes) {
		t.Error("failed to parse json")
		t.Log("got:     ", litter.Sdump(routesBack))
		t.Log("expected:", litter.Sdump(routes))
	}
}
