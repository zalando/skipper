package kubernetes

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/secrets/certregistry"
)

type testAPI struct {
	test      *testing.T
	services  *serviceList
	endpoints *endpointList
	ingresses *definitions.IngressV1List
	secrets   *secretList
	server    *httptest.Server
	failNext  bool
}

func init() {
	log.SetLevel(log.InfoLevel)
	// log.SetLevel(log.DebugLevel)
}

func testEndpointList() *endpointList {
	eps := make([]*endpoint, 0)
	eps = append(eps, testEndpoints("namespace1", "service1", map[string]string{}, "1.1.1", 1, map[string]int{"port1": 8080})...)
	eps = append(eps, testEndpoints("namespace1", "service2", map[string]string{}, "1.1.2", 1, map[string]int{"port2": 8181})...)
	eps = append(eps, testEndpoints("namespace2", "service3", map[string]string{}, "2.1.3", 1, map[string]int{"port3": 7272})...)
	eps = append(eps, testEndpoints("namespace2", "service4", map[string]string{}, "2.1.4", 1, map[string]int{"port4": 4444, "port5": 5555})...)
	return &endpointList{
		Items: eps,
	}
}

func testEndpoints(namespace, name string, labels map[string]string, base string, n int, ports map[string]int) []*endpoint {
	eps := make([]*endpoint, 0, 1)

	eps = append(eps, &endpoint{
		Meta: &definitions.Metadata{
			Namespace: namespace,
			Name:      name,
			Labels:    labels,
		},
		Subsets: []*subset{},
	})
	for j := range eps {
		s := &subset{
			Addresses: []*address{},
			Ports:     []*port{},
		}

		for k, v := range ports {
			s.Ports = append(s.Ports, &port{
				Name: k,
				Port: v,
			})
		}

		for i := 0; i < n; i++ {
			addr := &address{
				IP:   fmt.Sprintf("%s.%d", base, i),
				Node: fmt.Sprintf("node-%d", i),
			}
			s.Addresses = append(s.Addresses, addr)
		}
		eps[j].Subsets = append(eps[j].Subsets, s)
	}
	return eps
}

func testSecret(namespace, name string, labels map[string]string, clusterIP, t string, data map[string]string) *secret {
	return &secret{
		Metadata: &definitions.Metadata{
			Namespace: namespace,
			Name:      name,
			Labels:    labels,
		},
		Type: t,
		Data: data,
	}
}

func testService(namespace, name string, labels map[string]string, clusterIP string, ports map[string]int) *service {
	targetPorts := make(map[int]*definitions.BackendPort)
	for _, v := range ports {
		targetPorts[v] = &definitions.BackendPort{
			Value: v,
		}
	}
	return testServiceWithTargetPort(namespace, name, labels, clusterIP, ports, targetPorts)
}

func testServiceWithTargetPort(namespace, name string, labels map[string]string, clusterIP string, ports map[string]int, targetPorts map[int]*definitions.BackendPort) *service {
	sports := make([]*servicePort, 0, len(ports))
	for name, port := range ports {
		sports = append(sports, &servicePort{
			Name:       name,
			Port:       port,
			TargetPort: targetPorts[port],
		})
	}

	return &service{
		Meta: &definitions.Metadata{
			Namespace: namespace,
			Name:      name,
			Labels:    labels,
		},
		Spec: &serviceSpec{
			ClusterIP: clusterIP,
			Ports:     sports,
		},
	}
}

func testPathRule(path, serviceName string, port definitions.BackendPortV1) *definitions.PathRuleV1 {
	return &definitions.PathRuleV1{
		Path: path,
		Backend: &definitions.BackendV1{
			Service: definitions.Service{
				Name: serviceName,
				Port: port,
			},
		},
	}
}

func testRule(host string, paths ...*definitions.PathRuleV1) *definitions.RuleV1 {
	return &definitions.RuleV1{
		Host: host,
		Http: &definitions.HTTPRuleV1{
			Paths: paths,
		},
	}
}

func setAnnotation(i *definitions.IngressV1Item, key, value string) {
	if i.Metadata == nil {
		i.Metadata = &definitions.Metadata{}
	}

	if i.Metadata.Annotations == nil {
		i.Metadata.Annotations = make(map[string]string)
	}

	i.Metadata.Annotations[key] = value
}

func testIngress(ns, name, defaultService, ratelimitCfg, filterString, predicateString, routesString, pathModeString, lbAlgorithm string, labels map[string]string, defaultPort definitions.BackendPortV1, rules ...*definitions.RuleV1) *definitions.IngressV1Item {
	var defaultBackend *definitions.BackendV1
	if len(defaultService) != 0 {
		defaultBackend = &definitions.BackendV1{
			Service: definitions.Service{
				Name: defaultService,
				Port: defaultPort,
			},
		}
	}

	meta := definitions.Metadata{
		Namespace:   ns,
		Name:        name,
		Annotations: make(map[string]string),
		Labels:      labels,
	}
	i := &definitions.IngressV1Item{
		Metadata: &meta,
		Spec: &definitions.IngressV1Spec{
			DefaultBackend: defaultBackend,
			Rules:          rules,
		},
	}
	if ratelimitCfg != "" {
		setAnnotation(i, ratelimitAnnotationKey, ratelimitCfg)
	}
	if filterString != "" {
		setAnnotation(i, definitions.IngressFilterAnnotation, filterString)
	}
	if predicateString != "" {
		setAnnotation(i, definitions.IngressPredicateAnnotation, predicateString)
	}
	if routesString != "" {
		setAnnotation(i, definitions.IngressRoutesAnnotation, routesString)
	}
	if pathModeString != "" {
		setAnnotation(i, pathModeAnnotationKey, pathModeString)
	}
	if lbAlgorithm != "" {
		setAnnotation(i, skipperLoadBalancerAnnotationKey, lbAlgorithm)
	}

	return i
}

func testSecrets() *secretList {
	return &secretList{
		Items: []*secret{
			testSecret("namespace1", "secret1", map[string]string{}, "1.2.3.4", "Opaque", map[string]string{"foo": "bar"}),
			testSecret("namespace1", "secret2", map[string]string{}, "5.6.7.8", "Opaque", map[string]string{"bar": "foo"}),
			testSecret("namespace2", "secret3", map[string]string{}, "9.0.1.2", "Opaque", map[string]string{"foo": "foo"}),
			testSecret("namespace2", "secret4", map[string]string{}, "10.0.1.2", "Opaque", map[string]string{"bar": "bar"}),
		},
	}
}

func testServices() *serviceList {
	return &serviceList{
		Items: []*service{
			testService("namespace1", "service1", map[string]string{}, "1.2.3.4", map[string]int{"port1": 8080}),
			testService("namespace1", "service2", map[string]string{}, "5.6.7.8", map[string]int{"port2": 8181}),
			testService("namespace2", "service3", map[string]string{}, "9.0.1.2", map[string]int{"port3": 7272}),
			testService("namespace2", "service4", map[string]string{}, "10.0.1.2", map[string]int{"port4": 4444, "port5": 5555}),
		},
	}
}

func testIngresses() []*definitions.IngressV1Item {
	return []*definitions.IngressV1Item{
		testIngress("namespace1", "default-only", "service1", "", "", "", "", "", "", map[string]string{}, definitions.BackendPortV1{Number: 8080}),
		testIngress(
			"namespace2",
			"path-rule-only",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			map[string]string{},
			definitions.BackendPortV1{},
			testRule(
				"www.example.org",
				testPathRule("/", "service3", definitions.BackendPortV1{Name: "port3"}),
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
			"",
			map[string]string{},
			definitions.BackendPortV1{Name: "port1"},
			testRule(
				"foo.example.org",
				testPathRule("/test1", "service1", definitions.BackendPortV1{Name: "port1"}),
				testPathRule("/test2", "service2", definitions.BackendPortV1{Name: "port2"}),
			),
			testRule(
				"bar.example.org",
				testPathRule("/test1", "service1", definitions.BackendPortV1{Name: "port1"}),
				testPathRule("/test2", "service2", definitions.BackendPortV1{Name: "port2"}),
			),
		),
		testIngress("namespace1", "ratelimit", "service1", "localRatelimit(20,\"1m\")", "", "", "", "", "", map[string]string{}, definitions.BackendPortV1{Number: 8080}),
		testIngress("namespace1", "ratelimitAndBreaker", "service1", "", "localRatelimit(20,\"1m\") -> consecutiveBreaker(15)", "", "", "", "", map[string]string{}, definitions.BackendPortV1{Number: 8080}),
		testIngress("namespace2", "svcwith2ports", "service4", "", "", "", "", "", "", map[string]string{}, definitions.BackendPortV1{Number: 4444}),
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
					t.Errorf("invalid backend for route %s, %v", ri.Id, cmp.Diff(ri.Backend, backend))
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

func checkHealthcheck(t *testing.T, got []*eskip.Route, expected, reversed bool) {
	if expected {
		for _, r := range healthcheckRoutes(reversed) {
			assert.Contains(t, got, r)
		}
	} else {
		for _, r := range healthcheckRoutes(reversed) {
			assert.NotContains(t, got, r)
		}
	}
}

func newTestAPI(t *testing.T, s *serviceList, i *definitions.IngressV1List) *testAPI {
	return newTestAPIWithEndpoints(t, s, i, &endpointList{}, &secretList{})
}

func newTestAPIWithEndpoints(t *testing.T, s *serviceList, i *definitions.IngressV1List, e *endpointList, sec *secretList) *testAPI {
	api := &testAPI{
		test:      t,
		services:  s,
		ingresses: i,
		endpoints: e,
		secrets:   sec,
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

func labelsMatch(labelSelectors []string, labels map[string]string) bool {
	for _, selector := range labelSelectors {
		kv := strings.Split(selector, "=")
		if labels[kv[0]] != kv[1] {
			return false
		}
	}
	return true
}

func filterIngressByLabels(labelSelectors []string, ingresses *definitions.IngressV1List) interface{} {
	if len(labelSelectors) == 0 {
		return ingresses
	}

	result := &definitions.IngressV1List{}
	for _, item := range ingresses.Items {
		if labelsMatch(labelSelectors, item.Metadata.Labels) {
			result.Items = append(result.Items, item)
		}
	}
	return result
}

func filterServicesByLabels(labelSelectors []string, services *serviceList) interface{} {
	if len(labelSelectors) == 0 {
		return services
	}

	result := &serviceList{}
	for _, item := range services.Items {
		if labelsMatch(labelSelectors, item.Meta.Labels) {
			result.Items = append(result.Items, item)
		}
	}
	return result
}

func filterEndpointsByLabels(labelSelectors []string, endpoints *endpointList) interface{} {
	if len(labelSelectors) == 0 {
		return endpoints
	}

	result := &endpointList{}
	for _, item := range endpoints.Items {
		if labelsMatch(labelSelectors, item.Meta.Labels) {
			result.Items = append(result.Items, item)
		}
	}
	return result
}

func filterSecretsByLabels(labelSelectors []string, secrets *secretList) interface{} {
	if len(labelSelectors) == 0 {
		return secrets
	}

	result := &secretList{}
	for _, item := range secrets.Items {
		if labelsMatch(labelSelectors, item.Metadata.Labels) {
			result.Items = append(result.Items, item)
		}
	}
	return result
}

func (api *testAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if api.failNext {
		api.failNext = false
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	queryParam := r.URL.Query().Get("labelSelector")
	var labelSelectors []string
	if queryParam != "" {
		labelSelectors = strings.Split(queryParam, ",")
	}

	switch r.URL.Path {
	case IngressesV1ClusterURI:
		if err := respondJSON(w, filterIngressByLabels(labelSelectors, api.ingresses)); err != nil {
			api.test.Error(err)
		}
		return
	case ServicesClusterURI:
		if err := respondJSON(w, filterServicesByLabels(labelSelectors, api.services)); err != nil {
			api.test.Error(err)
		}
		return
	case EndpointsClusterURI:
		if err := respondJSON(w, filterEndpointsByLabels(labelSelectors, api.endpoints)); err != nil {
			api.test.Error(err)
		}
		return
	case SecretsClusterURI:
		if err := respondJSON(w, filterSecretsByLabels(labelSelectors, api.secrets)); err != nil {
			api.test.Error(err)
		}
		return
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
}

func (api *testAPI) Close() {
	api.server.Close()
}

func TestIngressClassFilter(t *testing.T) {
	tests := []struct {
		testTitle     string
		items         []*definitions.IngressV1Item
		ingressClass  string
		expectedItems []*definitions.IngressV1Item
	}{
		{
			testTitle: "filter no class ingresses",
			items: []*definitions.IngressV1Item{
				{Metadata: &definitions.Metadata{
					Name: "test1_valid1",
				}},
				{Metadata: &definitions.Metadata{
					Name: "test1_valid2",
				}},
			},
			ingressClass: "^test-filter$",
			expectedItems: []*definitions.IngressV1Item{
				{Metadata: &definitions.Metadata{
					Name: "test1_valid1",
				}},
				{Metadata: &definitions.Metadata{
					Name: "test1_valid2",
				}},
			},
		},
		{
			testTitle: "filter specific key ingress",
			items: []*definitions.IngressV1Item{
				{Metadata: &definitions.Metadata{
					Name: "test1_valid1",
					Annotations: map[string]string{
						ingressClassKey: "test-filter",
					},
				}},
				{Metadata: &definitions.Metadata{
					Name: "test1_not_valid2",
					Annotations: map[string]string{
						ingressClassKey: "another-test-filter",
					},
				}},
			},
			ingressClass: "^test-filter$",
			expectedItems: []*definitions.IngressV1Item{
				{Metadata: &definitions.Metadata{
					Name: "test1_valid1",
				}},
			},
		},
		{
			testTitle: "filter empty class ingresses",
			items: []*definitions.IngressV1Item{
				{Metadata: &definitions.Metadata{
					Name: "test1_valid1",
					Annotations: map[string]string{
						ingressClassKey: "",
					},
				}},
				{Metadata: &definitions.Metadata{
					Name: "test1_not_valid2",
					Annotations: map[string]string{
						ingressClassKey: "another-test-filter",
					},
				}},
			},
			ingressClass: "^test-filter$",
			expectedItems: []*definitions.IngressV1Item{
				{Metadata: &definitions.Metadata{
					Name: "test1_valid1",
				}},
			},
		},
		{
			testTitle: "explicitly include any ingress class",
			items: []*definitions.IngressV1Item{
				{Metadata: &definitions.Metadata{
					Name: "test1_valid1",
					Annotations: map[string]string{
						ingressClassKey: "",
					},
				}},
				{Metadata: &definitions.Metadata{
					Name: "test1_valid2",
					Annotations: map[string]string{
						ingressClassKey: "test-filter",
					},
				}},
			},
			ingressClass: ".*",
			expectedItems: []*definitions.IngressV1Item{
				{Metadata: &definitions.Metadata{
					Name: "test1_valid1",
				}},
				{Metadata: &definitions.Metadata{
					Name: "test1_valid2",
				}},
			},
		},
		{
			testTitle: "match from a set of ingress classes",
			items: []*definitions.IngressV1Item{
				{Metadata: &definitions.Metadata{
					Name: "test1_valid1",
					Annotations: map[string]string{
						ingressClassKey: "skipper-test, other-test",
					},
				}},
				{Metadata: &definitions.Metadata{
					Name: "test1_valid2",
					Annotations: map[string]string{
						ingressClassKey: "other-test",
					},
				}},
			},
			ingressClass: "skipper-test",
			expectedItems: []*definitions.IngressV1Item{
				{Metadata: &definitions.Metadata{
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

			c := &clusterClient{
				ingressClass: clsRx,
			}

			result := c.filterIngressesV1ByClass(test.items)
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
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
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

		api.endpoints = testEndpointList()
		api.services = testServices()

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("update failed")
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__default_only______":                           "http://1.1.1.0:8080",
			"kube_namespace2__path_rule_only__www_example_org_____service3": "http://2.1.3.0:7272",
			"kube_namespace1__mega______":                                   "http://1.1.1.0:8080",
			"kube_namespace1__mega__foo_example_org___test1__service1":      "http://1.1.1.0:8080",
			"kube_namespace1__mega__foo_example_org___test2__service2":      "http://1.1.2.0:8181",
			"kube___catchall__foo_example_org____":                          "",
			"kube_namespace1__mega__bar_example_org___test1__service1":      "http://1.1.1.0:8080",
			"kube_namespace1__mega__bar_example_org___test2__service2":      "http://1.1.2.0:8181",
			"kube___catchall__bar_example_org____":                          "",
			"kube_namespace1__ratelimit______":                              "http://1.1.1.0:8080",
			"kube_namespace1__ratelimitAndBreaker______":                    "http://1.1.1.0:8080",
			"kube_namespace2__svcwith2ports______":                          "http://2.1.4.0:4444",
		})
	})

	t.Run("receives invalid ingress, parses the rest, gets fixed", func(t *testing.T) {
		api.endpoints = testEndpointList()
		api.services = testServices()
		api.ingresses.Items = testIngresses()
		api.ingresses.Items[2].Spec.Rules[0].Http.Paths[0].Backend.Service.Port.Name = "not-existing"
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

		api.ingresses.Items = testIngresses()

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("update failed")
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__mega__foo_example_org___test1__service1": "http://1.1.1.0:8080",
		})
	})

	t.Run("has ingresses, receive initial", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
			"kube_namespace1__default_only______":                           "http://1.1.1.0:8080",
			"kube_namespace2__path_rule_only__www_example_org_____service3": "http://2.1.3.0:7272",
			"kube_namespace1__mega______":                                   "http://1.1.1.0:8080",
			"kube_namespace1__mega__foo_example_org___test1__service1":      "http://1.1.1.0:8080",
			"kube_namespace1__mega__foo_example_org___test2__service2":      "http://1.1.2.0:8181",
			"kube___catchall__foo_example_org____":                          "",
			"kube_namespace1__mega__bar_example_org___test1__service1":      "http://1.1.1.0:8080",
			"kube_namespace1__mega__bar_example_org___test2__service2":      "http://1.1.2.0:8181",
			"kube___catchall__bar_example_org____":                          "",
			"kube_namespace1__ratelimit______":                              "http://1.1.1.0:8080",
			"kube_namespace1__ratelimitAndBreaker______":                    "http://1.1.1.0:8080",
			"kube_namespace2__svcwith2ports______":                          "http://2.1.4.0:4444",
		})
	})

	t.Run("has ingresses, with service, without endpoints, update some of them, result in shunt routes", func(t *testing.T) {
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

		api.ingresses.Items[0].Spec.DefaultBackend.Service.Port.Number = 6363
		api.ingresses.Items[2].Spec.Rules[0].Http.Paths[0].Backend.Service.Port.Number = 9999

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("failed to receive delete", err, len(r))
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__default_only______":                      "", //shunted because invalid backend
			"kube_namespace1__mega__foo_example_org___test1__service1": "", //shunted because invalid backend
		})
	})

	t.Run("has ingresses, with service, with endpoints, update some of them, result in some routes are changed", func(t *testing.T) {
		api.endpoints = testEndpointList()
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

		// change service name and port of one prule will delete old route id and create a new
		api.ingresses.Items[2].Spec.Rules[0].Http.Paths[0].Backend.Service.Name = "service2"
		api.ingresses.Items[2].Spec.Rules[0].Http.Paths[0].Backend.Service.Port.Number = 8181

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 1 {
			t.Error("failed to receive delete", err, len(r))
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__mega__foo_example_org___test1__service2": "http://1.1.2.0:8181",
		})
	})

	t.Run("has ingresses, lose a service", func(t *testing.T) {
		api.endpoints = testEndpointList()
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

		var updatedServices []*service
		for _, svc := range api.services.Items {
			if svc.Meta.Namespace == "namespace1" && svc.Meta.Name == "service2" {
				continue
			}
			updatedServices = append(updatedServices, svc)
		}
		api.services.Items = updatedServices

		r, d, err := dc.LoadUpdate()
		if err != nil || len(r) != 0 {
			t.Error("failed to receive delete", err, len(r))
		}

		checkIDs(
			t,
			d,
			"kube_namespace1__mega__foo_example_org___test2__service2",
			"kube_namespace1__mega__bar_example_org___test2__service2",
		)
	})

	t.Run("has ingresses, delete some ingresses", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
			"kube_namespace2__path_rule_only__www_example_org_____service3",
			"kube_namespace1__mega______",
			"kube_namespace1__mega__foo_example_org___test1__service1",
			"kube_namespace1__mega__foo_example_org___test2__service2",
			"kube___catchall__foo_example_org____",
			"kube_namespace1__mega__bar_example_org___test1__service1",
			"kube_namespace1__mega__bar_example_org___test2__service2",
			"kube___catchall__bar_example_org____",
			"kube_namespace1__ratelimit______",
			"kube_namespace1__ratelimitAndBreaker______",
			"kube_namespace2__svcwith2ports______",
		)
	})

	t.Run("has ingresses, delete some ingress rules", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
			"kube_namespace1__mega__bar_example_org___test1__service1",
			"kube_namespace1__mega__bar_example_org___test2__service2",
			"kube___catchall__bar_example_org____",
		)
	})

	t.Run("has ingresses, add new ones", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
				"",
				map[string]string{},
				definitions.BackendPortV1{Name: ""},
				testRule(
					"new1.example.org",
					testPathRule("/", "service1", definitions.BackendPortV1{Name: "port1"}),
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
				"",
				map[string]string{},
				definitions.BackendPortV1{Name: ""},
				testRule(
					"new2.example.org",
					testPathRule("/", "service2", definitions.BackendPortV1{Name: "port2"}),
				),
			),
		)

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("invalid updated received", err, len(d))
			return
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__new1__new1_example_org_____service1": "http://1.1.1.0:8080",
			"kube_namespace1__new2__new2_example_org_____service2": "http://1.1.2.0:8181",
		})
	})

	t.Run("has ingresses, mixed insert, update, delete", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
				"",
				map[string]string{},
				definitions.BackendPortV1{Name: ""},
				testRule(
					"new1.example.org",
					testPathRule("/", "service1", definitions.BackendPortV1{Name: "port1"}),
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
				"",
				map[string]string{},
				definitions.BackendPortV1{Name: ""},
				testRule(
					"new2.example.org",
					testPathRule("/", "service2", definitions.BackendPortV1{Name: "port2"}),
				),
			),
		)

		api.ingresses.Items[1].Spec.Rules[0].Http.Paths[0].Backend.Service.Port.Number = 9999
		api.ingresses.Items[2].Spec.Rules = api.ingresses.Items[2].Spec.Rules[:1]

		r, d, err := dc.LoadUpdate()
		if err != nil {
			t.Error("invalid updated received", err, len(d))
			return
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__new1__new1_example_org_____service1":          "http://1.1.1.0:8080",
			"kube_namespace1__new2__new2_example_org_____service2":          "http://1.1.2.0:8181",
			"kube_namespace2__path_rule_only__www_example_org_____service3": "", // shunt because service referenced not found
		})

		checkIDs(
			t,
			d,
			"kube_namespace1__mega__bar_example_org___test1__service1",
			"kube_namespace1__mega__bar_example_org___test2__service2",
			"kube___catchall__bar_example_org____",
		)
	})
	t.Run("has ingresses, add new ones and filter not valid ones using class ingress", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
			"",
			map[string]string{},
			definitions.BackendPortV1{Name: ""},
			testRule(
				"new1.example.org",
				testPathRule("/", "service1", definitions.BackendPortV1{Name: "port1"}),
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
			"",
			map[string]string{},
			definitions.BackendPortV1{Name: ""},
			testRule(
				"new2.example.org",
				testPathRule("/", "service2", definitions.BackendPortV1{Name: "port2"}),
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
			"kube_namespace1__new1__new1_example_org_____service1": "http://1.1.1.0:8080",
		})
	})

	t.Run("has ingresses, filter out only requested on a server-side", func(t *testing.T) {
		api.endpoints = testEndpointList()
		api.endpoints.Items = append(api.endpoints.Items, testEndpoints("namespace1", "with-label",
			map[string]string{"skipper-ingress": "true"}, "1.1.1", 1, map[string]int{"port1": 8080})...)

		api.services = testServices()
		api.services.Items = append(api.services.Items, testService("namespace1", "with-label",
			map[string]string{"skipper-ingress": "true"}, "1.1.1.0", map[string]int{"port1": 8080}))

		api.ingresses.Items = testIngresses()
		api.ingresses.Items = append(api.ingresses.Items, testIngress("namespace1", "with-label", "with-label", "", "", "", "", "", "",
			map[string]string{"skipper-ingress": "true"}, definitions.BackendPortV1{Number: 8080}))

		dc, err := New(Options{KubernetesURL: api.server.URL, IngressLabelSelectors: map[string]string{"skipper-ingress": "true"}})
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
			"kube_namespace1__with_label______": "http://1.1.1.0:8080",
		})
	})

	t.Run("has ingresses, filter out only requested on a server-side, multiple labels", func(t *testing.T) {
		api.endpoints = testEndpointList()
		api.endpoints.Items = append(api.endpoints.Items, testEndpoints("namespace1", "with-label",
			map[string]string{"skipper-ingress": "true", "team": "avengers"}, "1.1.1", 1, map[string]int{"port1": 8080})...)

		api.services = testServices()
		api.services.Items = append(api.services.Items, testService("namespace1", "with-label",
			map[string]string{"skipper-ingress": "true", "team": "avengers"}, "1.1.1.0", map[string]int{"port1": 8080}))

		api.ingresses.Items = testIngresses()
		api.ingresses.Items = append(api.ingresses.Items, testIngress("namespace1", "with-label", "with-label", "", "", "", "", "", "",
			map[string]string{"skipper-ingress": "true", "team": "avengers"}, definitions.BackendPortV1{Number: 8080}))

		dc, err := New(Options{KubernetesURL: api.server.URL, IngressLabelSelectors: map[string]string{"skipper-ingress": "true", "team": "avengers"}})
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
			"kube_namespace1__with_label______": "http://1.1.1.0:8080",
		})
	})

	t.Run("has ingresses, filter out only requested on a server-side, different labels per resource", func(t *testing.T) {
		api.endpoints = testEndpointList()
		api.endpoints.Items = append(api.endpoints.Items, testEndpoints("namespace1", "with-label",
			map[string]string{"skipper-ingress": "true", "team": "avengers"}, "1.1.1", 1, map[string]int{"port1": 8080})...)

		api.services = testServices()
		api.services.Items = append(api.services.Items, testService("namespace1", "with-label",
			map[string]string{"skipper-ingress": "true", "group": "theavengers"}, "1.1.1.0", map[string]int{"port1": 8080}))

		api.ingresses.Items = testIngresses()
		api.ingresses.Items = append(api.ingresses.Items, testIngress("namespace1", "with-label", "with-label", "", "", "", "", "", "",
			map[string]string{"skipper-ingress": "true"}, definitions.BackendPortV1{Number: 8080}))

		dc, err := New(Options{KubernetesURL: api.server.URL,
			IngressLabelSelectors:   map[string]string{"skipper-ingress": "true"},
			ServicesLabelSelectors:  map[string]string{"skipper-ingress": "true", "group": "theavengers"},
			EndpointsLabelSelectors: map[string]string{"skipper-ingress": "true", "team": "avengers"},
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

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__with_label______": "http://1.1.1.0:8080",
		})
	})
}

func TestConvertPathRule(t *testing.T) {
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
	defer api.Close()

	t.Run("has ingresses, receive two equal backends", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
				"",
				map[string]string{},
				definitions.BackendPortV1{Name: ""},
				testRule(
					"new1.example.org",
					testPathRule("/test1", "service1", definitions.BackendPortV1{Name: "port1"}),
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
				"",
				map[string]string{},
				definitions.BackendPortV1{Name: ""},
				testRule(
					"new1.example.org",
					testPathRule("/test2", "service1", definitions.BackendPortV1{Name: "port1"}),
				),
			),
		)

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("update failed")
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__new1__new1_example_org___test1__service1": "http://1.1.1.0:8080",
			"kube___catchall__new1_example_org____":                     "",
			"kube_namespace1__new1__new1_example_org___test2__service1": "http://1.1.1.0:8080",
		})
	})
}

func TestConvertPathRuleEastWestEnabled(t *testing.T) {
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
	defer api.Close()
	t.Run("has one ingress, receive two backends pointing to the same backend", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
				"",
				map[string]string{},
				definitions.BackendPortV1{Name: ""},
				testRule(
					"new1.example.org",
					testPathRule("/test1", "service1", definitions.BackendPortV1{Name: "port1"}),
				),
			),
		)

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("update failed")
		}

		checkRoutes(t, r, map[string]string{
			"kube___catchall__new1_example_org____":                       "",
			"kube_namespace1__new1__new1_example_org___test1__service1":   "http://1.1.1.0:8080",
			"kubeew_namespace1__new1__new1_example_org___test1__service1": "http://1.1.1.0:8080",
			"kube___catchall__new1_namespace1_skipper_cluster_local____":  "",
		})
	})

	t.Run("has ingresses, receive two equal backends", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
				"",
				map[string]string{},
				definitions.BackendPortV1{Name: ""},
				testRule(
					"new1.example.org",
					testPathRule("/test1", "service1", definitions.BackendPortV1{Name: "port1"}),
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
				"",
				map[string]string{},
				definitions.BackendPortV1{Name: ""},
				testRule(
					"new1.example.org",
					testPathRule("/test2", "service1", definitions.BackendPortV1{Name: "port1"}),
				),
			),
		)

		r, d, err := dc.LoadUpdate()
		if err != nil || len(d) != 0 {
			t.Error("update failed")
		}

		checkRoutes(t, r, map[string]string{
			"kube_namespace1__new1__new1_example_org___test1__service1":   "http://1.1.1.0:8080",
			"kube___catchall__new1_example_org____":                       "",
			"kube_namespace1__new1__new1_example_org___test2__service1":   "http://1.1.1.0:8080",
			"kubeew_namespace1__new1__new1_example_org___test1__service1": "http://1.1.1.0:8080",
			"kubeew_namespace1__new1__new1_example_org___test2__service1": "http://1.1.1.0:8080",
			"kube___catchall__new1_namespace1_skipper_cluster_local____":  "",
		})
	})
}

func TestHealthcheckRoutes(t *testing.T) {
	for _, tc := range []struct {
		logLevel               log.Level
		reverseSourcePredicate bool
		expected               string
	}{
		{
			logLevel:               log.InfoLevel,
			reverseSourcePredicate: false,
			expected: `
				kube__healthz_up:   Path("/kube-system/healthz") && Source("10.0.0.0/8", "192.168.0.0/16", "172.16.0.0/12", "127.0.0.1/8", "fd00::/8", "::1/128") -> disableAccessLog(200) -> status(200) -> <shunt>;
				kube__healthz_down: Path("/kube-system/healthz") && Source("10.0.0.0/8", "192.168.0.0/16", "172.16.0.0/12", "127.0.0.1/8", "fd00::/8", "::1/128") && Shutdown() -> status(503) -> <shunt>;
			`,
		},
		{
			logLevel:               log.InfoLevel,
			reverseSourcePredicate: true,
			expected: `
				kube__healthz_up:   Path("/kube-system/healthz") && SourceFromLast("10.0.0.0/8", "192.168.0.0/16", "172.16.0.0/12", "127.0.0.1/8", "fd00::/8", "::1/128") -> disableAccessLog(200) -> status(200) -> <shunt>;
				kube__healthz_down: Path("/kube-system/healthz") && SourceFromLast("10.0.0.0/8", "192.168.0.0/16", "172.16.0.0/12", "127.0.0.1/8", "fd00::/8", "::1/128") && Shutdown() -> status(503) -> <shunt>;
			`,
		},
		{
			logLevel:               log.DebugLevel,
			reverseSourcePredicate: false,
			expected: `
				kube__healthz_up:   Path("/kube-system/healthz") && Source("10.0.0.0/8", "192.168.0.0/16", "172.16.0.0/12", "127.0.0.1/8", "fd00::/8", "::1/128") -> status(200) -> <shunt>;
				kube__healthz_down: Path("/kube-system/healthz") && Source("10.0.0.0/8", "192.168.0.0/16", "172.16.0.0/12", "127.0.0.1/8", "fd00::/8", "::1/128") && Shutdown() -> status(503) -> <shunt>;
			`,
		},
		{
			logLevel:               log.DebugLevel,
			reverseSourcePredicate: true,
			expected: `
				kube__healthz_up:   Path("/kube-system/healthz") && SourceFromLast("10.0.0.0/8", "192.168.0.0/16", "172.16.0.0/12", "127.0.0.1/8", "fd00::/8", "::1/128") -> status(200) -> <shunt>;
				kube__healthz_down: Path("/kube-system/healthz") && SourceFromLast("10.0.0.0/8", "192.168.0.0/16", "172.16.0.0/12", "127.0.0.1/8", "fd00::/8", "::1/128") && Shutdown() -> status(503) -> <shunt>;
			`,
		},
	} {
		t.Run(fmt.Sprintf("log: %s, reverse: %v", tc.logLevel.String(), tc.reverseSourcePredicate), func(t *testing.T) {
			level := log.GetLevel()
			defer func() { log.SetLevel(level) }()
			log.SetLevel(tc.logLevel)

			expected := eskip.MustParse(tc.expected)

			assert.EqualValues(t, expected, healthcheckRoutes(tc.reverseSourcePredicate))
		})
	}
}

func TestHealthcheckInitial(t *testing.T) {
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
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

		checkHealthcheck(t, r, false, false)
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

		checkHealthcheck(t, r, false, false)
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

		checkHealthcheck(t, r, true, false)
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

		checkHealthcheck(t, r, true, false)
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

		checkHealthcheck(t, r, true, true)
	})
}

func TestHealthcheckUpdate(t *testing.T) {
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
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

		checkHealthcheck(t, r, false, false)
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

		checkHealthcheck(t, r, false, false)
		if len(d) != 0 {
			t.Error("unexpected delete")
		}
	})
}

func TestHealthcheckReload(t *testing.T) {
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
	defer api.Close()

	t.Run("no healthcheck, reload fail", func(t *testing.T) {
		api.endpoints = testEndpointList()
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

		checkHealthcheck(t, r, false, false)
	})

	t.Run("use healthcheck, reload succeeds", func(t *testing.T) {
		api.endpoints = testEndpointList()
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

		checkHealthcheck(t, r, true, false)
		checkRoutes(t, r, map[string]string{
			"kube__healthz_up":                    "",
			"kube__healthz_down":                  "",
			"kube_namespace1__default_only______": "http://1.1.1.0:8080",
			"kube_namespace2__path_rule_only__www_example_org_____service3": "http://2.1.3.0:7272",
			"kube_namespace1__mega______":                                   "http://1.1.1.0:8080",
			"kube_namespace1__mega__foo_example_org___test1__service1":      "http://1.1.1.0:8080",
			"kube_namespace1__mega__foo_example_org___test2__service2":      "http://1.1.2.0:8181",
			"kube___catchall__foo_example_org____":                          "",
			"kube_namespace1__mega__bar_example_org___test1__service1":      "http://1.1.1.0:8080",
			"kube_namespace1__mega__bar_example_org___test2__service2":      "http://1.1.2.0:8181",
			"kube___catchall__bar_example_org____":                          "",
			"kube_namespace1__ratelimit______":                              "http://1.1.1.0:8080",
			"kube_namespace1__ratelimitAndBreaker______":                    "http://1.1.1.0:8080",
			"kube_namespace2__svcwith2ports______":                          "http://2.1.4.0:4444",
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
	rc := io.NopCloser(&buf)

	client := &clusterClient{}

	url = "A%"
	_, err = client.createRequest(url, rc)
	if err == nil {
		t.Error("request creation should fail")
	}

	url = "https://www.example.org"
	_, err = client.createRequest(url, rc)
	if err != nil {
		t.Error(err)
	}

	client.tokenProvider = mockSecretProvider("1234")
	req, err = client.createRequest(url, rc)
	if err != nil {
		t.Error(err)
	}
	if req.URL.String() != url {
		t.Errorf("request creation incorrect url is set")
	}
	if req.Header.Get("Authorization") != "Bearer 1234" {
		t.Errorf("incorrect authorization header set")
	}
	if req.Method != "GET" {
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

	dummyHost := "10.0.0.2"
	dummyPort := "8080"

	// There is t.Unsetenv so set to properly restore/unset
	t.Setenv(serviceHostEnvVar, "")
	t.Setenv(servicePortEnvVar, "")
	os.Unsetenv(serviceHostEnvVar)
	os.Unsetenv(servicePortEnvVar)

	apiURL, err = buildAPIURL(o)
	if apiURL != "" || err != errAPIServerURLNotFound {
		t.Error("build API url should fail if env var is missing")
	}

	t.Setenv(serviceHostEnvVar, dummyHost)
	apiURL, err = buildAPIURL(o)
	if apiURL != "" || err != errAPIServerURLNotFound {
		t.Error("build API url should fail if env var is missing")
	}

	t.Setenv(servicePortEnvVar, dummyPort)
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

	_, err = buildHTTPClient("rumplestilzchen", true, quit)
	if err == nil {
		t.Errorf("expected to fail for non-existing file")
	}

	_, err = buildHTTPClient("kube_test.go", true, quit)
	if err != errInvalidCertificate {
		t.Errorf("should return invalid certificate")
	}

	err = os.WriteFile("ca.empty.crt", []byte(""), 0644)
	if err != nil {
		t.Error(err)
	}
	defer os.Remove("ca.empty.crt")

	_, err = buildHTTPClient("ca.empty.crt", true, quit)
	if err != errInvalidCertificate {
		t.Error("empty certificate is invalid certificate")
	}

	//create CA file
	err = os.WriteFile("ca.temp.crt", generateSSCert(), 0644)
	if err != nil {
		t.Error(err)
	}
	defer os.Remove("ca.temp.crt")

	_, err = buildHTTPClient("ca.temp.crt", true, quit)
	if err != nil {
		t.Error(err)
	}
}

func TestScoping(t *testing.T) {
	client := &clusterClient{}

	client.setNamespace("test")
	assert.Equal(t, "/apis/networking.k8s.io/v1/namespaces/test/ingresses", client.ingressesURI)
	assert.Equal(t, "/api/v1/namespaces/test/services", client.servicesURI)
	assert.Equal(t, "/api/v1/namespaces/test/endpoints", client.endpointsURI)
}

func TestLabelSelectorsSet(t *testing.T) {
	tests := []struct {
		name                        string
		options                     Options
		expectedIngressSelector     string
		expectedServicesSelector    string
		expectedEndpointsSelector   string
		expectedSecretsSelector     string
		expectedRouteGroupsSelector string
	}{
		{
			name: "different labels for different resources",
			options: Options{
				IngressLabelSelectors:     map[string]string{"skipper-enabled": "true"},
				ServicesLabelSelectors:    map[string]string{"best-services": "true"},
				EndpointsLabelSelectors:   map[string]string{"endpoints-needed": "true"},
				SecretsLabelSelectors:     map[string]string{"relevant-secrets": "true"},
				RouteGroupsLabelSelectors: map[string]string{"routegroups-skipper": "true"},
			},
			expectedIngressSelector:     "?labelSelector=skipper-enabled%3Dtrue",
			expectedServicesSelector:    "?labelSelector=best-services%3Dtrue",
			expectedEndpointsSelector:   "?labelSelector=endpoints-needed%3Dtrue",
			expectedSecretsSelector:     "?labelSelector=relevant-secrets%3Dtrue",
			expectedRouteGroupsSelector: "?labelSelector=routegroups-skipper%3Dtrue",
		},
		{
			name: "multiple labels can be specified",
			options: Options{
				IngressLabelSelectors:     map[string]string{"skipper-enabled": "true", "team": "avengers"},
				ServicesLabelSelectors:    map[string]string{"best-services": "true", "group": "marvel"},
				EndpointsLabelSelectors:   map[string]string{"endpoints-needed": "true", "hero": "thor"},
				SecretsLabelSelectors:     map[string]string{"relevant-secrets": "true", "iron": "man"},
				RouteGroupsLabelSelectors: map[string]string{"routegroups-skipper": "true", "black": "widow"},
			},
			expectedIngressSelector:     "?labelSelector=skipper-enabled%3Dtrue%2Cteam%3Davengers",
			expectedServicesSelector:    "?labelSelector=best-services%3Dtrue%2Cgroup%3Dmarvel",
			expectedEndpointsSelector:   "?labelSelector=endpoints-needed%3Dtrue%2Chero%3Dthor",
			expectedSecretsSelector:     "?labelSelector=iron%3Dman%2Crelevant-secrets%3Dtrue",
			expectedRouteGroupsSelector: "?labelSelector=black%3Dwidow%2Croutegroups-skipper%3Dtrue",
		},
		{
			name:                        "if not set, it defaults to an empty string",
			options:                     Options{},
			expectedIngressSelector:     "",
			expectedServicesSelector:    "",
			expectedEndpointsSelector:   "",
			expectedSecretsSelector:     "",
			expectedRouteGroupsSelector: "",
		},
		{
			name:                        "if not set, it defaults to an empty string and doesn't default to Ingress labels",
			options:                     Options{IngressLabelSelectors: map[string]string{"skipper-enabled": "true"}},
			expectedIngressSelector:     "?labelSelector=skipper-enabled%3Dtrue",
			expectedServicesSelector:    "",
			expectedEndpointsSelector:   "",
			expectedSecretsSelector:     "",
			expectedRouteGroupsSelector: "",
		},
		{
			name: "value can be skipped",
			options: Options{
				IngressLabelSelectors:     map[string]string{"skipper-enabled": ""},
				ServicesLabelSelectors:    map[string]string{"skipper-enabled": ""},
				EndpointsLabelSelectors:   map[string]string{"skipper-enabled": ""},
				SecretsLabelSelectors:     map[string]string{"skipper-enabled": ""},
				RouteGroupsLabelSelectors: map[string]string{"skipper-enabled": ""},
			},
			expectedIngressSelector:     "?labelSelector=skipper-enabled",
			expectedServicesSelector:    "?labelSelector=skipper-enabled",
			expectedEndpointsSelector:   "?labelSelector=skipper-enabled",
			expectedSecretsSelector:     "?labelSelector=skipper-enabled",
			expectedRouteGroupsSelector: "?labelSelector=skipper-enabled",
		},
		{
			name:                        "labels are not required at all",
			options:                     Options{},
			expectedIngressSelector:     "",
			expectedServicesSelector:    "",
			expectedEndpointsSelector:   "",
			expectedSecretsSelector:     "",
			expectedRouteGroupsSelector: "",
		},
	}

	// in case of multiple labels, the order of the resulting query isn't guaranteed
	// this solves the issue for the test as we split queries again and compare string slices
	sortedSlice := func(query string) []string {
		if query == "" {
			return []string{}
		}

		params := strings.Split(query, "=")
		require.Len(t, params, 2)

		labels := strings.Split(params[1], "%2C")

		result := []string{params[0]}
		result = append(result, labels...)

		sort.Strings(result)

		return result
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := New(test.options)
			require.NoError(t, err)

			assert.Equal(t, sortedSlice(test.expectedIngressSelector), sortedSlice(client.ClusterClient.ingressLabelSelectors))
			assert.Equal(t, sortedSlice(test.expectedServicesSelector), sortedSlice(client.ClusterClient.servicesLabelSelectors))
			assert.Equal(t, sortedSlice(test.expectedEndpointsSelector), sortedSlice(client.ClusterClient.endpointsLabelSelectors))
			assert.Equal(t, sortedSlice(test.expectedSecretsSelector), sortedSlice(client.ClusterClient.secretsLabelSelectors))
			assert.Equal(t, sortedSlice(test.expectedRouteGroupsSelector), sortedSlice(client.ClusterClient.routeGroupsLabelSelectors))
		})
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
			catchAll := hasCatchAllRoutes(tc.routes)
			if catchAll != tc.hasCatchAll {
				t.Errorf("expected %t, got %t", tc.hasCatchAll, catchAll)
			}
		})
	}
}

func TestRatelimits(t *testing.T) {
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
	defer api.Close()

	t.Run("check localratelimit", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
	defer api.Close()

	t.Run("check localratelimit", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
	defer api.Close()

	t.Run("check ingress filter", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
	defer api.Close()

	t.Run("check ingress filter", func(t *testing.T) {
		api.endpoints = testEndpointList()
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
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
	defer api.Close()

	ingWithPredicate := testIngress("namespace1", "predicate", "service1", "", "", "QueryParam(\"query\", \"^example$\")", "", "", "", map[string]string{}, definitions.BackendPortV1{Number: 8080})

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
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
	defer api.Close()

	ingWithPredicate := testIngress("namespace1", "predicate", "service1", "", "", "QueryParam(\"query\", \"^example$\")", "", "", "", map[string]string{}, definitions.BackendPortV1{Number: 8080})

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
		endpoints      []*endpoint
		services       []*service
		ingresses      []*definitions.IngressV1Item
		secrets        []*secret
		expectedRoutes map[string]string
	}{{
		msg:       "ingress with 1 host definitions and 1 additional custom route",
		endpoints: testEndpoints("foo", "bar", map[string]string{}, "1.1.1", 1, map[string]int{"baz": 8181}),
		services: []*service{
			testService("foo", "bar", map[string]string{}, "1.2.3.4", map[string]int{"baz": 8181}),
		},
		ingresses: []*definitions.IngressV1Item{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", "", map[string]string{}, definitions.BackendPortV1{},
			testRule("www1.example.org", testPathRule("/", "bar", definitions.BackendPortV1{Name: "baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux__0__www1_example_org_____": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
		},
	}, {
		msg:       "ingress with 1 host definitions with path and 1 additional custom route",
		endpoints: testEndpoints("foo", "bar", map[string]string{}, "1.1.1", 1, map[string]int{"baz": 8181}),
		services: []*service{
			testService("foo", "bar", map[string]string{}, "1.2.3.4", map[string]int{"baz": 8181}),
		},
		ingresses: []*definitions.IngressV1Item{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", "", map[string]string{}, definitions.BackendPortV1{},
			testRule("www1.example.org", testPathRule("/a/path", "bar", definitions.BackendPortV1{Name: "baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org___a_path__bar": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux__0__www1_example_org_a_path____": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) && Method(\"OPTIONS\") -> <shunt>",
			"kube___catchall__www1_example_org____":         "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) -> <shunt>",
		},
	}, {
		msg:       "ingress with 2 host definitions and 1 additional custom route",
		endpoints: testEndpoints("foo", "bar", map[string]string{}, "1.1.1", 1, map[string]int{"baz": 8181}),
		services: []*service{
			testService("foo", "bar", map[string]string{}, "1.2.3.4", map[string]int{"baz": 8181}),
		},
		ingresses: []*definitions.IngressV1Item{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", "", map[string]string{}, definitions.BackendPortV1{},
			testRule("www1.example.org", testPathRule("/", "bar", definitions.BackendPortV1{Name: "baz"})),
			testRule("www2.example.org", testPathRule("/", "bar", definitions.BackendPortV1{Name: "baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux__0__www1_example_org_____": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux__www2_example_org_____bar": "Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux__0__www2_example_org_____": "Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
		},
	}, {
		msg:       "ingress with 2 host definitions with path and 1 additional custom route",
		endpoints: testEndpoints("foo", "bar", map[string]string{}, "1.1.1", 1, map[string]int{"baz": 8181}),
		services: []*service{
			testService("foo", "bar", map[string]string{}, "1.2.3.4", map[string]int{"baz": 8181}),
		},
		ingresses: []*definitions.IngressV1Item{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"", "", map[string]string{}, definitions.BackendPortV1{},
			testRule("www1.example.org", testPathRule("/a/path", "bar", definitions.BackendPortV1{Name: "baz"})),
			testRule("www2.example.org", testPathRule("/another/path", "bar", definitions.BackendPortV1{Name: "baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org___a_path__bar":       "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux__0__www1_example_org_a_path____":       "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) && Method(\"OPTIONS\") -> <shunt>",
			"kube___catchall__www1_example_org____":               "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) -> <shunt>",
			"kube_foo__qux__www2_example_org___another_path__bar": "Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/another\\/path)/) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux__0__www2_example_org_another_path____": "Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/another\\/path)/) && Method(\"OPTIONS\") -> <shunt>",
			"kube___catchall__www2_example_org____":               "Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) -> <shunt>",
		},
	}, {
		msg: "ingress with 3 host definitions with one path and 3 additional custom routes",
		endpoints: append(testEndpoints("foo", "bar", map[string]string{}, "1.1.1", 1, map[string]int{"baz": 8181}),
			testEndpoints("foo", "baz", map[string]string{}, "1.1.2", 1, map[string]int{"baz": 8181})...),
		services: []*service{
			testService("foo", "bar", map[string]string{}, "1.2.3.4", map[string]int{"baz": 8181}),
			testService("foo", "baz", map[string]string{}, "1.2.3.6", map[string]int{"baz": 8181}),
		},
		ingresses: []*definitions.IngressV1Item{testIngress("foo", "qux", "", "", "", "",
			`a: Method("OPTIONS") -> <shunt>;
                         b: Cookie("alpha", /^enabled$/) -> "http://1.1.2.0:8181";
                         c: Path("/a/path/somewhere") -> "https://some.other-url.org/a/path/somewhere";`,
			"", "", map[string]string{}, definitions.BackendPortV1{},
			testRule("www1.example.org", testPathRule("/", "bar", definitions.BackendPortV1{Name: "baz"})),
			testRule("www2.example.org", testPathRule("/", "bar", definitions.BackendPortV1{Name: "baz"})),
			testRule("www3.example.org", testPathRule("/a/path", "bar", definitions.BackendPortV1{Name: "baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar":  "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux_a_0__www1_example_org_____": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www1_example_org_____": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.1.2.0:8181\"",
			"kube_foo__qux_c_2__www1_example_org_____": "Path(\"/a/path/somewhere\") && Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www2_example_org_____bar":  "Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux_a_0__www2_example_org_____": "Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www2_example_org_____": "Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.1.2.0:8181\"",
			"kube_foo__qux_c_2__www2_example_org_____": "Path(\"/a/path/somewhere\") && Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www3_example_org___a_path__bar":  "Host(/^(www3[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux_a_0__www3_example_org_a_path____": "Host(/^(www3[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www3_example_org_a_path____": "Host(/^(www3[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.1.2.0:8181\"",
			"kube_foo__qux_c_2__www3_example_org_a_path____": "Path(\"/a/path/somewhere\") && Host(/^(www3[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) -> \"https://some.other-url.org/a/path/somewhere\"",
			"kube___catchall__www3_example_org____":          "Host(/^(www3[.]example[.]org[.]?(:[0-9]+)?)$/) -> <shunt>",
		},
	}, {
		msg: "ingress with 3 host definitions with one without path and 3 additional custom routes",
		endpoints: append(testEndpoints("foo", "bar", map[string]string{}, "1.1.1", 1, map[string]int{"baz": 8181}),
			testEndpoints("foo", "baz", map[string]string{}, "1.1.2", 1, map[string]int{"baz": 8181})...),
		services: []*service{
			testService("foo", "bar", map[string]string{}, "1.2.3.4", map[string]int{"baz": 8181}),
			testService("foo", "baz", map[string]string{}, "1.2.3.6", map[string]int{"baz": 8181}),
		},
		ingresses: []*definitions.IngressV1Item{testIngress("foo", "qux", "", "", "", "",
			`a: Method("OPTIONS") -> <shunt>;
                         b: Cookie("alpha", /^enabled$/) -> "http://1.1.2.0:8181";
                         c: Path("/a/path/somewhere") -> "https://some.other-url.org/a/path/somewhere";`,
			"", "", map[string]string{}, definitions.BackendPortV1{},
			testRule("www1.example.org", testPathRule("", "bar", definitions.BackendPortV1{Name: "baz"})),
			testRule("www2.example.org", testPathRule("/", "bar", definitions.BackendPortV1{Name: "baz"})),
			testRule("www3.example.org", testPathRule("/a/path", "bar", definitions.BackendPortV1{Name: "baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org____bar":  "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux_a_0__www1_example_org____": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www1_example_org____": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.1.2.0:8181\"",
			"kube_foo__qux_c_2__www1_example_org____": "Path(\"/a/path/somewhere\") && Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www2_example_org_____bar":  "Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux_a_0__www2_example_org_____": "Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www2_example_org_____": "Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.1.2.0:8181\"",
			"kube_foo__qux_c_2__www2_example_org_____": "Path(\"/a/path/somewhere\") && Host(/^(www2[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^\\//) -> \"https://some.other-url.org/a/path/somewhere\"",

			"kube_foo__qux__www3_example_org___a_path__bar":  "Host(/^(www3[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux_a_0__www3_example_org_a_path____": "Host(/^(www3[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) && Method(\"OPTIONS\") -> <shunt>",
			"kube_foo__qux_b_1__www3_example_org_a_path____": "Host(/^(www3[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) && Cookie(\"alpha\", \"^enabled$\") -> \"http://1.1.2.0:8181\"",
			"kube_foo__qux_c_2__www3_example_org_a_path____": "Path(\"/a/path/somewhere\") && Host(/^(www3[.]example[.]org[.]?(:[0-9]+)?)$/) && PathRegexp(/^(\\/a\\/path)/) -> \"https://some.other-url.org/a/path/somewhere\"",
			"kube___catchall__www3_example_org____":          "Host(/^(www3[.]example[.]org[.]?(:[0-9]+)?)$/) -> <shunt>",
		},
	}, {
		msg:       "ingress with 1 host definitions and 1 additional custom route, changed pathmode to PathSubtree",
		endpoints: testEndpoints("foo", "bar", map[string]string{}, "1.1.1", 1, map[string]int{"baz": 8181}),
		services: []*service{
			testService("foo", "bar", map[string]string{}, "1.2.3.4", map[string]int{"baz": 8181}),
		},
		ingresses: []*definitions.IngressV1Item{testIngress("foo", "qux", "", "", "", "",
			`Method("OPTIONS") -> <shunt>`,
			"path-prefix", "", map[string]string{}, definitions.BackendPortV1{},
			testRule("www1.example.org", testPathRule("/", "bar", definitions.BackendPortV1{Name: "baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathSubtree(\"/\") -> \"http://1.1.1.0:8181\"",
			"kube_foo__qux__0__www1_example_org_____": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && Method(\"OPTIONS\") && PathSubtree(\"/\") -> <shunt>",
		},
	}, {
		msg:       "ingress with 1 host definitions and 1 additional custom route with path predicate, changed pathmode to PathSubtree",
		endpoints: testEndpoints("foo", "bar", map[string]string{}, "1.1.1", 1, map[string]int{"baz": 8181}),
		services: []*service{
			testService("foo", "bar", map[string]string{}, "1.2.3.4", map[string]int{"baz": 8181}),
		},
		ingresses: []*definitions.IngressV1Item{testIngress("foo", "qux", "", "", "", "",
			`Path("/foo") -> <shunt>`,
			"path-prefix", "", map[string]string{}, definitions.BackendPortV1{},
			testRule("www1.example.org", testPathRule("/", "bar", definitions.BackendPortV1{Name: "baz"})),
		)},
		expectedRoutes: map[string]string{
			"kube_foo__qux__www1_example_org_____bar": "Host(/^(www1[.]example[.]org[.]?(:[0-9]+)?)$/) && PathSubtree(\"/\") -> \"http://1.1.1.0:8181\"",
		},
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			api := newTestAPIWithEndpoints(t, &serviceList{Items: ti.services}, &definitions.IngressV1List{Items: ti.ingresses}, &endpointList{
				Items: ti.endpoints,
			}, &secretList{Items: ti.secrets})
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
			ewr := createEastWestRouteIng(defaultEastWestDomain, "foo", "qux", ti.route)
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

func TestCreateEastWestRouteOverwriteDomain(t *testing.T) {
	for _, ti := range []struct {
		msg            string
		domain         string
		name           string
		namespace      string
		route          *eskip.Route
		expectedID     string
		expectedDomain string
	}{{
		msg:       "valid non default DNS domain",
		domain:    "internal.cluster.local",
		name:      "foo",
		namespace: "qux",
		route: &eskip.Route{
			Id:          "kube_foo__qux_a_0__www2_example_org_____",
			HostRegexps: []string{"www2[.]example[.]org"},
		},
		expectedID:     "kubeew_foo__qux_a_0__www2_example_org_____",
		expectedDomain: "internal.cluster.local",
	}, {
		msg:       "valid non default DNS domain with dot as prefix",
		domain:    ".internal.cluster.local",
		name:      "foo",
		namespace: "qux",
		route: &eskip.Route{
			Id:          "kube_foo__qux__www3_example_org___a_path__bar",
			HostRegexps: []string{"www3[.]example[.]org"},
		},
		expectedID:     "kubeew_foo__qux__www3_example_org___a_path__bar",
		expectedDomain: "internal.cluster.local",
	}, {
		msg:       "valid non default DNS domain with dot as suffix",
		domain:    "internal.cluster.local.",
		name:      "foo",
		namespace: "qux",
		route: &eskip.Route{
			Id:          "kube_foo__qux__www3_example_org___a_path__bar",
			HostRegexps: []string{"www3[.]example[.]org"},
		},
		expectedID:     "kubeew_foo__qux__www3_example_org___a_path__bar",
		expectedDomain: "internal.cluster.local",
	}, {
		msg:       "valid non default DNS domain with dot as prefix and suffix",
		domain:    ".internal.cluster.local.",
		name:      "foo",
		namespace: "qux",
		route: &eskip.Route{
			Id:          "kube_foo__qux__www3_example_org___a_path__bar",
			HostRegexps: []string{"www3[.]example[.]org"},
		},
		expectedID:     "kubeew_foo__qux__www3_example_org___a_path__bar",
		expectedDomain: "internal.cluster.local",
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			kube, err := New(Options{KubernetesEnableEastWest: true, KubernetesEastWestDomain: ti.domain})
			if err != nil {
				t.Fatal(err)
			}

			ing := kube.ingress
			ewr := createEastWestRouteIng(ing.kubernetesEastWestDomain, ti.name, ti.namespace, ti.route)
			if ewr.Id != ti.expectedID {
				t.Errorf("Failed to create east west route ID, %s, but expected %s", ewr.Id, ti.expectedID)
			}

			hostRegexp := ewr.HostRegexps[0]
			reg := regexp.MustCompile(hostRegexp)
			if !reg.MatchString(fmt.Sprintf("%s.%s.%s", ti.name, ti.namespace, ti.expectedDomain)) {
				t.Errorf("Failed to create correct east west hostregexp, got %s, expected to match %s", ewr.HostRegexps, fmt.Sprintf("%s.%s.%s", ti.name, ti.namespace, ti.expectedDomain))
			}
		})
	}
}

func TestSkipperDefaultFilters(t *testing.T) {
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
	defer api.Close()

	t.Run("check routes are created if default filters dir is not set", func(t *testing.T) {
		api.services = testServices()
		api.ingresses.Items = testIngresses()

		dc, err := New(Options{ // DefaultFiltersDir setting is not set
			KubernetesURL: api.server.URL,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()

		if err != nil {
			t.Error("should not return an error", err)
			return
		}
		if len(r) != 12 {
			t.Error("number of routes is incorrect", len(r))
			return
		}
	})

	t.Run("check default filters are applied to the route", func(t *testing.T) {
		api.services = &serviceList{Items: []*service{testService("namespace1", "service1", map[string]string{}, "1.2.3.4", map[string]int{"port1": 8080})}}
		api.ingresses = &definitions.IngressV1List{Items: []*definitions.IngressV1Item{testIngress("namespace1", "default-only",
			"service1", "", "", "", "", "", "", map[string]string{}, definitions.BackendPortV1{Number: 8080},
			testRule("www.example.org", testPathRule("/", "service1", definitions.BackendPortV1{Number: 8080})))}}

		defaultFiltersDir, err := os.MkdirTemp("", "filters")
		if err != nil {
			t.Error(err)
		}
		file := filepath.Join(defaultFiltersDir, "service1.namespace1")
		if err := os.WriteFile(file, []byte("consecutiveBreaker(15)"), 0666); err != nil {
			t.Error(err)
		}

		dc, err := New(Options{
			KubernetesURL:     api.server.URL,
			DefaultFiltersDir: defaultFiltersDir,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()

		if err != nil || r == nil {
			t.Error("should not fail", err, r)
			return
		}

		if len(r) != 2 && len(r[1].Filters) != 1 && r[1].Filters[0].Name != "consecutiveBreaker" {
			t.Error("should contain default filter", r[1].Filters)
			return
		}
	})

	t.Run("check default filters are prepended to the ingress filters", func(t *testing.T) {
		api.endpoints = testEndpointList()
		api.services = &serviceList{Items: []*service{testServiceWithTargetPort("namespace1", "service1", map[string]string{}, "1.2.3.4", map[string]int{"port1": 8080}, map[int]*definitions.BackendPort{8080: {Value: 8080}})}}
		api.ingresses = &definitions.IngressV1List{Items: []*definitions.IngressV1Item{testIngress("namespace1", "default-only",
			"service1", "", "localRatelimit(20,\"1m\")", "", "", "", "", map[string]string{}, definitions.BackendPortV1{Number: 8080},
			testRule("www.example.org", testPathRule("/", "service1", definitions.BackendPortV1{Name: "port1"})))}}

		// store default configuration in the file
		dir, err := os.MkdirTemp("", "filters")
		if err != nil {
			t.Error(err)
		}
		file := filepath.Join(dir, "service1.namespace1")
		if err := os.WriteFile(file, []byte("consecutiveBreaker(15)"), 0666); err != nil {
			t.Error(err)
		}

		dc, err := New(Options{
			KubernetesURL:     api.server.URL,
			DefaultFiltersDir: dir,
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		r, err := dc.LoadAll()

		if err != nil || r == nil {
			t.Error("should not fail", err, r)
			return
		}
		if len(r) != 2 || len(r[1].Filters) != 2 || r[1].Filters[0].Name != "consecutiveBreaker" || r[1].Filters[1].Name != "localRatelimit" {
			t.Errorf("should prepend the default filter to the ingress filters: %d %d %s %s", len(r), len(r[1].Filters), r[1].Filters[0].Name, r[1].Filters[1].Name)
			return
		}
	})

	t.Run("check getDefaultFilterConfigurations ignores files names not following the pattern, directories and huge files", func(t *testing.T) {
		defaultFiltersDir, err := os.MkdirTemp("", "filters")
		if err != nil {
			t.Error(err)
		}
		invalidFileName := filepath.Join(defaultFiltersDir, "file.name.doesnt.match.our.pattern")
		if err := os.WriteFile(invalidFileName, []byte("consecutiveBreaker(15)"), 0666); err != nil {
			t.Error(err)
		}
		err = os.Mkdir(filepath.Join(defaultFiltersDir, "some.directory"), os.ModePerm)
		if err != nil {
			t.Error(err)
		}
		bigFile := filepath.Join(defaultFiltersDir, "huge.file")
		if err := os.WriteFile(bigFile, make([]byte, 1024*1024+1), 0666); err != nil {
			t.Error(err)
		}

		df, err := readDefaultFilters(defaultFiltersDir)

		if err != nil || len(df) != 0 {
			t.Error("should return empty slice", err, df)
			return
		}
	})

	t.Run("check empty default filters do not panic", func(t *testing.T) {
		dc, err := New(Options{
			DefaultFiltersDir: "dir-does-not-exists",
		})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		df := dc.fetchDefaultFilterConfigs()
		defer func() {
			if err := recover(); err != nil {
				t.Error("failed to call empty default filters")
			}
		}()

		df.get(definitions.ResourceID{})
	})
}

func TestCertificateRegistry(t *testing.T) {
	api := newTestAPI(t, nil, &definitions.IngressV1List{})
	defer api.Close()

	t.Run("certificate registry is enabled, secrets are loaded", func(t *testing.T) {
		api.secrets = testSecrets()
		dc, err := New(Options{KubernetesURL: api.server.URL, CertificateRegistry: &certregistry.CertRegistry{}})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		state, err := dc.ClusterClient.fetchClusterState()
		if err != nil {
			t.Error(err)
		}

		if len(state.secrets) == 0 {
			t.Errorf("no secrets were loaded")
		}
	})

	t.Run("certificate registry is disabled, secrets are not loaded", func(t *testing.T) {
		api.secrets = testSecrets()
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		defer dc.Close()

		state, err := dc.ClusterClient.fetchClusterState()
		if err != nil {
			t.Error(err)
		}

		if len(state.secrets) > 0 {
			t.Errorf("secrets are loaded but registry is disabled")
		}
	})
}

type mockSecretProvider string

func (sp mockSecretProvider) GetSecret(string) ([]byte, bool) {
	return []byte(sp), true
}

func (mockSecretProvider) Add(string) error {
	return nil
}

func (mockSecretProvider) Close() {}
