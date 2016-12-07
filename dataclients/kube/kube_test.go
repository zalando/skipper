package kube

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
)

type services map[string]map[string]*service

type testAPI struct {
	test      *testing.T
	services  services
	ingresses *ingressList
	server    *httptest.Server
}

var serviceURIRx = regexp.MustCompile("^/api/v1/namespaces/([^/]+)/services/([^/]+)$")

func init() {
	log.SetLevel(log.DebugLevel)
}

func testService(clusterIP string, ports map[string]int) *service {
	sports := make([]*servicePort, 0, len(ports))
	for name, port := range ports {
		sports = append(sports, &servicePort{
			Name: name,
			Port: port,
		})
	}

	return &service{
		Spec: &serviceSpec{
			ClusterIP: clusterIP,
			Ports:     sports,
		},
	}
}

func testPathRule(path, serviceName string, port backendPort) *pathRule {
	return &pathRule{
		Path: path,
		Backend: &backend{
			ServiceName: serviceName,
			ServicePort: port,
		},
	}
}

func testRule(host string, paths ...*pathRule) *rule {
	return &rule{
		Host: host,
		Http: &httpRule{
			Paths: paths,
		},
	}
}

func testIngress(ns, name, defaultService string, defaultPort backendPort, rules ...*rule) *ingressItem {
	var defaultBackend *backend
	if len(defaultService) != 0 {
		defaultBackend = &backend{
			ServiceName: defaultService,
			ServicePort: defaultPort,
		}
	}

	return &ingressItem{
		Metadata: &metadata{
			Namespace: ns,
			Name:      name,
		},
		Spec: &ingressSpec{
			DefaultBackend: defaultBackend,
			Rules:          rules,
		},
	}
}

func testServices() services {
	return services{
		"namespace1": map[string]*service{
			"service1": testService("1.2.3.4", map[string]int{"port1": 8080}),
			"service2": testService("5.6.7.8", map[string]int{"port2": 8181}),
		},
		"namespace2": map[string]*service{
			"service3": testService("9.0.1.2", map[string]int{"port3": 7272}),
		},
	}
}

func testIngresses() []*ingressItem {
	return []*ingressItem{
		testIngress("namespace1", "default-only", "service1", backendPort{8080}),
		testIngress(
			"namespace2",
			"path-rule-only",
			"",
			backendPort{},
			testRule(
				"www.example.org",
				testPathRule("/", "service3", backendPort{"port3"}),
			),
		),
		testIngress(
			"namespace1",
			"mega",
			"service1",
			backendPort{"port1"},
			testRule(
				"foo.example.org",
				testPathRule("/test1", "service1", backendPort{"port1"}),
				testPathRule("/test2", "service2", backendPort{"port2"}),
			),
			testRule(
				"bar.example.org",
				testPathRule("/test1", "service1", backendPort{"port1"}),
				testPathRule("/test2", "service2", backendPort{"port2"}),
			),
		),
	}
}

func checkRoutes(t *testing.T, r []*eskip.Route, expected map[string]string) {
	if len(r) != len(expected) {
		t.Error("number of routes doesn't match expected", len(r), len(expected))
		return
	}

	for id, backend := range expected {
		var found bool
		for _, ri := range r {
			if ri.Id == id {
				if ri.Backend != backend {
					t.Error("invalid backend", ri.Backend, backend)
					return
				}

				found = true
			}
		}

		if !found {
			t.Error("expected route not found", id, backend)
			return
		}
	}
}

func checkIDs(t *testing.T, got []string, expected ...string) {
	if len(got) != len(expected) {
		t.Error("number of IDs doesn't match expected", len(got), len(expected))
		return
	}

	for _, id := range got {
		var found bool
		for _, eid := range expected {
			if eid == id {
				found = true
				break
			}
		}

		if !found {
			t.Error("invalid ID received", id)
			return
		}
	}
}

func newTestAPI(t *testing.T, s services, i *ingressList) *testAPI {
	api := &testAPI{
		test:      t,
		services:  s,
		ingresses: i,
	}

	api.server = httptest.NewServer(api)
	return api
}

func respondJSON(w io.Writer, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	_, err = w.Write(b)
	return err
}

func (api *testAPI) getTestService(uri string) (*service, bool) {
	if m := serviceURIRx.FindAllStringSubmatch(uri, -1); len(m) != 0 {
		ns, n := m[0][1], m[0][2]
		return api.services[ns][n], true
	}

	return nil, false
}

func (api *testAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == ingressesURI {
		if err := respondJSON(w, api.ingresses); err != nil {
			api.test.Error(err)
		}

		return
	}

	s, ok := api.getTestService(r.URL.Path)
	if !ok {
		s = &service{}
	}

	if err := respondJSON(w, s); err != nil {
		api.test.Error(err)
	}
}

func (api *testAPI) Close() {
	api.server.Close()
}

func TestIngressData(t *testing.T) {
	for _, ti := range []struct {
		msg            string
		services       services
		ingresses      []*ingressItem
		expectedRoutes map[string]string
	}{{
		"service backend from ingress and service, default",
		services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", nil),
			},
		},
		[]*ingressItem{testIngress("foo", "baz", "bar", backendPort{8080})},
		map[string]string{
			"kube_foo__baz____": "http://1.2.3.4:8080",
		},
	}, {
		"service backend from ingress and service, path rule",
		services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", nil),
			},
		},
		[]*ingressItem{testIngress(
			"foo",
			"baz",
			"",
			backendPort{},
			testRule(
				"www.example.org",
				testPathRule(
					"/",
					"bar",
					backendPort{8181},
				),
			),
		)},
		map[string]string{
			"kube_foo__baz__www_example_org___": "http://1.2.3.4:8181",
		},
	}, {
		"service backend from ingress and service, default and path rule",
		services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", nil),
				"baz": testService("5.6.7.8", nil),
			},
		},
		[]*ingressItem{testIngress(
			"foo",
			"qux",
			"bar",
			backendPort{8080},
			testRule(
				"www.example.org",
				testPathRule(
					"/",
					"baz",
					backendPort{8181},
				),
			),
		)},
		map[string]string{
			"kube_foo__qux____":                 "http://1.2.3.4:8080",
			"kube_foo__qux__www_example_org___": "http://5.6.7.8:8181",
		},
	}, {
		"service backend from ingress and service, with port name",
		services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		[]*ingressItem{testIngress(
			"foo",
			"qux",
			"",
			backendPort{},
			testRule(
				"www.example.org",
				testPathRule(
					"/",
					"bar",
					backendPort{"baz"},
				),
			),
		)},
		map[string]string{
			"kube_foo__qux__www_example_org___": "http://1.2.3.4:8181",
		},
	}, {
		"ignore ingress entries with missing metadata",
		services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		[]*ingressItem{
			testIngress(
				"foo",
				"qux",
				"",
				backendPort{},
				testRule(
					"www.example.org",
					testPathRule(
						"/",
						"bar",
						backendPort{"baz"},
					),
				),
			),
			&ingressItem{
				Spec: &ingressSpec{
					Rules: []*rule{
						testRule(
							"ignored.example.org",
							testPathRule(
								"/ignored",
								"ignored",
								backendPort{"baz"},
							),
						),
					},
				},
			},
		},
		map[string]string{
			"kube_foo__qux__www_example_org___": "http://1.2.3.4:8181",
		},
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			api := newTestAPI(t, ti.services, &ingressList{Items: ti.ingresses})
			defer api.Close()
			dc := New(Options{APIAddress: api.server.URL})

			r, err := dc.LoadAll()
			if err != nil {
				t.Error(err)
				return
			}

			checkRoutes(t, r, ti.expectedRoutes)
		})
	}
}

func Test(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	t.Run("no services, no ingresses, load empty initial and update", func(t *testing.T) {
		dc := New(Options{APIAddress: api.server.URL})

		if r, err := dc.LoadAll(); err != nil || len(r) != 0 {
			t.Error("failed to load initial")
		}

		if r, d, err := dc.LoadUpdate(); err != nil || len(r) != 0 || len(d) != 0 {
			t.Error("failed to load update", err)
		}
	})

	t.Run("has ingress but no according services, load empty initial and update", func(t *testing.T) {
		api.ingresses.Items = testIngresses()
		dc := New(Options{APIAddress: api.server.URL})

		if r, err := dc.LoadAll(); err != nil || len(r) != 0 {
			t.Error("failed to load initial")
		}

		if r, d, err := dc.LoadUpdate(); err != nil || len(r) != 0 || len(d) != 0 {
			t.Error("failed to load update", err)
		}
	})

	t.Run("has ingress but no according services, service gets created", func(t *testing.T) {
		api.ingresses.Items = testIngresses()
		dc := New(Options{APIAddress: api.server.URL})

		if r, err := dc.LoadAll(); err != nil || len(r) != 0 {
			t.Error("failed to load initial")
		}

		api.services = testServices()

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("udpate failed")
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__default_only____":                   "http://1.2.3.4:8080",
			"kube_namespace2__path_rule_only__www_example_org___": "http://9.0.1.2:7272",
			"kube_namespace1__mega____":                           "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test1":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test2":      "http://5.6.7.8:8181",
			"kube_namespace1__mega__bar_example_org___test1":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__bar_example_org___test2":      "http://5.6.7.8:8181",
		})
	})

	t.Run("receives invalid ingress, parses the rest, gets fixed", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		api.ingresses.Items[2].Spec.Rules[0].Http.Paths[0].Backend.ServicePort = backendPort{"not-existing"}
		dc := New(Options{APIAddress: api.server.URL})

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		api.ingresses.Items = testIngresses()

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("update failed")
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__mega__foo_example_org___test1": "http://1.2.3.4:8080",
		})
	})

	t.Run("has ingresses, receive initial", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc := New(Options{APIAddress: api.server.URL})

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__default_only____":                   "http://1.2.3.4:8080",
			"kube_namespace2__path_rule_only__www_example_org___": "http://9.0.1.2:7272",
			"kube_namespace1__mega____":                           "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test1":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test2":      "http://5.6.7.8:8181",
			"kube_namespace1__mega__bar_example_org___test1":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__bar_example_org___test2":      "http://5.6.7.8:8181",
		})
	})

	t.Run("has ingresses, update some of them", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc := New(Options{APIAddress: api.server.URL})

		_, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		api.ingresses.Items[0].Spec.DefaultBackend.ServicePort = backendPort{6363}
		api.ingresses.Items[2].Spec.Rules[0].Http.Paths[0].Backend.ServicePort = backendPort{9999}

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("failed to receive delete", err, len(r))
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__default_only____":              "http://1.2.3.4:6363",
			"kube_namespace1__mega__foo_example_org___test1": "http://1.2.3.4:9999",
		})
	})

	t.Run("has ingresses, loose a service", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc := New(Options{APIAddress: api.server.URL})

		_, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		delete(api.services["namespace1"], "service2")

		r, d, err := dc.LoadUpdate()
		if err != nil || len(r) != 0 {
			t.Error("failed to receive delete", err, len(r))
		}

		checkIDs(
			t,
			d,
			"kube_namespace1__mega__foo_example_org___test2",
			"kube_namespace1__mega__bar_example_org___test2",
		)
	})

	t.Run("has ingresses, delete some ingresses", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc := New(Options{APIAddress: api.server.URL})

		_, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		api.ingresses.Items = api.ingresses.Items[:1]

		r, d, err := dc.LoadUpdate()
		if err != nil || len(r) != 0 {
			t.Error("failed to receive delete", err, len(r))
		}

		checkIDs(
			t,
			d,
			"kube_namespace2__path_rule_only__www_example_org___",
			"kube_namespace1__mega____",
			"kube_namespace1__mega__foo_example_org___test1",
			"kube_namespace1__mega__foo_example_org___test2",
			"kube_namespace1__mega__bar_example_org___test1",
			"kube_namespace1__mega__bar_example_org___test2",
		)
	})

	t.Run("has ingresses, delete some ingress rules", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc := New(Options{APIAddress: api.server.URL})

		_, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		api.ingresses.Items[2].Spec.Rules = api.ingresses.Items[2].Spec.Rules[:1]

		r, d, err := dc.LoadUpdate()
		if err != nil || len(r) != 0 {
			t.Error("failed to receive delete", err, len(r))
		}

		checkIDs(
			t,
			d,
			"kube_namespace1__mega__bar_example_org___test1",
			"kube_namespace1__mega__bar_example_org___test2",
		)
	})

	t.Run("has ingresses, add new ones", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc := New(Options{APIAddress: api.server.URL})

		_, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		api.ingresses.Items = append(
			api.ingresses.Items,
			testIngress(
				"namespace1",
				"new1",
				"",
				backendPort{""},
				testRule(
					"new1.example.org",
					testPathRule("/", "service1", backendPort{"port1"}),
				),
			),
			testIngress(
				"namespace1",
				"new2",
				"",
				backendPort{""},
				testRule(
					"new2.example.org",
					testPathRule("/", "service2", backendPort{"port2"}),
				),
			),
		)

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("invalid updated received", err, len(d))
			return
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__new1__new1_example_org___": "http://1.2.3.4:8080",
			"kube_namespace1__new2__new2_example_org___": "http://5.6.7.8:8181",
		})
	})

	t.Run("has ingresses, mixed insert, update, delete", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc := New(Options{APIAddress: api.server.URL})

		_, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		api.ingresses.Items = append(
			api.ingresses.Items,
			testIngress(
				"namespace1",
				"new1",
				"",
				backendPort{""},
				testRule(
					"new1.example.org",
					testPathRule("/", "service1", backendPort{"port1"}),
				),
			),
			testIngress(
				"namespace1",
				"new2",
				"",
				backendPort{""},
				testRule(
					"new2.example.org",
					testPathRule("/", "service2", backendPort{"port2"}),
				),
			),
		)

		api.ingresses.Items[1].Spec.Rules[0].Http.Paths[0].Backend.ServicePort = backendPort{9999}
		api.ingresses.Items[2].Spec.Rules = api.ingresses.Items[2].Spec.Rules[:1]

		r, d, err := dc.LoadUpdate()
		if err != nil {
			t.Error("invalid updated received", err, len(d))
			return
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__new1__new1_example_org___":          "http://1.2.3.4:8080",
			"kube_namespace1__new2__new2_example_org___":          "http://5.6.7.8:8181",
			"kube_namespace2__path_rule_only__www_example_org___": "http://9.0.1.2:9999",
		})

		checkIDs(
			t,
			d,
			"kube_namespace1__mega__bar_example_org___test1",
			"kube_namespace1__mega__bar_example_org___test2",
		)
	})
}
