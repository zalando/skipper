package kubernetes

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

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
			"X-Forwarded-Port":  []string{"80"},
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
			"X-Forwarded-Port":  []string{"80"},
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
