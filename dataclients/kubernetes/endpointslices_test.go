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
