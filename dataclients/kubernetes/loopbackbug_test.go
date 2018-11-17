package kubernetes

import (
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
)

func splitURL(u string) (ip string, port int, err error) {
	var typed *url.URL
	typed, err = url.Parse(u)
	if err != nil {
		return
	}

	var portString string
	ip, portString, err = net.SplitHostPort(typed.Host)
	port, err = strconv.Atoi(portString)
	return
}

func TestInfiniteLoopback(t *testing.T) {
	endpoint1 := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer endpoint1.Close()
	endpoint2 := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer endpoint2.Close()

	epIP1, epPort1, err := splitURL(endpoint1.URL)
	if err != nil {
		log.Fatal(err)
	}

	epIP2, epPort2, err := splitURL(endpoint2.URL)
	if err != nil {
		log.Fatal(err)
	}

	services := services{
		"namespace1": map[string]*service{
			"service1": testServiceWithTargetPort(
				"1.2.3.4",
				map[string]int{
					"port1": 8080,
					"port2": 8081,
				},
				map[int]*backendPort{
					8080: {epPort1},
					8081: {epPort2},
				},
			),
		},
	}

	subsets := []*subset{{
		Addresses: []*address{{
			IP: epIP1,
		}, {
			IP: epIP2,
		}},
		Ports: []*port{{
			Name: "targetPort1",
			Port: epPort1,
		}, {
			Name: "targetPort2",
			Port: epPort2,
		}},
	}}

	endpoints := endpoints{
		"namespace1": map[string]endpoint{
			"service1": {Subsets: subsets},
		},
	}

	ingress1 := testIngressSimple(
		"namespace1",
		"ingress1",
		"service1",
		backendPort{"port1"},
		testRule(
			"www.example.org",
			testPathRule("/foo", "service1", backendPort{"port1"}),
		),
	)

	ingress2 := testIngressSimple(
		"namespace1",
		"ingress2",
		"service1",
		backendPort{"port2"},
		testRule(
			"www.example.org",
			testPathRule("/foo", "service1", backendPort{"port2"}),
		),
	)

	api := newTestAPIWithEndpoints(
		t,
		services,
		&ingressList{
			Items: []*ingressItem{
				ingress1,
				ingress2,
			},
		},
		endpoints,
	)
	defer api.Close()

	dc, err := New(Options{KubernetesURL: api.server.URL})
	if err != nil {
		t.Fatal(err)
	}

	defer dc.Close()

	p := proxy.WithParams(proxy.Params{
		Routing: routing.New(routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
			PollTimeout:    12 * time.Millisecond,
			DataClients:    []routing.DataClient{dc},
			Predicates: []routing.PredicateSpec{
				loadbalancer.NewGroup(),
				loadbalancer.NewMember(),
			},
		}),
		MaxLoopbacks: 1,
	})
	defer p.Close()

	var routesApplied bool
	finish := time.After(9 * time.Second)
	for {
		select {
		case <-finish:
			return
		default:
		}

		req, err := http.NewRequest("GET", "/foo", nil)
		if err != nil {
			t.Fatal(err)
		}

		req.Host = "www.example.org"
		rsp := httptest.NewRecorder()
		p.ServeHTTP(rsp, req)

		if rsp.Code != http.StatusNotFound {
			routesApplied = true
		}

		if !routesApplied {
			continue
		}

		if rsp.Code != http.StatusOK {
			t.Fatal("unexpected status code:", rsp.Code)
		}
	}
}
