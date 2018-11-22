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
	mrand "math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"sort"
	"syscall"
	"testing"
	"testing/quick"
	"time"

	"github.com/google/go-cmp/cmp"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/predicates/source"
)

type (
	services  map[string]map[string]*service
	endpoints map[string]map[string]endpoint
)

type testAPI struct {
	test      *testing.T
	services  services
	ingresses *ingressList
	endpoints endpoints
	server    *httptest.Server
	failNext  bool
}

var (
	serviceURIRx  = regexp.MustCompile("^/api/v1/namespaces/([^/]+)/services/([^/]+)$")
	endpointURIRx = regexp.MustCompile("^/api/v1/namespaces/([^/]+)/endpoints/([^/]+)")
)

func init() {
	log.SetLevel(log.InfoLevel)
	// log.SetLevel(log.DebugLevel)
}

func testService(clusterIP string, ports map[string]int) *service {
	return testServiceWithTargetPort(clusterIP, ports, nil)
}

func testServiceWithTargetPort(clusterIP string, ports map[string]int, targetPorts map[int]*backendPort) *service {
	sports := make([]*servicePort, 0, len(ports))
	for name, port := range ports {
		sports = append(sports, &servicePort{
			Name:       name,
			Port:       port,
			TargetPort: targetPorts[port],
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

func setAnnotation(i *ingressItem, key, value string) {
	if i.Metadata == nil {
		i.Metadata = &metadata{}
	}

	if i.Metadata.Annotations == nil {
		i.Metadata.Annotations = make(map[string]string)
	}

	i.Metadata.Annotations[key] = value
}

func testIngress(ns, name, defaultService, ratelimitCfg, filterString, predicateString, routesString, pathModeString string, defaultPort backendPort, traffic float64, rules ...*rule) *ingressItem {
	var defaultBackend *backend
	if len(defaultService) != 0 {
		defaultBackend = &backend{
			ServiceName: defaultService,
			ServicePort: defaultPort,
			Traffic:     traffic,
		}
	}

	meta := metadata{
		Namespace:   ns,
		Name:        name,
		Annotations: make(map[string]string),
	}
	i := &ingressItem{
		Metadata: &meta,
		Spec: &ingressSpec{
			DefaultBackend: defaultBackend,
			Rules:          rules,
		},
	}
	if ratelimitCfg != "" {
		setAnnotation(i, ratelimitAnnotationKey, ratelimitCfg)
	}
	if filterString != "" {
		setAnnotation(i, skipperfilterAnnotationKey, filterString)
	}
	if predicateString != "" {
		setAnnotation(i, skipperpredicateAnnotationKey, predicateString)
	}
	if routesString != "" {
		setAnnotation(i, skipperRoutesAnnotationKey, routesString)
	}
	if pathModeString != "" {
		setAnnotation(i, pathModeAnnotationKey, pathModeString)
	}

	return i
}

func testIngressSimple(ns, name, defaultService string, defaultPort backendPort, rules ...*rule) *ingressItem {
	return testIngress(
		ns,
		name,
		defaultService,
		"",
		"",
		"",
		"",
		"",
		defaultPort,
		0,
		rules...,
	)
}

func testServices() services {
	return services{
		"namespace1": map[string]*service{
			"service1": testService("1.2.3.4", map[string]int{"port1": 8080}),
			"service2": testService("5.6.7.8", map[string]int{"port2": 8181}),
		},
		"namespace2": map[string]*service{
			"service3": testService("9.0.1.2", map[string]int{"port3": 7272}),
			"service4": testService("10.0.1.2", map[string]int{"port4": 4444, "port5": 5555}),
		},
	}
}

func testIngresses() []*ingressItem {
	return []*ingressItem{
		testIngress("namespace1", "default-only", "service1", "", "", "", "", "", backendPort{8080}, 1.0),
		testIngress(
			"namespace2",
			"path-rule-only",
			"",
			"",
			"",
			"",
			"",
			"",
			backendPort{},
			1.0,
			testRule(
				"www.example.org",
				testPathRule("/", "service3", backendPort{"port3"}),
			),
		),
		testIngress(
			"namespace1",
			"mega",
			"service1",
			"",
			"",
			"",
			"",
			"",
			backendPort{"port1"},
			1.0,
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
		testIngress("namespace1", "ratelimit", "service1", "localRatelimit(20,\"1m\")", "", "", "", "", backendPort{8080}, 1.0),
		testIngress("namespace1", "ratelimitAndBreaker", "service1", "", "localRatelimit(20,\"1m\") -> consecutiveBreaker(15)", "", "", "", backendPort{8080}, 1.0),
		testIngress("namespace2", "svcwith2ports", "service4", "", "", "", "", "", backendPort{4444}, 1.0),
	}
}

func checkRoutes(t *testing.T, r []*eskip.Route, expected map[string]string) {
	if len(r) != len(expected) {
		curIDs := make([]string, len(r))
		expectedIDs := make([]string, len(expected))
		for i := range r {
			curIDs[i] = r[i].Id
		}
		j := 0
		for k := range expected {
			expectedIDs[j] = k
			j++
		}
		sort.Strings(expectedIDs)
		sort.Strings(curIDs)
		t.Errorf("number of routes %d doesn't match expected %d: %v", len(r), len(expected), cmp.Diff(expectedIDs, curIDs))
		return
	}

	for id, backend := range expected {
		var found bool
		for _, ri := range r {
			if ri.Id == id {
				if ri.Backend != backend {
					t.Errorf("invalid backend %v", cmp.Diff(ri.Backend, backend))
					return
				}

				found = true
			}
		}

		if !found {
			t.Error("expected route not found", id, backend)
			t.Errorf("routes %v", r)
			return
		}
	}
}

func checkRoutesDoc(t *testing.T, r []*eskip.Route, expect string) {
	expectRoutes, err := eskip.Parse(expect)
	if err != nil {
		t.Fatal(err)
	}

	checkRoutesExact(t, r, expectRoutes)
}

func checkRoutesExact(t *testing.T, r []*eskip.Route, expect []*eskip.Route) {
	if len(r) != len(expect) {
		t.Error("length doesn't match")
		t.Log("got:     ", len(r))
		t.Log("expected:", len(expect))
		return
	}

	for i := range r {
		var found bool
		for j := range expect {
			if r[i].Id == expect[j].Id && r[i].String() == expect[j].String() {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("route not found: %s: %v", r[i].Id, r[i])
			return
		}
	}
}

func checkIDs(t *testing.T, got []string, expected ...string) {
	if len(got) != len(expected) {
		t.Errorf("number of IDs %d doesn't match expected %d", len(got), len(expected))
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

func checkHealthcheck(t *testing.T, got []*eskip.Route, expected, healthy, reversed bool) {
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
			if reversed && p.Name != source.NameLast {
				continue
			}
			if !reversed && p.Name != source.Name {
				continue
			}

			found = true

			if len(p.Args) != len(internalIPs) {
				t.Error("invalid source predicate")
				return
			}

			for _, s := range internalIPs {
				var found2 bool
				for _, sp := range p.Args {
					if sp == s {
						found2 = true
						break
					}
				}

				if !found2 {
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
	return newTestAPIWithEndpoints(t, s, i, nil)
}

func newTestAPIWithEndpoints(t *testing.T, s services, i *ingressList, e endpoints) *testAPI {
	api := &testAPI{
		test:      t,
		services:  s,
		ingresses: i,
		endpoints: e,
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

func (api *testAPI) getEndpoints(uri string) endpoint {
	var ep endpoint
	if m := endpointURIRx.FindAllStringSubmatch(uri, -1); len(m) != 0 {
		ns, n := m[0][1], m[0][2]
		ep = api.endpoints[ns][n]
	}

	return ep
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

	if r.URL.Path == ingressesClusterURI {
		if err := respondJSON(w, api.ingresses); err != nil {
			api.test.Error(err)
		}

		return
	}

	if endpointURIRx.MatchString(r.URL.Path) {
		ep := api.getEndpoints(r.URL.Path)
		if err := respondJSON(w, ep); err != nil {
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
		[]*ingressItem{testIngress("foo", "baz", "bar", "", "", "", "", "", backendPort{8080}, 1.0)},
		map[string]string{
			"kube_foo__baz_______0": "http://1.2.3.4:8080",
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
			"",
			"",
			"",
			"",
			"",
			backendPort{},
			1.0,
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
			"kube_foo__baz__www_example_org_____bar_0": "http://1.2.3.4:8181",
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
			"",
			"",
			"",
			"",
			"",
			backendPort{8080},
			1.0,
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
			"kube_foo__qux_______0":                    "http://1.2.3.4:8080",
			"kube_foo__qux__www_example_org_____baz_0": "http://5.6.7.8:8181",
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
			"",
			"",
			"",
			"",
			"",
			backendPort{},
			1.0,
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
			"kube_foo__qux__www_example_org_____bar_0": "http://1.2.3.4:8181",
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
				"",
				"",
				"",
				"",
				"",
				backendPort{},
				1.0,
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
			"kube_foo__qux__www_example_org_____bar_0": "http://1.2.3.4:8181",
		},
	}, {
		"skipper-routes annotation",
		services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		[]*ingressItem{testIngress(
			"foo",
			"qux",
			"",
			"",
			"",
			"",
			`Method("OPTIONS") -> <shunt>`,
			"",
			backendPort{},
			1.0,
			testRule(
				"www1.example.org",
				testPathRule(
					"/",
					"bar",
					backendPort{"baz"},
				),
			),
			testRule(
				"www2.example.org",
				testPathRule(
					"/",
					"bar",
					backendPort{"baz"},
				),
			),
			testRule(
				"www3.example.org",
				testPathRule(
					"/foo",
					"bar",
					backendPort{"baz"},
				),
			),
		)},
		map[string]string{
			"kube_foo__qux__www1_example_org_____bar_0":    "http://1.2.3.4:8181",
			"kube_foo__qux__www2_example_org_____bar_0":    "http://1.2.3.4:8181",
			"kube_foo__qux__www3_example_org___foo__bar_0": "http://1.2.3.4:8181",
			"kube_foo__qux__0__www1_example_org______0":    "",
			"kube_foo__qux__0__www2_example_org______0":    "",
			"kube_foo__qux__0__www3_example_org_foo_____0": "",
			"kube___catchall__www3_example_org_____0":      "",
		},
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			api := newTestAPI(t, ti.services, &ingressList{Items: ti.ingresses})
			defer api.Close()
			dc, err := New(Options{KubernetesURL: api.server.URL})
			if err != nil {
				t.Error(err)
			}

			defer dc.Close()

			r, err := dc.LoadAll()
			if err != nil {
				t.Error(err)
				return
			}

			checkRoutes(t, r, ti.expectedRoutes)
		})
	}
}

func TestIngressClassFilter(t *testing.T) {
	tests := []struct {
		testTitle     string
		items         []*ingressItem
		ingressClass  string
		expectedItems []*ingressItem
	}{
		{
			testTitle: "filter no class ingresses",
			items: []*ingressItem{
				{Metadata: &metadata{
					Name: "test1_valid1",
				}},
				{Metadata: &metadata{
					Name: "test1_valid2",
				}},
			},
			ingressClass: "^test-filter$",
			expectedItems: []*ingressItem{
				{Metadata: &metadata{
					Name: "test1_valid1",
				}},
				{Metadata: &metadata{
					Name: "test1_valid2",
				}},
			},
		},
		{
			testTitle: "filter specific key ingress",
			items: []*ingressItem{
				{Metadata: &metadata{
					Name: "test1_valid1",
					Annotations: map[string]string{
						ingressClassKey: "test-filter",
					},
				}},
				{Metadata: &metadata{
					Name: "test1_not_valid2",
					Annotations: map[string]string{
						ingressClassKey: "another-test-filter",
					},
				}},
			},
			ingressClass: "^test-filter$",
			expectedItems: []*ingressItem{
				{Metadata: &metadata{
					Name: "test1_valid1",
				}},
			},
		},
		{
			testTitle: "filter empty class ingresses",
			items: []*ingressItem{
				{Metadata: &metadata{
					Name: "test1_valid1",
					Annotations: map[string]string{
						ingressClassKey: "",
					},
				}},
				{Metadata: &metadata{
					Name: "test1_not_valid2",
					Annotations: map[string]string{
						ingressClassKey: "another-test-filter",
					},
				}},
			},
			ingressClass: "^test-filter$",
			expectedItems: []*ingressItem{
				{Metadata: &metadata{
					Name: "test1_valid1",
				}},
			},
		},
		{
			testTitle: "explicitly include any ingress class",
			items: []*ingressItem{
				{Metadata: &metadata{
					Name: "test1_valid1",
					Annotations: map[string]string{
						ingressClassKey: "",
					},
				}},
				{Metadata: &metadata{
					Name: "test1_valid2",
					Annotations: map[string]string{
						ingressClassKey: "test-filter",
					},
				}},
			},
			ingressClass: ".*",
			expectedItems: []*ingressItem{
				{Metadata: &metadata{
					Name: "test1_valid1",
				}},
				{Metadata: &metadata{
					Name: "test1_valid2",
				}},
			},
		},
		{
			testTitle: "match from a set of ingress classes",
			items: []*ingressItem{
				{Metadata: &metadata{
					Name: "test1_valid1",
					Annotations: map[string]string{
						ingressClassKey: "skipper-test, other-test",
					},
				}},
				{Metadata: &metadata{
					Name: "test1_valid2",
					Annotations: map[string]string{
						ingressClassKey: "other-test",
					},
				}},
			},
			ingressClass: "skipper-test",
			expectedItems: []*ingressItem{
				{Metadata: &metadata{
					Name: "test1_valid1",
				}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.testTitle, func(t *testing.T) {
			clsRx, err := regexp.Compile(test.ingressClass)
			if err != nil {
				t.Error(err)
				return
			}

			c := &Client{
				ingressClass: clsRx,
			}

			result := c.filterIngressesByClass(test.items)
			lRes := len(result)
			eRes := len(test.expectedItems)
			// Check length
			if lRes != eRes {
				t.Errorf("filtered ingresses length is wrong, got: %d, want:%d", lRes, eRes)
			} else {
				// Check items
				for i, exIng := range test.expectedItems {
					exName := exIng.Metadata.Name
					name := result[i].Metadata.Name
					if exName != name {
						t.Errorf("filtered ingress doesn't match, got: %s, want: %s", exName, name)
					}
				}
			}
		})
	}
}

func TestIngress(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	t.Run("no services, no ingresses, load empty initial and update", func(t *testing.T) {
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

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

		defer dc.Close()

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

		defer dc.Close()

		if r, err2 := dc.LoadAll(); err2 != nil || len(r) != 0 {
			t.Error("failed to load initial")
		}

		api.services = testServices()

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("udpate failed")
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__default_only_______0":                           "http://1.2.3.4:8080",
			"kube_namespace2__path_rule_only__www_example_org_____service3_0": "http://9.0.1.2:7272",
			"kube_namespace1__mega_______0":                                   "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test1__service1_0":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test2__service2_1":      "http://5.6.7.8:8181",
			"kube___catchall__foo_example_org_____0":                          "",
			"kube_namespace1__mega__bar_example_org___test1__service1_0":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__bar_example_org___test2__service2_1":      "http://5.6.7.8:8181",
			"kube___catchall__bar_example_org_____0":                          "",
			"kube_namespace1__ratelimit_______0":                              "http://1.2.3.4:8080",
			"kube_namespace1__ratelimitAndBreaker_______0":                    "http://1.2.3.4:8080",
			"kube_namespace2__svcwith2ports_______0":                          "http://10.0.1.2:4444",
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

		defer dc.Close()

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
			"kube_namespace1__mega__foo_example_org___test1__service1_0": "http://1.2.3.4:8080",
		})
	})

	t.Run("has ingresses, receive initial", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__default_only_______0":                           "http://1.2.3.4:8080",
			"kube_namespace2__path_rule_only__www_example_org_____service3_0": "http://9.0.1.2:7272",
			"kube_namespace1__mega_______0":                                   "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test1__service1_0":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test2__service2_1":      "http://5.6.7.8:8181",
			"kube___catchall__foo_example_org_____0":                          "",
			"kube_namespace1__mega__bar_example_org___test1__service1_0":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__bar_example_org___test2__service2_1":      "http://5.6.7.8:8181",
			"kube___catchall__bar_example_org_____0":                          "",
			"kube_namespace1__ratelimit_______0":                              "http://1.2.3.4:8080",
			"kube_namespace1__ratelimitAndBreaker_______0":                    "http://1.2.3.4:8080",
			"kube_namespace2__svcwith2ports_______0":                          "http://10.0.1.2:4444",
		})
	})

	t.Run("has ingresses, update some of them", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

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
			"kube_namespace1__default_only_______0":                      "http://1.2.3.4:6363",
			"kube_namespace1__mega__foo_example_org___test1__service1_0": "http://1.2.3.4:9999",
		})
	})

	t.Run("has ingresses, lose a service", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

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
			"kube_namespace1__mega__foo_example_org___test2__service2_1",
			"kube_namespace1__mega__bar_example_org___test2__service2_1",
		)
	})

	t.Run("has ingresses, delete some ingresses", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

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
			"kube_namespace2__path_rule_only__www_example_org_____service3_0",
			"kube_namespace1__mega_______0",
			"kube_namespace1__mega__foo_example_org___test1__service1_0",
			"kube_namespace1__mega__foo_example_org___test2__service2_1",
			"kube___catchall__foo_example_org_____0",
			"kube_namespace1__mega__bar_example_org___test1__service1_0",
			"kube_namespace1__mega__bar_example_org___test2__service2_1",
			"kube___catchall__bar_example_org_____0",
			"kube_namespace1__ratelimit_______0",
			"kube_namespace1__ratelimitAndBreaker_______0",
			"kube_namespace2__svcwith2ports_______0",
		)
	})

	t.Run("has ingresses, delete some ingress rules", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

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
			"kube_namespace1__mega__bar_example_org___test1__service1_0",
			"kube_namespace1__mega__bar_example_org___test2__service2_1",
			"kube___catchall__bar_example_org_____0",
		)
	})

	t.Run("has ingresses, add new ones", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

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
				"",
				"",
				"",
				"",
				"",
				backendPort{""},
				1.0,
				testRule(
					"new1.example.org",
					testPathRule("/", "service1", backendPort{"port1"}),
				),
			),
			testIngress(
				"namespace1",
				"new2",
				"",
				"",
				"",
				"",
				"",
				"",
				backendPort{""},
				1.0,
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
			"kube_namespace1__new1__new1_example_org_____service1_0": "http://1.2.3.4:8080",
			"kube_namespace1__new2__new2_example_org_____service2_0": "http://5.6.7.8:8181",
		})
	})

	t.Run("has ingresses, mixed insert, update, delete", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

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
				"",
				"",
				"",
				"",
				"",
				backendPort{""},
				1.0,
				testRule(
					"new1.example.org",
					testPathRule("/", "service1", backendPort{"port1"}),
				),
			),
			testIngress(
				"namespace1",
				"new2",
				"",
				"",
				"",
				"",
				"",
				"",
				backendPort{""},
				1.0,
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
			"kube_namespace1__new1__new1_example_org_____service1_0":          "http://1.2.3.4:8080",
			"kube_namespace1__new2__new2_example_org_____service2_0":          "http://5.6.7.8:8181",
			"kube_namespace2__path_rule_only__www_example_org_____service3_0": "http://9.0.1.2:9999",
		})

		checkIDs(
			t,
			d,
			"kube_namespace1__mega__bar_example_org___test1__service1_0",
			"kube_namespace1__mega__bar_example_org___test2__service2_1",
			"kube___catchall__bar_example_org_____0",
		)
	})
	t.Run("has ingresses, add new ones and filter not valid ones using class ingress", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		_, err = dc.LoadAll()
		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		ti1 := testIngress(
			"namespace1",
			"new1",
			"",
			"",
			"",
			"",
			"",
			"",
			backendPort{""},
			1.0,
			testRule(
				"new1.example.org",
				testPathRule("/", "service1", backendPort{"port1"}),
			),
		)
		ti2 := testIngress(
			"namespace1",
			"new2",
			"",
			"",
			"",
			"",
			"",
			"",
			backendPort{""},
			1.0,
			testRule(
				"new2.example.org",
				testPathRule("/", "service2", backendPort{"port2"}),
			),
		)
		// Set class ingress class annotation
		ti1.Metadata.Annotations = map[string]string{ingressClassKey: defaultIngressClass}
		ti2.Metadata.Annotations = map[string]string{ingressClassKey: "nginx"}

		api.ingresses.Items = append(api.ingresses.Items, ti1, ti2)
		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("invalid updated received", err, len(d))
			return
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__new1__new1_example_org_____service1_0": "http://1.2.3.4:8080",
		})
	})
}

func TestConvertPathRule(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	t.Run("has ingresses, receive two equal backends", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()

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
				"",
				"",
				"",
				"",
				"",
				backendPort{""},
				1.0,
				testRule(
					"new1.example.org",
					testPathRule("/test1", "service1", backendPort{"port1"}),
				),
			),
			testIngress(
				"namespace1",
				"new1",
				"",
				"",
				"",
				"",
				"",
				"",
				backendPort{""},
				1.0,
				testRule(
					"new1.example.org",
					testPathRule("/test2", "service1", backendPort{"port1"}),
				),
			),
		)

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("update failed")
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__new1__new1_example_org___test1__service1_0": "http://1.2.3.4:8080",
			"kube___catchall__new1_example_org_____0":                     "",
			"kube_namespace1__new1__new1_example_org___test2__service1_0": "http://1.2.3.4:8080",
		})
	})

	t.Run("has one ingress, with two rules with same service name, but two different ports", func(t *testing.T) {
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()

		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		api.services = services{
			"namespace1": map[string]*service{
				"svcname1": {
					Spec: &serviceSpec{
						ClusterIP: "10.3.2.1",
						Ports: []*servicePort{
							{
								Name:       "svcname1",
								Port:       8080,
								TargetPort: &backendPort{value: 8080},
							},
							{
								Name:       "svcname1",
								Port:       8181,
								TargetPort: &backendPort{value: 8181},
							},
						},
					},
				},
			},
		}

		api.ingresses.Items = append(
			api.ingresses.Items,
			&ingressItem{
				Metadata: &metadata{
					Namespace: "namespace1",
					Name:      "test1",
				},
				Spec: &ingressSpec{
					DefaultBackend: &backend{
						ServiceName: "svcname1",
						ServicePort: backendPort{value: 8080},
					},
					Rules: []*rule{
						{
							Host: "host1",
							Http: &httpRule{
								Paths: []*pathRule{
									{
										Path: "/",
										Backend: &backend{
											ServiceName: "svcname1",
											ServicePort: backendPort{value: 8080},
										},
									},
								},
							},
						},
						{
							Host: "host2",
							Http: &httpRule{
								Paths: []*pathRule{
									{
										Path: "/",
										Backend: &backend{
											ServiceName: "svcname1",
											ServicePort: backendPort{value: 8181},
										},
									},
								},
							},
						},
					},
				},
			},
		)

		r, _, err = dc.LoadUpdate()
		if err != nil {
			t.Errorf("update failed: %v", err)
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__test1__host1_____svcname1_0": "http://10.3.2.1:8080",
			"kube_namespace1__test1__host2_____svcname1_0": "http://10.3.2.1:8181",
			"kube_namespace1__test1_______0":               "http://10.3.2.1:8080",
		})
	})

}

func TestConvertPathRuleEastWestEnabled(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()
	t.Run("has one ingress, receive two backends pointing to the same backend", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{
			KubernetesURL:            api.server.URL,
			KubernetesEnableEastWest: true,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()

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
				"",
				"",
				"",
				"",
				"",
				backendPort{""},
				1.0,
				testRule(
					"new1.example.org",
					testPathRule("/test1", "service1", backendPort{"port1"}),
				),
			),
		)

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("update failed")
		}

		checkRoutes(t, r, map[string]string{
			"kube___catchall__new1_example_org_____0":                       "",
			"kube_namespace1__new1__new1_example_org___test1__service1_0":   "http://1.2.3.4:8080",
			"kubeew_namespace1__new1__new1_example_org___test1__service1_0": "http://1.2.3.4:8080",
			"kubeew___catchall__new1_example_org_____0":                     "",
		})
	})

	t.Run("has ingresses, receive two equal backends", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{
			KubernetesURL:            api.server.URL,
			KubernetesEnableEastWest: true,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()

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
				"",
				"",
				"",
				"",
				"",
				backendPort{""},
				1.0,
				testRule(
					"new1.example.org",
					testPathRule("/test1", "service1", backendPort{"port1"}),
				),
			),
			testIngress(
				"namespace1",
				"new1",
				"",
				"",
				"",
				"",
				"",
				"",
				backendPort{""},
				1.0,
				testRule(
					"new1.example.org",
					testPathRule("/test2", "service1", backendPort{"port1"}),
				),
			),
		)

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("update failed")
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__new1__new1_example_org___test1__service1_0":   "http://1.2.3.4:8080",
			"kube___catchall__new1_example_org_____0":                       "",
			"kube_namespace1__new1__new1_example_org___test2__service1_0":   "http://1.2.3.4:8080",
			"kubeew_namespace1__new1__new1_example_org___test1__service1_0": "http://1.2.3.4:8080",
			"kubeew_namespace1__new1__new1_example_org___test2__service1_0": "http://1.2.3.4:8080",
			"kubeew___catchall__new1_example_org_____0":                     "",
		})
	})

	t.Run("has one ingress, with two rules with same service name, but two different ports", func(t *testing.T) {
		dc, err := New(Options{
			KubernetesURL:            api.server.URL,
			KubernetesEnableEastWest: true,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()

		if err != nil {
			t.Error("failed to load initial routes", err)
			return
		}

		api.services = services{
			"namespace1": map[string]*service{
				"svcname1": {
					Spec: &serviceSpec{
						ClusterIP: "10.3.2.1",
						Ports: []*servicePort{
							{
								Name:       "svcname1",
								Port:       8080,
								TargetPort: &backendPort{value: 8080},
							},
							{
								Name:       "svcname1",
								Port:       8181,
								TargetPort: &backendPort{value: 8181},
							},
						},
					},
				},
			},
		}

		api.ingresses.Items = append(
			api.ingresses.Items,
			&ingressItem{
				Metadata: &metadata{
					Namespace: "namespace1",
					Name:      "test1",
				},
				Spec: &ingressSpec{
					DefaultBackend: &backend{
						ServiceName: "svcname1",
						ServicePort: backendPort{value: 8080},
					},
					Rules: []*rule{
						{
							Host: "host1",
							Http: &httpRule{
								Paths: []*pathRule{
									{
										Path: "/",
										Backend: &backend{
											ServiceName: "svcname1",
											ServicePort: backendPort{value: 8080},
										},
									},
								},
							},
						},
						{
							Host: "host2",
							Http: &httpRule{
								Paths: []*pathRule{
									{
										Path: "/",
										Backend: &backend{
											ServiceName: "svcname1",
											ServicePort: backendPort{value: 8181},
										},
									},
								},
							},
						},
					},
				},
			},
		)

		r, _, err = dc.LoadUpdate()
		if err != nil {
			t.Errorf("update failed: %v", err)
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__test1__host1_____svcname1_0":   "http://10.3.2.1:8080",
			"kube_namespace1__test1__host2_____svcname1_0":   "http://10.3.2.1:8181",
			"kube_namespace1__test1_______0":                 "http://10.3.2.1:8080",
			"kubeew_namespace1__test1__host1_____svcname1_0": "http://10.3.2.1:8080",
			"kubeew_namespace1__test1__host2_____svcname1_0": "http://10.3.2.1:8181",
		})
	})

}

func TestConvertPathRuleTraffic(t *testing.T) {
	for _, tc := range []struct {
		msg   string
		rule  *pathRule
		route *eskip.Route
	}{
		{
			msg: "if traffic weight is between 0 and 1 predicate should be added to route",
			rule: &pathRule{
				Path: "",
				Backend: &backend{
					ServiceName: "service1",
					ServicePort: backendPort{"port1"},
					Traffic:     0.3,
				},
			},
			route: &eskip.Route{
				Id:      routeID("namespace1", "", "", "", "service1", 0),
				Backend: "http://1.2.3.4:8080",
				Predicates: []*eskip.Predicate{
					{
						Name: "Traffic",
						Args: []interface{}{0.3},
					},
				},
			},
		},
		{
			msg: "if traffic weight is 0, don't include traffic predicate",
			rule: &pathRule{
				Path: "",
				Backend: &backend{
					ServiceName: "service1",
					ServicePort: backendPort{"port1"},
					Traffic:     0.0,
				},
			},
			route: &eskip.Route{
				Id:      routeID("namespace1", "", "", "", "service1", 0),
				Backend: "http://1.2.3.4:8080",
			},
		},
		{
			msg: "if traffic weight is 1, don't include traffic predicate",
			rule: &pathRule{
				Path: "",
				Backend: &backend{
					ServiceName: "service1",
					ServicePort: backendPort{"port1"},
					Traffic:     1.0,
				},
			},
			route: &eskip.Route{
				Id:      routeID("namespace1", "", "", "", "service1", 0),
				Backend: "http://1.2.3.4:8080",
			},
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			api := newTestAPI(t, testServices(), &ingressList{})
			defer api.Close()
			dc, err := New(Options{KubernetesURL: api.server.URL})
			if err != nil {
				t.Error(err)
			}

			defer dc.Close()

			_, err = dc.LoadAll()
			if err != nil {
				t.Error("failed to load initial routes", err)
				return
			}

			route, err := dc.convertPathRule("namespace1", "", "", tc.rule, KubernetesIngressMode, map[string][]string{}, 0)
			if err != nil {
				t.Errorf("should not fail: %v", err)
			}

			if !reflect.DeepEqual(tc.route, route[0]) {
				t.Errorf("generated route should match expected route")
			}
		})
	}
}

func TestHealthcheckInitial(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	t.Run("no healthcheck, empty", func(t *testing.T) {
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, false, false, false)
	})

	t.Run("no healthcheck", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, false, false, false)
	})

	t.Run("no healthcheck, fail", func(t *testing.T) {
		api.failNext = true
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		_, err = dc.LoadAll()
		if err == nil {
			t.Error("failed to fail")
		}
	})

	t.Run("use healthcheck, empty", func(t *testing.T) {
		api.ingresses.Items = nil
		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, true, true, false)
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

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, true, true, false)
	})

	t.Run("use reverse healthcheck", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		dc, err := New(Options{
			KubernetesURL:          api.server.URL,
			ProvideHealthcheck:     true,
			ReverseSourcePredicate: true,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, true, true, true)
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

		defer dc.Close()

		dc.LoadAll()
		api.failNext = true

		r, d, err := dc.LoadUpdate()
		if err == nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, false, false, false)
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

		defer dc.Close()

		dc.LoadAll()
		api.failNext = true

		if _, _, err := dc.LoadUpdate(); err == nil {
			t.Error("failed to fail")
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

		defer dc.Close()

		dc.LoadAll()

		r, d, err := dc.LoadUpdate()
		if err != nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, false, false, false)
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

		defer dc.Close()

		dc.LoadAll()

		api.failNext = true
		dc.LoadUpdate()

		r, d, err := dc.LoadUpdate()
		if err != nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, false, false, false)
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

		defer dc.Close()

		dc.LoadAll()
		api.failNext = true

		r, err := dc.LoadAll()
		if err == nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, false, false, false)
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

		defer dc.Close()

		dc.LoadAll()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, true, true, false)
		checkRoutes(t, r, map[string]string{
			healthcheckRouteID:                                                "",
			"kube_namespace1__default_only_______0":                           "http://1.2.3.4:8080",
			"kube_namespace2__path_rule_only__www_example_org_____service3_0": "http://9.0.1.2:7272",
			"kube_namespace1__mega_______0":                                   "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test1__service1_0":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__foo_example_org___test2__service2_1":      "http://5.6.7.8:8181",
			"kube___catchall__foo_example_org_____0":                          "",
			"kube_namespace1__mega__bar_example_org___test1__service1_0":      "http://1.2.3.4:8080",
			"kube_namespace1__mega__bar_example_org___test2__service2_1":      "http://5.6.7.8:8181",
			"kube___catchall__bar_example_org_____0":                          "",
			"kube_namespace1__ratelimit_______0":                              "http://1.2.3.4:8080",
			"kube_namespace1__ratelimitAndBreaker_______0":                    "http://1.2.3.4:8080",
			"kube_namespace2__svcwith2ports_______0":                          "http://10.0.1.2:4444",
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
	quit := make(chan struct{})
	defer func() { close(quit) }()

	httpClient, err := buildHTTPClient("", false, quit)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(httpClient, http.DefaultClient) {
		t.Errorf("should return default client if outside the cluster``")
	}

	httpClient, err = buildHTTPClient("rumplestilzchen", true, quit)
	if err == nil {
		t.Errorf("expected to fail for non-existing file")
	}

	httpClient, err = buildHTTPClient("kube_test.go", true, quit)
	if err != errInvalidCertificate {
		t.Errorf("should return invalid certificate")
	}

	err = ioutil.WriteFile("ca.empty.crt", []byte(""), 0644)
	if err != nil {
		t.Error(err)
	}
	defer os.Remove("ca.empty.crt")

	_, err = buildHTTPClient("ca.empty.crt", true, quit)
	if err != errInvalidCertificate {
		t.Error("empty certificate is invalid certificate")
	}

	//create CA file
	err = ioutil.WriteFile("ca.temp.crt", generateSSCert(), 0644)
	if err != nil {
		t.Error(err)
	}
	defer os.Remove("ca.temp.crt")

	httpClient, err = buildHTTPClient("ca.temp.crt", true, quit)
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

func TestIngressScoping(t *testing.T) {
	client := &Client{
		namespace: "test",
	}
	expected := "/apis/extensions/v1beta1/namespaces/test/ingresses"
	uri := client.getIngressesURI()
	if uri != expected {
		t.Errorf("unexpected ingress uri returned: %s should be %s", uri, expected)
	}

	client.namespace = ""
	expected = "/apis/extensions/v1beta1/ingresses"
	uri = client.getIngressesURI()
	if uri != expected {
		t.Errorf("unexpected ingress uri returned: %s should be %s", uri, expected)
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

	_, rootCertPEM, _ := createCert(tmpl, tmpl, &rootKey.PublicKey, rootKey)
	return rootCertPEM
}

func createCert(template, parent *x509.Certificate, pub interface{}, parentPriv interface{}) (
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

func TestHealthcheckOnTerm(t *testing.T) {
	api := newTestAPI(t, testServices(), &ingressList{})
	defer api.Close()

	t.Run("no difference after term when healthcheck disabled", func(t *testing.T) {
		c, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: false,
		})
		if err != nil {
			t.Error(err)
			return
		}

		defer c.Close()

		// send term only when the client is handling it:
		select {
		case c.sigs <- syscall.SIGTERM:
		default:
		}

		r, err := c.LoadAll()
		if err != nil {
			t.Error(err)
			return
		}

		checkHealthcheck(t, r, false, false, false)
	})

	t.Run("healthcheck false after term", func(t *testing.T) {
		c, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
			return
		}

		defer c.Close()

		// send term only when the client is handling it:
		select {
		case c.sigs <- syscall.SIGTERM:
		default:
		}

		r, err := c.LoadAll()
		if err != nil {
			t.Error(err)
			return
		}

		checkHealthcheck(t, r, true, false, false)
	})

	t.Run("no difference after term when disabled, update", func(t *testing.T) {
		c, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: false,
		})
		if err != nil {
			t.Error(err)
			return
		}

		defer c.Close()

		_, err = c.LoadAll()
		if err != nil {
			t.Error(err)
			return
		}

		// send term only when the client is handling it:
		select {
		case c.sigs <- syscall.SIGTERM:
		default:
		}

		r, _, err := c.LoadUpdate()
		if err != nil {
			t.Error(err)
			return
		}

		checkHealthcheck(t, r, false, false, false)
	})

	t.Run("healthcheck false after term, update", func(t *testing.T) {
		c, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
			return
		}

		defer c.Close()

		_, err = c.LoadAll()
		if err != nil {
			t.Error(err)
			return
		}

		// send term only when the client is handling it:
		select {
		case c.sigs <- syscall.SIGTERM:
		default:
		}

		r, _, err := c.LoadUpdate()
		if err != nil {
			t.Error(err)
			return
		}

		checkHealthcheck(t, r, true, false, false)
	})
}

func TestCatchAllRoutes(t *testing.T) {
	for _, tc := range []struct {
		msg         string
		routes      []*eskip.Route
		hasCatchAll bool
	}{
		{
			msg: "empty path expression is a catchall",
			routes: []*eskip.Route{
				{
					PathRegexps: []string{},
				},
			},
			hasCatchAll: true,
		},
		{
			msg: "^/ path expression is a catchall",
			routes: []*eskip.Route{
				{
					PathRegexps: []string{"^/"},
				},
			},
			hasCatchAll: true,
		},
		{
			msg: "^/test path expression is not a catchall",
			routes: []*eskip.Route{
				{
					PathRegexps: []string{"^/test"},
				},
			},
			hasCatchAll: false,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			catchAll := catchAllRoutes(tc.routes)
			if catchAll != tc.hasCatchAll {
				t.Errorf("expected %t, got %t", tc.hasCatchAll, catchAll)
			}
		})
	}
}

func TestComputeBackendWeights(t *testing.T) {
	for _, tc := range []struct {
		msg     string
		weights map[string]float64
		input   *rule
		output  *rule
	}{
		{
			msg: `if only one backend has a weight, only one backend should get 100% traffic`,
			weights: map[string]float64{
				"foo": 59,
			},
			input: &rule{
				Http: &httpRule{
					Paths: []*pathRule{
						{
							Path: "",
							Backend: &backend{
								ServiceName: "foo",
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "bar",
							},
						},
					},
				},
			},
			output: &rule{
				Http: &httpRule{
					Paths: []*pathRule{
						{
							Path: "",
							Backend: &backend{
								ServiceName: "foo",
								Traffic:     1.0,
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "bar",
								Traffic:     0.0,
							},
						},
					},
				},
			},
		},
		{
			msg:     `if two backends doesn't have any weight, they get equal amount of traffic.`,
			weights: map[string]float64{},
			input: &rule{
				Http: &httpRule{
					Paths: []*pathRule{
						{
							Path: "",
							Backend: &backend{
								ServiceName: "foo",
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "bar",
							},
						},
					},
				},
			},
			output: &rule{
				Http: &httpRule{
					Paths: []*pathRule{
						{
							Path: "",
							Backend: &backend{
								ServiceName: "foo",
								Traffic:     0.5,
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "bar",
								Traffic:     1.0,
							},
						},
					},
				},
			},
		},
		{
			msg: `if all backends has a weight, all should get relative weight.`,
			weights: map[string]float64{
				"foo": 20,
				"bar": 60,
				"baz": 20,
			},
			input: &rule{
				Http: &httpRule{
					Paths: []*pathRule{
						{
							Path: "",
							Backend: &backend{
								ServiceName: "foo",
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "bar",
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "baz",
							},
						},
					},
				},
			},
			output: &rule{
				Http: &httpRule{
					Paths: []*pathRule{
						{
							Path: "",
							Backend: &backend{
								ServiceName: "foo",
								Traffic:     0.2,
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "bar",
								Traffic:     0.75,
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "baz",
								Traffic:     1.0,
							},
						},
					},
				},
			},
		},
		{
			msg: `weights are relative and should always sum to 1.0.`,
			weights: map[string]float64{
				"foo": 60,
				"bar": 140,
			},
			input: &rule{
				Http: &httpRule{
					Paths: []*pathRule{
						{
							Path: "",
							Backend: &backend{
								ServiceName: "foo",
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "bar",
							},
						},
					},
				},
			},
			output: &rule{
				Http: &httpRule{
					Paths: []*pathRule{
						{
							Path: "",
							Backend: &backend{
								ServiceName: "foo",
								Traffic:     0.3,
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "bar",
								Traffic:     1.0,
							},
						},
					},
				},
			},
		},
		{
			msg: `if two of three backends has a weight, only two should get traffic.`,
			weights: map[string]float64{
				"foo": 30,
				"bar": 70,
			},
			input: &rule{
				Http: &httpRule{
					Paths: []*pathRule{
						{
							Path: "",
							Backend: &backend{
								ServiceName: "foo",
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "bar",
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "baz",
							},
						},
					},
				},
			},
			output: &rule{
				Http: &httpRule{
					Paths: []*pathRule{
						{
							Path: "",
							Backend: &backend{
								ServiceName: "foo",
								Traffic:     0.3,
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "bar",
								Traffic:     1.0,
							},
						},
						{
							Path: "",
							Backend: &backend{
								ServiceName: "baz",
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			computeBackendWeights(tc.weights, tc.input)
			if !reflect.DeepEqual(tc.input, tc.output) {
				t.Errorf("modified input and output should match")
			}
		})
	}
}

type backendWeight float64

func (w backendWeight) Generate(rand *mrand.Rand, size int) reflect.Value {
	generatedWeight := rand.Float64() * 100
	return reflect.ValueOf(backendWeight(generatedWeight))
}

func TestComputeBackendWeightMustHaveFallback(t *testing.T) {
	beweight := func(a, b, c, d backendWeight) bool {
		if a+b+c+d == 0.0 {
			return true
		}

		weights := map[string]float64{"foo": float64(a), "bar": float64(b), "baz": float64(c), "quux": float64(d)}
		fooBackend := &backend{
			ServiceName: "foo",
		}
		barBackend := &backend{
			ServiceName: "bar",
		}
		bazBackend := &backend{
			ServiceName: "baz",
		}
		quuxBackend := &backend{
			ServiceName: "quux",
		}
		allBackends := []*backend{fooBackend, barBackend, bazBackend, quuxBackend}

		input := &rule{
			Http: &httpRule{
				Paths: []*pathRule{
					{
						Path:    "",
						Backend: fooBackend,
					},
					{
						Path:    "",
						Backend: barBackend,
					},
					{
						Path:    "",
						Backend: bazBackend,
					},
					{
						Path:    "",
						Backend: quuxBackend,
					},
				},
			},
		}
		computeBackendWeights(weights, input)

		// check that there's one backend with weight of 1.0
		for _, backend := range allBackends {
			if backend.Traffic == 1.0 {
				return true
			}
		}
		return false
	}

	if err := quick.Check(beweight, nil); err != nil {
		t.Error(err)
	}
}

func TestRatelimits(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	t.Run("check localratelimit", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()

		dc, err := New(Options{
			KubernetesURL: api.server.URL,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to fail")
		}

		checkLocalRatelimit(t, r, map[string]string{
			"kube_namespace1__ratelimit______": "localRatelimit(20,\"1m\")",
		})
	})
}

func TestRatelimitsEastWest(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	t.Run("check localratelimit", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()

		dc, err := New(Options{
			KubernetesURL:            api.server.URL,
			KubernetesEnableEastWest: true,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to fail")
		}

		checkLocalRatelimit(t, r, map[string]string{
			"kube_namespace1__ratelimit______":   "localRatelimit(20,\"1m\")",
			"kubeew_namespace1__ratelimit______": "localRatelimit(20,\"1m\")",
		})
	})
}

func checkLocalRatelimit(t *testing.T, got []*eskip.Route, expected map[string]string) {
	for _, r := range got {
		if r.Filters != nil {
			for _, f := range r.Filters {
				_, ok := expected[r.Id]
				if ok && f.Name != "localRatelimit" {
					t.Errorf("%s should have a localratelimit", r.Id)
				}
				if !ok && f.Name == "localRatelimit" {
					t.Errorf("%s should not have a localratelimit", r.Id)
				}
			}
		}
	}
}

func TestSkipperFilter(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	t.Run("check ingress filter", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()

		dc, err := New(Options{
			KubernetesURL: api.server.URL,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to fail")
		}

		checkSkipperFilter(t, r, map[string][]string{
			"kube_namespace1__ratelimitAndBreaker______": {"localRatelimit(20,\"1m\")", "consecutiveBreaker(15)"},
		})
	})
}

func TestSkipperFilterEastWest(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	t.Run("check ingress filter", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()

		dc, err := New(Options{
			KubernetesURL:            api.server.URL,
			KubernetesEnableEastWest: true,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to fail")
		}

		checkSkipperFilter(t, r, map[string][]string{
			"kube_namespace1__ratelimitAndBreaker______":   {"localRatelimit(20,\"1m\")", "consecutiveBreaker(15)"},
			"kubeew_namespace1__ratelimitAndBreaker______": {"localRatelimit(20,\"1m\")", "consecutiveBreaker(15)"},
		})
	})
}

func checkSkipperFilter(t *testing.T, got []*eskip.Route, expected map[string][]string) {
	for _, r := range got {
		if len(r.Filters) == 2 {
			f1 := r.Filters[0]
			f2 := r.Filters[1]
			_, ok := expected[r.Id]
			if ok && f1.Name != "localRatelimit" {
				t.Errorf("%s should have a localratelimit", r.Id)
			}
			if ok && f2.Name != "consecutiveBreaker" {
				t.Errorf("%s should have a consecutiveBreaker", r.Id)
			}
		}
	}
}

func TestSkipperPredicate(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	ingWithPredicate := testIngress("namespace1", "predicate", "service1", "", "", "QueryParam(\"query\", \"^example$\")", "", "", backendPort{8080}, 1.0)

	t.Run("check ingress predicate", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		api.ingresses.Items = append(api.ingresses.Items, ingWithPredicate)

		dc, err := New(Options{
			KubernetesURL: api.server.URL,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to fail")
		}

		checkSkipperPredicate(t, r, map[string]string{
			"kube_namespace1__predicate______": "QueryParam(\"query\", \"^example$\")",
		})
	})
}

func TestSkipperPredicateEastWest(t *testing.T) {
	api := newTestAPI(t, nil, &ingressList{})
	defer api.Close()

	ingWithPredicate := testIngress("namespace1", "predicate", "service1", "", "", "QueryParam(\"query\", \"^example$\")", "", "", backendPort{8080}, 1.0)

	t.Run("check ingress predicate", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		api.ingresses.Items = append(api.ingresses.Items, ingWithPredicate)

		dc, err := New(Options{
			KubernetesURL:            api.server.URL,
			KubernetesEnableEastWest: true,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to fail")
		}

		checkSkipperPredicate(t, r, map[string]string{
			"kube_namespace1__predicate______":   "QueryParam(\"query\", \"^example$\")",
			"kubeew_namespace1__predicate______": "QueryParam(\"query\", \"^example$\")",
		})
	})
}

func checkSkipperPredicate(t *testing.T, got []*eskip.Route, expected map[string]string) {
	for _, r := range got {
		if len(r.Predicates) == 1 {
			p1 := r.Predicates[0]
			_, ok := expected[r.Id]
			if ok && p1.Name != "QueryParam" {
				t.Errorf("%s should have a QueryParam predicate", r.Id)
			}
		}
	}
}

func TestSkipperCustomRoutes(t *testing.T) {
	for _, ti := range []struct {
		msg            string
		services       services
		ingresses      []*ingressItem
		expectedRoutes map[string]string
	}{{
		msg: "ingress with 1 host definitions and 1 additional custom route",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar_0": "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www1_example_org______0": "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
		},
	}, {
		msg: "ingress with 1 host definitions with path and 1 additional custom route",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/a/path", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org___a_path__bar_0": "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www1_example_org_a_path_____0": "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kube___catchall__www1_example_org_____0":         "Host(/^www1[.]example[.]org$/) -> <shunt>",
		},
	}, {
		msg: "ingress with 2 host definitions and 1 additional custom route",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/", "bar", backendPort{"baz"})),
			testRule("www2.example.org", testPathRule("/", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar_0": "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www1_example_org______0": "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux__www2_example_org_____bar_0": "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www2_example_org______0": "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
		},
	}, {
		msg: "ingress with 2 host definitions with path and 1 additional custom route",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/a/path", "bar", backendPort{"baz"})),
			testRule("www2.example.org", testPathRule("/another/path", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org___a_path__bar_0":       "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www1_example_org_a_path_____0":       "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kube___catchall__www1_example_org_____0":               "Host(/^www1[.]example[.]org$/) -> <shunt>",
			"kube_foo__qux__www2_example_org___another_path__bar_0": "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\/another\\/path/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www2_example_org_another_path_____0": "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\/another\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kube___catchall__www2_example_org_____0":               "Host(/^www2[.]example[.]org$/) -> <shunt>",
		},
	}, {
		msg: "ingress with 3 host definitions with one path and 3 additional custom routes",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
				"baz": testService("1.2.3.6", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`a: Method("OPTIONS") -> <shunt>;
                         b: Cookie("alpha", /^enabled$/) -> "http://1.2.3.6:8181";
                         c: Path("/a/path/somewhere") -> "https://some.other-url.org/a/path/somewhere";`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/", "bar", backendPort{"baz"})),
			testRule("www2.example.org", testPathRule("/", "bar", backendPort{"baz"})),
			testRule("www3.example.org", testPathRule("/a/path", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar_0":  "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www1_example_org______0": "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www1_example_org______0": "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www1_example_org______0": "Path(\"/a/path/somewhere\") && Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www2_example_org_____bar_0":  "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www2_example_org______0": "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www2_example_org______0": "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www2_example_org______0": "Path(\"/a/path/somewhere\") && Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www3_example_org___a_path__bar_0":  "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www3_example_org_a_path_____0": "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www3_example_org_a_path_____0": "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www3_example_org_a_path_____0": "Path(\"/a/path/somewhere\") && Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"https://some.other-url.org/a/path/somewhere\"",
			"kube___catchall__www3_example_org_____0":          "Host(/^www3[.]example[.]org$/) -> <shunt>",
		},
	}, {
		msg: "ingress with 3 host definitions with one without path and 3 additional custom routes",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
				"baz": testService("1.2.3.6", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`a: Method("OPTIONS") -> <shunt>;
                         b: Cookie("alpha", /^enabled$/) -> "http://1.2.3.6:8181";
                         c: Path("/a/path/somewhere") -> "https://some.other-url.org/a/path/somewhere";`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("", "bar", backendPort{"baz"})),
			testRule("www2.example.org", testPathRule("/", "bar", backendPort{"baz"})),
			testRule("www3.example.org", testPathRule("/a/path", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org____bar_0":  "Host(/^www1[.]example[.]org$/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www1_example_org_____0": "Host(/^www1[.]example[.]org$/) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www1_example_org_____0": "Host(/^www1[.]example[.]org$/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www1_example_org_____0": "Path(\"/a/path/somewhere\") && Host(/^www1[.]example[.]org$/) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www2_example_org_____bar_0":  "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www2_example_org______0": "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www2_example_org______0": "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www2_example_org______0": "Path(\"/a/path/somewhere\") && Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www3_example_org___a_path__bar_0":  "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www3_example_org_a_path_____0": "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www3_example_org_a_path_____0": "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www3_example_org_a_path_____0": "Path(\"/a/path/somewhere\") && Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"https://some.other-url.org/a/path/somewhere\"",
			"kube___catchall__www3_example_org_____0":          "Host(/^www3[.]example[.]org$/) -> <shunt>",
		},
	}, {
		msg: "ingress with 1 host definitions and 1 additional custom route, changed pathmode to PathSubtree",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"path-prefix", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar_0": "Host(/^www1[.]example[.]org$/) && PathSubtree(\"/\") -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www1_example_org______0": "Host(/^www1[.]example[.]org$/) && Method(\"OPTIONS\") && PathSubtree(\"/\") -> <shunt>",
		},
	}, {
		msg: "ingress with 1 host definitions and 1 additional custom route with path predicate, changed pathmode to PathSubtree",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Path("/foo") -> <shunt>`,
			"path-prefix", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar_0": "Host(/^www1[.]example[.]org$/) && PathSubtree(\"/\") -> \"http://1.2.3.4:8181\"",
		},
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			api := newTestAPI(t, ti.services, &ingressList{Items: ti.ingresses})
			defer api.Close()
			dc, err := New(Options{
				KubernetesURL: api.server.URL,
			})
			if err != nil {
				t.Error(err)
			}

			defer dc.Close()

			r, err := dc.LoadAll()
			if err != nil {
				t.Error(err)
				return
			}

			checkPrettyRoutes(t, r, ti.expectedRoutes)
		})
	}
}

func TestSkipperCustomRoutesEastWest(t *testing.T) {
	for _, ti := range []struct {
		msg            string
		services       services
		ingresses      []*ingressItem
		expectedRoutes map[string]string
	}{{
		msg: "ingress with 1 host definitions and 1 additional custom route",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar_0":   "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www1_example_org______0":   "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew_foo__qux__www1_example_org_____bar_0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux__0__www1_example_org______0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
		},
	}, {
		msg: "ingress with 1 host definitions with path and 1 additional custom route",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/a/path", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org___a_path__bar_0":   "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www1_example_org_a_path_____0":   "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kube___catchall__www1_example_org_____0":           "Host(/^www1[.]example[.]org$/) -> <shunt>",
			"kubeew_foo__qux__www1_example_org___a_path__bar_0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux__0__www1_example_org_a_path_____0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew___catchall__www1_example_org_____0":         "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) -> <shunt>",
		},
	}, {
		msg: "ingress with 2 host definitions and 1 additional custom route",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/", "bar", backendPort{"baz"})),
			testRule("www2.example.org", testPathRule("/", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar_0":   "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www1_example_org______0":   "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux__www2_example_org_____bar_0":   "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www2_example_org______0":   "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew_foo__qux__www1_example_org_____bar_0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux__0__www1_example_org______0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew_foo__qux__www2_example_org_____bar_0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux__0__www2_example_org______0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
		},
	}, {
		msg: "ingress with 2 host definitions with path and 1 additional custom route",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/a/path", "bar", backendPort{"baz"})),
			testRule("www2.example.org", testPathRule("/another/path", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org___a_path__bar_0":         "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www1_example_org_a_path_____0":         "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kube___catchall__www1_example_org_____0":                 "Host(/^www1[.]example[.]org$/) -> <shunt>",
			"kube_foo__qux__www2_example_org___another_path__bar_0":   "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\/another\\/path/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www2_example_org_another_path_____0":   "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\/another\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kube___catchall__www2_example_org_____0":                 "Host(/^www2[.]example[.]org$/) -> <shunt>",
			"kubeew_foo__qux__www1_example_org___a_path__bar_0":       "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux__0__www1_example_org_a_path_____0":       "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew___catchall__www1_example_org_____0":               "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) -> <shunt>",
			"kubeew_foo__qux__www2_example_org___another_path__bar_0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/another\\/path/) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux__0__www2_example_org_another_path_____0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/another\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew___catchall__www2_example_org_____0":               "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) -> <shunt>",
		},
	}, {
		msg: "ingress with 3 host definitions with one path and 3 additional custom routes",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
				"baz": testService("1.2.3.6", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`a: Method("OPTIONS") -> <shunt>;
                         b: Cookie("alpha", /^enabled$/) -> "http://1.2.3.6:8181";
                         c: Path("/a/path/somewhere") -> "https://some.other-url.org/a/path/somewhere";`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/", "bar", backendPort{"baz"})),
			testRule("www2.example.org", testPathRule("/", "bar", backendPort{"baz"})),
			testRule("www3.example.org", testPathRule("/a/path", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar_0":  "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www1_example_org______0": "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www1_example_org______0": "Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www1_example_org______0": "Path(\"/a/path/somewhere\") && Host(/^www1[.]example[.]org$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www2_example_org_____bar_0":  "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www2_example_org______0": "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www2_example_org______0": "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www2_example_org______0": "Path(\"/a/path/somewhere\") && Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www3_example_org___a_path__bar_0":  "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www3_example_org_a_path_____0": "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www3_example_org_a_path_____0": "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www3_example_org_a_path_____0": "Path(\"/a/path/somewhere\") && Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"https://some.other-url.org/a/path/somewhere\"",
			"kube___catchall__www3_example_org_____0":          "Host(/^www3[.]example[.]org$/) -> <shunt>",

			"kubeew_foo__qux__www1_example_org_____bar_0":  "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux_a_0__www1_example_org______0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew_foo__qux_b_1__www1_example_org______0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kubeew_foo__qux__www2_example_org_____bar_0":  "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux_a_0__www2_example_org______0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew_foo__qux_b_1__www2_example_org______0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",

			"kubeew_foo__qux__www3_example_org___a_path__bar_0":  "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux_a_0__www3_example_org_a_path_____0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew_foo__qux_b_1__www3_example_org_a_path_____0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",

			"kubeew_foo__qux_c_2__www3_example_org_a_path_____0": "Path(\"/a/path/somewhere\") && Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) -> \"https://some.other-url.org/a/path/somewhere\"",
			"kubeew_foo__qux_c_2__www1_example_org______0":       "Path(\"/a/path/somewhere\") && Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",
			"kubeew_foo__qux_c_2__www2_example_org______0":       "Path(\"/a/path/somewhere\") && Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",
			"kubeew___catchall__www3_example_org_____0":          "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) -> <shunt>"},
	}, {
		msg: "ingress with 3 host definitions with one without path and 3 additional custom routes",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
				"baz": testService("1.2.3.6", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`a: Method("OPTIONS") -> <shunt>;
		                 b: Cookie("alpha", /^enabled$/) -> "http://1.2.3.6:8181";
		                 c: Path("/a/path/somewhere") -> "https://some.other-url.org/a/path/somewhere";`,
			"", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("", "bar", backendPort{"baz"})),
			testRule("www2.example.org", testPathRule("/", "bar", backendPort{"baz"})),
			testRule("www3.example.org", testPathRule("/a/path", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org____bar_0":    "Host(/^www1[.]example[.]org$/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www1_example_org_____0":   "Host(/^www1[.]example[.]org$/) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www1_example_org_____0":   "Host(/^www1[.]example[.]org$/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www1_example_org_____0":   "Path(\"/a/path/somewhere\") && Host(/^www1[.]example[.]org$/) -> \"https://some.other-url.org/a/path/somewhere\"",
			"kubeew_foo__qux__www1_example_org____bar_0":  "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux_a_0__www1_example_org_____0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew_foo__qux_b_1__www1_example_org_____0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kubeew_foo__qux_c_2__www1_example_org_____0": "Path(\"/a/path/somewhere\") && Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www2_example_org_____bar_0":    "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www2_example_org______0":   "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www2_example_org______0":   "Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www2_example_org______0":   "Path(\"/a/path/somewhere\") && Host(/^www2[.]example[.]org$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",
			"kubeew_foo__qux__www2_example_org_____bar_0":  "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux_a_0__www2_example_org______0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew_foo__qux_b_1__www2_example_org______0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kubeew_foo__qux_c_2__www2_example_org______0": "Path(\"/a/path/somewhere\") && Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www3_example_org___a_path__bar_0":    "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux_a_0__www3_example_org_a_path_____0":   "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www3_example_org_a_path_____0":   "Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kube_foo__qux_c_2__www3_example_org_a_path_____0":   "Path(\"/a/path/somewhere\") && Host(/^www3[.]example[.]org$/) && PathRegexp(/^\\/a\\/path/) -> \"https://some.other-url.org/a/path/somewhere\"",
			"kubeew_foo__qux__www3_example_org___a_path__bar_0":  "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux_a_0__www3_example_org_a_path_____0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) && Method(\"OPTIONS\") -> <shunt>",
			"kubeew_foo__qux_b_1__www3_example_org_a_path_____0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.2.3.6:8181\"",
			"kubeew_foo__qux_c_2__www3_example_org_a_path_____0": "Path(\"/a/path/somewhere\") && Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathRegexp(/^\\/a\\/path/) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube___catchall__www3_example_org_____0":   "Host(/^www3[.]example[.]org$/) -> <shunt>",
			"kubeew___catchall__www3_example_org_____0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) -> <shunt>",
		},
	}, {
		msg: "ingress with 1 host definitions and 1 additional custom route, changed pathmode to PathSubtree",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"path-prefix", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar_0":   "Host(/^www1[.]example[.]org$/) && PathSubtree(\"/\") -> \"http://1.2.3.4:8181\"",
			"kube_foo__qux__0__www1_example_org______0":   "Host(/^www1[.]example[.]org$/) && Method(\"OPTIONS\") && PathSubtree(\"/\") -> <shunt>",
			"kubeew_foo__qux__www1_example_org_____bar_0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathSubtree(\"/\") -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux__0__www1_example_org______0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && Method(\"OPTIONS\") && PathSubtree(\"/\") -> <shunt>",
		},
	}, {
		msg: "ingress with 1 host definitions and 1 additional custom route with path predicate, changed pathmode to PathSubtree",
		services: services{
			"foo": map[string]*service{
				"bar": testService("1.2.3.4", map[string]int{"baz": 8181}),
			},
		},
		ingresses: []*ingressItem{testIngress("foo", "qux", "", "", "", "",
			`Path("/foo") -> <shunt>`,
			"path-prefix", backendPort{}, 1.0,
			testRule("www1.example.org", testPathRule("/", "bar", backendPort{"baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar_0":   "Host(/^www1[.]example[.]org$/) && PathSubtree(\"/\") -> \"http://1.2.3.4:8181\"",
			"kubeew_foo__qux__www1_example_org_____bar_0": "Host(/^qux[.]foo[.]skipper[.]cluster[.]local$/) && PathSubtree(\"/\") -> \"http://1.2.3.4:8181\"",
		},
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			api := newTestAPI(t, ti.services, &ingressList{Items: ti.ingresses})
			defer api.Close()
			dc, err := New(Options{
				KubernetesURL:            api.server.URL,
				KubernetesEnableEastWest: true,
			})
			if err != nil {
				t.Error(err)
			}

			defer dc.Close()

			r, err := dc.LoadAll()
			if err != nil {
				t.Error(err)
				return
			}

			checkPrettyRoutes(t, r, ti.expectedRoutes)
		})
	}
}

func checkPrettyRoutes(t *testing.T, r []*eskip.Route, expected map[string]string) {
	if len(r) != len(expected) {
		curIDs := make([]string, len(r))
		expectedIDs := make([]string, len(expected))
		for i := range r {
			curIDs[i] = r[i].Id
		}
		j := 0
		for k := range expected {
			expectedIDs[j] = k
			j++
		}
		sort.Strings(expectedIDs)
		sort.Strings(curIDs)
		t.Errorf("number of routes %d doesn't match expected %d: %v", len(r), len(expected), cmp.Diff(expectedIDs, curIDs))
		// TODO(sszuecs) kann weg
		for i := 0; i < len(r); i++ {
			t.Logf("%s: %v", r[i].Id, r[i])
		}

		return
	}

	for id, prettyExpectedRoute := range expected {
		var found bool
		for _, ri := range r {
			if ri.Id == id {
				prettyR := ri.Print(eskip.PrettyPrintInfo{})
				if prettyR != prettyExpectedRoute {
					t.Errorf("invalid route %v", cmp.Diff(prettyExpectedRoute, prettyR))
					return
				}

				found = true
			}
		}

		if !found {
			t.Error("expected route not found", id, prettyExpectedRoute)
			return
		}
	}
}

func TestCreateEastWestRoute(t *testing.T) {
	for _, ti := range []struct {
		msg        string
		name       string
		namespace  string
		route      *eskip.Route
		expectedID string
	}{{
		msg:       "valid kube route id",
		name:      "foo",
		namespace: "qux",
		route: &eskip.Route{
			Id:          "kube_foo__qux_a_0__www2_example_org_____",
			HostRegexps: []string{"www2[.]example[.]org"},
		},
		expectedID: "kubeew_foo__qux_a_0__www2_example_org_____",
	}, {
		msg:       "valid kube route id with path",
		name:      "foo",
		namespace: "qux",
		route: &eskip.Route{
			Id:          "kube_foo__qux__www3_example_org___a_path__bar",
			HostRegexps: []string{"www3[.]example[.]org"},
		},
		expectedID: "kubeew_foo__qux__www3_example_org___a_path__bar",
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			ewrs := createEastWestRoutes("foo", "qux", []*eskip.Route{ti.route})
			ewr := ewrs[0]
			if ewr.Id != ti.expectedID {
				t.Errorf("Failed to create east west route ID, %s, but expected %s", ewr.Id, ti.expectedID)
			}
			hostRegexp := ewr.HostRegexps[0]
			reg := regexp.MustCompile(hostRegexp)
			if !reg.MatchString("foo.qux.skipper.cluster.local") {
				t.Errorf("Failed to create correct east west hostregexp, got %s, expected to match %s", ewr.HostRegexps, "foo.qux.skipper.cluster.local")
			}
		})
	}
}
