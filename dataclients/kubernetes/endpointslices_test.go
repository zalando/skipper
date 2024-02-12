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
