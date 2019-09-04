package kubernetes

import (
	"testing"

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
}]}
`

type stringClient string

func (c stringClient) loadRouteGroups() ([]byte, error) {
	return []byte(c), nil
}

func TestTransformRouteGroups(t *testing.T) {
	t.Skip()

	t.Run("empty doc", func(t *testing.T) {
		const allRouteGroupsJSON = `{"items": []}`

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
	})

	t.Run("single route gropu", func(t *testing.T) {
		dc, err := NewRouteGroupClient(RouteGroupsOptions{
			Kubernetes: Options{},
			apiClient:  stringClient(singleRouteGroup),
		})
		if err != nil {
			t.Fatal(err)
		}

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("data client failed to convert route groups", err)
		}

		if len(r) != 1 {
			t.Error("failed to transform a single route")
		}
	})
}
