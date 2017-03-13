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
}

func newRedirectTest(t *testing.T, redirectEnabled bool) (*redirectTest, error) {
	s := testServices()
	i := &ingressList{Items: testIngresses()}
	api := newTestAPI(t, s, i)

	dc, err := New(Options{
		KubernetesURL:        api.server.URL,
		ProvideHTTPSRedirect: redirectEnabled,
	})
	if err != nil {
		return nil, err
	}

	l := loggingtest.New()
	router := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{dc},
		Log:            l,
	})

	const to = 120 * time.Millisecond
	l.WaitFor("all ingresses received", to)
	l.WaitFor("route settings applied", to)

	if err != nil {
		t.Error(err)
		return nil, err
	}

	ingress := i.Items[1]
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
	}, nil
}

func (rt *redirectTest) testRedirectRoute(req *http.Request, expectedID, expectedBackend string) bool {
	route, _ := rt.router.Route(req)
	switch {
	case expectedID != "" && route.Id != expectedID:
		return false
	case expectedBackend != "" && route.Backend != expectedBackend:
		return false
	case expectedID == "" && expectedBackend == "" && route != nil:
		return false
	default:
		return true
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

	if !rt.testRedirectRoute(httpsRequest, expectedID, expectedBackend) {
		rt.t.Error("failed to match the right route when checking normal request")
	}
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

	if !rt.testRedirectRoute(httpRequest, expectedID, expectedBackend) {
		rt.t.Error("failed to match the right route when checking redirect request")
	}
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

	if !rt.testRedirectRoute(nonExistingHTTP, expectedID, expectedBackend) {
		rt.t.Error("failed to match the right route when checking unmatched redirect")
	}
}

func (rt *redirectTest) close() {
	rt.router.Close()
	rt.api.Close()
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
