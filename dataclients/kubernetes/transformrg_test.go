package kubernetes

import (
	"testing"

	"github.com/zalando/skipper/eskip"
)

type stringClient string

func (c stringClient) loadRouteGroups() ([]byte, error) {
	return []byte(c), nil
}

func TestTransformRouteGroups(t *testing.T) {
	const allRouteGroupsJSON = `{"routeGroups": []}`

	dc, err := NewRouteGroupClient(RouteGroupsOptions{
		Kubernetes: Options{},
		apiClient:  stringClient(allRouteGroupsJSON),
	})
	if err != nil {
		t.Fatal(err)
	}

	r, err := dc.LoadAll()
	if err != nil {
		t.Error("data client failed to convert an empty route group document", err)
	}

	if len(r) != 0 {
		t.Error("data client returned unexpected routes")
		t.Log(eskip.String(r...))
	}
}
