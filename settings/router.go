package settings

import (
    "github.com/zalando/skipper/skipper"
    "regexp"
    "github.bus.zalan.do/spearheads/pathtree"
    "errors"
    "net/http"
)

type leafMatcher struct {
    route skipper.Route
    hostRxs []*regexp.Regexp
    method string
    headersExact map[string]string
    headersRegexp map[string]*regexp.Regexp
}

type pathMatcher struct {
    leaves []*leafMatcher
}

type pathRegexpMatcher struct {
    rx *regexp.Regexp
    leaves []*leafMatcher
}

type router struct {
    pathTree *pathtree.Tree
    pathRegexps []*pathRegexpMatcher
    topLeaves []*leafMatcher
}

type skipperRoute struct {
    id string
    filters []skipper.Filter
    backend skipper.Backend
}

type RouteError struct {
    Errors map[int]error
}

type RouteDefinition interface {
    Id() string
    Path() string
    IsPathRegexp() bool
    HostRegexps() []string
    Method() string
    Headers() map[string]string
    HeaderRegexps() map[string]string
    Filters() []skipper.Filter
    Backend() skipper.Backend
}

func isPathMatcher(exp RouteDefinition) bool {
    return exp.Path() != "" && !exp.IsPathRegexp()
}

func isPathRegexp(exp RouteDefinition) bool {
    return exp.Path() != "" && exp.IsPathRegexp()
}

func makeRoute(id string, f []skipper.Filter, b skipper.Backend) (skipper.Route, error) {
    if b == nil {
        return nil, errors.New("invalid route, missing backend")
    }

    return &skipperRoute{id, f, b}, nil
}

func makeLeaf(exp RouteDefinition) (*leafMatcher, error) {
    route, err := makeRoute(exp.Id(), exp.Filters(), exp.Backend())
    if err != nil {
        return nil, err
    }

    hostRxs := make([]*regexp.Regexp, len(exp.HostRegexps()))
    for i, s := range exp.HostRegexps() {
        rx, err := regexp.Compile(s)
        if err != nil {
            return nil, err
        }

        hostRxs[i] = rx
    }

    headerRxs := make(map[string]*regexp.Regexp)
    for k, v := range exp.HeaderRegexps() {
        rx, err := regexp.Compile(v)
        if err != nil {
            return nil, err
        }

        headerRxs[k] = rx
    }

    return &leafMatcher{
        route,
        hostRxs,
        exp.Method(),
        exp.Headers(),
        headerRxs}, nil
}

func addToPathMap(pm map[string]*pathMatcher, exp RouteDefinition) error {
    p := exp.Path()
    if len(p) == 0 || p[0] != '/' {
        return errors.New("path must be absolute")
    }

    matcher := pm[p]
    if matcher == nil {
        matcher = &pathMatcher{}
        pm[p] = matcher
    }

    l, err := makeLeaf(exp)
    if err != nil {
        return err
    }

    matcher.leaves = append(matcher.leaves, l)
    return nil
}

func addToRegexps(pm map[string]*pathRegexpMatcher, exp RouteDefinition) error {
    p := exp.Path()

    matcher := pm[p]
    if matcher == nil {
        rx, err := regexp.Compile(exp.Path())
        if err != nil {
            return err
        }

        matcher = &pathRegexpMatcher{rx: rx}
        pm[p] = matcher
    }

    l, err := makeLeaf(exp)
    if err != nil {
        return err
    }

    matcher.leaves = append(matcher.leaves, l)
    return nil
}

func addToTopLeaves(router *router, exp RouteDefinition) error {
    l, err := makeLeaf(exp)
    if err != nil {
        return err
    }

    router.topLeaves = append(router.topLeaves, l)
    return nil
}

func makeRouteError(errors map[int]error) error {
    return &RouteError{errors}
}

func convertParams(params []pathtree.Param) skipper.PathParams {
    pparams := make(skipper.PathParams, len(params))
    for i, p := range params {
        pparams[i] = &pathParam{p}
    }

    return pparams
}

func matchTree(rt *router, path string) (*pathMatcher, skipper.PathParams) {
    v, params, tailSlash := rt.pathTree.Get(path)
    if v != nil {
        return v.(*pathMatcher), convertParams(params)
    }

    if !tailSlash {
        return nil, nil
    }

    if path[len(path) - 1] == '/' {
        path = path + "/"
    } else {
        path = path[0:len(path) - 1]
    }

    v, params, _ = rt.pathTree.Get(path)
    if v == nil {
        // this should not happen
        return nil, nil
    }

    return v.(*pathMatcher), convertParams(params)
}

func matchRegexp(router *router, path string) *pathRegexpMatcher {
    for _, matcher := range router.pathRegexps {
        if matcher.rx.MatchString(path) {
            return matcher
        }
    }

    return nil
}

func matchLeaf(l *leafMatcher, req *http.Request) skipper.Route {
    if l.method != "" && l.method != req.Method {
        return nil
    }

    for _, h := range l.hostRxs {
        if !h.MatchString(req.Host) {
            return nil
        }
    }

    if len(l.headersExact) > 0 {
        found := false
        for k, v := range l.headersExact {
            vals, has := req.Header[k]
            if has {
                for _, val := range vals {
                    if val == v {
                        found = true
                        break
                    }
                }

                if found {
                    break
                }
            }
        }

        if !found {
            return nil
        }
    }

    if len(l.headersRegexp) > 0 {
        found := false
        for k, rx := range l.headersRegexp {
            vals, has := req.Header[k]
            if has {
                for _, val := range vals {
                    if rx.MatchString(val) {
                        found = true
                        break
                    }
                }

                if found {
                    break
                }
            }
        }

        if !found {
            return nil
        }
    }

    return l.route
}

func matchLeaves(leaves []*leafMatcher, req *http.Request) skipper.Route {
    for _, l := range leaves {
        r := matchLeaf(l, req)
        if r != nil {
            return r
        }
    }

    return nil
}

func Make(routes []RouteDefinition) (skipper.Router, error) {
    router := &router{}
    pm := make(map[string]*pathMatcher)
    prm := make(map[string]*pathRegexpMatcher)
    routeErrors := make(map[int]error)

    addPathRoute := func(index int, exp RouteDefinition,
        check func(exp RouteDefinition) bool,
        add func(exp RouteDefinition) error) bool {

        if !check(exp) {
            return false
        }

        if err := add(exp); err != nil {
            routeErrors[index] = err
        }

        return true
    }

    addPath := func(exp RouteDefinition) error { return addToPathMap(pm, exp) }
    addRegexp := func(exp RouteDefinition) error { return addToRegexps(prm, exp) }

    for index, exp := range routes {
        if !addPathRoute(index, exp, isPathMatcher, addPath) &&
            !addPathRoute(index, exp, isPathRegexp, addRegexp) {
            addToTopLeaves(router, exp)
        }
    }

    pathMap := make(pathtree.PathMap)
    for k, v := range pm {
        pathMap[k] = v
    }

    tree, err := pathtree.Make(pathMap)
    if err != nil {
        // individual invalid route not identified in this case
        return nil, err
    }

    router.pathTree = tree

    var routeErr error
    if len(routeErrors) > 0 {
        routeErr = makeRouteError(routeErrors)
    }

    return router, routeErr
}

func (sr *skipperRoute) Filters() []skipper.Filter {
    return sr.filters
}

func (sr *skipperRoute) Backend() skipper.Backend {
    return sr.backend
}

func (rt *router) Route(req *http.Request) (skipper.Route, skipper.PathParams, error) {
    p := pathtree.CleanPath(req.URL.Path)

    if matcher, params := matchTree(rt, p); matcher != nil {
        if route := matchLeaves(matcher.leaves, req); route != nil {
            return route, params, nil
        }
    }

    if matcher := matchRegexp(rt, p); matcher != nil {
        if route := matchLeaves(matcher.leaves, req); route != nil {
            return route, nil, nil
        }
    }

    return matchLeaves(rt.topLeaves, req), nil, nil
}

func (re *RouteError) Error() string {
    return ""
}
