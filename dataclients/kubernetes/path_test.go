package kubernetes

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
)

func findPathPredicate(r *eskip.Route, name string) (*eskip.Predicate, error) {
	var preds []*eskip.Predicate
	for _, p := range r.Predicates {
		if p.Name == name {
			preds = append(preds, p)
		}
	}

	if len(preds) > 1 {
		return nil, fmt.Errorf("multiple predicates of the same name: %d %s", len(preds), name)
	}

	if len(preds) == 1 {
		return preds[0], nil
	}

	return nil, nil
}

func findRouteWithRx(r []*eskip.Route) *eskip.Route {
	for _, ri := range r {
		if len(ri.PathRegexps) == 1 {
			return ri
		}
	}

	return nil
}

func findRouteWithPathPrefix(r []*eskip.Route) (*eskip.Route, error) {
	for _, ri := range r {
		p, err := findPathPredicate(ri, "PathSubtree")
		if err != nil {
			return nil, err
		}

		if p != nil {
			return ri, nil
		}
	}

	return nil, nil
}

func findRouteWithExactPath(r []*eskip.Route) (*eskip.Route, error) {
	for _, ri := range r {
		p, err := findPathPredicate(ri, "Path")
		if err != nil {
			return nil, err
		}

		if p != nil && ri.Path != "" {
			return nil, errors.New("route with duplicate path predicate found")
		}

		if p != nil || ri.Path != "" {
			return ri, nil
		}
	}

	return nil, nil
}

func TestPathMatchingModes(t *testing.T) {
	s := testServices()
	api := newTestAPI(t, s, &ingressList{})
	defer api.Close()

	setIngressWithPath := func(p string, annotations ...string) {
		i := testIngress(
			"namespace1", "ingress1", "service1", "", "", "", "", "", "", backendPort{8080}, 1.0,
			testRule("www.example.org", testPathRule(p, "service1", backendPort{8080})),
		)

		annotation := strings.Join(annotations, " && ")
		if len(annotations) > 0 {
			i.Metadata.Annotations[skipperpredicateAnnotationKey] = annotation
		}

		api.ingresses.Items = []*ingressItem{i}
	}

	loadRoutes := func(m PathMode) ([]*eskip.Route, error) {
		c, err := New(Options{
			KubernetesURL: api.server.URL,
			PathMode:      m,
		})
		if err != nil {
			return nil, err
		}

		defer c.Close()
		return c.LoadAll()
	}

	t.Run("default", func(t *testing.T) {
		setIngressWithPath("/foo")
		r, err := loadRoutes(KubernetesIngressMode)
		if err != nil {
			t.Fatal(err)
		}

		routeWithRx := findRouteWithRx(r)
		if routeWithRx == nil {
			t.Fatal("route with path regexp not found")
		}

		if routeWithRx.PathRegexps[0] != "^/foo" {
			t.Error("invalid path regexp value", routeWithRx.PathRegexps[0])
		}
	})

	t.Run("regexp", func(t *testing.T) {
		setIngressWithPath("^/foo")
		r, err := loadRoutes(PathRegexp)
		if err != nil {
			t.Fatal(err)
		}

		routeWithRx := findRouteWithRx(r)
		if routeWithRx == nil {
			t.Fatal("route with path regexp not found")
		}

		if routeWithRx.PathRegexps[0] != "^/foo" {
			t.Error("invalid path regexp value", routeWithRx.PathRegexps[0])
		}
	})

	t.Run("path prefix", func(t *testing.T) {
		setIngressWithPath("/foo")
		r, err := loadRoutes(PathPrefix)
		if err != nil {
			t.Fatal(err)
		}

		routeWithPathPrefix, err := findRouteWithPathPrefix(r)
		if err != nil {
			t.Fatal(err)
		}

		if routeWithPathPrefix == nil {
			t.Fatal("route with path prefix not found")
		}

		p, err := findPathPredicate(routeWithPathPrefix, "PathSubtree")
		if err != nil {
			t.Fatal(err)
		}

		if p.Args[0] != "/foo" {
			t.Error("invalid path prefix value", p.Args[0])
		}
	})

	t.Run("additional exact path from annotation", func(t *testing.T) {
		extendableModes := []PathMode{KubernetesIngressMode, PathRegexp}

		for _, mode := range extendableModes {
			t.Run(mode.String(), func(t *testing.T) {
				prx := "/foo"
				if mode == PathRegexp {
					prx = "^/foo"
				}

				setIngressWithPath(prx, "Path(\"/bar\")")
				r, err := loadRoutes(mode)
				if err != nil {
					t.Fatal(err)
				}

				routeWithRx := findRouteWithRx(r)
				if routeWithRx == nil {
					t.Fatal("route with path regexp not found")
				}

				if routeWithRx.PathRegexps[0] != "^/foo" {
					t.Error("invalid path regexp value", routeWithRx.PathRegexps[0])
				}

				p, err := findPathPredicate(routeWithRx, "Path")
				if err != nil {
					t.Fatal(err)
				}

				if p == nil || p.Args[0] != "/bar" {
					t.Error("missing or invalid exact path value")
				}
			})
		}
	})

	t.Run("additional path prefix from annotation", func(t *testing.T) {
		extendableModes := []PathMode{KubernetesIngressMode, PathRegexp}

		for _, mode := range extendableModes {
			t.Run(mode.String(), func(t *testing.T) {
				prx := "/foo"
				if mode == PathRegexp {
					prx = "^/foo"
				}

				setIngressWithPath(prx, "PathSubtree(\"/bar\")")
				r, err := loadRoutes(mode)
				if err != nil {
					t.Fatal(err)
				}

				routeWithRx := findRouteWithRx(r)
				if routeWithRx == nil {
					t.Fatal("route with path regexp not found")
				}

				if routeWithRx.PathRegexps[0] != "^/foo" {
					t.Error("invalid path regexp value", routeWithRx.PathRegexps[0])
				}

				p, err := findPathPredicate(routeWithRx, "PathSubtree")
				if err != nil {
					t.Fatal(err)
				}

				if p == nil || p.Args[0] != "/bar" {
					t.Error("missing or invalid path prefix value")
				}
			})
		}
	})

	t.Run("overriding with exact path from annotation", func(t *testing.T) {
		setIngressWithPath("/foo", "Path(\"/bar\")")
		r, err := loadRoutes(PathPrefix)
		if err != nil {
			t.Fatal(err)
		}

		routeWithExactPath, err := findRouteWithExactPath(r)
		if err != nil {
			t.Fatal(err)
		}

		if routeWithExactPath == nil {
			t.Fatal("route with path regexp not found")
		}

		p, err := findPathPredicate(routeWithExactPath, "Path")
		if err != nil {
			t.Fatal(err)
		}

		if p == nil || p.Args[0] != "/bar" {
			t.Error("missing or invalid exact path value")
		}
	})

	t.Run("overriding with path prefix from annotation", func(t *testing.T) {
		setIngressWithPath("/foo", "PathSubtree(\"/bar\")")
		r, err := loadRoutes(PathPrefix)
		if err != nil {
			t.Fatal(err)
		}

		routeWithPathPrefix, err := findRouteWithPathPrefix(r)
		if err != nil {
			t.Fatal(err)
		}

		if routeWithPathPrefix == nil {
			t.Fatal("route with path regexp not found")
		}

		p, err := findPathPredicate(routeWithPathPrefix, "PathSubtree")
		if err != nil {
			t.Fatal(err)
		}

		if p == nil || p.Args[0] != "/bar" {
			t.Error("missing or invalid exact path value")
		}
	})
}

func TestPathModeParsing(t *testing.T) {
	for _, test := range []struct {
		str  string
		mode PathMode
		fail bool
	}{{
		str:  "foo",
		fail: true,
	}, {
		str:  kubernetesIngressModeString,
		mode: KubernetesIngressMode,
	}, {
		str:  pathRegexpString,
		mode: PathRegexp,
	}, {
		str:  pathPrefixString,
		mode: PathPrefix,
	}} {
		t.Run(test.str, func(t *testing.T) {
			m, err := ParsePathMode(test.str)

			if err == nil && test.fail {
				t.Fatal("failed to fail")
			} else if err != nil && !test.fail {
				t.Fatal(err)
			} else if err != nil {
				return
			}

			if m != test.mode {
				t.Errorf(
					"failed to parse the right mode, got: %v, expected: %v",
					m,
					test.mode,
				)
			}
		})
	}
}

func TestIngressSpecificMode(t *testing.T) {
	s := testServices()
	api := newTestAPI(t, s, &ingressList{})
	defer api.Close()

	ingressWithDefault := testIngress(
		"namespace1", "ingress1", "service1", "", "", "", "", "", "", backendPort{8080}, 1.0,
		testRule("www.example.org", testPathRule("^/foo", "service1", backendPort{8080})),
	)

	ingressWithCustom := testIngress(
		"namespace1", "ingress1", "service1", "", "", "", "", "", "", backendPort{8080}, 1.0,
		testRule("www.example.org", testPathRule("/bar", "service1", backendPort{8080})),
	)
	ingressWithCustom.Metadata.Annotations[pathModeAnnotationKey] = pathPrefixString

	api.ingresses.Items = []*ingressItem{ingressWithDefault, ingressWithCustom}

	c, err := New(Options{
		KubernetesURL: api.server.URL,
		PathMode:      PathRegexp,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	r, err := c.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	fooRoute := findRouteWithRx(r)
	if fooRoute == nil {
		t.Fatal("failed to receive route with path regexp")
	}

	if fooRoute.PathRegexps[0] != "^/foo" {
		t.Error("failed to load route with regexp path")
	}

	barRoute, err := findRouteWithPathPrefix(r)
	if err != nil {
		t.Fatal(err)
	}

	if barRoute == nil {
		t.Fatal("failed to receive route with path prefix")
	}

	p, err := findPathPredicate(barRoute, "PathSubtree")
	if err != nil {
		t.Fatal(err)
	}

	if p == nil || p.Args[0] != "/bar" {
		t.Error("failed to load route with prefix path")
	}
}
