package routesrv

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
)

func makeRoute(id string, zoneEndpoints map[string]int) *eskip.Route {
	var eps []*eskip.LBEndpoint
	for zone, count := range zoneEndpoints {
		for i := range count {
			eps = append(eps, &eskip.LBEndpoint{
				Address: zone + "-ep-" + string(rune('a'+i)),
				Zone:    zone,
			})
		}
	}
	return &eskip.Route{Id: id, LBEndpoints: eps}
}

type routeExpect struct {
	id       string
	filtered bool
}

func TestFilterRoutesByZone(t *testing.T) {
	tests := []struct {
		name      string
		routeA    *eskip.Route
		routeB    *eskip.Route
		wantZone1 []routeExpect
		wantZone2 []routeExpect
	}{
		{
			name:      "A_z1=4_z2=4 / B_z1=4_z2=4",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 4}),
			routeB:    makeRoute("B", map[string]int{"zone1": 4, "zone2": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: true}},
			wantZone2: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: true}},
		},
		{
			name:      "A_z1=4_z2=4 / B_z1=0_z2=0",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 4}),
			routeB:    makeRoute("B", map[string]int{}),
			wantZone1: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=4_z2=4 / B_z1=4_z2=0",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 4}),
			routeB:    makeRoute("B", map[string]int{"zone1": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: true}},
			wantZone2: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=4_z2=4 / B_z1=0_z2=4",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 4}),
			routeB:    makeRoute("B", map[string]int{"zone2": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: true}},
		},
		{
			name:      "A_z1=4_z2=4 / B_no_endpoints",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 4}),
			routeB:    &eskip.Route{Id: "B"},
			wantZone1: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=1_z2=1 / B_z1=4_z2=4",
			routeA:    makeRoute("A", map[string]int{"zone1": 1, "zone2": 1}),
			routeB:    makeRoute("B", map[string]int{"zone1": 4, "zone2": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: true}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: true}},
		},
		{
			name:      "A_z1=1_z2=1 / B_z1=0_z2=0",
			routeA:    makeRoute("A", map[string]int{"zone1": 1, "zone2": 1}),
			routeB:    makeRoute("B", map[string]int{}),
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=1_z2=1 / B_z1=4_z2=0",
			routeA:    makeRoute("A", map[string]int{"zone1": 1, "zone2": 1}),
			routeB:    makeRoute("B", map[string]int{"zone1": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: true}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=1_z2=1 / B_z1=0_z2=4",
			routeA:    makeRoute("A", map[string]int{"zone1": 1, "zone2": 1}),
			routeB:    makeRoute("B", map[string]int{"zone2": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: true}},
		},
		{
			name:      "A_z1=1_z2=1 / B_no_endpoints",
			routeA:    makeRoute("A", map[string]int{"zone1": 1, "zone2": 1}),
			routeB:    &eskip.Route{Id: "B"},
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=1_z2=4 / B_z1=4_z2=4",
			routeA:    makeRoute("A", map[string]int{"zone1": 1, "zone2": 4}),
			routeB:    makeRoute("B", map[string]int{"zone1": 4, "zone2": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: true}},
			wantZone2: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: true}},
		},
		{
			name:      "A_z1=1_z2=4 / B_z1=0_z2=0",
			routeA:    makeRoute("A", map[string]int{"zone1": 1, "zone2": 4}),
			routeB:    makeRoute("B", map[string]int{}),
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=1_z2=4 / B_z1=4_z2=0",
			routeA:    makeRoute("A", map[string]int{"zone1": 1, "zone2": 4}),
			routeB:    makeRoute("B", map[string]int{"zone1": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: true}},
			wantZone2: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=1_z2=4 / B_z1=0_z2=4",
			routeA:    makeRoute("A", map[string]int{"zone1": 1, "zone2": 4}),
			routeB:    makeRoute("B", map[string]int{"zone2": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: true}},
		},
		{
			name:      "A_z1=1_z2=4 / B_no_endpoints",
			routeA:    makeRoute("A", map[string]int{"zone1": 1, "zone2": 4}),
			routeB:    &eskip.Route{Id: "B"},
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=4_z2=1 / B_z1=4_z2=4",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 1}),
			routeB:    makeRoute("B", map[string]int{"zone1": 4, "zone2": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: true}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: true}},
		},
		{
			name:      "A_z1=4_z2=1 / B_z1=0_z2=0",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 1}),
			routeB:    makeRoute("B", map[string]int{}),
			wantZone1: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=4_z2=1 / B_z1=4_z2=0",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 1}),
			routeB:    makeRoute("B", map[string]int{"zone1": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: true}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=4_z2=1 / B_z1=0_z2=4",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 1}),
			routeB:    makeRoute("B", map[string]int{"zone2": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: true}},
		},
		{
			name:      "A_z1=4_z2=1 / B_no_endpoints",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 1}),
			routeB:    &eskip.Route{Id: "B"},
			wantZone1: []routeExpect{{id: "A", filtered: true}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := filterRoutesByZone([]*eskip.Route{tc.routeA, tc.routeB})

			for zone, expects := range map[string][]routeExpect{
				"zone1": tc.wantZone1,
				"zone2": tc.wantZone2,
			} {
				got := result[zone]
				require.Equal(t, len(got), len(expects))
				gotByID := make(map[string]*eskip.Route)
				for _, r := range got {
					gotByID[r.Id] = r
				}
				for _, ex := range expects {
					r, ok := gotByID[ex.id]
					require.True(t, ok)
					if ex.filtered {
						for _, ep := range r.LBEndpoints {
							require.Equal(t, ep.Zone, zone)
						}
					} else {
						for _, ep := range r.LBEndpoints {
							if ep.Zone != "" && ep.Zone != zone {
								return
							}
						}
					}
				}
			}
		})
	}
}
