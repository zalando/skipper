package kubernetes

import (
	"testing"
)

func TestAnnotationFiltersInLBRoutes(t *testing.T) {
	svc := testServiceWithTargetPort(
		"1.2.3.4",
		map[string]int{"port1": 8080},
		map[int]*backendPort{8080: {8080}},
	)

	services := services{
		"namespace1": map[string]*service{"service1": svc},
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

	endpoints := endpoints{
		"namespace1": map[string]endpoint{
			"service1": {Subsets: subsets},
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

		// default backend, target 1:
		kube_namespace1__ingress1______0_0:
		  LBMember("kube_namespace1__ingress1_______0", 0)
		  -> dropRequestHeader("X-Load-Balancer-Member")
		  -> "http://42.0.1.0:8080";

		// default backend, target 2:
		kube_namespace1__ingress1______1_0:
		  LBMember("kube_namespace1__ingress1_______0", 1)
		  -> dropRequestHeader("X-Load-Balancer-Member")
		  -> "http://42.1.0.1:8080";

		// default group:
		kube_namespace1__ingress1_______0__lb_group:
		  LBGroup("kube_namespace1__ingress1_______0")
		  -> lbDecide("kube_namespace1__ingress1_______0", 2) ->
		  <loopback>;

		// path rule, target 1:
		kube_namespace1__ingress1__test_example_org___test1__service1_0_0:
		  Host(/^test[.]example[.]org$/)
		  && PathRegexp(/^\/test1/)
		  && LBMember("kube_namespace1__ingress1__test_example_org___test1__service1_0", 0)
		  -> dropRequestHeader("X-Load-Balancer-Member")
		  -> setPath("/foo")
		  -> "http://42.0.1.0:8080";

		// path rule, target 2:
		kube_namespace1__ingress1__test_example_org___test1__service1_1_0:
		  Host(/^test[.]example[.]org$/)
		  && PathRegexp(/^\/test1/)
		  && LBMember("kube_namespace1__ingress1__test_example_org___test1__service1_0", 1)
		  -> dropRequestHeader("X-Load-Balancer-Member")
		  -> setPath("/foo")
		  -> "http://42.1.0.1:8080";

		// path rule group:
		kube_namespace1__ingress1__test_example_org___test1__service1_0__lb_group:
		  Host(/^test[.]example[.]org$/) && PathRegexp(/^\/test1/)
		  && LBGroup("kube_namespace1__ingress1__test_example_org___test1__service1_0")
		  -> lbDecide("kube_namespace1__ingress1__test_example_org___test1__service1_0", 2)
		  -> <loopback>;

		// catch all:
		kube___catchall__test_example_org_____0:
		  Host(/^test[.]example[.]org$/)
		  -> <shunt>;
	`

	checkRoutesDoc(t, r, expectedRoutes)
}
