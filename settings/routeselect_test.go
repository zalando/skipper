package settings

import (
    "github.bus.zalan.do/spearheads/randpath"
    "github.com/zalando/skipper/skipper"
    "github.com/zalando/skipper/mock"
    "net/http"
    "net/url"
    "fmt"
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
    mailgun100 skipper.Router
    mailgun10000 skipper.Router
    mailgun1000000 skipper.Router

    pathtree1 skipper.Router
    pathtree100 skipper.Router
    pathtree10000 skipper.Router
    pathtree1000000 skipper.Router
)

func generatePaths(count int) []string {
    paths := make([]string, count)
    pg := randpath.Make(randpath.Options{})
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

func init() {
    const count = routesCountPhase4
    paths = generatePaths(count)
    routes = generateRoutes(count)

    reqs, err := generateRequests(paths)
    if err != nil {
        panic(err)
    }

    requests = reqs
}
