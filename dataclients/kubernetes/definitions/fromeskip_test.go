package definitions

import (
	"reflect"
	"sort"
	"testing"

	"github.com/go-yaml/yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/eskip"
)

const fromEskipExpectNoRoutes = `apiVersion: zalando.org/v1
kind: RouteGroup
metadata: {}
spec: {}`

const fromEskipEmptyShunt = `apiVersion: zalando.org/v1
kind: RouteGroup
metadata: {}
spec:
  backends:
  - name: backend0
    type: shunt
  routes:
  - backends:
    - backendName: backend0`

const fromEskipExpectNotCanonical = `apiVersion: zalando.org/v1
kind: RouteGroup
metadata: {}
spec:
  backends:
  - name: backend0
    type: shunt
  routes:
  - backends:
    - backendName: backend0
    predicates:
    - Header("X-Test", "foo")`

const fromEskipServiceBackend = `apiVersion: zalando.org/v1
kind: RouteGroup
metadata: {}
spec:
  backends:
  - name: backend0
    serviceName: foo
    servicePort: 80
  routes:
  - backends:
    - backendName: backend0`

const fromEskipPredicatesAndFilters = `apiVersion: zalando.org/v1
kind: RouteGroup
metadata: {}
spec:
  backends:
  - name: backend0
    type: service
    serviceName: my-service
    servicePort: 80
  defaultBackends:
  - backendName: backend0
  routes:
  - path: /foo
    predicates:
    - Foo(42)
    filters:
    - foo(42)
  - path: /bar
    predicates:
    - Bar(42)
    filters:
    - bar(42)`

// currently only what is enough for the tests
func sortRouteGroupFields(rg *RouteGroupItem) {
	sort.Slice(rg.Spec.Backends, func(i, j int) bool {
		return rg.Spec.Backends[i].Name < rg.Spec.Backends[j].Name
	})

	sort.Slice(rg.Spec.DefaultBackends, func(i, j int) bool {
		return rg.Spec.DefaultBackends[i].BackendName <
			rg.Spec.DefaultBackends[j].BackendName
	})

	sort.Slice(rg.Spec.Routes, func(i, j int) bool {
		if rg.Spec.Routes[i].Path < rg.Spec.Routes[j].Path {
			return true
		}

		if rg.Spec.Routes[i].PathSubtree < rg.Spec.Routes[j].PathSubtree {
			return true
		}

		if rg.Spec.Routes[i].PathRegexp < rg.Spec.Routes[j].PathRegexp {
			return true
		}

		return false
	})
}

func expectNumberOfBackendNamesAndTypes(nameCount, typeCount int) func(
	*testing.T,
	*RouteGroupItem,
	error,
) {
	return func(t *testing.T, rg *RouteGroupItem, err error) {
		if err != nil {
			t.Fatal(err)
		}

		names := make(map[string]struct{})
		types := make(map[eskip.BackendType]struct{})
		for _, b := range rg.Spec.Backends {
			names[b.Name] = struct{}{}
			types[b.Type] = struct{}{}
		}

		if len(names) != nameCount {
			t.Fatalf(
				"invalid number of unique names, expected: %d, got: %d",
				nameCount,
				len(names),
			)
		}

		if len(types) != typeCount {
			t.Fatalf(
				"invalid number of types, expected: %d, got: %d",
				typeCount,
				len(types),
			)
		}
	}
}

func TestFromEskip(t *testing.T) {
	for _, test := range []struct {
		title      string
		eskip      string
		routes     []*eskip.Route
		expect     string
		expectErr  bool
		expectFunc func(*testing.T, *RouteGroupItem, error)
	}{{
		title:  "no routes",
		expect: fromEskipExpectNoRoutes,
	}, {
		title: "not canonical",
		routes: []*eskip.Route{{
			Headers:     map[string]string{"X-Test": "foo"},
			BackendType: eskip.ShuntBackend,
		}},
		expect: fromEskipExpectNotCanonical,
	}, {
		title: "unique",
		eskip: `
			foo: * -> <shunt>;
			foo: * -> <shunt>;
		`,
		expect: fromEskipEmptyShunt,
	}, {
		title: "backend names and types",
		eskip: `
			shunt: * -> <shunt>;
			loopback: * -> <loopback>;
			dynamic: * -> <dynamic>;
			network: * -> "https://www.example.org";
			lb: * -> <"http://10.0.0.1", "http://10.0.0.2">;
			service: * -> "service://service1:8080";
		`,
		expectFunc: expectNumberOfBackendNamesAndTypes(6, 6),
	}, {
		title: "detect identical backends",
		eskip: `
			shunt0: * -> <shunt>;
			shunt1: * -> <shunt>;
			loopback0: * -> <loopback>;
			loopback1: * -> <loopback>;
			dynamic0: * -> <dynamic>;
			dynamic1: * -> <dynamic>;
			lb0: * -> <"http://10.0.0.1:8080", "http://10.0.0.2:8080">;
			lb1: * -> <"http://10.0.0.1:8080", "http://10.0.0.2:8080">;
			network0: * -> "https://www.example.org";
			network1: * -> "https://www.example.org";
			service0: * -> "service://service:80";
			service1: * -> "service://service:80";
		`,
		expectFunc: expectNumberOfBackendNamesAndTypes(6, 6),
	}, {
		title: "invalid algorithm",
		routes: []*eskip.Route{{
			BackendType: eskip.LBBackend,
			LBAlgorithm: "foo",
			LBEndpoints: []string{
				"http://10.0.0.1:8080",
				"http://10.0.0.2:8080",
			},
		}},
		expectErr: true,
	}, {
		title: "invalid backend address",
		routes: []*eskip.Route{{
			Backend: string(' ' - 1),
		}},
		expectErr: true,
	}, {
		title: "invalid service port",
		routes: []*eskip.Route{{
			Backend: "service://service1:foo",
		}},
		expectErr: true,
	}, {
		title: "set backend",
		eskip: `
			* -> "service://foo:80"
		`,
		expect: fromEskipServiceBackend,
	}, {
		title: "invalid path",
		routes: []*eskip.Route{{
			Predicates:  []*eskip.Predicate{{Name: "Path", Args: []interface{}{42}}},
			BackendType: eskip.ShuntBackend,
		}},
		expectErr: true,
	}, {
		title: "invalid path subtree",
		routes: []*eskip.Route{{
			Predicates:  []*eskip.Predicate{{Name: "PathSubtree", Args: []interface{}{42}}},
			BackendType: eskip.ShuntBackend,
		}},
		expectErr: true,
	}, {
		title: "invalid path regexp",
		routes: []*eskip.Route{{
			Predicates:  []*eskip.Predicate{{Name: "PathRegexp", Args: []interface{}{42}}},
			BackendType: eskip.ShuntBackend,
		}},
		expectErr: true,
	}, {
		title: "invalid method",
		routes: []*eskip.Route{{
			Predicates:  []*eskip.Predicate{{Name: "Method", Args: []interface{}{42}}},
			BackendType: eskip.ShuntBackend,
		}},
		expectErr: true,
	}, {
		title: "invalid methods",
		routes: []*eskip.Route{{
			Predicates:  []*eskip.Predicate{{Name: "Methods", Args: []interface{}{"GET", 42}}},
			BackendType: eskip.ShuntBackend,
		}},
		expectErr: true,
	}, {
		title: "valid predicates and filters",
		eskip: `
			foo: Path("/foo") && Foo(42) -> foo(42) -> "service://my-service:80";
			bar: Path("/bar") && Bar(42) -> bar(42) -> "service://my-service:80";
		`,
		expect: fromEskipPredicatesAndFilters,
	}} {
		t.Run(test.title, func(t *testing.T) {
			var expect RouteGroupItem
			if err := yaml.Unmarshal([]byte(test.expect), &expect); err != nil {
				t.Fatal(err)
			}

			r, err := eskip.Parse(test.eskip)
			if err != nil {
				t.Fatal(err)
			}

			r = append(r, test.routes...)
			rg, err := FromEskip(r)
			if test.expectFunc != nil {
				test.expectFunc(t, rg, err)
				return
			}

			if err != nil {
				if test.expectErr {
					t.Log(err)
					return
				}

				t.Fatal(err)
			}

			if test.expectErr {
				t.Fatal("Failed to fail.")
			}

			sortRouteGroupFields(rg)
			sortRouteGroupFields(&expect)
			if !reflect.DeepEqual(rg, &expect) {
				t.Log("Failed to convert eskip objects to the expected routegroup.")
				t.Log(cmp.Diff(rg, &expect, cmp.AllowUnexported(SkipperBackend{})))
				t.Fatal()
			}
		})
	}
}
