package kubernetes

import "testing"

func TestLBTargets(t *testing.T) {
	const expectedRoutes = `

		// default backend, target 1:
		kube_namespace1__ingress1______0:
		  LBMember("kube_namespace1__ingress1______", 0)
		  -> "http://42.0.1.2:8080";

		// default backend, target 2:
		kube_namespace1__ingress1______1:
		  LBMember("kube_namespace1__ingress1______", 1)
		  -> "http://42.0.1.3:8080";

		// default group:
		kube_namespace1__ingress1________lb_group:
		  LBGroup("kube_namespace1__ingress1______")
		  -> lbDecide("kube_namespace1__ingress1______", 2) ->
		  <loopback>;

		// path rule, target 1:
		kube_namespace1__ingress1__test_example_org___test1__service1_0:
		  Host(/^test[.]example[.]org$/)
		  && PathRegexp(/^\/test1/)
		  && LBMember("kube_namespace1__ingress1__test_example_org___test1__service1", 0)
		  -> "http://42.0.1.2:8080";

		// path rule, target 2:
		kube_namespace1__ingress1__test_example_org___test1__service1_1:
		  Host(/^test[.]example[.]org$/)
		  && PathRegexp(/^\/test1/)
		  && LBMember("kube_namespace1__ingress1__test_example_org___test1__service1", 1)
		  -> "http://42.0.1.3:8080";

		// path rule group:
		kube_namespace1__ingress1__test_example_org___test1__service1__lb_group:
		  Host(/^test[.]example[.]org$/) && PathRegexp(/^\/test1/)
		  && LBGroup("kube_namespace1__ingress1__test_example_org___test1__service1")
		  -> lbDecide("kube_namespace1__ingress1__test_example_org___test1__service1", 2)
		  -> <loopback>;

		// catch all:
		kube___catchall__test_example_org____:
		  Host(/^test[.]example[.]org$/)
		  -> <shunt>;
	`

	svc := testServiceWithTargetPort(
		"1.2.3.4",
		map[string]int{"port1": 8080},
		map[int]*backendPort{8080: {8080}},
	)

	services := services{
		"namespace1": map[string]*service{"service1": svc},
	}

	endpoints := endpoints{
		"namespace1": map[string]endpoint{
			"service1": {
				Subsets: []*subset{{
					Addresses: []*address{{
						IP: "42.0.1.2",
					}},
					Ports: []*port{{
						Name: "port1",
						Port: 8080,
					}},
				}, {
					Addresses: []*address{{
						IP: "42.0.1.3",
					}},
					Ports: []*port{{
						Name: "port1",
						Port: 8080,
					}},
				}},
			},
		},
	}

	ingress := testIngress(
		"namespace1",
		"ingress1",
		"service1",
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

	r, err := dc.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	checkRoutesDoc(t, r, expectedRoutes)
}
