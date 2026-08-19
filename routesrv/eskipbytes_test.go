package routesrv

import (
	"fmt"
	"reflect"
	"testing"
	"time"

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
