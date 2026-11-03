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
// poll tick where exactly one zone's endpoints change.
func BenchmarkFormatAndSetOneZoneChanged(b *testing.B) {
	zones := []string{"eu-central-1a", "eu-central-1b", "eu-central-1c"}
	routes := benchRoutes(20000, 3, zones)
	zoneAware := filterRoutesByZone(routes)

	e := &eskipBytes{now: time.Now}
	e.formatAndSet(routes, zoneAware) // prime, so the first measured call already has a baseline to diff against

	// altRoutes differs from routes only in eu-central-1a's endpoint
	// addresses; eu-central-1b/c endpoints are untouched
	altRoutes := make([]*eskip.Route, len(routes))
	for i, r := range routes {
		rc := *r
		eps := make([]*eskip.LBEndpoint, len(r.LBEndpoints))
		for j, ep := range r.LBEndpoints {
			epCopy := *ep
			if epCopy.Zone == "eu-central-1a" {
				epCopy.Address += "-alt"
			}
			eps[j] = &epCopy
		}
		rc.LBEndpoints = eps
		altRoutes[i] = &rc
	}
	altZoneAware := filterRoutesByZone(altRoutes)

	toggle := false
	b.ReportAllocs()
	for b.Loop() {
		if toggle {
			e.formatAndSet(altRoutes, altZoneAware)
		} else {
			e.formatAndSet(routes, zoneAware)
		}
		toggle = !toggle
	}
}

// BenchmarkFormatAndSetBalanceFlip measures the cost of the balance gate
// flipping while routes stays completely unchanged
func BenchmarkFormatAndSetBalanceFlip(b *testing.B) {
	zones := []string{"eu-central-1a", "eu-central-1b", "eu-central-1c"}
	routes := benchRoutes(20000, 3, zones)
	zoneAware := filterRoutesByZone(routes)

	e := &eskipBytes{now: time.Now}
	e.formatAndSet(routes, zoneAware) // prime with the gate open

	toggle := true
	b.ReportAllocs()
	for b.Loop() {
		if toggle {
			e.formatAndSet(routes, nil) // gate closes
		} else {
			e.formatAndSet(routes, zoneAware) // gate reopens
		}
		toggle = !toggle
	}
}

// TestFormatAndSetOnlyTouchesChangedZone verifies that formatAndSet updates
// only the zone whose content actually changed: an untouched zone keeps its
// exact Etag/Last-Modified/compressed bytes across a call where a sibling
// zone changes.
func TestFormatAndSetOnlyTouchesChangedZone(t *testing.T) {
	e := &eskipBytes{now: time.Now}
	z1eps := []*eskip.LBEndpoint{
		{Address: "http://z1-ep1", Zone: "z1"},
		{Address: "http://z1-ep2", Zone: "z1"},
		{Address: "http://z1-ep3", Zone: "z1"},
	}
	z2v1eps := []*eskip.LBEndpoint{
		{Address: "http://z2-ep1-v1", Zone: "z2"},
		{Address: "http://z2-ep2-v1", Zone: "z2"},
		{Address: "http://z2-ep3-v1", Zone: "z2"},
	}
	routes := []*eskip.Route{{Id: "r1", BackendType: eskip.LBBackend, LBEndpoints: append(append([]*eskip.LBEndpoint{}, z1eps...), z2v1eps...)}}
	zoneMap := filterRoutesByZone(routes)

	_, _, _, updated := e.formatAndSet(routes, zoneMap)
	require.True(t, updated)

	z1HashBefore := e.zoneHash["z1"]
	z1DataBefore := e.zoneDataCompressed["z1"]
	z1ModBefore := e.zoneLastModified["z1"]

	// change z2 routes
	z2v2eps := []*eskip.LBEndpoint{
		{Address: "http://z2-ep1-v2", Zone: "z2"},
		{Address: "http://z2-ep2-v2", Zone: "z2"},
		{Address: "http://z2-ep3-v2", Zone: "z2"},
	}
	routes2 := []*eskip.Route{{Id: "r1", BackendType: eskip.LBBackend, LBEndpoints: append(append([]*eskip.LBEndpoint{}, z1eps...), z2v2eps...)}}
	zoneMap2 := filterRoutesByZone(routes2)

	_, _, _, updated = e.formatAndSet(routes2, zoneMap2)
	require.True(t, updated, "updated should be true since the route (and therefore z2) changed")

	assert.Equal(t, z1HashBefore, e.zoneHash["z1"], "z1 hash must be untouched when only z2's endpoints change")
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
	routes := []*eskip.Route{{Id: "r1", BackendType: eskip.LBBackend, LBEndpoints: []*eskip.LBEndpoint{
		{Address: "http://z1-ep", Zone: "z1"},
		{Address: "http://z2-ep", Zone: "z2"},
	}}}
	zoneMap := filterRoutesByZone(routes)

	_, _, _, updated := e.formatAndSet(routes, zoneMap)
	require.True(t, updated)
	_, ok := e.zoneDataCompressed["z2"]
	require.True(t, ok, "z2 must be present after the first call")

	// the balance gate closes; routes is unchanged, zoneAwareRoutes becomes nil
	_, _, _, updated = e.formatAndSet(routes, nil)
	assert.True(t, updated, "updated should be true since the balance gate closed")

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
// updated == true when only the balance gate flips (global routes
// identical), and also when only the global routes change (gate state
// unchanged). It also verifies that a pure balance-gate flip leaves the
// GLOBAL hash/Last-Modified/count/compressed bytes completely untouched,
// since only zoneAwareRoutes differs in that case, not routes itself.
func TestFormatAndSetDetectsZoneOnlyChange(t *testing.T) {
	e := &eskipBytes{now: time.Now}
	routes := []*eskip.Route{{Id: "r1"}}

	_, _, initialized, updated := e.formatAndSet(routes, nil)
	require.True(t, initialized, "initialized should be true after fresh start")
	require.True(t, updated, "updated should be true after fresh start")

	globalHashBefore := e.hash
	globalModBefore := e.lastModified
	globalCountBefore := e.count
	globalDataBefore := e.zdata
	require.NotEmpty(t, globalHashBefore)
	require.NotEmpty(t, globalDataBefore)

	// same routes, still not balanced -> no update
	_, _, initialized, updated = e.formatAndSet(routes, nil)
	require.False(t, initialized, "initialized should be false with no change")
	require.False(t, updated, "updated should be false with no change")

	// same routes, balance gate opens -> updated must be true, but the
	// global hash/Last-Modified/count/compressed bytes must stay untouched,
	// since routes itself never changed - only zoneAwareRoutes did.
	zoneA := map[string][]*eskip.Route{"z1": {{Id: "r1"}}}
	_, _, initialized, updated = e.formatAndSet(routes, zoneA)
	assert.False(t, initialized, "should always be false after first call")
	assert.True(t, updated, "should be true when the balance gate opens")
	assert.Equal(t, globalHashBefore, e.hash, "global hash must be untouched by a pure gate flip")
	assert.Equal(t, globalModBefore, e.lastModified, "global Last-Modified must be untouched by a pure gate flip")
	assert.Equal(t, globalCountBefore, e.count, "global count must be untouched by a pure gate flip")
	assert.Equal(t, globalDataBefore, e.zdata, "global compressed bytes must be untouched by a pure gate flip")

	// same routes, gate stays open -> no update
	_, _, _, updated = e.formatAndSet(routes, zoneA)
	assert.False(t, updated, "should be false when neither routes nor the gate changed")

	// DIFFERENT global routes, gate stays open -> updated must be true
	routes2 := []*eskip.Route{{Id: "r2"}}
	_, _, initialized, updated = e.formatAndSet(routes2, zoneA)
	assert.False(t, initialized, "should always be false after first call")
	assert.True(t, updated, "should be true when global only routes change")

	// gate closes again, routes unchanged -> updated must be true, and the
	// global hash/Last-Modified/count/compressed bytes (now reflecting
	// routes2) must stay untouched by the gate closing.
	globalHashAfterRoutes2 := e.hash
	globalModAfterRoutes2 := e.lastModified
	globalCountAfterRoutes2 := e.count
	globalDataAfterRoutes2 := e.zdata

	_, _, initialized, updated = e.formatAndSet(routes2, nil)
	assert.False(t, initialized, "should always be false after first call")
	assert.True(t, updated, "should be true when the balance gate closes")
	assert.Equal(t, globalHashAfterRoutes2, e.hash, "global hash must be untouched when the gate closes")
	assert.Equal(t, globalModAfterRoutes2, e.lastModified, "global Last-Modified must be untouched when the gate closes")
	assert.Equal(t, globalCountAfterRoutes2, e.count, "global count must be untouched when the gate closes")
	assert.Equal(t, globalDataAfterRoutes2, e.zdata, "global compressed bytes must be untouched when the gate closes")
}

// TestFormatAndSetHandlesSimultaneousRouteChangeAndBalanceFlip verifies
// that formatAndSet is correct when routes change and the balance gate
// flips on the very same call - a timing coincidence the existing tests
// never exercise together, since each varies exactly one of the two
// signals per call. Both globalUpdated and presenceChanged are true at
// once, in both flip directions.
func TestFormatAndSetHandlesSimultaneousRouteChangeAndBalanceFlip(t *testing.T) {
	e := &eskipBytes{now: time.Now}
	routes := []*eskip.Route{{Id: "r1"}}

	_, _, _, updated := e.formatAndSet(routes, nil)
	require.True(t, updated, "initial call must report updated")

	// gate opens AND routes change on the same call
	routes2 := []*eskip.Route{{Id: "r2"}}
	zoneA := map[string][]*eskip.Route{"z1": {{Id: "r2"}}}
	_, _, _, updated = e.formatAndSet(routes2, zoneA)
	require.True(t, updated, "updated must be true when routes and the gate change together")
	_, ok := e.zoneDataCompressed["z1"]
	assert.True(t, ok, "z1 must be populated when the gate opens, even though routes changed in the same call")

	// gate closes AND routes change again on the same call
	routes3 := []*eskip.Route{{Id: "r3"}}
	_, _, _, updated = e.formatAndSet(routes3, nil)
	require.True(t, updated, "updated must be true when routes and the gate change together")
	_, ok = e.zoneDataCompressed["z1"]
	assert.False(t, ok, "z1 must be removed when the gate closes, even though routes changed in the same call")
}

// TestFormatAndSetTreatsEmptyZoneMapLikeNil verifies that a non-nil but
// empty zoneAwareRoutes map (e.g. filterRoutesByZone found no zone labels
// at all, even though balanced is true) is treated identically to nil by
// the presence check: it must not be mistaken for "zones present" and
// must not trigger spurious zone work.
func TestFormatAndSetTreatsEmptyZoneMapLikeNil(t *testing.T) {
	e := &eskipBytes{now: time.Now}
	routes := []*eskip.Route{{Id: "r1"}}

	_, _, _, updated := e.formatAndSet(routes, map[string][]*eskip.Route{})
	require.True(t, updated, "initial call must report updated")
	assert.Empty(t, e.zoneHash, "no zone should be recorded when the zone map is empty")

	// same routes, still an empty (non-nil) zone map -> no update
	_, _, _, updated = e.formatAndSet(routes, map[string][]*eskip.Route{})
	assert.False(t, updated, "updated must be false: neither routes nor zone presence changed")

	// same routes, nil this time instead of empty map -> must still be a no-op,
	// since nil and empty are equivalent "no zones" states
	_, _, _, updated = e.formatAndSet(routes, nil)
	assert.False(t, updated, "nil and an empty map must be treated as the same presence state")
}

// TestFormatAndSetHandlesRapidBalanceOscillation verifies that the gate
// flipping true->false->true across consecutive polls leaves no stale
// data behind: each reopen must repopulate zones from scratch, not reuse
// leftovers from before the gate closed.
func TestFormatAndSetHandlesRapidBalanceOscillation(t *testing.T) {
	e := &eskipBytes{now: time.Now}
	routes := []*eskip.Route{{Id: "r1"}}
	zoneA := map[string][]*eskip.Route{"z1": {{Id: "r1"}}}

	// poll 1: gate opens
	_, _, _, updated := e.formatAndSet(routes, zoneA)
	require.True(t, updated)
	firstHash, ok := e.zoneHash["z1"]
	require.True(t, ok, "z1 must be populated after the gate opens")

	// poll 2: gate closes
	_, _, _, updated = e.formatAndSet(routes, nil)
	require.True(t, updated, "updated must be true when the gate closes")
	_, ok = e.zoneHash["z1"]
	require.False(t, ok, "z1 must be removed when the gate closes")

	// poll 3: gate reopens, same routes/zone content as poll 1
	_, _, _, updated = e.formatAndSet(routes, zoneA)
	require.True(t, updated, "updated must be true when the gate reopens")
	secondHash, ok := e.zoneHash["z1"]
	require.True(t, ok, "z1 must be repopulated after the gate reopens")
	assert.Equal(t, firstHash, secondHash, "z1's hash must match poll 1's, since its content is identical")

	zdata1 := e.zoneDataCompressed["z1"]
	assert.NotEmpty(t, zdata1, "z1 must have compressed data after reopening, not a stale/empty entry")
}
