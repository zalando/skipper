package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddresses(t *testing.T) {
	assert.Equal(t, []string{"10.0.0.1"}, (&skipperEndpointSlice{
		Endpoints: []*skipperEndpoint{
			{
				Address: "10.0.0.1",
				Zone:    "zone-1",
			},
		},
		Ports: []*endpointSlicePort{
			{
				Name:     "main",
				Port:     8080,
				Protocol: "TCP",
			},
		},
	}).addresses())

	assert.Equal(t, []string{"10.0.0.1"}, (&skipperEndpointSlice{
		Endpoints: []*skipperEndpoint{
			{
				Address: "10.0.0.1",
				Zone:    "zone-1",
			},
		},
		Ports: []*endpointSlicePort{
			{
				Name:     "main",
				Port:     8080,
				Protocol: "TCP",
			},
			{
				Name:     "support",
				Port:     8081,
				Protocol: "TCP",
			},
		},
	}).addresses())

	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, (&skipperEndpointSlice{
		Endpoints: []*skipperEndpoint{
			{
				Address: "10.0.0.1",
				Zone:    "zone-1",
			},
			{
				Address: "10.0.0.2",
				Zone:    "zone-2",
			},
		},
		Ports: []*endpointSlicePort{
			{
				Name:     "main",
				Port:     8080,
				Protocol: "TCP",
			},
		},
	}).addresses())

	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, (&skipperEndpointSlice{
		Endpoints: []*skipperEndpoint{
			{
				Address: "10.0.0.1",
				Zone:    "zone-1",
			},
			{
				Address: "10.0.0.2",
				Zone:    "zone-2",
			},
		},
		Ports: []*endpointSlicePort{
			{
				Name:     "main",
				Port:     8080,
				Protocol: "TCP",
			},
			{
				Name:     "support",
				Port:     8081,
				Protocol: "TCP",
			},
		},
	}).addresses())
}

func TestAddressesByZone(t *testing.T) {
	for _, tt := range []struct {
		name      string
		endpoints []*skipperEndpoint
		zone      string
		want      []string
	}{
		{
			name:      "empty endpoints slice returns empty",
			endpoints: []*skipperEndpoint{},
			zone:      "zone-1",
			want:      []string{},
		},
		{
			name: "single matching zone returns only target address",
			endpoints: []*skipperEndpoint{
				{Address: "10.0.0.1", Zone: "zone-1"},
				{Address: "10.0.0.2", Zone: "zone-2"},
			},
			zone: "zone-1",
			want: []string{"10.0.0.1"},
		},
		{
			name: "multiple matching endpoints in target zone preserves order",
			endpoints: []*skipperEndpoint{
				{Address: "10.0.0.1", Zone: "zone-1"},
				{Address: "10.0.0.2", Zone: "zone-2"},
				{Address: "10.0.0.3", Zone: "zone-1"},
			},
			zone: "zone-1",
			want: []string{"10.0.0.1", "10.0.0.3"},
		},
		{
			name: "non-matching zone returns empty slice",
			endpoints: []*skipperEndpoint{
				{Address: "10.0.0.1", Zone: "zone-1"},
				{Address: "10.0.0.2", Zone: "zone-2"},
			},
			zone: "zone-3",
			want: []string{},
		},
		{
			name: "empty zone query matches only endpoints with empty zone",
			endpoints: []*skipperEndpoint{
				{Address: "10.0.0.1", Zone: "zone-1"},
				{Address: "10.0.0.2", Zone: ""},
			},
			zone: "",
			want: []string{"10.0.0.2"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			eps := &skipperEndpointSlice{Endpoints: tt.endpoints}
			got := eps.addressesByZone(tt.zone)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEndpointSliceEndpointIsReady(t *testing.T) {
	ready := true
	notReady := false
	terminating := true

	for _, tt := range []struct {
		name       string
		conditions *endpointsliceCondition
		want       bool
	}{
		{
			name: "nil conditions default to ready",
			want: true,
		},
		{
			name:       "nil ready condition defaults to ready",
			conditions: &endpointsliceCondition{},
			want:       true,
		},
		{
			name:       "ready true",
			conditions: &endpointsliceCondition{Ready: &ready},
			want:       true,
		},
		{
			name:       "ready false",
			conditions: &endpointsliceCondition{Ready: &notReady},
			want:       false,
		},
		{
			name:       "terminating overrides ready",
			conditions: &endpointsliceCondition{Terminating: &terminating},
			want:       false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ep := &EndpointSliceEndpoints{Conditions: tt.conditions}
			if got := ep.isReady(); got != tt.want {
				t.Fatalf("isReady() = %v, want = %v", got, tt.want)
			}
		})
	}
}
