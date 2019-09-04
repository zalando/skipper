package kubernetes

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/eskip"
)

const singleRouteGroup = `
{"items": [
{
  "apiVersion": "zalando.org/v1",
  "kind": "RouteGroup",
  "metadata": {
    "name": "my-routes"
  },
  "spec": {
    "hosts": [
      "foo.example.org"
    ],
    "backends": [
      {
        "serviceName": "foo-service",
        "servicePort": 80
      }
    ],
    "paths": [
      {
        "path": "/"
      }
    ]
  }
}
]}
`

const singleRouteGroupResult = `
	kube__rg__default__my_routes_____0:
		Host("^(foo.example.org)$") && Path("/") -> "http://foo-service"
`

type stringClient string

func (c stringClient) loadRouteGroups() ([]byte, error) {
	return []byte(c), nil
}

func TestTransformRouteGroups(t *testing.T) {
	for _, test := range []struct {
		title          string
		routeGroupJSON string
		expectedRoutes string
	}{{
		title:          "empty doc",
		routeGroupJSON: `{"items": []}`,
	}, {
		title:          "single route group",
		routeGroupJSON: singleRouteGroup,
		expectedRoutes: singleRouteGroupResult,
	}} {
		t.Run(test.title, func(t *testing.T) {
			dc, err := NewRouteGroupClient(RouteGroupsOptions{
				Kubernetes: Options{},
				apiClient:  stringClient(test.routeGroupJSON),
			})
			if err != nil {
				t.Fatal(err)
			}

			r, err := dc.LoadAll()
			if err != nil {
				t.Error("Failed to convert route group document:", err)
			}

			exp, err := eskip.Parse(test.expectedRoutes)
			if err != nil {
				t.Error("Failed to parse expected routes:", err)
			}

			if !eskip.EqLists(r, exp) {
				t.Error("Failed to convert the route groups to the right routes:", err)
				t.Log(cmp.Diff(r, exp))
			}
		})
	}
}
