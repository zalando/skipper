package kubernetes

import (
	"testing"
)

func TestAnnotationFiltersInLBRoutes(t *testing.T) {
	svc := testServiceWithTargetPort(
		"namespace1", "service1",
		"1.2.3.4",
		map[string]int{"port1": 8080},
		map[int]*backendPort{8080: {8080}},
	)

	services := &serviceList{
		Items: []*service{svc},
	}

	subsets := []*subset{{
		Addresses: []*address{{
			IP: "42.0.1.0",
		}},
		Ports: []*port{{
			Name: "port1",
			Port: 8080,
		}},
	}, {
		Addresses: []*address{{
			IP: "42.1.0.1",
		}},
		Ports: []*port{{
			Name: "port1",
			Port: 8080,
		}},
	}}

	endpoints := &endpointList{
		Items: []*endpoint{
			{
				Meta:    &metadata{Namespace: "namespace1", Name: "service1"},
				Subsets: subsets,
			},
		},
	}

	ingress := testIngress(
		"namespace1",
		"ingress1",
		"service1",
		"",
		`setPath("/foo")`,
		"",
		"",
		"",
		"",
		backendPort{"port1"},
		1.0,
		testRule(
			"test.example.org",
			testPathRule("/test1", "service1", backendPort{"port1"}),
		),
	)

	api := newTestAPIWithEndpoints(t, services, &ingressList{Items: []*ingressItem{ingress}}, endpoints)
	defer api.Close()

	dc, err := New(Options{KubernetesURL: api.server.URL})
	if err != nil {
		t.Fatal(err)
	}

	defer dc.Close()

	r, err := dc.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	const expectedRoutes = `
		// default backend:
		kube_namespace1__ingress1______:
		  *
		  -> <roundRobin, "http://42.0.1.0:8080", "http://42.1.0.1:8080">;

		// path rule, target 1:
		kube_namespace1__ingress1__test_example_org___test1__service1:
		  Host(/^test[.]example[.]org$/)
		  && PathRegexp(/^\/test1/)
		  -> setPath("/foo")
		  -> <roundRobin, "http://42.0.1.0:8080", "http://42.1.0.1:8080">;

		// catch all:
		kube___catchall__test_example_org____:
		  Host(/^test[.]example[.]org$/)
		  -> <shunt>;
	`

	checkRoutesDoc(t, r, expectedRoutes)
}

func TestLoadBalancerAnnotation(t *testing.T) {
	svc := testServiceWithTargetPort(
		"namespace1", "service1",
		"1.2.3.4",
		map[string]int{"port1": 8080},
		map[int]*backendPort{8080: {8080}},
	)

	services := &serviceList{
		Items: []*service{svc},
	}

	subsets := []*subset{{
		Addresses: []*address{{
			IP: "42.0.1.0",
		}},
		Ports: []*port{{
			Name: "port1",
			Port: 8080,
		}},
	}, {
		Addresses: []*address{{
			IP: "42.1.0.1",
		}},
		Ports: []*port{{
			Name: "port1",
			Port: 8080,
		}},
	}}

	endpoints := &endpointList{
		Items: []*endpoint{
			{
				Meta:    &metadata{Namespace: "namespace1", Name: "service1"},
				Subsets: subsets,
			},
		},
	}

	for _, ti := range []struct {
		msg            string
		ingress        *ingressItem
		expectedRoutes string
	}{{
		msg: "random algorithm should be set",
		ingress: testIngress(
			"namespace1",
			"ingress1",
			"service1",
			"",
			`setPath("/foo")`,
			"",
			"",
			"",
			"random",
			backendPort{"port1"},
			1.0,
			testRule(
				"test.example.org",
				testPathRule("/test1", "service1", backendPort{"port1"}),
			),
		),
		expectedRoutes: `
		// default backend:
		kube_namespace1__ingress1______:
		  *
		  -> <random, "http://42.0.1.0:8080", "http://42.1.0.1:8080">;

		// path rule, target 1:
		kube_namespace1__ingress1__test_example_org___test1__service1:
		  Host(/^test[.]example[.]org$/)
		  && PathRegexp(/^\/test1/)
		  -> setPath("/foo")
		  -> <random, "http://42.0.1.0:8080", "http://42.1.0.1:8080">;

		// catch all:
		kube___catchall__test_example_org____:
		  Host(/^test[.]example[.]org$/)
		  -> <shunt>;
	`,
	}, {
		msg: "consistentHash algorithm should be set",
		ingress: testIngress(
			"namespace1",
			"ingress1",
			"service1",
			"",
			`setPath("/foo")`,
			"",
			"",
			"",
			"consistentHash",
			backendPort{"port1"},
			1.0,
			testRule(
				"test.example.org",
				testPathRule("/test1", "service1", backendPort{"port1"}),
			),
		),
		expectedRoutes: `
		// default backend:
		kube_namespace1__ingress1______:
		  *
		  -> <consistentHash, "http://42.0.1.0:8080", "http://42.1.0.1:8080">;

		// path rule, target 1:
		kube_namespace1__ingress1__test_example_org___test1__service1:
		  Host(/^test[.]example[.]org$/)
		  && PathRegexp(/^\/test1/)
		  -> setPath("/foo")
		  -> <consistentHash, "http://42.0.1.0:8080", "http://42.1.0.1:8080">;

		// catch all:
		kube___catchall__test_example_org____:
		  Host(/^test[.]example[.]org$/)
		  -> <shunt>;
	`,
	}, {
		msg: "roundRobin algorithm should be set",
		ingress: testIngress(
			"namespace1",
			"ingress1",
			"service1",
			"",
			`setPath("/foo")`,
			"",
			"",
			"",
			"roundRobin",
			backendPort{"port1"},
			1.0,
			testRule(
				"test.example.org",
				testPathRule("/test1", "service1", backendPort{"port1"}),
			),
		),
		expectedRoutes: `
		// default backend:
		kube_namespace1__ingress1______:
		  *
		  -> <roundRobin, "http://42.0.1.0:8080", "http://42.1.0.1:8080">;

		// path rule, target 1:
		kube_namespace1__ingress1__test_example_org___test1__service1:
		  Host(/^test[.]example[.]org$/)
		  && PathRegexp(/^\/test1/)
		  -> setPath("/foo")
		  -> <roundRobin, "http://42.0.1.0:8080", "http://42.1.0.1:8080">;

		// catch all:
		kube___catchall__test_example_org____:
		  Host(/^test[.]example[.]org$/)
		  -> <shunt>;
	`,
	}} {
		t.Run(ti.msg, func(t *testing.T) {

			api := newTestAPIWithEndpoints(t, services, &ingressList{Items: []*ingressItem{ti.ingress}}, endpoints)
			defer api.Close()

			dc, err := New(Options{KubernetesURL: api.server.URL})
			if err != nil {
				t.Fatal(err)
			}

			defer dc.Close()

			r, err := dc.LoadAll()
			if err != nil {
				t.Fatal(err)
			}

			checkRoutesDoc(t, r, ti.expectedRoutes)
		})
	}
}
