package routesrv

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
)

// benchRoutes synthesizes n LB-backend routes whose endpoints are spread across
// the given zones, mirroring the shape of the profiled route table (each route
// carries a host predicate, a filter and per-zone endpoints).
func benchRoutes(n, endpointsPerZone int, zones []string) []*eskip.Route {
	routes := make([]*eskip.Route, n)
	for i := range routes {
		eps := make([]*eskip.LBEndpoint, 0, len(zones)*endpointsPerZone)
		for _, z := range zones {
			for j := range endpointsPerZone {
				eps = append(eps, &eskip.LBEndpoint{
					Address: fmt.Sprintf("https://10.%d.%d.%d:8080", i%256, j, len(z)),
					Zone:    z,
				})
			}
		}
		routes[i] = &eskip.Route{
			Id:          fmt.Sprintf("kube_namespace__ingress%d__example_org___svc%d", i, i),
			HostRegexps: []string{fmt.Sprintf("^svc%d[.]example[.]org$", i)},
			Filters:     []*eskip.Filter{{Name: "setRequestHeader", Args: []any{"X-Svc", fmt.Sprintf("svc%d", i)}}},
			BackendType: eskip.LBBackend,
			LBAlgorithm: "consistentHash",
			LBEndpoints: eps,
		}
	}
	return routes
}

// retainedBytes sums the length of every []byte and map[string][]byte field in
// eskipBytes via reflection, so it counts whatever route-table bytes the struct
// keeps resident without naming fields. This makes it deterministic and valid on
// both the pre-change struct (which also holds the uncompressed data/zoneData)
// and the current compressed-only struct.
func retainedBytes(e *eskipBytes) int {
	total := 0
	v := reflect.ValueOf(e).Elem()
	for _, f := range v.Fields() {
		switch f.Kind() {
		case reflect.Slice:
			if f.Type().Elem().Kind() == reflect.Uint8 {
				total += f.Len()
			}
		case reflect.Map:
			et := f.Type().Elem()
			if et.Kind() == reflect.Slice && et.Elem().Kind() == reflect.Uint8 {
				for iter := f.MapRange(); iter.Next(); {
					total += iter.Value().Len()
				}
			}
		}
	}
	return total
}

// BenchmarkFormatAndSet reports per-call allocation (via -benchmem) and
// retained_MB: the total route-table bytes the stored eskipBytes keeps resident
// (see retainedBytes). This drops when the base and per-zone tables are stored
// compressed-only instead of uncompressed, so run it before and after the change
// (e.g. across a git stash of the source) to see the difference. It is
// deterministic — no GC/heap-sampling noise.
func BenchmarkFormatAndSet(b *testing.B) {
	zones := []string{"eu-central-1a", "eu-central-1b", "eu-central-1c"}
	routes := benchRoutes(20000, 3, zones)
	zoneAware := filterRoutesByZone(routes)

	b.ReportAllocs()
	var e *eskipBytes
	for b.Loop() {
		e = &eskipBytes{now: time.Now}
		e.formatAndSet(routes, zoneAware)
	}
	b.StopTimer()

	b.ReportMetric(float64(retainedBytes(e))/1e6, "retained_MB")
}

// BenchmarkFormatAndSetNoChange measures the poll path where the serialized
// table is unchanged (updated == false) — the only place this change adds CPU:
// the current code computes a SHA-256 of the base table every call for change
// detection, where the pre-change code did a bytes.Equal. The base is
// re-serialized in both revisions, so that cost is shared.
func BenchmarkFormatAndSetNoChange(b *testing.B) {
	zones := []string{"eu-central-1a", "eu-central-1b", "eu-central-1c"}
	routes := benchRoutes(20000, 3, zones)
	zoneAware := filterRoutesByZone(routes)

	e := &eskipBytes{now: time.Now}
	e.formatAndSet(routes, zoneAware) // prime, so subsequent calls see no change

	b.ReportAllocs()
	for b.Loop() {
		e.formatAndSet(routes, zoneAware)
	}
}

// BenchmarkFormatAndSetOneZoneChanged measures the realistic steady-state
// poll tick where exactly one zone's route set changes while a sibling zone
// stays byte-identical (e.g. one AZ gains or loses an endpoint), and the
// global table is unchanged. Only eu-central-1a's zone routes differ between
// iterations; eu-central-1b is never mutated and is used by
// TestFormatAndSetOnlyTouchesChangedZone to assert it was left alone. This
// isolates the cost of handling a single changed zone from the
// all-zones-changed case already covered by BenchmarkFormatAndSet, and is
// the scenario the all-or-nothing vs. per-zone update strategy differ on
// the most.
func BenchmarkFormatAndSetOneZoneChanged(b *testing.B) {
	zones := []string{"eu-central-1a", "eu-central-1b", "eu-central-1c"}
	routes := benchRoutes(20000, 3, zones)
	zoneAware := filterRoutesByZone(routes)

	e := &eskipBytes{now: time.Now}
	e.formatAndSet(routes, zoneAware) // prime, so the first measured call already has a baseline to diff against

	const changedZone = "eu-central-1a"
	originalZoneRoutes := zoneAware[changedZone]
	altZoneRoutes := make([]*eskip.Route, len(originalZoneRoutes))
	for i, r := range originalZoneRoutes {
		rc := *r
		rc.Filters = append([]*eskip.Filter{{Name: "setRequestHeader", Args: []any{"X-Bench", "alt"}}}, r.Filters...)
		altZoneRoutes[i] = &rc
	}

	toggle := false
	b.ReportAllocs()
	for b.Loop() {
		if toggle {
			zoneAware[changedZone] = altZoneRoutes
		} else {
			zoneAware[changedZone] = originalZoneRoutes
		}
		toggle = !toggle
		e.formatAndSet(routes, zoneAware)
	}
}

// TestFormatAndSetOnlyTouchesChangedZone verifies that formatAndSet updates
// only the zone whose content actually changed: an untouched zone keeps its
// exact Etag/Last-Modified/compressed bytes across a call where a sibling
// zone changes.
func TestFormatAndSetOnlyTouchesChangedZone(t *testing.T) {
	e := &eskipBytes{now: time.Now}
	routes := []*eskip.Route{{Id: "r1"}}
	zoneMap := map[string][]*eskip.Route{
		"z1": {{Id: "r1"}},
		"z2": {{Id: "r1"}},
	}
	_, _, _, updated := e.formatAndSet(routes, zoneMap)
	require.True(t, updated)

	z1HashBefore := e.zoneHash["z1"]
	z1DataBefore := e.zoneDataCompressed["z1"]
	z1ModBefore := e.zoneLastModified["z1"]

	// z1's own content stays identical; only z2 changes
	zoneMap2 := map[string][]*eskip.Route{
		"z1": {{Id: "r1"}},
		"z2": {{Id: "r2"}},
	}
	_, _, _, updated = e.formatAndSet(routes, zoneMap2)
	require.True(t, updated, "updated should be true since z2 changed")

	assert.Equal(t, z1HashBefore, e.zoneHash["z1"], "z1 hash must be untouched when only z2 changes")
	assert.Equal(t, z1DataBefore, e.zoneDataCompressed["z1"], "z1 compressed bytes must be untouched when only z2 changes")
	assert.Equal(t, z1ModBefore, e.zoneLastModified["z1"], "z1 Last-Modified must be untouched when only z2 changes")
	assert.NotEqual(t, z1HashBefore, e.zoneHash["z2"], "z2 hash must differ from z1's since z2's content changed")
}

// TestFormatAndSetRemovesDisappearedZone verifies that a zone no longer
// present in zoneAwareRoutes is fully removed from all internal maps, so
// ServeHTTP falls back to serving the global routing table for it instead
// of stale per-zone data.
func TestFormatAndSetRemovesDisappearedZone(t *testing.T) {
	e := &eskipBytes{now: time.Now}
	routes := []*eskip.Route{{Id: "r1"}}
	zoneMap := map[string][]*eskip.Route{
		"z1": {{Id: "r1"}},
		"z2": {{Id: "r1"}},
	}
	_, _, _, updated := e.formatAndSet(routes, zoneMap)
	require.True(t, updated)
	_, ok := e.zoneDataCompressed["z2"]
	require.True(t, ok, "z2 must be present after the first call")

	// z2 disappears entirely (e.g. the balance gate closed, or its last
	// endpoint went away)
	zoneMap2 := map[string][]*eskip.Route{
		"z1": {{Id: "r1"}},
	}
	_, _, _, updated = e.formatAndSet(routes, zoneMap2)
	assert.True(t, updated, "updated should be true since z2 disappeared")

	_, ok = e.zoneDataCompressed["z2"]
	assert.False(t, ok, "z2 must be removed from zoneDataCompressed so ServeHTTP falls back to global")
	_, ok = e.zoneHash["z2"]
	assert.False(t, ok, "z2 must be removed from zoneHash")
	_, ok = e.zoneCount["z2"]
	assert.False(t, ok, "z2 must be removed from zoneCount")
	_, ok = e.zoneLastModified["z2"]
	assert.False(t, ok, "z2 must be removed from zoneLastModified")
}

// TestFormatAndSetDetectsZoneOnlyChange verifies that formatAndSet reports
// updated == true when only the zone map changes (global routes identical),
// and also when only the global routes change (zone map identical)
func TestFormatAndSetDetectsZoneOnlyChange(t *testing.T) {
	e := &eskipBytes{now: time.Now}
	routes := []*eskip.Route{{Id: "r1"}}
	zoneA := map[string][]*eskip.Route{"z1": {{Id: "r1"}}}

	_, _, initialized, updated := e.formatAndSet(routes, zoneA)
	require.True(t, initialized, "initialized should be true after fresh start")
	require.True(t, updated, "updated should be true after fresh start")

	// same global routes, same zone map -> no update
	_, _, initialized, updated = e.formatAndSet(routes, zoneA)
	require.False(t, initialized, "initialized should be false with no change")
	require.False(t, updated, "updated should be false with no change")

	// same global routes, DIFFERENT zone map -> updated must be true
	zoneB := map[string][]*eskip.Route{"z1": {{Id: "r1"}}, "z2": {{Id: "r1"}}}
	_, _, initialized, updated = e.formatAndSet(routes, zoneB)
	assert.False(t, initialized, "should always be false after first call")
	assert.True(t, updated, "should be true when zone only routes change")

	// DIFFERENT global routes, same zone map -> updated must be true
	routes2 := []*eskip.Route{{Id: "r2"}}
	_, _, initialized, updated = e.formatAndSet(routes2, zoneB)
	assert.False(t, initialized, "should always be false after first call")
	assert.True(t, updated, "should be true when global only routes change")

	// also true when both changes
	routes3 := []*eskip.Route{{Id: "r3"}}
	zoneC := map[string][]*eskip.Route{"z1": {{Id: "r1"}}, "z3": {{Id: "r1"}}}
	_, _, initialized, updated = e.formatAndSet(routes3, zoneC)
	assert.False(t, initialized, "should always be false after first call")
	assert.True(t, updated, "should be true when global only routes change")
}
