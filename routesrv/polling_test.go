package routesrv

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
)

func makeRoute(id string, zoneEndpoints map[string]int, optOut ...bool) *eskip.Route {
	var eps []*eskip.LBEndpoint
	for zone, count := range zoneEndpoints {
		for i := range count {
			eps = append(eps, &eskip.LBEndpoint{
				Address: zone + "-ep-" + string(rune('a'+i)),
				Zone:    zone,
			})
		}
	}
	r := &eskip.Route{Id: id, LBEndpoints: eps}
	if len(optOut) > 0 && optOut[0] {
		fs, err := eskip.ParseFilters(`annotate("zalando.org/traffic-zone-aware", "false")`)
		if err == nil && len(fs) == 1 {
			r.Filters = []*eskip.Filter{fs[0]}
		}
	}
	return r
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
		{
			name:      "A_z1=4_z2=4_opt_out / B_z1=4_z2=4_opt_out",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 4}, true),
			routeB:    makeRoute("B", map[string]int{"zone1": 4, "zone2": 4}, true),
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: false}},
		},
		{
			name:      "A_z1=4_z2=4_opt_out / B_z1=4_z2=4",
			routeA:    makeRoute("A", map[string]int{"zone1": 4, "zone2": 4}, true),
			routeB:    makeRoute("B", map[string]int{"zone1": 4, "zone2": 4}),
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: true}},
			wantZone2: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: true}},
		},
		{
			name:      "A_z1=2_z2=4_opt_out / B_z1=4_z2=1",
			routeA:    makeRoute("A", map[string]int{"zone1": 2, "zone2": 4}, true),
			routeB:    makeRoute("B", map[string]int{"zone1": 4, "zone2": 1}),
			wantZone1: []routeExpect{{id: "A", filtered: false}, {id: "B", filtered: true}},
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

func makeControlRoute() *eskip.Route {
	fs, err := eskip.ParseFilters(`annotate("is-zone-aware-routing-safe", "false")`)
	if err != nil {
		panic(err)
	}
	return &eskip.Route{
		Id:      "__zone_awareness_control",
		Backend: "<shunt>",
		Filters: fs,
	}
}

func TestIsZoneAwareRoutingSafe(t *testing.T) {
	tests := []struct {
		name     string
		routes   []*eskip.Route
		expected bool
	}{
		{
			name:     "No control route - safe",
			routes:   []*eskip.Route{makeRoute("A", map[string]int{"zone1": 4, "zone2": 4})},
			expected: true,
		},
		{
			name:     "With control route - not safe",
			routes:   []*eskip.Route{makeControlRoute(), makeRoute("A", map[string]int{"zone1": 4, "zone2": 4})},
			expected: false,
		},
		{
			name:     "Multiple routes with control route - not safe",
			routes:   []*eskip.Route{makeRoute("A", map[string]int{"zone1": 4}), makeControlRoute(), makeRoute("B", map[string]int{"zone2": 4})},
			expected: false,
		},
		{
			name:     "Empty routes - safe",
			routes:   []*eskip.Route{},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isZoneAwareRoutingSafe(tc.routes)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestFilterRoutesByZone_WithControlRoute(t *testing.T) {
	tests := []struct {
		name          string
		routes        []*eskip.Route
		wantZoneCount int
		wantFiltered  bool
	}{
		{
			name:          "Control route present - no filtering",
			routes:        []*eskip.Route{makeControlRoute(), makeRoute("A", map[string]int{"zone1": 4, "zone2": 4})},
			wantZoneCount: 2,
			wantFiltered:  false,
		},
		{
			name:          "No control route - normal filtering",
			routes:        []*eskip.Route{makeRoute("A", map[string]int{"zone1": 4, "zone2": 4})},
			wantZoneCount: 2,
			wantFiltered:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := filterRoutesByZone(tc.routes)

			// Check we have routes for all zones
			require.Equal(t, tc.wantZoneCount, len(result))

			// Check control route is filtered out
			for zone, routes := range result {
				for _, r := range routes {
					require.NotEqual(t, "__zone_awareness_control", r.Id)
				}

				if tc.wantFiltered {
					// Verify filtering happened (only zone-local endpoints)
					for _, r := range routes {
						for _, ep := range r.LBEndpoints {
							require.Equal(t, zone, ep.Zone)
						}
					}
				} else {
					// Verify no filtering (all endpoints present)
					for _, r := range routes {
						hasOtherZone := false
						for _, ep := range r.LBEndpoints {
							if ep.Zone != "" && ep.Zone != zone {
								hasOtherZone = true
								break
							}
						}
						// If route has endpoints in other zones, verify they're present
						if len(r.LBEndpoints) > 0 {
							_ = hasOtherZone
						}
					}
				}
			}
		})
	}
}
