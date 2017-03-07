package kubernetes

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"testing"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/predicates/source"
)

type services map[string]map[string]*service

type testAPI struct {
	test      *testing.T
	services  services
	ingresses *ingressList
	server    *httptest.Server
	failNext  bool
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

func checkHealthcheck(t *testing.T, got []*eskip.Route, expected, healthy bool) {
	for _, r := range got {
		if r.Id != healthcheckRouteID {
			continue
		}

		if !expected {
			t.Error("unexpected healthcheck route")
			return
		}

		if !r.Shunt {
			t.Error("healthcheck route must be a shunt")
			return
		}

		if r.Path != healthcheckPath {
			t.Error("invalid healthcheck path")
			return
		}

		var found bool
		for _, p := range r.Predicates {
			if p.Name != source.Name {
				continue
			}

			found = true

			if len(p.Args) != len(internalIPs) {
				t.Error("invalid source predicate")
				return
			}

			for _, s := range internalIPs {
				var found bool
				for _, sp := range p.Args {
					if sp == s {
						found = true
						break
					}
				}

				if !found {
					t.Error("invalid source ip")
				}
			}
		}

		if !found {
			t.Error("source predicate not found")
		}

		for _, f := range r.Filters {
			if f.Name != builtin.StatusName {
				continue
			}

			if len(f.Args) != 1 {
				t.Error("invalid healthcheck args")
				return
			}

			if healthy && f.Args[0] != http.StatusOK {
				t.Error("invalid healthcheck status", f.Args[0], http.StatusOK)
			} else if !healthy && f.Args[0] != http.StatusServiceUnavailable {
				t.Error("invalid healthcheck status", f.Args[0], http.StatusServiceUnavailable)
			}

			return
		}
	}

	if expected {
		t.Error("healthcheck route not found")
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
	if api.failNext {
		api.failNext = false
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

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
			{
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
			dc, err := New(Options{KubernetesURL: api.server.URL})
			if err != nil {
				t.Error(err)
			}

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
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		if r, err := dc.LoadAll(); err != nil || len(r) != 0 {
			t.Error("failed to load initial")
		}

		if r, d, err := dc.LoadUpdate(); err != nil || len(r) != 0 || len(d) != 0 {
			t.Error("failed to load update", err)
		}
	})

	t.Run("has ingress but no according services, load empty initial and update", func(t *testing.T) {
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		if r, err := dc.LoadAll(); err != nil || len(r) != 0 {
			t.Error("failed to load initial")
		}

		if r, d, err := dc.LoadUpdate(); err != nil || len(r) != 0 || len(d) != 0 {
			t.Error("failed to load update", err)
		}
	})

	t.Run("has ingress but no according services, service gets created", func(t *testing.T) {
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

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
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

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
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

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
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		_, err = dc.LoadAll()
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
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		_, err = dc.LoadAll()
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
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		_, err = dc.LoadAll()
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
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		_, err = dc.LoadAll()
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
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		_, err = dc.LoadAll()
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
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		_, err = dc.LoadAll()
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

func TestHealthcheckInitial(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	t.Run("no healthcheck, empty", func(t *testing.T) {
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, false, false)
	})

	t.Run("no healthcheck", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, false, false)
	})

	t.Run("no healthcheck, fail", func(t *testing.T) {
		api.failNext = true
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		_, err = dc.LoadAll()
		if err == nil {
			t.Error("failed to fail")
		}
	})

	t.Run("use healthceck, empty", func(t *testing.T) {
		api.ingresses.Items = nil
		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, true, true)
	})

	t.Run("use healthcheck", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, true, true)
	})
}

func TestHealthcheckUpdate(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	t.Run("no healthcheck, update fail", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()

		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()
		api.failNext = true

		r, d, err := dc.LoadUpdate()
		if err == nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, false, false)
		if len(d) != 0 {
			t.Error("unexpected delete")
		}
	})

	t.Run("use healthcheck, update fail", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()

		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()
		api.failNext = true

		r, d, err := dc.LoadUpdate()
		if err != nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, true, false)
		if len(d) != 0 {
			t.Error("unexpected delete")
		}
	})

	t.Run("use healthcheck, update succeeds", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()

		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()

		r, d, err := dc.LoadUpdate()
		if err != nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, true, true)
		if len(d) != 0 {
			t.Error("unexpected delete")
		}
	})

	t.Run("use healthcheck, update fails, gets fixed", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()

		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()

		api.failNext = true
		dc.LoadUpdate()

		r, d, err := dc.LoadUpdate()
		if err != nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, true, true)
		if len(d) != 0 {
			t.Error("unexpected delete")
		}
	})
}

func TestHealthcheckReload(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	t.Run("no healthcheck, reload fail", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()

		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()
		api.failNext = true

		r, err := dc.LoadAll()
		if err == nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, false, false)
	})

	t.Run("use healthcheck, reload succeeds", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()

		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, true, true)
		checkRoutes(t, r, map[string]string{
			healthcheckRouteID:                                    "",
			"kube_namespace1__default_only____":                   "http://1.2.3.4:8080",
			"kube_namespace2__path_rule_only__www_example_org___": "http://9.0.1.2:7272",
			"kube_namespace1__mega____":                           "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test1":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test2":      "http://5.6.7.8:8181",
			"kube_namespace1__mega__bar_example_org___test1":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__bar_example_org___test2":      "http://5.6.7.8:8181",
		})
	})
}

func TestCreateRequest(t *testing.T) {
	var (
		buf bytes.Buffer
		req *http.Request
		err error
		url string
	)
	rc := ioutil.NopCloser(&buf)

	client := &Client{}

	url = "A%"
	req, err = client.createRequest("GET", url, rc)
	if err == nil {
		t.Error("request creation should fail")
	}

	url = "https://www.example.org"
	req, err = client.createRequest("GET", url, rc)
	if err != nil {
		t.Error(err)
	}

	req, err = client.createRequest("//", url, rc)
	if err == nil {
		t.Error("request creation should fail")
	}

	client.token = "1234"
	req, err = client.createRequest("POST", url, rc)
	if err != nil {
		t.Error(err)
	}
	if req.URL.String() != url {
		t.Errorf("request creation incorrect url is set")
	}
	if req.Header.Get("Authorization") != "Bearer 1234" {
		t.Errorf("incorrect authorization header set")
	}
	if req.Method != "POST" {
		t.Errorf("incorrect method is set")
	}
}

func TestBuildAPIURL(t *testing.T) {
	var apiURL string
	var err error
	o := Options{}

	apiURL, err = buildAPIURL(o)
	if err != nil {
		t.Error(err)
	}
	if apiURL != defaultKubernetesURL {
		t.Errorf("unexpected default API URL")
	}

	o.KubernetesURL = "http://localhost:4040"
	apiURL, err = buildAPIURL(o)
	if err != nil {
		t.Error(err)
	}
	if apiURL != o.KubernetesURL {
		t.Errorf("unexpected kubernetes API server URL")
	}

	o.KubernetesInCluster = true

	curEnvHostVar, curEnvPortVar := os.Getenv(serviceHostEnvVar), os.Getenv(servicePortEnvVar)
	defer func(host, port string) {
		os.Setenv(serviceHostEnvVar, host)
		os.Setenv(servicePortEnvVar, port)
	}(curEnvHostVar, curEnvPortVar)

	dummyHost := "10.0.0.2"
	dummyPort := "8080"

	os.Unsetenv(serviceHostEnvVar)
	os.Unsetenv(servicePortEnvVar)

	apiURL, err = buildAPIURL(o)
	if apiURL != "" || err != errAPIServerURLNotFound {
		t.Error("build API url should fail if env var is missing")
	}

	os.Setenv(serviceHostEnvVar, dummyHost)
	apiURL, err = buildAPIURL(o)
	if apiURL != "" || err != errAPIServerURLNotFound {
		t.Error("build API url should fail if env var is missing")
	}

	os.Setenv(servicePortEnvVar, dummyPort)
	apiURL, err = buildAPIURL(o)
	if apiURL != "https://10.0.0.2:8080" || err != nil {
		t.Error("incorrect result of build api url")
	}
}

func TestBuildHTTPClient(t *testing.T) {
	httpClient, err := buildHTTPClient("", false)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(httpClient, http.DefaultClient) {
		t.Errorf("should return default client if outside the cluster``")
	}

	httpClient, err = buildHTTPClient("rumplestilzchen", true)
	if err == nil {
		t.Errorf("expected to fail for non-existing file")
	}

	httpClient, err = buildHTTPClient("kube_test.go", true)
	if err != errInvalidCertificate {
		t.Errorf("should return invalid certificate")
	}

	err = ioutil.WriteFile("ca.empty.crt", []byte(""), 0644)
	if err != nil {
		t.Error(err)
	}
	defer os.Remove("ca.empty.crt")

	_, err = buildHTTPClient("ca.empty.crt", true)
	if err != errInvalidCertificate {
		t.Error("empty certificate is invalid certificate")
	}

	//create CA file
	err = ioutil.WriteFile("ca.temp.crt", generateSSCert(), 0644)
	if err != nil {
		t.Error(err)
	}
	defer os.Remove("ca.temp.crt")

	httpClient, err = buildHTTPClient("ca.temp.crt", true)
	if err != nil {
		t.Error(err)
	}
}

func TestReadServiceAccountToken(t *testing.T) {
	var (
		token string
		err   error
	)

	token, err = readServiceAccountToken("kube_test.go", false)
	if err != nil {
		t.Error(err)
	}
	if token != "" {
		t.Errorf("unexpected token: %s", token)
	}

	token, err = readServiceAccountToken("kube_test.go", true)
	if err != nil {
		t.Error(err)
	}
	if token == "" {
		t.Errorf("unexpected token: %s", token)
	}

	token, err = readServiceAccountToken("rumplestilzchen", true)
	if err == nil {
		t.Errorf("expected error for a wrong filename")
	}
	if token != "" {
		t.Errorf("token must be empty")
	}
}

// generateSSCert only for testing purposes
func generateSSCert() []byte {
	//create root CA
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)

	tmpl := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{Organization: []string{"Yhat, Inc."}},
		SignatureAlgorithm:    x509.SHA256WithRSA,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour), // valid for an hour
		BasicConstraintsValid: true,
	}
	rootKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	tmpl.IsCA = true
	tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}

	_, rootCertPEM, _ := CreateCert(tmpl, tmpl, &rootKey.PublicKey, rootKey)
	return rootCertPEM
}

func CreateCert(template, parent *x509.Certificate, pub interface{}, parentPriv interface{}) (
	cert *x509.Certificate, certPEM []byte, err error) {

	certDER, err := x509.CreateCertificate(rand.Reader, template, parent, pub, parentPriv)
	if err != nil {
		return
	}
	// parse the resulting certificate so we can use it again
	cert, err = x509.ParseCertificate(certDER)
	if err != nil {
		return
	}
	// PEM encode the certificate (this is a standard TLS encoding)
	b := pem.Block{Type: "CERTIFICATE", Bytes: certDER}
	certPEM = pem.EncodeToMemory(&b)
	return
}
