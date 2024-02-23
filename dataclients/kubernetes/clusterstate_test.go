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
						{IP: "192.168.0.1", NodeName: "node1"},
						{IP: "192.168.0.2", NodeName: "node2"},
						{IP: "192.168.0.3", NodeName: "node3"},
						{IP: "192.168.0.4", NodeName: "node4"},
						{IP: "192.168.0.5", NodeName: "node5"},
						{IP: "192.168.0.6", NodeName: "node6"},
						{IP: "192.168.0.7", NodeName: "node7"},
						{IP: "192.168.0.8", NodeName: "node8"},
						{IP: "192.168.0.9", NodeName: "node9"},
						{IP: "192.168.0.10", NodeName: "node10"},
						{IP: "192.168.0.11", NodeName: "node11"},
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
