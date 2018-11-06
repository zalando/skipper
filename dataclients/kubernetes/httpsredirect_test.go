package kubernetes

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
)

type redirectTest struct {
	router          *routing.Routing
	api             *testAPI
	rule            *rule
	backend         string
	fallbackBackend string
	t               *testing.T
	l               *loggingtest.Logger
}

type sortRoutes []*eskip.Route

func (s sortRoutes) Len() int           { return len(s) }
func (s sortRoutes) Less(i, j int) bool { return s[i].Id < s[j].Id }
func (s sortRoutes) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func newRedirectTest(t *testing.T, redirectEnabled bool) (*redirectTest, error) {
	s := services{
		"namespace1": map[string]*service{
			"service1": testService("1.2.3.4", map[string]int{"port1": 8080}),
		},
	}
	i := &ingressList{Items: []*ingressItem{
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
	}}

	api := newTestAPI(t, s, i)

	dc, err := New(Options{
		KubernetesURL:        api.server.URL,
		ProvideHTTPSRedirect: redirectEnabled,
	})
	if err != nil {
		return nil, err
	}

	defer dc.Close()

	l := loggingtest.New()
	router := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{dc},
		Log:            l,
	})

	const to = 120 * time.Millisecond
	if err := l.WaitFor("route settings applied", to); err != nil {
		t.Fatal("waiting for route settings", err)
	}

	if err != nil {
		t.Error(err)
		return nil, err
	}

	ingress := i.Items[0]
	rule := ingress.Spec.Rules[0]
	service := s[ingress.Metadata.Namespace][rule.Http.Paths[0].Backend.ServiceName].Spec
	fallbackService := s[i.Items[0].Metadata.Namespace][i.Items[0].Spec.DefaultBackend.ServiceName].Spec
	backend := fmt.Sprintf("http://%s:%d", service.ClusterIP, service.Ports[0].Port)
	fallbackBackend := fmt.Sprintf("http://%s:%d", fallbackService.ClusterIP, fallbackService.Ports[0].Port)

	return &redirectTest{
		router:          router,
		api:             api,
		rule:            rule,
		backend:         backend,
		fallbackBackend: fallbackBackend,
		t:               t,
		l:               l,
	}, nil
}

func (rt *redirectTest) testRedirectRoute(testCase string, req *http.Request, expectedID, expectedBackend string) {
	if rt.t.Failed() {
		return
	}

	route, _ := rt.router.Route(req)
	rt.t.Log("got:     ", route.Id, route.Backend)
	if expectedID != "" && route.Id != expectedID {
		rt.t.Error(testCase, "failed to match the id")
		rt.t.Log("got:     ", route.Id, route.Backend)
		rt.t.Log("expected:", expectedID, expectedBackend)
	}

	if expectedBackend != "" && route.Backend != expectedBackend {
		rt.t.Error(testCase, "failed to match the backend")
		rt.t.Log("got:     ", route.Id, route.Backend)
		rt.t.Log("expected:", expectedID, expectedBackend)
	}

	if expectedID == "" && expectedBackend == "" && route != nil {
		rt.t.Error(testCase, "unexpected route matched")
	}
}

func (rt *redirectTest) testNormalHTTPS(expectedID, expectedBackend string) {
	httpsRequest := &http.Request{
		Host: rt.rule.Host,
		URL:  &url.URL{Path: rt.rule.Http.Paths[0].Path},
		Header: http.Header{
			"Host": []string{rt.rule.Host},
		},
	}

	rt.testRedirectRoute("normal", httpsRequest, expectedID, expectedBackend)
}

func (rt *redirectTest) testRedirectHTTP(expectedID, expectedBackend string) {
	httpRequest := &http.Request{
		Host: rt.rule.Host,
		URL:  &url.URL{Path: rt.rule.Http.Paths[0].Path},
		Header: http.Header{
			"Host":              []string{rt.rule.Host},
			"X-Forwarded-Proto": []string{"http"},
		},
	}

	rt.testRedirectRoute("redirect", httpRequest, expectedID, expectedBackend)
}

func (rt *redirectTest) testRedirectNotFound(expectedID, expectedBackend string) {
	nonExistingHTTP := &http.Request{
		Host: "www.notexists.org",
		URL:  &url.URL{Path: "/notexists"},
		Header: http.Header{
			"Host":              []string{"www.notexists.org"},
			"X-Forwarded-Proto": []string{"http"},
		},
	}

	rt.testRedirectRoute("unmatched", nonExistingHTTP, expectedID, expectedBackend)
}

func (rt *redirectTest) close() {
	rt.router.Close()
	rt.api.Close()
	rt.l.Close()
}

func TestHTTPSRedirect(t *testing.T) {
	rt, err := newRedirectTest(t, true)
	if err != nil {
		t.Error(err)
		return
	}

	defer rt.close()

	rt.testNormalHTTPS("", rt.backend)
	rt.testRedirectHTTP(httpRedirectRouteID, "")
	rt.testRedirectNotFound(httpRedirectRouteID, "")
}

func TestNoHTTPSRedirect(t *testing.T) {
	rt, err := newRedirectTest(t, false)
	if err != nil {
		t.Error(err)
		return
	}

	defer rt.close()

	rt.testNormalHTTPS("", rt.backend)
	rt.testRedirectHTTP("", rt.backend)
	rt.testRedirectNotFound("", rt.fallbackBackend)
}

func TestCustomGlobalHTTPSRedirectCode(t *testing.T) {
	testCode := func(config, expect int) {
		t.Run(http.StatusText(config), func(t *testing.T) {
			var o Options
			o.ProvideHTTPSRedirect = true
			o.HTTPSRedirectCode = config

			api := newTestAPI(t, nil, &ingressList{})
			defer api.Close()
			o.KubernetesURL = api.server.URL

			c, err := New(o)
			if err != nil {
				t.Fatal(err)
			}

			defer c.Close()

			r, err := c.LoadAll()
			if err != nil {
				t.Fatal(err)
			}

			var rr *eskip.Route
			for i := range r {
				if r[i].Id == httpRedirectRouteID {
					rr = r[i]
					break
				}
			}

			if rr == nil {
				t.Error("redirect route not found")
				return
			}

			var rf *eskip.Filter
			for i := range rr.Filters {
				if rr.Filters[i].Name == "redirectTo" {
					rf = rr.Filters[i]
					break
				}
			}

			if rf == nil {
				t.Error("redirect filter not found")
				return
			}

			if len(rf.Args) == 0 || int(rf.Args[0].(float64)) != expect {
				t.Error("expected redirect status code not set")
			}
		})
	}

	testCode(0, http.StatusPermanentRedirect)
	testCode(http.StatusPermanentRedirect, http.StatusPermanentRedirect)
	testCode(http.StatusMovedPermanently, http.StatusMovedPermanently)
}

func TestEnableHTTPSRedirectFromIngress(t *testing.T) {
	var o Options

	ingressWithRedirect := testIngressSimple(
		"namespace1",
		"ingress1",
		"service1",
		backendPort{"port1"},
		testRule(
			"www.example.org",
			testPathRule("/foo", "service1", backendPort{"port1"}),
		),
	)
	setAnnotation(ingressWithRedirect, redirectAnnotationKey, "true")

	ingressWithoutRedirect := testIngressSimple(
		"namespace1",
		"ingress2",
		"service2",
		backendPort{"port2"},
		testRule(
			"api.example.org",
			testPathRule("/bar", "service2", backendPort{"port2"}),
		),
	)

	api := newTestAPI(t, testServices(), &ingressList{Items: []*ingressItem{
		ingressWithRedirect,
		ingressWithoutRedirect,
	}})
	defer api.server.Close()
	o.KubernetesURL = api.server.URL

	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}

	defer c.Close()

	r, err := c.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	const expectEskip = `
		kube_namespace1__ingress1______: * -> "http://1.2.3.4:8080";
		kube_namespace1__ingress2______: *-> "http://5.6.7.8:8181";
		kube_namespace1__ingress1__www_example_org___foo__service1:
			Host("^www[.]example[.]org$") &&
			PathRegexp("^/foo")
			-> "http://1.2.3.4:8080";
		kube_namespace1__ingress1__www_example_org___foo__service1_https_redirect:
			Header("X-Forwarded-Proto", "http") &&
			Host("^www[.]example[.]org$") &&
			PathRegexp("^/foo") &&
			PathRegexp(".*") &&
			PathRegexp(".*")
			-> redirectTo(308, "https:")
			-> <shunt>;
		kube___catchall__www_example_org____:
			Host("^www[.]example[.]org$")
			-> <shunt>;
		kube___catchall__www_example_org_____https_redirect:
			PathRegexp(".*") &&
			PathRegexp(".*") &&
			Header("X-Forwarded-Proto", "http") &&
			Host("^www[.]example[.]org$")
			-> redirectTo(308, "https:")
			-> <shunt>;
		kube_namespace1__ingress2__api_example_org___bar__service2:
			Host("^api[.]example[.]org$") &&
			PathRegexp("^/bar")
			-> "http://5.6.7.8:8181";
		kube___catchall__api_example_org____:
			Host("^api[.]example[.]org$")
			-> <shunt>;
	`

	expect, err := eskip.Parse(expectEskip)
	if err != nil {
		t.Fatal(err)
	}

	// discard deprecated shunt parsing:
	for _, i := range []int{3, 4, 5, 7} {
		expect[i].Shunt = false
	}

	sort.Sort(sortRoutes(r))
	sort.Sort(sortRoutes(expect))

	diff := cmp.Diff(r, expect)
	if diff != "" {
		t.Error(diff)
	}
}

func TestDisableHTTPSRedirectFromIngress(t *testing.T) {
	var o Options
	o.ProvideHTTPSRedirect = true

	ingressWithRedirect := testIngressSimple(
		"namespace1",
		"ingress1",
		"service1",
		backendPort{"port1"},
		testRule(
			"www.example.org",
			testPathRule("/foo", "service1", backendPort{"port1"}),
		),
	)

	ingressWithoutRedirect := testIngressSimple(
		"namespace1",
		"ingress2",
		"service2",
		backendPort{"port2"},
		testRule(
			"api.example.org",
			testPathRule("/bar", "service2", backendPort{"port2"}),
		),
	)
	setAnnotation(ingressWithoutRedirect, redirectAnnotationKey, "false")

	api := newTestAPI(t, testServices(), &ingressList{Items: []*ingressItem{
		ingressWithRedirect,
		ingressWithoutRedirect,
	}})
	defer api.server.Close()
	o.KubernetesURL = api.server.URL

	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}

	defer c.Close()

	r, err := c.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	const expectEskip = `
		kube_namespace1__ingress1______: * -> "http://1.2.3.4:8080";
		kube_namespace1__ingress2______: * -> "http://5.6.7.8:8181";
		kube_namespace1__ingress1__www_example_org___foo__service1:
			Host("^www[.]example[.]org$") &&
			PathRegexp("^/foo")
			-> "http://1.2.3.4:8080";
		kube___catchall__www_example_org____:
			Host("^www[.]example[.]org$")
			-> <shunt>;
		kube_namespace1__ingress2__api_example_org___bar__service2:
			Host("^api[.]example[.]org$") &&
			PathRegexp("^/bar")
			-> "http://5.6.7.8:8181";
		kube_namespace1__ingress2__api_example_org___bar__service2_disable_https_redirect:
			Host("^api[.]example[.]org$") &&
			PathRegexp("^/bar") &&
			PathRegexp(".*") &&
			PathRegexp(".*") &&
			Header("X-Forwarded-Proto", "http")
			-> "http://5.6.7.8:8181";
		kube___catchall__api_example_org____:
			Host("^api[.]example[.]org$")
			-> <shunt>;
		kube___catchall__api_example_org_____disable_https_redirect:
			Host("^api[.]example[.]org$") &&
			PathRegexp(".*") &&
			PathRegexp(".*") &&
			Header("X-Forwarded-Proto", "http")
			-> <shunt>;
		kube__redirect:
			PathRegexp(/.*/) &&
			PathRegexp(/.*/) &&
			Header("X-Forwarded-Proto", "http")
			-> redirectTo(308, "https:")
			-> <shunt>;
	`

	expect, err := eskip.Parse(expectEskip)
	if err != nil {
		t.Fatal(err)
	}

	// discard deprecated shunt parsing:
	for _, i := range []int{3, 6, 7, 8} {
		expect[i].Shunt = false
	}

	sort.Sort(sortRoutes(r))
	sort.Sort(sortRoutes(expect))

	diff := cmp.Diff(r, expect)
	if diff != "" {
		t.Error(diff)
	}
}

func TestChangeRedirectCodeFromIngress(t *testing.T) {
	var o Options
	o.ProvideHTTPSRedirect = true

	ingressWithCustomRedirectCode := testIngressSimple(
		"namespace1",
		"ingress1",
		"service1",
		backendPort{"port1"},
		testRule(
			"www.example.org",
			testPathRule("/foo", "service1", backendPort{"port1"}),
		),
	)
	setAnnotation(ingressWithCustomRedirectCode, redirectCodeAnnotationKey, "301")

	ingressWithoutCustomRedirectCode := testIngressSimple(
		"namespace1",
		"ingress2",
		"service2",
		backendPort{"port2"},
		testRule(
			"api.example.org",
			testPathRule("/bar", "service2", backendPort{"port2"}),
		),
	)

	api := newTestAPI(t, testServices(), &ingressList{Items: []*ingressItem{
		ingressWithCustomRedirectCode,
		ingressWithoutCustomRedirectCode,
	}})
	defer api.server.Close()
	o.KubernetesURL = api.server.URL

	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}

	defer c.Close()

	r, err := c.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	const expectEskip = `
		kube_namespace1__ingress1______: * -> "http://1.2.3.4:8080";
		kube_namespace1__ingress2______: *-> "http://5.6.7.8:8181";
		kube_namespace1__ingress1__www_example_org___foo__service1:
			Host("^www[.]example[.]org$") &&
			PathRegexp("^/foo")
			-> "http://1.2.3.4:8080";
		kube_namespace1__ingress1__www_example_org___foo__service1_https_redirect:
			Header("X-Forwarded-Proto", "http") &&
			Host("^www[.]example[.]org$") &&
			PathRegexp("^/foo") &&
			PathRegexp(".*") &&
			PathRegexp(".*")
			-> redirectTo(301, "https:")
			-> <shunt>;
		kube___catchall__www_example_org____:
			Host("^www[.]example[.]org$")
			-> <shunt>;
		kube___catchall__www_example_org_____https_redirect:
			PathRegexp(".*") &&
			PathRegexp(".*") &&
			Header("X-Forwarded-Proto", "http") &&
			Host("^www[.]example[.]org$")
			-> redirectTo(301, "https:")
			-> <shunt>;
		kube_namespace1__ingress2__api_example_org___bar__service2:
			Host("^api[.]example[.]org$") &&
			PathRegexp("^/bar")
			-> "http://5.6.7.8:8181";
		kube___catchall__api_example_org____:
			Host("^api[.]example[.]org$")
			-> <shunt>;
		kube__redirect:
			PathRegexp(/.*/) &&
			PathRegexp(/.*/) &&
			Header("X-Forwarded-Proto", "http")
			-> redirectTo(308, "https:")
			-> <shunt>;
	`

	expect, err := eskip.Parse(expectEskip)
	if err != nil {
		t.Fatal(err)
	}

	// discard deprecated shunt parsing:
	for _, i := range []int{3, 4, 5, 7, 8} {
		expect[i].Shunt = false
	}

	sort.Sort(sortRoutes(r))
	sort.Sort(sortRoutes(expect))

	diff := cmp.Diff(r, expect)
	if diff != "" {
		t.Error(diff)
	}
}

func TestEnableRedirectWithCustomCode(t *testing.T) {
	var o Options

	ingressWithRedirect := testIngressSimple(
		"namespace1",
		"ingress1",
		"service1",
		backendPort{"port1"},
		testRule(
			"www.example.org",
			testPathRule("/foo", "service1", backendPort{"port1"}),
		),
	)
	setAnnotation(ingressWithRedirect, redirectAnnotationKey, "true")
	setAnnotation(ingressWithRedirect, redirectCodeAnnotationKey, "301")

	ingressWithoutRedirect := testIngressSimple(
		"namespace1",
		"ingress2",
		"service2",
		backendPort{"port2"},
		testRule(
			"api.example.org",
			testPathRule("/bar", "service2", backendPort{"port2"}),
		),
	)

	api := newTestAPI(t, testServices(), &ingressList{Items: []*ingressItem{
		ingressWithRedirect,
		ingressWithoutRedirect,
	}})
	defer api.server.Close()
	o.KubernetesURL = api.server.URL

	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}

	defer c.Close()

	r, err := c.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	const expectEskip = `
		kube_namespace1__ingress1______: * -> "http://1.2.3.4:8080";
		kube_namespace1__ingress2______: *-> "http://5.6.7.8:8181";
		kube_namespace1__ingress1__www_example_org___foo__service1:
			Host("^www[.]example[.]org$") &&
			PathRegexp("^/foo")
			-> "http://1.2.3.4:8080";
		kube_namespace1__ingress1__www_example_org___foo__service1_https_redirect:
			Header("X-Forwarded-Proto", "http") &&
			Host("^www[.]example[.]org$") &&
			PathRegexp("^/foo") &&
			PathRegexp(".*") &&
			PathRegexp(".*")
			-> redirectTo(301, "https:")
			-> <shunt>;
		kube___catchall__www_example_org____:
			Host("^www[.]example[.]org$")
			-> <shunt>;
		kube___catchall__www_example_org_____https_redirect:
			PathRegexp(".*") &&
			PathRegexp(".*") &&
			Header("X-Forwarded-Proto", "http") &&
			Host("^www[.]example[.]org$")
			-> redirectTo(301, "https:")
			-> <shunt>;
		kube_namespace1__ingress2__api_example_org___bar__service2:
			Host("^api[.]example[.]org$") &&
			PathRegexp("^/bar")
			-> "http://5.6.7.8:8181";
		kube___catchall__api_example_org____:
			Host("^api[.]example[.]org$")
			-> <shunt>;
	`

	expect, err := eskip.Parse(expectEskip)
	if err != nil {
		t.Fatal(err)
	}

	// discard deprecated shunt parsing:
	for _, i := range []int{3, 4, 5, 7} {
		expect[i].Shunt = false
	}

	sort.Sort(sortRoutes(r))
	sort.Sort(sortRoutes(expect))

	diff := cmp.Diff(r, expect)
	if diff != "" {
		t.Error(diff)
	}
}
