package kubernetes

import (
	"testing"
)

func TestLBWithTrafficControl(t *testing.T) {
	const expectedDoc = `
		kube_namespace1__ingress1______:
                  *
		  -> lbEndpoints("http://42.0.1.2:8080", "http://42.0.1.3:8080")
                  -> roundRobin()
		  -> <dynamic>;

		kube_namespace1__ingress1__test_example_org___test1__service1v1:
		  Host(/^test[.]example[.]org$/) &&
		  PathRegexp(/^\/test1/) &&
		  Traffic(0.3)
		  -> lbEndpoints("http://42.0.1.2:8080", "http://42.0.1.3:8080")
                  -> roundRobin()
		  -> <dynamic>;

		kube_namespace1__ingress1__test_example_org___test1__service1v2:
		  Host(/^test[.]example[.]org$/) &&
		  PathRegexp(/^\/test1/)
		  -> lbEndpoints("http://42.0.1.4:8080", "http://42.0.1.5:8080")
                  -> roundRobin()
		  -> <dynamic>;

		kube___catchall__test_example_org____:
		  Host(/^test[.]example[.]org$/)
		  -> <shunt>;
	`

	services := services{
		"namespace1": map[string]*service{
			"service1v1": testServiceWithTargetPort(
				"1.2.3.4",
				map[string]int{"port1": 8080},
				map[int]*backendPort{8080: {8080}},
			),
			"service1v2": testServiceWithTargetPort(
				"1.2.3.5",
				map[string]int{"port1": 8080},
				map[int]*backendPort{8080: {8080}},
			),
		},
	}

	endpoints := endpoints{
		"namespace1": map[string]endpoint{
			"service1v1": {Subsets: []*subset{
				{
					Addresses: []*address{{
						IP: "42.0.1.2",
					}},
					Ports: []*port{{
						Name: "port1",
						Port: 8080,
					}},
				},
				{
					Addresses: []*address{{
						IP: "42.0.1.3",
					}},
					Ports: []*port{{
						Name: "port1",
						Port: 8080,
					}},
				},
			}},
			"service1v2": {Subsets: []*subset{
				{
					Addresses: []*address{{
						IP: "42.0.1.4",
					}},
					Ports: []*port{{
						Name: "port1",
						Port: 8080,
					}},
				},
				{
					Addresses: []*address{{
						IP: "42.0.1.5",
					}},
					Ports: []*port{{
						Name: "port1",
						Port: 8080,
					}},
				},
			}},
		},
	}

	ingress := testIngress("namespace1", "ingress1", "service1v1",
		"", "", "", "", "",
		backendPort{"port1"},
		1.0,
		testRule(
			"test.example.org",
			&pathRule{
				Path: "/test1",
				Backend: &backend{
					ServiceName: "service1v1",
					ServicePort: backendPort{"port1"},
				},
			},
			&pathRule{
				Path: "/test1",
				Backend: &backend{
					ServiceName: "service1v2",
					ServicePort: backendPort{"port1"},
				},
			},
		),
	)
	ingress.Metadata.Annotations["zalando.org/backend-weights"] = `{"service1v1": 30, "service1v2": 70}`
	ingresses := []*ingressItem{ingress}

	api := newTestAPIWithEndpoints(t, services, &ingressList{Items: ingresses}, endpoints)
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

	checkRoutesDoc(t, r, expectedDoc)
}
