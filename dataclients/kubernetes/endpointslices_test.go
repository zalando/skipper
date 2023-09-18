package kubernetes

import (
	"net/url"
	"strconv"
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

func TestTargets(t *testing.T) {
	want := "http://10.0.0.1:8080"
	u, err := url.Parse(want)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	ses := &skipperEndpointSlice{
		Meta: &definitions.Metadata{
			Namespace: "ns1",
			Name:      "a-slice",
		},
		Endpoints: []*skipperEndpoint{
			{
				Address: u.Hostname(),
				Zone:    "zone-1",
			},
		},
		Ports: []*endpointSlicePort{
			{
				Name:     "main",
				Port:     port,
				Protocol: "TCP",
			},
		},
	}
	res := ses.targets("TCP", "http")
	if l := len(res); l != 1 {
		t.Fatalf("Failed to get same number of results than expected %d, got: %d", 1, l)
	}

	for i := 0; i < len(res); i++ {
		if want != res[i] {
			t.Fatalf("Failed to get the right target: %s != %s", want, res[i])
		}
	}
}
