package definitions

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/loadbalancer"
	"gopkg.in/yaml.v2"
)

func TestRouteGroupYAML(t *testing.T) {
	rg := RouteGroupItem{
		Metadata: &Metadata{
			Namespace: "foo",
			Name:      "bar",
		},
		Spec: &RouteGroupSpec{
			Hosts: []string{
				"one.example.org",
				"two.example.org",
			},
			Backends: []*SkipperBackend{{
				Name:      "backend0",
				Type:      eskip.LBBackend,
				Algorithm: loadbalancer.Random,
				Endpoints: []string{
					"10.0.0.1:8080",
					"10.0.0.2:8080",
				},
			}, {
				Name:        "backend1-0",
				Type:        ServiceBackend,
				Algorithm:   loadbalancer.RoundRobin,
				ServiceName: "service1-0",
				ServicePort: 80,
			}, {
				Name:        "backend1-1",
				Type:        ServiceBackend,
				Algorithm:   loadbalancer.RoundRobin,
				ServiceName: "service1-1",
				ServicePort: 80,
			}, {
				Name:    "backend2",
				Type:    eskip.NetworkBackend,
				Address: "http://service1.cluster.local",
			}},
			DefaultBackends: []*BackendReference{{
				BackendName: "backend1-0",
				Weight:      3,
			}, {
				BackendName: "backend1-1",
				Weight:      6,
			}},
			Routes: []*RouteSpec{{
				Path: "/foo",
				Backends: []*BackendReference{{
					BackendName: "backend2",
				}},
			}, {
				PathSubtree: "/bar",
				PathRegexp:  "^/bar/[^/]+$",
				Methods: []string{
					"GET",
					"POST",
				},
				Predicates: []string{
					"Foo()",
					"Bar()",
				},
				Filters: []string{
					"baz()",
					"qux()",
				},
			}},
		},
	}

	const y = `metadata:
  namespace: foo
  name: bar
spec:
  hosts:
  - one.example.org
  - two.example.org
  backends:
  - algorithm: random
    endpoints:
    - 10.0.0.1:8080
    - 10.0.0.2:8080
    name: backend0
    type: lb
  - name: backend1-0
    serviceName: service1-0
    servicePort: 80
    type: service
  - name: backend1-1
    serviceName: service1-1
    servicePort: 80
    type: service
  - address: http://service1.cluster.local
    name: backend2
    type: network
  defaultBackends:
  - backendName: backend1-0
    weight: 3
  - backendName: backend1-1
    weight: 6
  routes:
  - path: /foo
    backends:
    - backendName: backend2
  - pathSubtree: /bar
    pathRegexp: ^/bar/[^/]+$
    filters:
    - baz()
    - qux()
    predicates:
    - Foo()
    - Bar()
    methods:
    - GET
    - POST`

	marshaled, err := yaml.Marshal(rg)
	if err != nil {
		t.Fatal(err)
	}

	var unmarshaledBack RouteGroupItem
	if err := yaml.Unmarshal(marshaled, &unmarshaledBack); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(rg, unmarshaledBack) {
		t.Log("Failed to marshal and unmarshal into the same routegroup")
		t.Log(cmp.Diff(rg, unmarshaledBack, cmp.AllowUnexported(SkipperBackend{})))
		t.Fatal()
	}

	var unmarshaled RouteGroupItem
	if err := yaml.Unmarshal([]byte(y), &unmarshaled); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(rg, unmarshaled) {
		t.Log("Failed to unmarshal into the right routegroup")
		t.Log(cmp.Diff(unmarshaled, rg, cmp.AllowUnexported(SkipperBackend{})))
		t.Fatal()
	}
}
