package settings

import (
    "github.bus.zalan.do/spearheads/randpath"
    "github.com/zalando/skipper/skipper"
    "github.com/zalando/skipper/mock"
    "net/http"
    "net/url"
    "fmt"
    "errors"
    "github.com/mailgun/route"
    "github.bus.zalan.do/spearheads/pathtree"
    "testing"
)

const (
    routesCountPhase1 = 1
    routesCountPhase2 = 100
    routesCountPhase3 = 10000
    routesCountPhase4 = 1000000
)

// create tests to check used features like wildcards
// generate  same set of routes
// generate router of both types with different number of paths
// create tests for all combinations

var (
    paths []string
    routes []skipper.Route
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

func generatePaths(count int) []string {
    paths := make([]string, count)

    // we need to avoid '/' paths here, because we are not testing conflicting cases
    // here, and with 0 or 1 MinNamesInPath, there would be multiple '/'s.
    // At the same time, with too many paths, conflicts still may occur, that's why
    // RandSeed is set to value, where not.
    pg := randpath.Make(randpath.Options{
        MinNamesInPath: 2,
        MaxNamesInPath: 15})

    for i := 0; i < count; i++ {
        paths[i] = pg.Next()
    }

    return paths
}

func generateRoutes(count int) []skipper.Route {
    routes := make([]skipper.Route, count)
    for i := 0; i < count; i++ {
        routes[i] = &mock.Route{}
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
        router.AddRoute(fmt.Sprintf("Path(\"%s\")", p), routes[i % len(routes)])
    }

    return router, nil
}

func makeRouterPathTree(paths []string, routes []skipper.Route) (skipper.Router, error) {
    if len(routes) == 0 {
        return nil, errors.New("we need at least one route for this test")
    }

    pm := make(pathtree.PathMap)
    for i, p := range paths {
        pm[p] = routes[i % len(routes)]
    }

    tree, err := pathtree.Make(pm)
    if err != nil {
        return nil, err
    }

    return &pathTreeRouter{tree}, nil
}

func init() {
    const count = routesCountPhase4

    var err error
    defer func() {
        if err != nil {
            panic(err)
        }
    }()

    paths = generatePaths(count)
    routes = generateRoutes(count)

    requests, err = generateRequests(paths)
    if err != nil {
        panic(err)
    }

    unregisteredPaths := generatePaths(count)
    unregisteredRequests, err := generateRequests(unregisteredPaths)
    if err != nil {
        panic(err)
    }

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

    mailgun1 = makeRouter(makeRouterMailgun, paths[0:routesCountPhase1], routes[0:routesCountPhase1])
    mailgun2 = makeRouter(makeRouterMailgun, paths[0:routesCountPhase2], routes[0:routesCountPhase2])

    // this number of routes takes too long for the mailgun router to construct
    // mailgun3 = makeRouter(makeRouterMailgun, paths[0:routesCountPhase3], routes[0:routesCountPhase3])
    // mailgun4 = makeRouter(makeRouterMailgun, paths[0:routesCountPhase4], routes[0:routesCountPhase4])

    pathTree1 = makeRouter(makeRouterPathTree, paths[0:routesCountPhase1], routes[0:routesCountPhase1])
    pathTree2 = makeRouter(makeRouterPathTree, paths[0:routesCountPhase2], routes[0:routesCountPhase2])
    pathTree3 = makeRouter(makeRouterPathTree, paths[0:routesCountPhase3], routes[0:routesCountPhase3])
    pathTree4 = makeRouter(makeRouterPathTree, paths[0:routesCountPhase4], routes[0:routesCountPhase4])
}

func integrityTest(t *testing.T, router skipper.Router, phaseCount int) {
    index := phaseCount / 2
    r, err := router.Route(requests[index])
    if err != nil || r != routes[index] {
        t.Error("failed to route")
    }
}

func benchmarkLookup(b *testing.B, router skipper.Router, phaseCount int) {
    requestCount := phaseCount * 2
    var index int
    for i := 0; i < b.N; i++ {
        index = i % requestCount
        r, err := router.Route(requests[index])
        if err != nil || (index < phaseCount && r != routes[index]) || (index >= phaseCount && r != nil) {
            b.Log("benchmark failed", err, r, i, b.N, phaseCount, requestCount, paths[index])
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

func BenchmarkMailgun1(b *testing.B) {
    benchmarkLookup(b, mailgun1, routesCountPhase1)
}

func BenchmarkMailgun2(b *testing.B) {
    benchmarkLookup(b, mailgun2, routesCountPhase2)
}

// only when in patience mode
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
