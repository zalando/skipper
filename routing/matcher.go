package routing

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/dimfeld/httppath"
	"github.com/zalando/skipper/pathmux"
)

type leafRequestMatcher struct {
	r         *http.Request
	path      string
	exactPath string
}

func (m *leafRequestMatcher) Match(value interface{}) (bool, interface{}) {
	v, ok := value.(*pathMatcher)
	if !ok {
		return false, nil
	}

	l := matchLeaves(v.leaves, m.r, m.path, m.exactPath)
	return l != nil, l
}

type leafMatcher struct {
	wildcardParamNames   []string // in reverse order
	hasFreeWildcardParam bool
	exactPath            string
	method               string
	weight               int
	hostRxs              []*regexp.Regexp
	pathRxs              []*regexp.Regexp
	headersExact         map[string]string
	headersRegexp        map[string][]*regexp.Regexp
	predicates           []Predicate
	route                *Route
}

type leafMatchers []*leafMatcher

func leafWeight(l *leafMatcher) int {
	w := l.weight

	if l.method != "" {
		w++
	}

	for _, rx := range l.hostRxs {
		if strings.HasPrefix(rx.String(), "[a-z0-9]+((-[a-z0-9]+)?)*") {
			// this is a free wildcard, skip it from the first matching
			w += 0
		} else {
			w += 1
		}
	}

	// w += len(l.hostRxs)
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
	leaves leafMatchers
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
var freeWildcardRx = regexp.MustCompile("/[*][^/]*$")

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

// extracts the expected wildcard param names and returns them in reverse order
func extractWildcardParamNames(r *Route) []string {
	path := r.path
	if path == "" {
		path = r.pathSubtree
	}
	path = httppath.Clean(path)

	var wildcards []string
	pathTokens := strings.Split(path, "/")
	for _, token := range pathTokens {
		if len(token) > 1 && (token[0] == ':' || token[0] == '*') {
			//prepend
			wildcards = append([]string{token[1:]}, wildcards...)
		}
	}

	if strings.HasSuffix(path, "/*") ||
		r.path == "" && r.pathSubtree != "" && !freeWildcardRx.MatchString(path) {

		wildcards = append([]string{"*"}, wildcards...)
	}

	return wildcards
}

func hasFreeWildcardParam(r *Route) bool {
	return r.pathSubtree != "" || freeWildcardRx.MatchString(httppath.Clean(r.path))
}

// returns a cleaned path where all wildcard names have been replaced with *
func normalizePath(r *Route) (string, error) {
	path := r.path
	if path == "" {
		path = r.pathSubtree
	}
	path = httppath.Clean(path)

	var sb strings.Builder
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c == '/' {
			sb.WriteByte(path[i])
		} else {
			nextSlash := strings.IndexByte(path[i:], '/')
			nextSlashExists := true
			if nextSlash == -1 {
				nextSlash = len(path)
				nextSlashExists = false
			} else {
				nextSlash += i
			}
			if c == ':' || c == '*' {
				if nextSlashExists && c == '*' {
					return "", errors.New("free wildcard param should be last")
				} else {
					sb.WriteByte(c)
				}
				sb.WriteByte('*')
			} else {
				sb.WriteString(path[i:nextSlash])
			}
			i = nextSlash - 1
		}
	}

	return sb.String(), nil
}

// creates a new leaf matcher. preprocesses the
// Host, PathRegexp, Header and HeaderRegexp
// conditions.
//
// Using a set of regular expressions shared in
// the current generation to preserve the
// compiled instances.
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
		wildcardParamNames:   extractWildcardParamNames(r),
		hasFreeWildcardParam: hasFreeWildcardParam(r),

		weight:        r.weight,
		method:        r.Method,
		hostRxs:       hostRxs,
		pathRxs:       pathRxs,
		headersExact:  canonicalizeHeaders(r.Headers),
		headersRegexp: canonicalizeHeaderRegexps(allHeaderRxs),
		predicates:    r.Predicates,
		route:         r}, nil
}

func trimTrailingSlash(path string) string {
	if len(path) > 1 && path[len(path)-1] == '/' {
		return path[:len(path)-1]
	}

	return path
}

// add each path matcher to the path tree. If a matcher is a subtree, add it with the
// additional paths.
func addTreeMatchers(pathTree *pathmux.Tree, matchers map[string]*pathMatcher) []*definitionError {
	var errors []*definitionError
	for p, m := range matchers {

		// sort leaves during construction time, based on their priority
		sort.Stable(m.leaves)

		if err := pathTree.Add(p, m); err != nil {
			errors = append(errors, &definitionError{Index: -1, Original: err})
		}
	}

	return errors
}

func addLeafToPath(pms map[string]*pathMatcher, path string, l *leafMatcher) {
	pm, ok := pms[path]
	if !ok {
		pm = &pathMatcher{}
		pms[path] = pm
	}

	pm.leaves = append(pm.leaves, l)
}

func addSubtreeLeafsToPath(pms map[string]*pathMatcher, path string, l *leafMatcher, o MatchingOptions) {
	basePath := freeWildcardRx.ReplaceAllLiteralString(path, "")
	basePath = strings.TrimSuffix(basePath, "/")
	if basePath == "" {
		addLeafToPath(pms, "/", l)
		addLeafToPath(pms, "/**", l)
	} else {
		addLeafToPath(pms, basePath, l)
		addLeafToPath(pms, basePath+"/**", l)
		if !o.ignoreTrailingSlash() {
			addLeafToPath(pms, basePath+"/", l)
		}
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
	compiledRxs := make(map[string]*regexp.Regexp)

	for i, r := range rs {
		l, err := newLeaf(r, compiledRxs)
		if err != nil {
			errors = append(errors, &definitionError{r.Id, i, err})
			continue
		}

		path, err := normalizePath(r)
		if err != nil {
			errors = append(errors, &definitionError{r.Id, i, err})
			continue
		}

		if r.pathSubtree != "" {
			addSubtreeLeafsToPath(pathMatchers, path, l, o)
			continue
		}

		if r.path == "" {
			rootLeaves = append(rootLeaves, l)
			continue
		}

		if o.ignoreTrailingSlash() {
			path = trimTrailingSlash(path)
		}
		addLeafToPath(pathMatchers, path, l)
	}

	pathTree := &pathmux.Tree{}
	errors = append(errors, addTreeMatchers(pathTree, pathMatchers)...)

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

	lm := value.(*leafMatcher)

	if len(lm.wildcardParamNames) == len(params)+1 {
		// prepend an empty string for the path subtree match
		params = append([]string{""}, params...)
	}

	l := len(params)
	if l > len(lm.wildcardParamNames) {
		l = len(lm.wildcardParamNames)
	}

	paramsMap := make(map[string]string)
	for i := 0; i < l; i += 1 {
		paramsMap[lm.wildcardParamNames[i]] = params[i]
	}
	if l > 0 && lm.hasFreeWildcardParam {
		paramsMap[lm.wildcardParamNames[0]] = "/" + params[0]
	}

	return paramsMap, lm
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
func matchLeaf(l *leafMatcher, req *http.Request, path, exactPath string) bool {
	if l.exactPath != "" && l.exactPath != path {
		return false
	}

	if l.method != "" && l.method != req.Method {
		return false
	}

	if !matchRegexps(l.hostRxs, req.Host) {
		return false
	}

	if !matchRegexps(l.pathRxs, exactPath) {
		return false
	}

	if !matchHeaders(l.headersExact, l.headersRegexp, req.Header) {
		return false
	}

	if !matchPredicates(l.predicates, req) {
		return false
	}

	return true
}

// matches a request to a set of leaf matchers
func matchLeaves(leaves leafMatchers, req *http.Request, path, exactPath string) *leafMatcher {
	for _, l := range leaves {
		if matchLeaf(l, req, path, exactPath) {
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
	path := httppath.Clean(r.URL.Path)
	exact := path
	if m.matchingOptions.ignoreTrailingSlash() {
		path = trimTrailingSlash(path)
	}
	lrm := &leafRequestMatcher{r: r, path: path, exactPath: exact}

	// first match fixed and wildcard paths
	params, l := matchPathTree(m.paths, path, lrm)

	if l != nil {
		return l.route, params
	}

	// if no path match, match root leaves for other conditions
	l = matchLeaves(m.rootLeaves, r, path, exact)
	if l != nil {
		return l.route, nil
	}

	return nil, nil
}
