package settings

import (
	"errors"
	"fmt"
	"github.bus.zalan.do/spearheads/randpath"
	"github.com/mailgun/route"
	"github.com/zalando/skipper/mock"
	"github.com/zalando/skipper/skipper"
	"net/http"
	"net/url"
	"strconv"
	"testing"
)

const (
	routesCountPhase1 = 1
	routesCountPhase2 = 100
	routesCountPhase3 = 10000
	routesCountPhase4 = 1000000
)

var (
	paths    []string
	routes   []skipper.Route
	requests []*http.Request

	mailgun1 skipper.Router
	mailgun2 skipper.Router
	mailgun3 skipper.Router
	mailgun4 skipper.Router

	pathTree1 skipper.Router
	pathTree2 skipper.Router
	pathTree3 skipper.Router
	pathTree4 skipper.Router
)

func generatePaths(pg randpath.PathGenerator, count int) []string {
	paths := make([]string, count)

	for i := 0; i < count; i++ {
		paths[i] = pg.Next()
	}

	return paths
}

func generateRoutes(count int) []skipper.Route {
	routes := make([]skipper.Route, count)
	for i := 0; i < count; i++ {

		// empty route is good enough here,
		// only the instance needs to be different
		routes[i] = &mock.Route{FBackend: &mock.Backend{FHost: strconv.Itoa(i)}}
	}

	return routes
}

func generateRequests(paths []string) ([]*http.Request, error) {
	requests := make([]*http.Request, len(paths))
	for i := 0; i < len(paths); i++ {
		url, err := url.Parse(fmt.Sprintf("http://example.com%s", paths[i]))
		if err != nil {
			return nil, err
		}

		requests[i] = &http.Request{Method: "GET", URL: url}
	}

	return requests, nil
}

func makeRouterMailgun(paths []string, routes []skipper.Route) (skipper.Router, error) {
	if len(routes) == 0 {
		return nil, errors.New("we need at least one route for this test")
	}

	router := route.New()
	for i, p := range paths {
		router.AddRoute(fmt.Sprintf("Path(\"%s\")", p), routes[i%len(routes)])
	}

	return &mailgunRouter{router}, nil
}

// func makeRouterPathTree(paths []string, routes []skipper.Route) (skipper.Router, error) {
// 	if len(routes) == 0 {
// 		return nil, errors.New("we need at least one route for this test")
// 	}
//
// 	pm := make(pathtree.PathMap)
// 	for i, p := range paths {
// 		pm[p] = routes[i%len(routes)]
// 	}
//
// 	tree, err := pathtree.Make(pm)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	return &pathTreeRouter{tree}, nil
// }

type pathRouteDefinition struct {
	path  string
	route skipper.Route
}

func (prd *pathRouteDefinition) Id() string                         { return "" }
func (prd *pathRouteDefinition) Path() string                       { return prd.path }
func (prd *pathRouteDefinition) Method() string                     { return "" }
func (prd *pathRouteDefinition) HostRegexps() []string              { return nil }
func (prd *pathRouteDefinition) PathRegexps() []string              { return nil }
func (prd *pathRouteDefinition) Headers() map[string]string         { return nil }
func (prd *pathRouteDefinition) HeaderRegexps() map[string][]string { return nil }
func (prd *pathRouteDefinition) Filters() []skipper.Filter          { return prd.route.Filters() }
func (prd *pathRouteDefinition) Backend() skipper.Backend           { return prd.route.Backend() }

func makeSkipperRouter(paths []string, routes []skipper.Route) (skipper.Router, error) {
	if len(routes) == 0 {
		return nil, errors.New("we need at least one route for this test")
	}

	definitions := make([]RouteDefinition, len(paths))
	for i, p := range paths {
		definitions[i] = &pathRouteDefinition{p, routes[i%len(routes)]}
	}

	router, errs := makeMatcher(definitions, false)
	if len(errs) != 0 {
		return nil, errors.New("failed to create matcher")
	}

	return router, nil
}

// initialize in advance whatever is possible
func init() {
	const count = routesCountPhase4

	// we need to avoid '/' paths here, because we are not testing conflicting cases
	// here, and with 0 or 1 MinNamesInPath, there would be multiple '/'s.
	pg := randpath.Make(randpath.Options{
		MinNamesInPath: 2,
		MaxNamesInPath: 15})

	var err error

	paths = generatePaths(pg, count)
	routes = generateRoutes(count)

	requests, err = generateRequests(paths)
	if err != nil {
		panic(err)
	}

	unregisteredPaths := generatePaths(pg, count)
	unregisteredRequests, err := generateRequests(unregisteredPaths)
	if err != nil {
		panic(err)
	}

	// the upper half of the requests should not be found
	requests = append(requests, unregisteredRequests...)

	makeRouter := func(make func([]string, []skipper.Route) (skipper.Router, error),
		paths []string, routes []skipper.Route) skipper.Router {

		if err != nil {
			return nil
		}

		r, e := make(paths, routes)
		err = e
		return r
	}

	defer func() {
		if err != nil {
			panic(err)
		}
	}()

	mailgun1 = makeRouter(makeRouterMailgun, paths[0:routesCountPhase1], routes[0:routesCountPhase1])
	mailgun2 = makeRouter(makeRouterMailgun, paths[0:routesCountPhase2], routes[0:routesCountPhase2])

	// this number of routes takes too long for the mailgun router to construct
	// mailgun3 = makeRouter(makeRouterMailgun, paths[0:routesCountPhase3], routes[0:routesCountPhase3])
	// mailgun4 = makeRouter(makeRouterMailgun, paths[0:routesCountPhase4], routes[0:routesCountPhase4])

	pathTree1 = makeRouter(makeSkipperRouter, paths[0:routesCountPhase1], routes[0:routesCountPhase1])
	pathTree2 = makeRouter(makeSkipperRouter, paths[0:routesCountPhase2], routes[0:routesCountPhase2])
	pathTree3 = makeRouter(makeSkipperRouter, paths[0:routesCountPhase3], routes[0:routesCountPhase3])
	pathTree4 = makeRouter(makeSkipperRouter, paths[0:routesCountPhase4], routes[0:routesCountPhase4])
}

func integrityTest(t *testing.T, router skipper.Router, phaseCount int) {
	index := phaseCount / 2 // select one from right in the middle
	r, _, err := router.Route(requests[index])
	if err != nil || r.Backend().Host() != routes[index].Backend().Host() {
		t.Error("failed to route")
	}
}

func benchmarkLookup(b *testing.B, router skipper.Router, phaseCount int) {
	// see init, double as much requests as routes
	requestCount := phaseCount * 2

	var index int
	for i := 0; i < b.N; i++ {

		// b.N comes from the test vault, doesn't matter if it matches the available
		// number of requests or routes, because in successful case, b.N will be far bigger
		index = i % requestCount

		r, _, err := router.Route(requests[index])

		// error, or should have found but didn't, or shouldn't have found but did
		if err != nil ||
			(index < phaseCount && r.Backend().Host() != routes[index].Backend().Host()) ||
			(index >= phaseCount && r != nil) {
			b.Log("benchmark failed", err, r, index, i, b.N)
			b.FailNow()
		}
	}
}

func TestIntegrity(t *testing.T) {
	integrityTest(t, mailgun1, routesCountPhase1)
	integrityTest(t, mailgun2, routesCountPhase2)

	integrityTest(t, pathTree1, routesCountPhase1)
	integrityTest(t, pathTree2, routesCountPhase2)
	integrityTest(t, pathTree3, routesCountPhase3)
	integrityTest(t, pathTree4, routesCountPhase4)
}

func TestMethodOnlyVsPathAndMethod(t *testing.T) {
	router := route.New()
	router.AddRoute("Path(\"/test\") && Method(\"PUT\")", 36)
	router.AddRoute("Method(\"PUT\")", 42)

	u, err := url.Parse("https://example.com/test")
	if err != nil {
		t.Error(err)
	}

	uw, err := url.Parse("https://example.com/wrong")
	if err != nil {
		t.Error(err)
	}

	r, err := router.Route(&http.Request{Method: "PUT", URL: u})
	if err != nil || r != 36 {
		t.Error(err, r)
	}

	r, err = router.Route(&http.Request{Method: "PUT", URL: uw})
	if err != nil || r != 42 {
		t.Error(err, r)
	}

	r, err = router.Route(&http.Request{Method: "GET", URL: uw})
	if err != nil || r != nil {
		t.Error(err, "shouldn't match")
	}

	r, err = router.Route(&http.Request{Method: "GET", URL: u})
	if err != nil || r != nil {
		t.Error(err, "shouldn't match")
	}
}

func BenchmarkMailgun1(b *testing.B) {
	benchmarkLookup(b, mailgun1, routesCountPhase1)
}

func BenchmarkMailgun2(b *testing.B) {
	benchmarkLookup(b, mailgun2, routesCountPhase2)
}

// only when in highest patience mode, lunch for 3, weekend for 4 :)
//
// func BenchmarkMailgun3(b *testing.B) {
//     benchmarkLookup(b, mailgun3, routesCountPhase3)
// }
//
// func BenchmarkMailgun4(b *testing.B) {
//     benchmarkLookup(b, mailgun4, routesCountPhase4)
// }

func BenchmarkPathTree1(b *testing.B) {
	benchmarkLookup(b, pathTree1, routesCountPhase1)
}

func BenchmarkPathTree2(b *testing.B) {
	benchmarkLookup(b, pathTree2, routesCountPhase2)
}

func BenchmarkPathTree3(b *testing.B) {
	benchmarkLookup(b, pathTree3, routesCountPhase3)
}

func BenchmarkPathTree4(b *testing.B) {
	benchmarkLookup(b, pathTree4, routesCountPhase4)
}
