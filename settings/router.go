package settings

import (
	"github.bus.zalan.do/spearheads/pathmux"
	"github.com/dimfeld/httppath"
	"github.com/zalando/skipper/skipper"
	"net/http"
	"regexp"
	"sort"
)

const freeWildcardExp = "/[*][^/]+$"

type leafMatcher struct {
	method         string
	hostRxs        []*regexp.Regexp
	pathRxs        []*regexp.Regexp
	headersExact   map[string]string
	headersRegexps map[string][]*regexp.Regexp
	route          skipper.Route
}

type leafMatchers []*leafMatcher

type pathMatcher struct {
	leaves            leafMatchers
	freeWildcardParam string
}

type rootMatcher struct {
	paths               *pathmux.Tree
	rootLeaves          leafMatchers
	ignoreTrailingSlash bool
}

type skipperRoute struct {
	id      string
	filters []skipper.Filter
	backend skipper.Backend
}

type RouteDefinition interface {
	Id() string
	Path() string
	Method() string
	HostRegexps() []string
	PathRegexps() []string
	Headers() map[string]string
	HeaderRegexps() map[string][]string
	Filters() []skipper.Filter
	Backend() skipper.Backend
}

type RouteError struct {
	Id       string
	Original error
}

var freeWildcardRx *regexp.Regexp

func init() {
	freeWildcardRx = regexp.MustCompile(freeWildcardExp)
}

func (ls leafMatchers) Len() int      { return len(ls) }
func (ls leafMatchers) Swap(i, j int) { ls[i], ls[j] = ls[j], ls[i] }

func leafWeight(l *leafMatcher) int {
	w := 0

	if l.method != "" {
		w++
	}

	w += len(l.hostRxs)
	w += len(l.pathRxs)
	w += len(l.headersExact)
	w += len(l.headersRegexps)

	return w
}

func (ls leafMatchers) Less(i, j int) bool {
	return leafWeight(ls[i]) > leafWeight(ls[j])
}

func matchRegexps(rxs []*regexp.Regexp, s string) bool {
	for _, rx := range rxs {
		if !rx.MatchString(s) {
			return false
		}
	}

	return true
}

func findHeader(h http.Header, key string, check func(string) bool) bool {
	vals, has := h[key]
	if !has {
		return false
	}

	for _, val := range vals {
		if check(val) {
			return true
		}
	}

	return false
}

func matchHeaders(exact map[string]string, hrxs map[string][]*regexp.Regexp, h http.Header) bool {
	for k, v := range exact {
		if !findHeader(h, k, func(val string) bool { return val == v }) {
			return false
		}
	}

	for k, rxs := range hrxs {
		for _, rx := range rxs {
			if !findHeader(h, k, rx.MatchString) {
				return false
			}
		}
	}

	return true
}

func matchLeaf(l *leafMatcher, req *http.Request, path string) bool {
	if l.method != "" && l.method != req.Method {
		return false
	}

	if !matchRegexps(l.hostRxs, req.Host) {
		return false
	}

	if !matchRegexps(l.pathRxs, path) {
		return false
	}

	if !matchHeaders(l.headersExact, l.headersRegexps, req.Header) {
		return false
	}

	return true
}

func matchLeaves(leaves leafMatchers, req *http.Request, path string) *leafMatcher {
	for _, l := range leaves {
		if matchLeaf(l, req, path) {
			return l
		}
	}

	return nil
}

func matchPathTree(tree *pathmux.Tree, path string) (leafMatchers, skipper.PathParams) {
	v, params := tree.Lookup(path)
	if v == nil {
		return nil, nil
	}

	pm := v.(*pathMatcher)
	if pm.freeWildcardParam != "" {
		freeParam := params[pm.freeWildcardParam]
		freeParam = "/" + freeParam
		params[pm.freeWildcardParam] = freeParam
	}

	return pm.leaves, params
}

func match(matcher *rootMatcher, req *http.Request) (skipper.Route, skipper.PathParams) {
	path := httppath.Clean(req.URL.Path)
	if matcher.ignoreTrailingSlash && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}

	leaves, params := matchPathTree(matcher.paths, path)

	l := matchLeaves(leaves, req, path)
	if l != nil {
		return l.route, params
	}

	l = matchLeaves(matcher.rootLeaves, req, path)
	if l != nil {
		return l.route, nil
	}

	return nil, nil
}

func compileRxs(exps []string) ([]*regexp.Regexp, error) {
	rxs := make([]*regexp.Regexp, len(exps))
	for i, exp := range exps {
		rx, err := regexp.Compile(exp)
		if err != nil {
			return nil, err
		}

		rxs[i] = rx
	}

	return rxs, nil
}

func makeLeaf(d RouteDefinition) (*leafMatcher, error) {
	hostRxs, err := compileRxs(d.HostRegexps())
	if err != nil {
		return nil, err
	}

	pathRxs, err := compileRxs(d.PathRegexps())
	if err != nil {
		return nil, err
	}

	headerExps := d.HeaderRegexps()
	allHeaderRxs := make(map[string][]*regexp.Regexp)
	for k, exps := range headerExps {
		headerRxs, err := compileRxs(exps)
		if err != nil {
			return nil, err
		}

		allHeaderRxs[k] = headerRxs
	}

	return &leafMatcher{
		method:         d.Method(),
		hostRxs:        hostRxs,
		pathRxs:        pathRxs,
		headersExact:   d.Headers(),
		headersRegexps: allHeaderRxs,
		route:          &skipperRoute{d.Id(), d.Filters(), d.Backend()}}, nil
}

func freeWildcardParam(path string) string {
	param := freeWildcardRx.FindString(path)
	if param == "" {
		return ""
	}

	return param[2:]
}

func makeMatcher(definitions []RouteDefinition, ignoreTrailingSlash bool) (*rootMatcher, []*RouteError) {
	var (
		routeErrors []*RouteError
		rootLeaves  leafMatchers
	)

	pathMatchers := make(map[string]*pathMatcher)

	for _, d := range definitions {
		l, err := makeLeaf(d)
		if err != nil {
			routeErrors = append(routeErrors, &RouteError{d.Id(), err})
			continue
		}

		p := d.Path()
		if p == "" {
			rootLeaves = append(rootLeaves, l)
			continue
		}

		p = httppath.Clean(p)
		if ignoreTrailingSlash && p[len(p)-1] == '/' {
			p = p[:len(p)-1]
		}

		pm := pathMatchers[p]
		if pm == nil {
			pm = &pathMatcher{freeWildcardParam: freeWildcardParam(p)}
			pathMatchers[p] = pm
		}

		pm.leaves = append(pm.leaves, l)
	}

	pathTree := &pathmux.Tree{}
	for p, m := range pathMatchers {
		sort.Sort(m.leaves)
		err := pathTree.Add(p, m)
		if err != nil {
			routeErrors = append(routeErrors, &RouteError{Original: err})
		}
	}

	sort.Sort(rootLeaves)
	return &rootMatcher{pathTree, rootLeaves, ignoreTrailingSlash}, routeErrors
}

func (sr *skipperRoute) Filters() []skipper.Filter {
	return sr.filters
}

func (sr *skipperRoute) Backend() skipper.Backend {
	return sr.backend
}

func (m *rootMatcher) Route(req *http.Request) (skipper.Route, skipper.PathParams, error) {
	r, p := match(m, req)
	return r, p, nil
}

func (re *RouteError) Error() string {
	return re.Original.Error()
}
