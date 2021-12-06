package eskip

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

func TestMarshalUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name   string
		routes []*Route
		json   string
	}{
		{
			"empty fields",
			[]*Route{{BackendType: LoopBackend}},
			`[{"backend":{"type":"loopback"}}]`,
		},
		{
			"network backend",
			[]*Route{{Id: "foo", BackendType: NetworkBackend, Backend: "127.0.0.1"}},
			`[{"id":"foo","backend":{"type":"network","address":"127.0.0.1"}}]`,
		},
		{
			"loop backend",
			[]*Route{{Id: "bar", BackendType: LoopBackend}},
			`[{"id":"bar","backend":{"type":"loopback"}}]`,
		},
		{
			"lb backend",
			[]*Route{{Id: "beef", BackendType: LBBackend, LBAlgorithm: "yolo", LBEndpoints: []string{"localhost"}}},
			`[{"id":"beef","backend":{"type":"lb","algorithm":"yolo","endpoints":["localhost"]}}]`,
		},
		{
			"shunt backend",
			[]*Route{{Id: "shunty", BackendType: ShuntBackend}},
			`[{"id":"shunty","backend":{"type":"shunt"}}]`,
		},
		{
			"predicates and filters",
			[]*Route{
				{
					Id: "predfilter",
					Predicates: []*Predicate{
						{Name: "Method", Args: []interface{}{"GET"}},
					},
					Filters: []*Filter{
						{Name: "setPath", Args: []interface{}{"/foo"}},
					},
					Backend: "127.0.0.1",
				},
			},
			`[
				{
					"id":"predfilter",
					"backend":{"type":"network","address":"127.0.0.1"},
					"predicates":[{"name":"Method","args":["GET"]}],
					"filters":[{"name":"setPath","args":["/foo"]}]
				}
			]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strippedJSON := strings.Map(func(r rune) rune {
				if unicode.IsSpace(r) {
					return -1
				}
				return r
			}, tc.json)
			gotJSON, err := json.Marshal(tc.routes)
			if err != nil {
				t.Fatalf("cannot marshal %v, got error: %v", tc.routes, err)
			}
			b := []byte(strippedJSON)
			if !bytes.Equal(gotJSON, b) {
				t.Fatalf("marshal result incorrect, %s != %s", gotJSON, strippedJSON)
			}
			gotRoutes := make([]*Route, 0, len(tc.routes))
			err = json.Unmarshal(b, &gotRoutes)
			if err != nil {
				t.Fatalf("cannot unmarshal %s, got error: %v", gotJSON, err)
			}
			if !reflect.DeepEqual(gotRoutes, tc.routes) {
				t.Fatalf("unmarshal result incorrect, %v != %v", gotRoutes, tc.routes)
			}
		})
	}
}

func TestMarshalJSONLegacy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		routes []*Route
		json   string
	}{
		{
			"shunt",
			[]*Route{{Id: "sh", Shunt: true}},
			`[{"id":"sh","backend":{"type":"shunt"}}]`,
		},
		{
			"network backend",
			[]*Route{{Id: "network", Backend: "127.0.0.1"}},
			`[{"id":"network","backend":{"type":"network","address":"127.0.0.1"}}]`,
		},
		{
			"predicates",
			[]*Route{{Id: "oldschool", Path: "/foo", Headers: map[string]string{"Bar": "baz"}, Backend: "127.0.0.1"}},
			`[
				{
					"id":"oldschool",
					"backend":{
						"type":"network",
						"address":"127.0.0.1"
					},
					"predicates":[
						{"name":"Header","args":["Bar", "baz"]},
						{"name":"Path","args":["/foo"]}
					]
				}
			]`,
		},
	} {
		strippedJSON := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, tc.json)
		gotJSON, err := json.Marshal(tc.routes)
		if err != nil {
			t.Fatalf("cannot marshal %v, got error: %v", tc.routes, err)
		}
		b := []byte(strippedJSON)
		if !bytes.Equal(gotJSON, b) {
			t.Fatalf("marshal result incorrect, %s != %s", gotJSON, strippedJSON)
		}
	}
}
