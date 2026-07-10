package kubernetes

import (
	"reflect"
	"testing"
)

func TestSplitHosts(t *testing.T) {
	tests := []struct {
		name                  string
		hosts                 []string
		domains               []string
		expectedInternalHosts []string
		expectedExternalHosts []string
	}{
		{
			name: "single internal domain",
			hosts: []string{
				"api.example.org",
				"batch.skipper.cluster.local",
			},
			domains: []string{
				"skipper.cluster.local",
			},
			expectedInternalHosts: []string{
				"batch.skipper.cluster.local",
			},
			expectedExternalHosts: []string{
				"api.example.org",
			},
		},
		{
			name: "multiple internal domains",
			hosts: []string{
				"api.example.org",
				"batch.skipper.cluster.local",
				"single-get.internal.cluster.local",
			},
			domains: []string{
				"skipper.cluster.local",
				"internal.cluster.local",
			},
			expectedInternalHosts: []string{
				"batch.skipper.cluster.local",
				"single-get.internal.cluster.local",
			},
			expectedExternalHosts: []string{
				"api.example.org",
			},
		},
		{
			name: "no internal domains",
			hosts: []string{
				"api.example.org",
				"batch.example.org",
			},
			domains:               nil,
			expectedInternalHosts: []string{},
			expectedExternalHosts: []string{
				"api.example.org",
				"batch.example.org",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			internalHosts, externalHosts := splitHosts(test.hosts, test.domains)

			if !reflect.DeepEqual(internalHosts, test.expectedInternalHosts) {
				t.Fatalf("unexpected internal hosts: got %v, want %v", internalHosts, test.expectedInternalHosts)
			}
			if !reflect.DeepEqual(externalHosts, test.expectedExternalHosts) {
				t.Fatalf("unexpected external hosts: got %v, want %v", externalHosts, test.expectedExternalHosts)
			}
		})
	}
}
