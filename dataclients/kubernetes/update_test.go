package kubernetes

import (
	"testing"

	"github.com/zalando/skipper/eskip"
)

func TestUpdateOnlyChangedRoutes(t *testing.T) {
	api := newTestAPIWithEndpoints(t, &serviceList{}, &ingressList{}, &endpointList{})
	defer api.Close()

	k, err := New(Options{
		KubernetesURL:      api.server.URL,
		ProvideHealthcheck: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer k.Close()

	r, err := k.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(r) != 1 || r[0].Id != healthcheckRouteID {
		t.Fatal("no healthcheck route received")
	}

	for i := 0; i < 3; i++ {
		update, del, err := k.LoadUpdate()
		if err != nil {
			t.Fatal(err)
		}

		if len(update) != 0 || len(del) != 0 {
			t.Fatal("unexpected udpate received")
		}
	}
}

func TestOriginMarkerNotStored(t *testing.T) {
	expectOriginMarker := func(r []*eskip.Route) {
		for _, ri := range r {
			for _, f := range ri.Filters {
				if f.Name == "originMarker" {
					return
				}
			}
		}

		t.Fatal("origin marker not found")
	}

	expectNoOriginMarker := func(r map[string]*eskip.Route) {
		for _, ri := range r {
			for _, f := range ri.Filters {
				if f.Name == "originMarker" {
					t.Fatal("unexpected origin marker found in stored routes")
				}
			}
		}
	}

	api := newTestAPI(t,
		&serviceList{
			Items: []*service{
				testService(
					"namespace1",
					"service1",
					"1.2.3.4",
					map[string]int{"port1": 8080},
				),
			}},
		&ingressList{
			Items: []*ingressItem{
				testIngressSimple(
					"namespace1",
					"ingress1",
					"service1",
					backendPort{8080},
					testRule(
						"example.org",
						testPathRule("/", "service1", backendPort{8080}),
					),
				),
			},
		},
	)
	defer api.Close()

	k, err := New(Options{
		KubernetesURL:      api.server.URL,
		ProvideHealthcheck: true,
		OriginMarker:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer k.Close()

	// receive initial routes
	r, err := k.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	expectOriginMarker(r)
	expectNoOriginMarker(k.current)

	// check for an update, expect none
	r, d, err := k.LoadUpdate()
	if err != nil {
		t.Fatal(err)
	}

	if len(r) != 0 || len(d) != 0 {
		t.Fatal("unexpected route update received")
	}

	expectNoOriginMarker(k.current)

	// make an update and receive it
	api.ingresses.Items = append(
		api.ingresses.Items,
		testIngressSimple(
			"namespace1",
			"ingress2",
			"service1",
			backendPort{8080},
			testRule(
				"api.example.org",
				testPathRule("/v1", "service1", backendPort{8080}),
			),
		),
	)

	r, _, err = k.LoadUpdate()
	if err != nil {
		t.Fatal(err)
	}

	expectOriginMarker(r)
	expectNoOriginMarker(k.current)
}
