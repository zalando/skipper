package kubernetes

import (
	"strconv"
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

var dummy2 []string

func benchmarkCachedEndpoints(b *testing.B, n int) {
	endpoints := make(map[definitions.ResourceID]*endpoint)
	for i := 0; i < n; i++ {
		name := "foo-" + strconv.Itoa(i)
		rid := newResourceID("default", name)
		ep := &endpoint{
			Meta: &definitions.Metadata{
				Namespace: "default",
				Name:      name,
				Labels:    map[string]string{},
			},
			Subsets: []*subset{
				{
					Addresses: []*address{
						{"192.168.0.1", "node1"},
						{"192.168.0.2", "node2"},
						{"192.168.0.3", "node3"},
						{"192.168.0.4", "node4"},
						{"192.168.0.5", "node5"},
						{"192.168.0.6", "node6"},
						{"192.168.0.7", "node7"},
						{"192.168.0.8", "node8"},
						{"192.168.0.9", "node9"},
						{"192.168.0.10", "node10"},
						{"192.168.0.11", "node11"},
					},
					Ports: []*port{
						{"ssh", 22, "TCP"},
						{"http", 80, "TCP"},
					},
				},
			},
		}
		endpoints[rid] = ep
	}

	cs := &clusterState{
		ingressesV1:     nil,
		routeGroups:     nil,
		services:        nil,
		endpoints:       endpoints,
		secrets:         nil,
		cachedEndpoints: make(map[endpointID][]string),
	}

	b.ResetTimer()
	dummy := []string{}
	for i := 0; i < b.N; i++ {
		dummy = cs.GetEndpointsByTarget("default", "foo-0", "TCP", "http", &definitions.BackendPort{})
	}
	dummy2 = dummy
}

func BenchmarkCachedEndpoint(b *testing.B) {
	for _, tt := range []struct {
		name            string
		endpointsNumber int
	}{
		{
			name:            "1M Endpoints",
			endpointsNumber: 1_000_000,
		},
		{
			name:            "10K Endpoints",
			endpointsNumber: 10_000,
		},
		{
			name:            "3 Endpoints",
			endpointsNumber: 3,
		}} {
		b.Run(tt.name, func(b *testing.B) {
			benchmarkCachedEndpoints(b, tt.endpointsNumber)
		})
	}
}

// makeSkipperEndpointSlice builds a skipperEndpointSlice from a map of zone→count.
func makeSkipperEndpointSlice(zoneCounts map[string]int) *skipperEndpointSlice {
	eps := &skipperEndpointSlice{}
	i := 0
	for zone, count := range zoneCounts {
		for range count {
			eps.Endpoints = append(eps.Endpoints, &skipperEndpoint{
				Address: "10.0." + strconv.Itoa(i) + ".1",
				Zone:    zone,
			})
			i++
		}
	}
	return eps
}

func TestIsServiceBalancedAcrossZones(t *testing.T) {
	const (
		ns   = DefaultIngressControllerNamespace
		name = DefaultIngressControllerName
	)

	for _, tt := range []struct {
		name                 string
		endpointSlicesEnable bool
		endpointSlicePresent bool
		zoneCounts           map[string]int
		maxZoneSkew          int
		customNS             string
		customName           string
		want                 bool
	}{
		{
			name:                 "endpoint slices disabled",
			endpointSlicesEnable: false,
			endpointSlicePresent: true,
			zoneCounts:           map[string]int{"a": 3, "b": 3, "c": 3},
			maxZoneSkew:          1,
			want:                 false,
		},
		{
			name:                 "no endpointslice for ingress controller",
			endpointSlicesEnable: true,
			endpointSlicePresent: false,
			zoneCounts:           map[string]int{"a": 3, "b": 3, "c": 3},
			maxZoneSkew:          1,
			want:                 false,
		},
		{
			name:                 "fewer than minZones zones",
			endpointSlicesEnable: true,
			endpointSlicePresent: true,
			zoneCounts:           map[string]int{"a": 3, "b": 3},
			maxZoneSkew:          1,
			want:                 false,
		},
		{
			name:                 "balanced zones, maxSkew=0",
			endpointSlicesEnable: true,
			endpointSlicePresent: true,
			zoneCounts:           map[string]int{"a": 3, "b": 3, "c": 3},
			maxZoneSkew:          0,
			want:                 true,
		},
		{
			name:                 "balanced zones, maxSkew=1",
			endpointSlicesEnable: true,
			endpointSlicePresent: true,
			zoneCounts:           map[string]int{"a": 3, "b": 3, "c": 3},
			maxZoneSkew:          1,
			want:                 true,
		},
		{
			name:                 "skew=1 within maxSkew=1",
			endpointSlicesEnable: true,
			endpointSlicePresent: true,
			zoneCounts:           map[string]int{"a": 4, "b": 3, "c": 3},
			maxZoneSkew:          1,
			want:                 true,
		},
		{
			name:                 "skew=1 exceeds maxSkew=0",
			endpointSlicesEnable: true,
			endpointSlicePresent: true,
			zoneCounts:           map[string]int{"a": 4, "b": 3, "c": 3},
			maxZoneSkew:          0,
			want:                 false,
		},
		{
			name:                 "skew=2 exceeds maxSkew=1",
			endpointSlicesEnable: true,
			endpointSlicePresent: true,
			zoneCounts:           map[string]int{"a": 5, "b": 3, "c": 3},
			maxZoneSkew:          1,
			want:                 false,
		},
		{
			name:                 "skew=2 within maxSkew=2",
			endpointSlicesEnable: true,
			endpointSlicePresent: true,
			zoneCounts:           map[string]int{"a": 5, "b": 3, "c": 3},
			maxZoneSkew:          2,
			want:                 true,
		},
		{
			name:                 "custom namespace and name match",
			endpointSlicesEnable: true,
			endpointSlicePresent: true,
			zoneCounts:           map[string]int{"a": 3, "b": 3, "c": 3},
			maxZoneSkew:          1,
			customNS:             "custom-ns",
			customName:           "custom-ingress",
			want:                 true,
		},
		{
			name:                 "custom namespace and name mismatch",
			endpointSlicesEnable: true,
			endpointSlicePresent: true,
			zoneCounts:           map[string]int{"a": 3, "b": 3, "c": 3},
			maxZoneSkew:          1,
			customNS:             "wrong-ns",
			customName:           "wrong-ingress",
			want:                 false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lookupNS := ns
			lookupName := name
			if tt.customNS != "" {
				lookupNS = tt.customNS
			}
			if tt.customName != "" {
				lookupName = tt.customName
			}

			endpointSlices := map[definitions.ResourceID]*skipperEndpointSlice{}
			if tt.endpointSlicePresent {
				// always store under the canonical ns/name; custom test cases
				// store under the custom key so that the lookup matches.
				storeNS, storeName := ns, name
				if tt.customNS != "" && tt.customNS != "wrong-ns" {
					storeNS = tt.customNS
				}
				if tt.customName != "" && tt.customName != "wrong-ingress" {
					storeName = tt.customName
				}
				endpointSlices[newResourceID(storeNS, storeName)] = makeSkipperEndpointSlice(tt.zoneCounts)
			}

			state := &clusterState{
				enableEndpointSlices: tt.endpointSlicesEnable,
				endpointSlices:       endpointSlices,
			}

			got := state.isServiceBalancedAcrossZones(lookupNS, lookupName, tt.maxZoneSkew)
			if got != tt.want {
				t.Errorf("isServiceBalancedAcrossZones(%q, %q, %d) = %v, want %v",
					lookupNS, lookupName, tt.maxZoneSkew, got, tt.want)
			}
		})
	}
}
