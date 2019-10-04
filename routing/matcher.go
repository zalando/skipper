package routing

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"

	"github.com/dimfeld/httppath"
	"github.com/zalando/skipper/pathmux"
)

type leafRequestMatcher struct {
	r    *http.Request
	path string
}

func (m *leafRequestMatcher) Match(value interface{}) (bool, interface{}) {
	v, ok := value.(*pathMatcher)
	if !ok {
		return false, nil
	}

	l := matchLeaves(v.leaves, m.r, m.path)
	return l != nil, l
}

type subtreeMergeControl struct {
	noSubtreeRoot     bool
	subtreeRoot       string
	freeWildcardParamFrom string
	freeWildcardParamTo string
}

type leafMatcher struct {
	exactPath           string
	method              string
	hostRxs             []*regexp.Regexp
	pathRxs             []*regexp.Regexp
	headersExact        map[string]string
	headersRegexp       map[string][]*regexp.Regexp
	predicates          []Predicate
	route               *Route
	subtreeMergeControl subtreeMergeControl
}

type leafMatchers []*leafMatcher

func leafWeight(l *leafMatcher) int {
	w := 0

	if l.method != "" {
		w++
	}

	w += len(l.hostRxs)
	w += len(l.pathRxs)
	w += len(l.headersExact)
	w += len(l.headersRegexp)
	w += len(l.predicates)

	return w
}

// Sorting of leaf matchers:
func (ls leafMatchers) Len() int           { return len(ls) }
func (ls leafMatchers) Swap(i, j int)      { ls[i], ls[j] = ls[j], ls[i] }
func (ls leafMatchers) Less(i, j int) bool { return leafWeight(ls[i]) > leafWeight(ls[j]) }

type pathMatcher struct {
	leaves            leafMatchers
	freeWildcardParam string
}

// root structure representing the routing tree.
type matcher struct {
	paths           *pathmux.Tree
	rootLeaves      leafMatchers
	matchingOptions MatchingOptions
}

// An error created if a route definition cannot be processed.
type definitionError struct {
	ID       string
	Index    int
	Original error
}

func (err *definitionError) Error() string {
	if err.Index < 0 {
		return err.Original.Error()
	}

	return fmt.Sprintf("%s [%d]: %v", err.ID, err.Index, err.Original)
}

// rx identifying the 'free form' wildcards at the end of the paths
var freeWildcardRx = regexp.MustCompile("/[*][^/]+$")

// compiles all rxs or fails
func getCompiledRxs(compiled map[string]*regexp.Regexp, exps []string) ([]*regexp.Regexp, error) {
	rxs := make([]*regexp.Regexp, 0, len(exps))
	for _, exp := range exps {
		if rx, ok := compiled[exp]; ok {
			rxs = append(rxs, rx)
			continue
		}

		rx, err := regexp.Compile(exp)
		if err != nil {
			return nil, err
		}

		compiled[exp] = rx
		rxs = append(rxs, rx)
	}

	return rxs, nil
}

// canonicalizes the keys of the header conditions
func canonicalizeHeaders(h map[string]string) map[string]string {
	ch := make(map[string]string)
	for k, v := range h {
		ch[http.CanonicalHeaderKey(k)] = v
	}

	return ch
}

// canonicalizes the keys of the header regexp conditions
func canonicalizeHeaderRegexps(hrx map[string][]*regexp.Regexp) map[string][]*regexp.Regexp {
	chrx := make(map[string][]*regexp.Regexp)
	for k, v := range hrx {
		chrx[http.CanonicalHeaderKey(k)] = v
	}

	return chrx
}

// creates a new leaf matcher. preprocesses the
// Host, PathRegexp, Header and HeaderRegexp
// conditions.
//
// Using a set of regular expressions shared in
// the current generation to preserve the
// compiled instances.
//
func newLeaf(r *Route, rxs map[string]*regexp.Regexp) (*leafMatcher, error) {
	hostRxs, err := getCompiledRxs(rxs, r.HostRegexps)
	if err != nil {
		return nil, err
	}

	pathRxs, err := getCompiledRxs(rxs, r.PathRegexps)
	if err != nil {
		return nil, err
	}

	headerExps := r.HeaderRegexps
	allHeaderRxs := make(map[string][]*regexp.Regexp)
	for k, exps := range headerExps {
		headerRxs, err := getCompiledRxs(rxs, exps)
		if err != nil {
			return nil, err
		}

		allHeaderRxs[k] = headerRxs
	}

	return &leafMatcher{
		method:        r.Method,
		hostRxs:       hostRxs,
		pathRxs:       pathRxs,
		headersExact:  canonicalizeHeaders(r.Headers),
		headersRegexp: canonicalizeHeaderRegexps(allHeaderRxs),
		predicates:    r.Predicates,
		route:         r}, nil
}

// returns the free form wildcard parameter of a path
func freeWildcardParam(path string) string {
	param := freeWildcardRx.FindString(path)
	if param == "" {
		return ""
	}

	// clip '/*' and return only the name
	return param[2:]
}

func trimTrailingSlash(path string) string {
	if len(path) > 1 && path[len(path)-1] == '/' {
		return path[:len(path)-1]
	}

	return path
}

func cleanPath(path string, o MatchingOptions) string {
	path = httppath.Clean(path)
	if o.ignoreTrailingSlash() {
		path = trimTrailingSlash(path)
	}

	return path
}

// add all required tree entries for a subtree with patching the path and
// the wildcard name if required
func addSubtreeMatcher(pathTree *pathmux.Tree, path string, m *pathMatcher) error {
	// if has named free wildcard, use its name and take the path only
	// otherwise set the free wildcard name by convention to "*", as in "/foo/**"
	fwp := m.freeWildcardParam
	if fwp == "" {
		fwp = "*"
		m.freeWildcardParam = "*"
	} else {
		path = path[:len(path)-len(fwp)-1]
	}

	// if ends with '/' then set one without
	// otherwise set one with '/'
	//
	// the subtree will be represented as "/foo/**" or "/foo/*wildcard"
	var pathAlt, pathSubtree string
	if path[len(path)-1] == '/' {
		pathAlt = path[:len(path)-1]
		pathSubtree = path + "*" + fwp
	} else {
		pathAlt = path + "/"
		pathSubtree = pathAlt + "*" + fwp
	}

	if err := pathTree.Add(path, m); err != nil {
		return err
	}

	if pathAlt != "" {
		if err := pathTree.Add(pathAlt, m); err != nil {
			return err
		}
	}

	return pathTree.Add(pathSubtree, m)
}

// add each path matcher to the path tree. If a matcher is a subtree, add it with the
// additional paths.
func addTreeMatchers(pathTree *pathmux.Tree, matchers map[string]*pathMatcher, subtree bool) []*definitionError {
	var errors []*definitionError
	for p, m := range matchers {

		// sort leaves during construction time, based on their priority
		sort.Stable(m.leaves)

		if p == "" {
			p = "/"
		}

		if subtree {
			if err := addSubtreeMatcher(pathTree, p, m); err != nil {
				errors = append(errors, &definitionError{Index: -1, Original: err})
			}
		} else {
			if err := pathTree.Add(p, m); err != nil {
				errors = append(errors, &definitionError{Index: -1, Original: err})
				continue
			}
		}
	}

	return errors
}

func addLeafToPath(pms map[string]*pathMatcher, path string, l *leafMatcher) {
	pm, ok := pms[path]
	if !ok {
		pm = &pathMatcher{freeWildcardParam: freeWildcardParam(path)}
		pms[path] = pm
	}

	pm.leaves = append(pm.leaves, l)
}

func moveToSubtreeIfExists(subtree *pathMatcher, paths map[string]*pathMatcher, path string) {
	pm, exists := paths[path]
	if !exists {
		return
	}

	subtree.leaves = append(subtree.leaves, pm.leaves...)
	delete(paths, path)
}

func moveConflictingToSubtree(subtrees, paths map[string]*pathMatcher) {
	for p, stm := range subtrees {
		moveToSubtreeIfExists(stm, paths, p)

		// TODO: this may mean that a non-subtree route will match even when ignore-trailing-match is
		// false:
		moveToSubtreeIfExists(stm, paths, p+"/")
	}

	var removeFromPaths []string
	for p, pm := range paths {
		fwm := freeWildcardRx.FindString(p)
		if fwm == "" {
			continue
		}

		subRoot := p[:len(p)-len(fwm)]
		if subRoot == "" {
			subRoot  = "/"
		}

		for stp, stm := range subtrees {
			var subtreePath string
			stpParam := freeWildcardRx.FindString(stp)
			if stpParam != "" {
				subtreePath = stp[:len(stp) - len(stpParam)]
			} else {
				subtreePath = stp
			}

			if subtreePath == subRoot {
				fromParam := stm.freeWildcardParam
				if fromParam == "" {
					fromParam = "*"
				}

				for _, l := range pm.leaves {
					l.subtreeMergeControl.noSubtreeRoot = true
					l.subtreeMergeControl.subtreeRoot = stp
					l.subtreeMergeControl.freeWildcardParamFrom = fromParam
					l.subtreeMergeControl.freeWildcardParamTo = pm.freeWildcardParam
				}

				/*
				for _, l := range stm.leaves {
					l.subtreeMergeControl.freeWildcardParamTo = stm.freeWildcardParam
				}
				*/

				pm.leaves = append(pm.leaves, stm.leaves...)
				pm.freeWildcardParam = stm.freeWildcardParam
				subtrees[stp] = pm

				removeFromPaths = append(removeFromPaths, p)
			}
		}
	}

	for _, p := range removeFromPaths {
		delete(paths, p)
	}
}

// constructs a matcher based on the provided definitions.
//
// If `ignoreTrailingSlash` is true, the matcher handles
// paths with or without a trailing slash equally.
//
// It constructs the route definition into a trie structure
// based on their path condition, if any, and puts the routes
// with the same path condition into a leaf matcher structure
// where they get evaluated after the leaf was matched based
// on the rest of the conditions so that most strict route
// definition matches first.
func newMatcher(rs []*Route, o MatchingOptions) (*matcher, []*definitionError) {
	var (
		errors     []*definitionError
		rootLeaves leafMatchers
	)

	pathMatchers := make(map[string]*pathMatcher)
	subtreeMatchers := make(map[string]*pathMatcher)
	compiledRxs := make(map[string]*regexp.Regexp)

	for i, r := range rs {
		l, err := newLeaf(r, compiledRxs)
		if err != nil {
			errors = append(errors, &definitionError{r.Id, i, err})
			continue
		}

		if r.pathSubtree != "" {
			path := cleanPath(r.pathSubtree, o|IgnoreTrailingSlash)
			addLeafToPath(subtreeMatchers, path, l)
			continue
		}

		if r.path == "" {
			rootLeaves = append(rootLeaves, l)
			continue
		}

		path := cleanPath(r.path, o)
		addLeafToPath(pathMatchers, path, l)
	}

	moveConflictingToSubtree(subtreeMatchers, pathMatchers)

	pathTree := &pathmux.Tree{}
	errors = append(errors, addTreeMatchers(pathTree, pathMatchers, false)...)
	errors = append(errors, addTreeMatchers(pathTree, subtreeMatchers, true)...)

	// sort root leaves during construction time, based on their priority
	sort.Stable(rootLeaves)

	return &matcher{pathTree, rootLeaves, o}, errors
}

// matches a path in the path trie structure.
func matchPathTree(tree *pathmux.Tree, path string, lrm *leafRequestMatcher) (map[string]string, *leafMatcher) {
	v, params, value := tree.LookupMatcher(path, lrm)
	if v == nil {
		return nil, nil
	}

	fmt.Println("params found", params)
	pm := v.(*pathMatcher)
	lm := value.(*leafMatcher)

	if lm.subtreeMergeControl.freeWildcardParamFrom != "" {
		params[lm.subtreeMergeControl.freeWildcardParamTo] =
			"/" + params[lm.subtreeMergeControl.freeWildcardParamFrom]
		delete(params, lm.subtreeMergeControl.freeWildcardParamFrom)
	} else if pm.freeWildcardParam != "" {
		params[pm.freeWildcardParam] = "/" + params[pm.freeWildcardParam]
		fmt.Println("params modded", params)
	}

	return params, lm
}

// matches the path regexp conditions in a leaf matcher.
func matchRegexps(rxs []*regexp.Regexp, s string) bool {
	for _, rx := range rxs {
		if !rx.MatchString(s) {
			return false
		}
	}

	return true
}

// matches a set of request headers to a fix and regexp header condition
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

// matches a set of request headers to the fix and regexp header conditions
func matchHeaders(exact map[string]string, hrxs map[string][]*regexp.Regexp, h http.Header) bool {
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

// check if all defined custom predicates are matched
func matchPredicates(cps []Predicate, req *http.Request) bool {
	for _, cp := range cps {
		if !cp.Match(req) {
			return false
		}
	}

	return true
}

// matches a request to the conditions in a leaf matcher
func matchLeaf(l *leafMatcher, req *http.Request, path string) bool {
	if l.exactPath != "" && l.exactPath != path {
		return false
	}

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

	if !matchPredicates(l.predicates, req) {
		return false
	}

	if l.subtreeMergeControl.noSubtreeRoot && path == l.subtreeMergeControl.subtreeRoot {
		return false
	}

	return true
}

// matches a request to a set of leaf matchers
func matchLeaves(leaves leafMatchers, req *http.Request, path string) *leafMatcher {
	for _, l := range leaves {
		if matchLeaf(l, req, path) {
			return l
		}
	}

	return nil
}

// tries to match a request against the available definitions. If a match is found,
// returns the associated value, and the wildcard parameters from the path definition,
// if any.
func (m *matcher) match(r *http.Request) (*Route, map[string]string) {
	// normalize path before matching
	// in case ignoring trailing slashes, match without the trailing slash
	path := cleanPath(r.URL.Path, m.matchingOptions)
	lrm := &leafRequestMatcher{r, path}

	// first match fixed and wildcard paths
	params, l := matchPathTree(m.paths, path, lrm)

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
