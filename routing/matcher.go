// Package requestmatch implements matching http requests to associated values.
//
// Matching is based on the attributes of http requests, where a request matches
// a definition if it fulfills all condition in it. The evaluation happens in the
// following order:
//
// 1. The request path is used to find leaf definitions in a lookup tree. If no
// path match was found, the leaf definitions in the root are taken that don't
// have a condition for path matching.
//
// 2. If any leaf definitions were found, they are evaluated against the request
// and the associated value of the first matching definition is returned. The order
// of the evaluation happens from the strictest definition to the least strict
// definition, where strictness is proportional to the number of non-empty
// conditions in the definition.
//
// Path matching supports two kind of wildcards:
//
// - a simple wildcard matches a single tag in a path. E.g: /users/:name/roles
// will be matched by /users/jdoe/roles, and the value of the parameter 'name' will
// be 'jdoe'
//
// - a freeform wildcard matches the last segment of a path, with any number of
// tags in it. E.g: /assets/*assetpath will be matched by /assets/images/logo.png,
// and the value of the parameter 'assetpath' will be '/images/logo.png'.
//
package routing

import (
	"fmt"
	"github.com/dimfeld/httppath"
	"github.com/zalando/pathmux"
	"net/http"
	"regexp"
	"sort"
)

type leafMatcher struct {
	method        string
	hostRxs       []*regexp.Regexp
	pathRxs       []*regexp.Regexp
	headersExact  map[string]string
	headersRegexp map[string][]*regexp.Regexp
	route         *Route
}

type leafMatchers []*leafMatcher

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
	w += len(l.headersRegexp)

	return w
}

func (ls leafMatchers) Less(i, j int) bool {
	return leafWeight(ls[i]) > leafWeight(ls[j])
}

type pathMatcher struct {
	leaves            leafMatchers
	freeWildcardParam string
}

// A Matcher represents a preprocessed set of definitions and their associated
// values.
type matcher struct {
	paths           *pathmux.Tree
	rootLeaves      leafMatchers
	matchingOptions MatchingOptions
}

// An error created if a definition cannot be preprocessed.
type definitionError struct {
	Id       string
	Index    int
	Original error
}

func (err *definitionError) Error() string {
	if err.Index < 0 {
		return err.Original.Error()
	}

	return fmt.Sprintf("%s [%d]: %v", err.Id, err.Index, err.Original)
}

var freeWildcardRx *regexp.Regexp

func init() {
	freeWildcardRx = regexp.MustCompile("/[*][^/]+$")
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

func canonicalizeHeaders(h map[string]string) map[string]string {
	ch := make(map[string]string)
	for k, v := range h {
		ch[http.CanonicalHeaderKey(k)] = v
	}

	return ch
}

func canonicalizeHeaderRegexps(hrx map[string][]*regexp.Regexp) map[string][]*regexp.Regexp {
	chrx := make(map[string][]*regexp.Regexp)
	for k, v := range hrx {
		chrx[http.CanonicalHeaderKey(k)] = v
	}

	return chrx
}

func newLeaf(r *Route) (*leafMatcher, error) {
	hostRxs, err := compileRxs(r.HostRegexps)
	if err != nil {
		return nil, err
	}

	pathRxs, err := compileRxs(r.PathRegexps)
	if err != nil {
		return nil, err
	}

	headerExps := r.HeaderRegexps
	allHeaderRxs := make(map[string][]*regexp.Regexp)
	for k, exps := range headerExps {
		headerRxs, err := compileRxs(exps)
		if err != nil {
			return nil, err
		}

		allHeaderRxs[k] = headerRxs
	}

	return &leafMatcher{
		method:        r.Method,
		hostRxs:       hostRxs,
		pathRxs:       pathRxs,
		headersExact:   canonicalizeHeaders(r.Headers),
		headersRegexp: canonicalizeHeaderRegexps(allHeaderRxs),
		route:         r}, nil
}

func freeWildcardParam(path string) string {
	param := freeWildcardRx.FindString(path)
	if param == "" {
		return ""
	}

	// clip '/*' and return only the name
	return param[2:]
}

// Constructs a Matcher based on the provided definitions. If `ignoreTrailingSlash`
// is true, the Matcher handles paths with or without a trailing slash equally.
func newMatcher(rs []*Route, o MatchingOptions) (*matcher, []*definitionError) {
	var (
		errors     []*definitionError
		rootLeaves leafMatchers
	)

	pathMatchers := make(map[string]*pathMatcher)

	for i, r := range rs {
		l, err := newLeaf(r)
		if err != nil {
			errors = append(errors, &definitionError{r.Id, i, err})
			continue
		}

		p := r.Path
		if p == "" {
			rootLeaves = append(rootLeaves, l)
			continue
		}

		// normalize path
		// in case ignoring trailing slashes, store and match all paths
		// without the trailing slash
		p = httppath.Clean(p)
		if o.ignoreTrailingSlash() && p[len(p)-1] == '/' {
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

		// sort leaves during construction time, based on their priority
		sort.Sort(m.leaves)

		err := pathTree.Add(p, m)
		if err != nil {
			errors = append(errors, &definitionError{"", -1, err})
		}
	}

	// sort root leaves during construction time, based on their priority
	sort.Sort(rootLeaves)

	return &matcher{pathTree, rootLeaves, o}, errors
}

func matchPathTree(tree *pathmux.Tree, path string) (leafMatchers, map[string]string) {
	v, params := tree.Lookup(path)
	if v == nil {
		return nil, nil
	}

	// prepend slash in case of free form wildcards path segments (`/*name`),
	pm := v.(*pathMatcher)
	if pm.freeWildcardParam != "" {
		freeParam := params[pm.freeWildcardParam]
		freeParam = "/" + freeParam
		params[pm.freeWildcardParam] = freeParam
	}

	return pm.leaves, params
}

func matchRegexps(rxs []*regexp.Regexp, s string) bool {
	for _, rx := range rxs {
		if !rx.MatchString(s) {
			return false
		}
	}

	return true
}

func matchHeader(h http.Header, key string, check func(string) bool) bool {
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
	// todo: would be better to allow any that match, even if slower

	for k, v := range exact {
		if !matchHeader(h, k, func(val string) bool { return val == v }) {
			return false
		}
	}

	for k, rxs := range hrxs {
		for _, rx := range rxs {
			if !matchHeader(h, k, rx.MatchString) {
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

	if !matchHeaders(l.headersExact, l.headersRegexp, req.Header) {
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

// Tries to match a request against the available definitions. If a match is found,
// returns the associated value, and the wildcard parameters from the path definition,
// if any.
func (m *matcher) match(r *http.Request) (*Route, map[string]string) {
	// normalize path before matching
	// in case ignoring trailing slashes, match without the trailing slash
	path := httppath.Clean(r.URL.Path)
	if m.matchingOptions.ignoreTrailingSlash() && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}

	// first match fixed and wildcard paths
	leaves, params := matchPathTree(m.paths, path)
	l := matchLeaves(leaves, r, path)
	if l != nil {
		return l.route, params
	}

	// if no path match, match root leaves for other conditions
	l = matchLeaves(m.rootLeaves, r, path)
	if l != nil {
		return l.route, nil
	}

	return nil, nil
}
