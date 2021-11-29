package eskip

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestMarshalUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name   string
		routes []*Route
		json   string
	}{
		// {
		// 	"network backend",
		// 	[]*Route{{Id: "test", BackendType: NetworkBackend, Backend: "127.0.0.1"}},
		// 	`[{"id":"test","backend":{"type":"network","address":"127.0.0.1"},"predicates":[],"filters":[]}]`,
		// },
		// {
		// 	"lb backend",
		// 	[]*Route{{Id: "test", BackendType: LBBackend, LBAlgorithm: "yolo", LBEndpoints: []string{"localhost"}}},
		// 	`[{"id":"test","backend":{"type":"lb","algorithm":"yolo","endpoints":["localhost"]},"predicates":[],"filters":[]}]`,
		// },
		// {
		// 	"shunt backend",
		// 	[]*Route{{Id: "test", BackendType: ShuntBackend}},
		// 	`[{"id":"test","backend":{"type":"shunt"},"predicates":[],"filters":[]}]`,
		// },
		{
			"shunt backend legacy",
			[]*Route{{Id: "test", Shunt: true}},
			`[{"id":"test","backend":{"type":"shunt"},"predicates":[],"filters":[]}]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotJSON, err := json.Marshal(tc.routes)
			if err != nil {
				t.Fatalf("cannot marshal %v, got error: %v", tc.routes, err)
			}
			b := []byte(tc.json)
			if !bytes.Equal(gotJSON, b) {
				t.Fatalf("marshal result incorrect, %s != %s", gotJSON, tc.json)
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
