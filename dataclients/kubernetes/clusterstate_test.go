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

// makeService builds a *service with the given namespace/name/labels, with
// no spec/status needed for label-matching purposes.
func makeService(namespace, name string, labels map[string]string) *service {
	return &service{
		Meta: &definitions.Metadata{
			Namespace: namespace,
			Name:      name,
			Labels:    labels,
		},
	}
}

func TestIsServiceBalancedAcrossZones(t *testing.T) {
	defaultSelector := map[string]string{"application": "skipper-ingress", "component": "ingress"}
	defaultLabels := map[string]string{"application": "skipper-ingress", "component": "ingress"}

	for _, tt := range []struct {
		name                 string
		endpointSlicesEnable bool
		services             map[definitions.ResourceID]*service
		endpointSlices       map[definitions.ResourceID]*skipperEndpointSlice
		labelSelector        map[string]string
		maxZoneSkew          int
		want                 bool
	}{
		{
			name:                 "endpoint slices disabled",
			endpointSlicesEnable: false,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "skipper-ingress"): makeService("kube-system", "skipper-ingress", defaultLabels)},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("kube-system", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 3, "b": 3, "c": 3})},
			labelSelector:        defaultSelector,
			maxZoneSkew:          1,
			want:                 false,
		},
		{
			name:                 "no service matches the selector",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "other"): makeService("kube-system", "other", map[string]string{"application": "other"})},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("kube-system", "other"): makeSkipperEndpointSlice(map[string]int{"a": 3, "b": 3, "c": 3})},
			labelSelector:        defaultSelector,
			maxZoneSkew:          1,
			want:                 false,
		},
		{
			name:                 "service matches but has no endpointslice",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "skipper-ingress"): makeService("kube-system", "skipper-ingress", defaultLabels)},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{},
			labelSelector:        defaultSelector,
			maxZoneSkew:          1,
			want:                 false,
		},
		{
			name:                 "fewer than minZones zones",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "skipper-ingress"): makeService("kube-system", "skipper-ingress", defaultLabels)},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("kube-system", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 3, "b": 3})},
			labelSelector:        defaultSelector,
			maxZoneSkew:          1,
			want:                 false,
		},
		{
			name:                 "balanced zones, maxSkew=0",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "skipper-ingress"): makeService("kube-system", "skipper-ingress", defaultLabels)},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("kube-system", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 3, "b": 3, "c": 3})},
			labelSelector:        defaultSelector,
			maxZoneSkew:          0,
			want:                 true,
		},
		{
			name:                 "balanced zones, maxSkew=1",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "skipper-ingress"): makeService("kube-system", "skipper-ingress", defaultLabels)},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("kube-system", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 3, "b": 3, "c": 3})},
			labelSelector:        defaultSelector,
			maxZoneSkew:          1,
			want:                 true,
		},
		{
			name:                 "skew=1 within maxSkew=1",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "skipper-ingress"): makeService("kube-system", "skipper-ingress", defaultLabels)},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("kube-system", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 4, "b": 3, "c": 3})},
			labelSelector:        defaultSelector,
			maxZoneSkew:          1,
			want:                 true,
		},
		{
			name:                 "skew=1 exceeds maxSkew=0",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "skipper-ingress"): makeService("kube-system", "skipper-ingress", defaultLabels)},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("kube-system", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 4, "b": 3, "c": 3})},
			labelSelector:        defaultSelector,
			maxZoneSkew:          0,
			want:                 false,
		},
		{
			name:                 "skew=2 exceeds maxSkew=1",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "skipper-ingress"): makeService("kube-system", "skipper-ingress", defaultLabels)},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("kube-system", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 5, "b": 3, "c": 3})},
			labelSelector:        defaultSelector,
			maxZoneSkew:          1,
			want:                 false,
		},
		{
			name:                 "skew=2 within maxSkew=2",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "skipper-ingress"): makeService("kube-system", "skipper-ingress", defaultLabels)},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("kube-system", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 5, "b": 3, "c": 3})},
			labelSelector:        defaultSelector,
			maxZoneSkew:          2,
			want:                 true,
		},
		{
			name:                 "custom selector matches a differently-named/namespaced service",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("custom-ns", "custom-ingress"): makeService("custom-ns", "custom-ingress", map[string]string{"team": "platform"})},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("custom-ns", "custom-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 3, "b": 3, "c": 3})},
			labelSelector:        map[string]string{"team": "platform"},
			maxZoneSkew:          1,
			want:                 true,
		},
		{
			name:                 "selector requires all labels to match (AND semantics)",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "skipper-ingress"): makeService("kube-system", "skipper-ingress", map[string]string{"application": "skipper-ingress"})}, // missing "component"
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("kube-system", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 3, "b": 3, "c": 3})},
			labelSelector:        defaultSelector,
			maxZoneSkew:          1,
			want:                 false,
		},
		{
			name:                 "empty selector matches no service",
			endpointSlicesEnable: true,
			services:             map[definitions.ResourceID]*service{newResourceID("kube-system", "skipper-ingress"): makeService("kube-system", "skipper-ingress", defaultLabels)},
			endpointSlices:       map[definitions.ResourceID]*skipperEndpointSlice{newResourceID("kube-system", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 3, "b": 3, "c": 3})},
			labelSelector:        map[string]string{},
			maxZoneSkew:          1,
			want:                 false,
		},
		{
			name:                 "two services matching the selector are aggregated into one balanced pool",
			endpointSlicesEnable: true,
			services: map[definitions.ResourceID]*service{
				newResourceID("region-a", "skipper-ingress"): makeService("region-a", "skipper-ingress", defaultLabels),
				newResourceID("region-b", "skipper-ingress"): makeService("region-b", "skipper-ingress", defaultLabels),
			},
			endpointSlices: map[definitions.ResourceID]*skipperEndpointSlice{
				newResourceID("region-a", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 2}),
				newResourceID("region-b", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"b": 2, "c": 2}),
			},
			labelSelector: defaultSelector,
			maxZoneSkew:   1,
			want:          true, // combined: a=2, b=2, c=2 -> balanced
		},
		{
			name:                 "two matched services combine into an unbalanced pool even though each alone looks fine",
			endpointSlicesEnable: true,
			services: map[definitions.ResourceID]*service{
				newResourceID("region-a", "skipper-ingress"): makeService("region-a", "skipper-ingress", defaultLabels),
				newResourceID("region-b", "skipper-ingress"): makeService("region-b", "skipper-ingress", defaultLabels),
			},
			endpointSlices: map[definitions.ResourceID]*skipperEndpointSlice{
				newResourceID("region-a", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 10, "b": 1, "c": 1}),
				newResourceID("region-b", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 1, "b": 1, "c": 1}),
			},
			labelSelector: defaultSelector,
			maxZoneSkew:   1,
			want:          false, // combined: a=11, b=2, c=2 -> skew of 9 exceeds maxZoneSkew=1
		},
		{
			name:                 "one matched service has no endpointslice but another does: the one with data still counts",
			endpointSlicesEnable: true,
			services: map[definitions.ResourceID]*service{
				newResourceID("region-a", "skipper-ingress"): makeService("region-a", "skipper-ingress", defaultLabels),
				newResourceID("region-b", "skipper-ingress"): makeService("region-b", "skipper-ingress", defaultLabels),
			},
			endpointSlices: map[definitions.ResourceID]*skipperEndpointSlice{
				newResourceID("region-a", "skipper-ingress"): makeSkipperEndpointSlice(map[string]int{"a": 3, "b": 3, "c": 3}),
				// region-b has no endpointslice entry at all
			},
			labelSelector: defaultSelector,
			maxZoneSkew:   1,
			want:          true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := &clusterState{
				enableEndpointSlices: tt.endpointSlicesEnable,
				services:             tt.services,
				endpointSlices:       tt.endpointSlices,
			}

			got := state.isServiceBalancedAcrossZones(tt.labelSelector, tt.maxZoneSkew)
			if got != tt.want {
				t.Errorf("isServiceBalancedAcrossZones(%v, %d) = %v, want %v",
					tt.labelSelector, tt.maxZoneSkew, got, tt.want)
			}
		})
	}
}
