package kubernetes

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/eskip"
)

type transformationTest struct {
	title          string
	routeGroupJSON string
	expectedRoutes string
}

var transTests []transformationTest = []transformationTest{
	{
		title:          "empty doc",
		routeGroupJSON: `{"items": []}`,
	},
	{
		title: "simplest host Route Group",
		routeGroupJSON: `{"items": [
{
  "apiVersion": "zalando.org/v1",
  "kind": "RouteGroup",
  "metadata": {
    "name": "my-routes",
    "namespace": "default"
  },
  "spec": {
    "hosts": [
      "foo.example.org"
    ],
    "backends": [
      {
        "name": "be1",
        "type": "shunt"
      }
    ],
    "defaultBackends": [
      {
        "backendName": "be1"
      }
    ]
  }
}
]}`,
		expectedRoutes: `kube__rg__default__my_routes_____0:
		Host("^(foo.example.org)$") -> <shunt>`,
	},
	/*	{
				title: "single Kubernetes RouteGroup",
				routeGroupJSON: `{"items": [
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
		    "routes": [
		      {
		        "path": "/"
		      }
		    ]
		  }
		}
		]}`,
				expectedRoutes: `kube__rg__default__my_routes_____0:
				Host("^(foo.example.org)$") && Path("/") -> "http://foo-service"`,
			},*/
}

type stringClient string

func (c stringClient) loadRouteGroups() ([]byte, error) {
	return []byte(c), nil
}

func TestTransformRouteGroups(t *testing.T) {
	for _, test := range transTests {
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
				t.Fatal("Failed to convert route group document:", err)
			}

			exp, err := eskip.Parse(test.expectedRoutes)
			if err != nil {
				t.Fatal("Failed to parse expected routes:", err)
			}

			if len(r) != len(exp) {
				t.Fatalf("Failed to get number of routes expected %d, got %d", len(exp), len(r))
				t.Log(cmp.Diff(eskip.CanonicalList(r), eskip.CanonicalList(exp)))
			}

			if !eskip.EqLists(r, exp) {
				t.Error("Failed to convert the route groups to the right routes.")
				t.Log(cmp.Diff(eskip.CanonicalList(r), eskip.CanonicalList(exp)))
			}
		})
	}
}
