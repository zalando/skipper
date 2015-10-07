package routing

import (
	"errors"
	"fmt"
	"github.com/zalando/pathmux"
	"github.com/zalando/skipper/eskip"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"testing"
)

const (
	benchmarkingCountPhase1 = 1
	benchmarkingCountPhase2 = 100
	benchmarkingCountPhase3 = 10000
	benchmarkingCountPhase4 = 1000000
)

// defs in eskip format
const testRouteDoc = `
    header: Path("/tessera/header") -> "https://header.my-department.example.org";
    footer: Path("/tessera/footer") -> "https://footer.my-department.example.org";
    pdp: PathRegexp(/.*\.html$/) -> "https://pdp.layout-service.my-department.example.org";
    pdpAsync: Path("/sls-async/*_") && PathRegexp(/.*\.html$/) -> "https://async.pdp.streaming-layout-service.my-department.example.org";
    pdpsc: Path("/sc/*_") && PathRegexp(/.*\.html$/) -> "https://pdpsc.compositor-layout-service.my-department.example.org";
    pdpsls: Path("/sls/*_") && PathRegexp(/.*\.html$/) -> "https://pdpsls.streaming-layout-service.my-department.example.org";
    catalog: Any() -> "https://catalog.layout-service.my-department.example.org";
    catalogAsync: Path("/sls-async/*_") -> "https://catalog-async.layout-service.my-department.example.org";
    catalogsc: Path("/sc/*_") -> "https://catalogsc.compositor-layout-service.my-department.example.org";
    catalogsls: Path("/sls/*_") -> "https://catalogsls.streaming-layout-service.my-department.example.org";
    slow: Path("/slow") -> "https://bugfactory.my-department.example.org";
    debug: Path("/debug") -> "https://debug.bugfactory.my-department.example.org";
    cart: Path("/api/cart/*_") -> "https://cart.my-department.example.org";
    login: Path("/login") && Method("POST") -> "https://login-fragment.my-department.example.org";
    logout: Path("/logout") && Method("POST") -> "https://logout.login-fragment.my-department.example.org";
    healthcheck: Path("/healthcheck") -> <shunt>;
    humanstxt: Path("/humans.txt") -> <shunt>;
    baseAssetsAssets: Path("/assets/base-assets/*_") -> "https://base-assets.my-department.example.org";
    headerAssets: Path("/assets/header/*_") -> "https://assets.header.my-department.example.org";
    footerAssets: Path("/assets/footer/*_") -> "https://assets.footer.my-department.example.org";
    cartAssets: Path("/assets/cart/*_") -> "https://assets.cart.my-department.example.org";
    pdpAssets: Path("/assets/pdp/*_") -> "https://assets.pdp-fragment-alt.my-department.example.org";
    catalogAssets: Path("/assets/catalog/*_") -> "https://assets.catalog-face.my-department.example.org";
    loginAssets: Path("/assets/login/*_") -> "https://assets.login-fragment.my-department.example.org";

    catalogHerren: Path("/herren/*_") -> "https://herren.layout-service.my-department.example.org";
    catalogDamen: Path("/damen/*_") -> "https://damen.layout-service.my-department.example.org";
    catalogAsyncHerren: Path("/sls-async/herren/*_") -> "https://herren-async.streaming-layout-service.my-department.example.org";
    catalogAsyncDamen: Path("/sls-async/damen/*_") -> "https://damen-async.streaming-layout-service.my-department.example.org";
    catalogscHerren: Path("/sc/herren/*_") -> "https://herren-sc.compositor-layout-service.my-department.example.org";
    catalogscDamen: Path("/sc/damen/*_") -> "https://damen-sc.compositor-layout-service.my-department.example.org";
    catalogslsHerren: Path("/sls/herren/*_") -> "https://herren-sls.streaming-layout-service.my-department.example.org";
    catalogslsDamen: Path("/sls/damen/*_") -> "https://damen-sls.streaming-layout-service.my-department.example.org";

    catalogHerrenEn: Path("/men/*_") -> "https://herren-en.layout-service.my-department.example.org";
    catalogDamenEn: Path("/women/*_") -> "https://damen-en.layout-service.my-department.example.org";
    catalogAsyncHerrenEn: Path("/sls-async/men/*_") -> "https://herren-en.streaming-layout-service.my-department.example.org";
    catalogAsyncDamenEn: Path("/sls-async/women/*_") -> "https://damen-en.streaming-layout-service.my-department.example.org";
    catalogscHerrenEn: Path("/sc/men/*_") -> "https://herren-en.compositor-layout-service.my-department.example.org";
    catalogscDamenEn: Path("/sc/women/*_") -> "https://damen-en.compositor-layout-service.my-department.example.org";
    catalogslsHerrenEn: Path("/sls/men/*_") -> "https://herren-en.streaming-layout-service.my-department.example.org";
    catalogslsDamenEn: Path("/sls/women/*_") -> "https://damen-en.streaming-layout-service.my-department.example.org";
`

var (
	randomRoutes   []*Route
	randomRequests []*http.Request

	testMatcher1 *matcher
	testMatcher2 *matcher
	testMatcher3 *matcher
	testMatcher4 *matcher

	testMatcherGeneric *matcher
)

func docToRoutes(doc string) ([]*Route, error) {
	defs, err := eskip.Parse(doc)
	if err != nil {
		return nil, err
	}

	return processRouteDefs(nil, defs), nil
}

func docToRoute(doc string) (*Route, error) {
	routes, err := docToRoutes(doc)
	if err != nil {
		return nil, err
	}

	if len(routes) != 1 {
		return nil, errors.New("invalid number of routes")
	}

	return routes[0], nil
}

func newTestMatcherOpts(routes []*Route, o MatchingOptions) (*matcher, error) {
	if len(routes) == 0 {
		return nil, errors.New("we need at least one route for this test")
	}

	matcher, errs := newMatcher(routes, o)
	if len(errs) != 0 {
		for _, err := range errs {
			log.Println(err.Error())
		}

		return nil, errors.New("failed to create matcher")
	}

	return matcher, nil
}

func newTestMatcher(routes []*Route) (*matcher, error) {
	return newTestMatcherOpts(routes, MatchingOptionsNone)
}

func docToMatcherOpts(doc string, o MatchingOptions) (*matcher, error) {
	rs, err := docToRoutes(doc)
	if err != nil {
		return nil, err
	}

	return newTestMatcherOpts(rs, o)
}

func docToMatcher(doc string) (*matcher, error) {
	return docToMatcherOpts(doc, MatchingOptionsNone)
}

func initGenericMatcher() {
	m, err := docToMatcher(testRouteDoc)
	if err != nil {
		panic(err)
	}

	testMatcherGeneric = m
}

func generatePaths(pg *pathGenerator, count int) []string {
	paths := make([]string, count)

	for i := 0; i < count; i++ {
		paths[i] = pg.Next()
	}

	return paths
}

func generateRoutes(paths []string) []*Route {
	defs := make([]*eskip.Route, len(paths))
	for i, p := range paths {

		// the path for the backend is fine here,
		// because it is only used for checking the
		// found routes
		defs[i] = &eskip.Route{Id: fmt.Sprintf("route%d", i), Path: p, Backend: p}
	}

	return processRouteDefs(nil, defs)
}

func generateRequests(paths []string) ([]*http.Request, error) {
	requests := make([]*http.Request, len(paths))
	for i := 0; i < len(paths); i++ {
		url, err := url.Parse(fmt.Sprintf("https://example.org%s", paths[i]))
		if err != nil {
			return nil, err
		}

		requests[i] = &http.Request{Method: "GET", URL: url}
	}

	return requests, nil
}

func initRandomPaths() {
	const count = benchmarkingCountPhase4

	// we need to avoid '/' paths here, because we are not testing conflicting cases
	// here, and with 0 or 1 MinNamesInPath, there would be multiple '/'s.
	pg := newPathGenerator(pathGeneratorOptions{
		MinNamesInPath: 2,
		MaxNamesInPath: 15})

	var err error

	randomPaths := generatePaths(pg, count)
	randomRoutes = generateRoutes(randomPaths)

	randomRequests, err = generateRequests(randomPaths)
	if err != nil {
		panic(err)
	}

	unregisteredPaths := generatePaths(pg, count)
	unregisteredRequests, err := generateRequests(unregisteredPaths)
	if err != nil {
		panic(err)
	}

	// the upper half of the requests should not be found
	randomRequests = append(randomRequests, unregisteredRequests...)

	mkmatcher := func(paths []string, routes []*Route) *matcher {
		if err != nil {
			return nil
		}

		r, e := newTestMatcher(routes)
		err = e
		return r
	}

	defer func() {
		if err != nil {
			panic(err)
		}
	}()

	testMatcher1 = mkmatcher(randomPaths[0:benchmarkingCountPhase1], randomRoutes[0:benchmarkingCountPhase1])
	testMatcher2 = mkmatcher(randomPaths[0:benchmarkingCountPhase2], randomRoutes[0:benchmarkingCountPhase2])
	testMatcher3 = mkmatcher(randomPaths[0:benchmarkingCountPhase3], randomRoutes[0:benchmarkingCountPhase3])
	testMatcher4 = mkmatcher(randomPaths[0:benchmarkingCountPhase4], randomRoutes[0:benchmarkingCountPhase4])
}

func init() {
	initGenericMatcher()
	initRandomPaths()
}

func newRequest(method, path string) (*http.Request, error) {
	u := fmt.Sprintf("https://example.com%s", path)
	r := &http.Request{}

	up, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	r.URL = up
	r.Method = method
	r.Header = make(http.Header)
	return r, nil
}

func checkMatch(t testing.TB, r *Route, err error, host string) {
	if err != nil {
		t.Error(err)
		return
	}

	if r.Backend != host {
		t.Error("failed to match the right value", r.Backend, host)
	}
}

func testMatch(t testing.TB, method, path, host string) {
	req, err := newRequest(method, path)
	if err != nil {
		t.Error(err)
	}

	r, _ := testMatcherGeneric.match(req)
	checkMatch(t, r, err, host)
}

func benchmarkLookup(b *testing.B, matcher *matcher, phaseCount int) {
	// see init, double as much requests as routes, to benchmark the cases
	// when no route is found
	requestCount := phaseCount * 2

	var index int
	for i := 0; i < b.N; i++ {

		// b.N comes from the test vault, doesn't matter if it matches the available
		// number of requests or routes, because in case of longer runs, b.N will be far bigger
		index = i % requestCount

		r, _ := matcher.match(randomRequests[index])

		if (index < phaseCount && r.Backend != randomRoutes[index].Backend) ||
			(index >= phaseCount && r != nil) {
			b.Log("benchmark failed", r == nil, fmt.Sprintf("(%s != %s)", r.Backend, randomRoutes[index].Backend),
				index, i, b.N, randomRequests[index].URL.Path)
			b.FailNow()
		}
	}
}

func TestGeneric(t *testing.T) {
	testMatch(t, "GET", "/tessera/header", "https://header.my-department.example.org")
	testMatch(t, "GET", "/tessera/footer", "https://footer.my-department.example.org")
	testMatch(t, "GET", "/some.html", "https://pdp.layout-service.my-department.example.org")
	testMatch(t, "GET", "/path/to/some.html", "https://pdp.layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls-async/some.html", "https://async.pdp.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sc/some.html", "https://pdpsc.compositor-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls/some.html", "https://pdpsls.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "", "https://catalog.layout-service.my-department.example.org")
	testMatch(t, "GET", "/", "https://catalog.layout-service.my-department.example.org")
	testMatch(t, "GET", "/nike", "https://catalog.layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls-async/nike", "https://catalog-async.layout-service.my-department.example.org")
	testMatch(t, "GET", "/sc/nike", "https://catalogsc.compositor-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls/nike", "https://catalogsls.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/nike/sports", "https://catalog.layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls-async/nike/sports", "https://catalog-async.layout-service.my-department.example.org")
	testMatch(t, "GET", "/sc/nike/sports", "https://catalogsc.compositor-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls/nike/sports", "https://catalogsls.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/slow", "https://bugfactory.my-department.example.org")
	testMatch(t, "GET", "/debug", "https://debug.bugfactory.my-department.example.org")
	testMatch(t, "GET", "/api/cart/42", "https://cart.my-department.example.org")
	testMatch(t, "GET", "/api/cart/42/all", "https://cart.my-department.example.org")
	testMatch(t, "POST", "/login", "https://login-fragment.my-department.example.org")
	testMatch(t, "POST", "/logout", "https://logout.login-fragment.my-department.example.org")
	testMatch(t, "GET", "/healthcheck", "")
	testMatch(t, "GET", "/humans.txt", "")
	testMatch(t, "GET", "/assets/base-assets/some.css", "https://base-assets.my-department.example.org")
	testMatch(t, "GET", "/assets/header/some.css", "https://assets.header.my-department.example.org")
	testMatch(t, "GET", "/assets/footer/some.css", "https://assets.footer.my-department.example.org")
	testMatch(t, "GET", "/assets/cart/some.css", "https://assets.cart.my-department.example.org")
	testMatch(t, "GET", "/assets/pdp/some.css", "https://assets.pdp-fragment-alt.my-department.example.org")
	testMatch(t, "GET", "/assets/catalog/some.css", "https://assets.catalog-face.my-department.example.org")
	testMatch(t, "GET", "/assets/login/some.css", "https://assets.login-fragment.my-department.example.org")
	testMatch(t, "GET", "/assets/base-assets/dir/some.css", "https://base-assets.my-department.example.org")
	testMatch(t, "GET", "/assets/header/dir/some.css", "https://assets.header.my-department.example.org")
	testMatch(t, "GET", "/assets/footer/dir/some.css", "https://assets.footer.my-department.example.org")
	testMatch(t, "GET", "/assets/cart/dir/some.css", "https://assets.cart.my-department.example.org")
	testMatch(t, "GET", "/assets/pdp/dir/some.css", "https://assets.pdp-fragment-alt.my-department.example.org")
	testMatch(t, "GET", "/assets/catalog/dir/some.css", "https://assets.catalog-face.my-department.example.org")
	testMatch(t, "GET", "/assets/login/dir/some.css", "https://assets.login-fragment.my-department.example.org")
	testMatch(t, "GET", "/herren/nike", "https://herren.layout-service.my-department.example.org")
	testMatch(t, "GET", "/damen/nike", "https://damen.layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls-async/herren/nike", "https://herren-async.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls-async/damen/nike", "https://damen-async.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sc/herren/nike", "https://herren-sc.compositor-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sc/damen/nike", "https://damen-sc.compositor-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls/herren/nike", "https://herren-sls.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls/damen/nike", "https://damen-sls.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/men/nike", "https://herren-en.layout-service.my-department.example.org")
	testMatch(t, "GET", "/women/nike", "https://damen-en.layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls-async/men/nike", "https://herren-en.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls-async/women/nike", "https://damen-en.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sc/men/nike", "https://herren-en.compositor-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sc/women/nike", "https://damen-en.compositor-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls/men/nike", "https://herren-en.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls/women/nike", "https://damen-en.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/herren/nike/sports", "https://herren.layout-service.my-department.example.org")
	testMatch(t, "GET", "/damen/nike/sports", "https://damen.layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls-async/herren/nike/sports", "https://herren-async.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls-async/damen/nike/sports", "https://damen-async.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sc/herren/nike/sports", "https://herren-sc.compositor-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sc/damen/nike/sports", "https://damen-sc.compositor-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls/herren/nike/sports", "https://herren-sls.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls/damen/nike/sports", "https://damen-sls.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/men/nike/sports", "https://herren-en.layout-service.my-department.example.org")
	testMatch(t, "GET", "/women/nike/sports", "https://damen-en.layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls-async/men/nike/sports", "https://herren-en.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls-async/women/nike/sports", "https://damen-en.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sc/men/nike/sports", "https://herren-en.compositor-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sc/women/nike/sports", "https://damen-en.compositor-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls/men/nike/sports", "https://herren-en.streaming-layout-service.my-department.example.org")
	testMatch(t, "GET", "/sls/women/nike/sports", "https://damen-en.streaming-layout-service.my-department.example.org")
}

func TestMatchRegexpsNone(t *testing.T) {
	if !matchRegexps(nil, "/some/path") {
		t.Error("failed to match nil regexps")
	}
}

func TestMatchRegexpsFalse(t *testing.T) {
	rx0 := regexp.MustCompile("/some")
	rx1 := regexp.MustCompile("/path")
	rx2 := regexp.MustCompile("/something-else")
	if matchRegexps([]*regexp.Regexp{rx0, rx1, rx2}, "/some/path") {
		t.Error("failed not match wrong regexp")
	}
}

func TestMatchRegexpsTrue(t *testing.T) {
	rx0 := regexp.MustCompile("/some")
	rx1 := regexp.MustCompile("/path")
	if !matchRegexps([]*regexp.Regexp{rx0, rx1}, "/some/path") {
		t.Error("failed not match wrong regexp")
	}
}

func TestFindHeaderFalse(t *testing.T) {
	h := make(http.Header)
	h["Some-Header"] = []string{"some-value"}
	h["Some-Other-Header"] = []string{"some-other-value-0", "some-other-value-1"}
	if matchHeader(h, "Some-Header", func(v string) bool { return v == "some-wrong-value" }) {
		t.Error("failed not to find header")
	}
}

func TestFindHeaderTrue(t *testing.T) {
	h := make(http.Header)
	h["Some-Header"] = []string{"some-value"}
	h["Some-Other-Header"] = []string{"some-other-value-0", "some-other-value-1"}
	if !matchHeader(h, "Some-Header", func(v string) bool { return v == "some-value" }) {
		t.Error("failed not to find header")
	}
}

func TestMatchHeadersExactFalse(t *testing.T) {
	h := make(http.Header)
	h["Some-Header"] = []string{"some-value"}
	h["Some-Other-Header"] = []string{"some-other-value-0", "some-other-value-1"}
	if matchHeaders(map[string]string{"Some-Header": "some-wrong-value"}, nil, h) {
		t.Error("failed not to match header")
	}
}

func TestMatchHeadersExactTrue(t *testing.T) {
	h := make(http.Header)
	h["Some-Header"] = []string{"some-value"}
	h["Some-Other-Header"] = []string{"some-other-value-0", "some-other-value-1"}
	if !matchHeaders(map[string]string{"Some-Header": "some-value"}, nil, h) {
		t.Error("failed not to match header")
	}
}

func TestMatchHeadersRegexpFalse(t *testing.T) {
	rx := regexp.MustCompile("something-else")
	h := make(http.Header)
	h["Some-Header"] = []string{"some-value"}
	h["Some-Other-Header"] = []string{"some-other-value-0", "some-other-value-1"}
	if matchHeaders(nil, map[string][]*regexp.Regexp{"Some-Header": []*regexp.Regexp{rx}}, h) {
		t.Error("failed not to match header")
	}
}

func TestMatchHeadersRegexpTrue(t *testing.T) {
	rx := regexp.MustCompile("some")
	h := make(http.Header)
	h["Some-Header"] = []string{"some-value"}
	h["Some-Other-Header"] = []string{"some-other-value-0", "some-other-value-1"}
	if !matchHeaders(nil, map[string][]*regexp.Regexp{"Some-Header": []*regexp.Regexp{rx}}, h) {
		t.Error("failed not to match header")
	}
}

func TestMatchLeafWrongMethod(t *testing.T) {
	rxh := regexp.MustCompile("example\\.org")
	rxp := regexp.MustCompile("/some/path")
	rxhd := regexp.MustCompile("some-other-value")
	req := &http.Request{
		Method: "GET",
		Host:   "example.org",
		Header: http.Header{
			"Some-Header":       []string{"some-value"},
			"Some-Other-Header": []string{"some-other-value"}}}
	l := &leafMatcher{
		method:        "PUT",
		hostRxs:       []*regexp.Regexp{rxh},
		pathRxs:       []*regexp.Regexp{rxp},
		headersExact:  map[string]string{"Some-Header": "some-value"},
		headersRegexp: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
	if matchLeaf(l, req, "/some/path") {
		t.Error("failed not to match leaf method")
	}
}

func TestMatchLeafWrongHost(t *testing.T) {
	rxh := regexp.MustCompile("example\\.org")
	rxp := regexp.MustCompile("/some/path")
	rxhd := regexp.MustCompile("some-other-value")
	req := &http.Request{
		Method: "PUT",
		Host:   "example.com",
		Header: http.Header{
			"Some-Header":       []string{"some-value"},
			"Some-Other-Header": []string{"some-other-value"}}}
	l := &leafMatcher{
		method:        "PUT",
		hostRxs:       []*regexp.Regexp{rxh},
		pathRxs:       []*regexp.Regexp{rxp},
		headersExact:  map[string]string{"Some-Header": "some-value"},
		headersRegexp: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
	if matchLeaf(l, req, "/some/path") {
		t.Error("failed not to match leaf host")
	}
}

func TestMatchLeafWrongPath(t *testing.T) {
	rxh := regexp.MustCompile("example\\.org")
	rxp := regexp.MustCompile("/some/path")
	rxhd := regexp.MustCompile("some-other-value")
	req := &http.Request{
		Method: "PUT",
		Host:   "example.org",
		Header: http.Header{
			"Some-Header":       []string{"some-value"},
			"Some-Other-Header": []string{"some-other-value"}}}
	l := &leafMatcher{
		method:        "PUT",
		hostRxs:       []*regexp.Regexp{rxh},
		pathRxs:       []*regexp.Regexp{rxp},
		headersExact:  map[string]string{"Some-Header": "some-value"},
		headersRegexp: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
	if matchLeaf(l, req, "/some/other/path") {
		t.Error("failed not to match leaf path")
	}
}

func TestMatchLeafWrongExactHeader(t *testing.T) {
	rxh := regexp.MustCompile("example\\.org")
	rxp := regexp.MustCompile("/some/path")
	rxhd := regexp.MustCompile("some-other-value")
	req := &http.Request{
		Method: "PUT",
		Host:   "example.org",
		Header: http.Header{
			"Some-Header":       []string{"some-wrong-value"},
			"Some-Other-Header": []string{"some-other-value"}}}
	l := &leafMatcher{
		method:        "PUT",
		hostRxs:       []*regexp.Regexp{rxh},
		pathRxs:       []*regexp.Regexp{rxp},
		headersExact:  map[string]string{"Some-Header": "some-value"},
		headersRegexp: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
	if matchLeaf(l, req, "/some/path") {
		t.Error("failed not to match leaf exact header")
	}
}

func TestMatchLeafWrongRegexpHeader(t *testing.T) {
	rxh := regexp.MustCompile("example\\.org")
	rxp := regexp.MustCompile("/some/path")
	rxhd := regexp.MustCompile("some-other-value")
	req := &http.Request{
		Method: "PUT",
		Host:   "example.org",
		Header: http.Header{
			"Some-Header":       []string{"some-value"},
			"Some-Other-Header": []string{"some-other-wrong-value"}}}
	l := &leafMatcher{
		method:        "PUT",
		hostRxs:       []*regexp.Regexp{rxh},
		pathRxs:       []*regexp.Regexp{rxp},
		headersExact:  map[string]string{"Some-Header": "some-value"},
		headersRegexp: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
	if matchLeaf(l, req, "/some/path") {
		t.Error("failed not to match leaf regexp header")
	}
}

func TestMatchLeaf(t *testing.T) {
	rxh := regexp.MustCompile("example\\.org")
	rxp := regexp.MustCompile("/some/path")
	rxhd := regexp.MustCompile("some-other-value")
	req := &http.Request{
		Method: "PUT",
		Host:   "example.org",
		Header: http.Header{
			"Some-Header":       []string{"some-value"},
			"Some-Other-Header": []string{"some-other-value"}}}
	l := &leafMatcher{
		method:        "PUT",
		hostRxs:       []*regexp.Regexp{rxh},
		pathRxs:       []*regexp.Regexp{rxp},
		headersExact:  map[string]string{"Some-Header": "some-value"},
		headersRegexp: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
	if !matchLeaf(l, req, "/some/path") {
		t.Error("failed to match leaf")
	}
}

func TestMatchLeavesFalse(t *testing.T) {
	l0 := &leafMatcher{method: "PUT"}
	l1 := &leafMatcher{method: "POST"}
	req := &http.Request{Method: "GET"}
	if matchLeaves([]*leafMatcher{l0, l1}, req, "/some/path") != nil {
		t.Error("failed not to match leaves")
	}
}

func TestMatchLeavesTrue(t *testing.T) {
	l0 := &leafMatcher{method: "PUT"}
	l1 := &leafMatcher{method: "POST"}
	req := &http.Request{Method: "PUT"}
	if matchLeaves([]*leafMatcher{l0, l1}, req, "/some/path") != l0 {
		t.Error("failed not to match leaves")
	}
}

func TestMatchPathTreeNoMatch(t *testing.T) {
	tree := &pathmux.Tree{}
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{}}}
	pm1 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{}}}
	err := tree.Add("/some/path", pm0)
	if err != nil {
		t.Error(err)
	}
	err = tree.Add("/some/other/path", pm1)
	if err != nil {
		t.Error(err)
	}
	m, p := matchPathTree(tree, "/some/wrong/path")
	if len(m) != 0 || len(p) != 0 {
		t.Error("failed not to match path")
	}
}

func TestMatchPathTree(t *testing.T) {
	tree := &pathmux.Tree{}
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{}}}
	pm1 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{}}}
	err := tree.Add("/some/path", pm0)
	if err != nil {
		t.Error(err)
	}
	err = tree.Add("/some/other/path", pm1)
	if err != nil {
		t.Error(err)
	}
	m, p := matchPathTree(tree, "/some/path")
	if len(m) != 1 || len(p) != 0 || m[0] != pm0.leaves[0] {
		t.Error("failed to match path", len(m), len(p))
	}
}

func TestMatchPathTreeWithWildcards(t *testing.T) {
	tree := &pathmux.Tree{}
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{}}}
	pm1 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{}}}
	err := tree.Add("/some/path/:param0/:param1", pm0)
	if err != nil {
		t.Error(err)
	}
	err = tree.Add("/some/other/path/*_", pm1)
	if err != nil {
		t.Error(err)
	}
	m, p := matchPathTree(tree, "/some/path/and/params")
	if len(m) != 1 || len(p) != 2 || m[0] != pm0.leaves[0] ||
		p["param0"] != "and" || p["param1"] != "params" {
		t.Error("failed to match path", len(m), len(p))
	}
}

func TestMatchPath(t *testing.T) {
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{}}}
	tree := &pathmux.Tree{}
	err := tree.Add("/some/path", pm0)
	if err != nil {
		t.Error(err)
	}
	m := &matcher{paths: tree}
	req := &http.Request{URL: &url.URL{Path: "/some/path"}}
	r, p := m.match(req)
	if r != pm0.leaves[0].route || len(p) != 0 {
		t.Error("failed to match path", r == nil, len(p))
	}
}

func TestMatchPathResolved(t *testing.T) {
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{}}}
	tree := &pathmux.Tree{}
	err := tree.Add("/some/path", pm0)
	if err != nil {
		t.Error(err)
	}
	m := &matcher{paths: tree}
	req := &http.Request{URL: &url.URL{Path: "/some/some-other/../path"}}
	r, p := m.match(req)
	if r != pm0.leaves[0].route || len(p) != 0 {
		t.Error("failed to match path", r == nil, len(p))
	}
}

func TestMatchWrongMethod(t *testing.T) {
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{method: "PUT"}}}
	tree := &pathmux.Tree{}
	err := tree.Add("/some/path/*_", pm0)
	if err != nil {
		t.Error(err)
	}
	m := &matcher{paths: tree}
	req := &http.Request{Method: "GET", URL: &url.URL{Path: "/some/some-other/../path"}}
	r, p := m.match(req)
	if r != nil || len(p) != 0 {
		t.Error("failed to match path", r == nil, len(p))
	}
}

func TestMatchTopLeaves(t *testing.T) {
	tree := &pathmux.Tree{}
	l := &leafMatcher{method: "PUT"}
	pm := &pathMatcher{leaves: leafMatchers{l}}
	err := tree.Add("/*", pm)
	if err != nil {
		t.Error(err)
	}
	m := &matcher{paths: tree}
	req := &http.Request{Method: "PUT", URL: &url.URL{Path: "/some/some-other/../path"}}
	r, _ := m.match(req)
	if r != l.route {
		t.Error("failed to match path", r == nil)
	}
}

func TestMatchWildcardPaths(t *testing.T) {
	tree := &pathmux.Tree{}
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{}}}
	pm1 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{}}}
	err := tree.Add("/some/path/:param0/:param1", pm0)
	if err != nil {
		t.Error(err)
	}
	err = tree.Add("/some/other/path/*_", pm1)
	if err != nil {
		t.Error(err)
	}
	rm := &matcher{paths: tree}
	req := &http.Request{URL: &url.URL{Path: "/some/path/and/params"}}
	r, p := rm.match(req)
	if r != pm0.leaves[0].route || len(p) != 2 ||
		p["param0"] != "and" || p["param1"] != "params" {
		t.Error("failed to match path")
	}
}

func TestCompileRegexpsError(t *testing.T) {
	_, err := compileRxs([]string{"**"})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestCompileRegexps(t *testing.T) {
	rxs, err := compileRxs([]string{"some", "expressions"})
	if err != nil || len(rxs) != 2 {
		t.Error("failed to compile regexps", err, len(rxs))
	}
}

func TestMakeLeafInvalidHostRx(t *testing.T) {
	const routeExp = "Host(\"**\") -> \"https://example.org\""
	r, err := docToRoute(routeExp)
	if err != nil {
		t.Error(err)
	}

	_, err = newLeaf(r)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestMakeLeafInvalidPathRx(t *testing.T) {
	const routeExp = "PathRegexp(\"**\") -> \"https://example.org\""
	r, err := docToRoute(routeExp)
	if err != nil {
		t.Error(err)
	}

	_, err = newLeaf(r)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestMakeLeafInvalidHeaderRegexp(t *testing.T) {
	const routeExp = "HeaderRegexp(\"Some-Header\", \"**\") -> \"https://example.org\""
	r, err := docToRoute(routeExp)
	if err != nil {
		t.Error(err)
	}

	_, err = newLeaf(r)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestMakeLeaf(t *testing.T) {
	const routeExp = `testRoute:
        Method("PUT") &&
        Host("some-host") &&
        PathRegexp("some-path") &&
        Header("Some-Header", "some-value") &&
        HeaderRegexp("Some-Header", "some-value") ->
        "https://example.org"`
	r, err := docToRoute(routeExp)
	if err != nil {
		t.Error(err)
	}

	l, err := newLeaf(r)
	if err != nil || l.method != "PUT" ||
		len(l.hostRxs) != 1 || len(l.pathRxs) != 1 ||
		len(l.headersExact) != 1 || len(l.headersRegexp) != 1 ||
		l.route.Backend != "https://example.org" {
		t.Error("failed to create leaf")
	}
}

func TestMakeMatcherEmpty(t *testing.T) {
	m, errs := newMatcher(nil, MatchingOptionsNone)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make empty matcher")
	}

	r, params := m.match(&http.Request{Method: "PUT", URL: &url.URL{Path: "/some/path"}})
	if len(params) != 0 || r != nil {
		t.Error("failed not to match request")
	}
}

func TestMakeMatcherRootLeavesOnly(t *testing.T) {
	rs, err := docToRoutes(`Method("PUT") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := newMatcher(rs, MatchingOptionsNone)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make matcher")
	}

	rback, _ := m.match(&http.Request{Method: "PUT", URL: &url.URL{Path: "/some/path"}})
	if rback == nil || rback.Backend != "https://example.org" {
		t.Error("failed to match request")
	}
}

func TestMakeMatcherExactPathOnly(t *testing.T) {
	rs, err := docToRoutes(`Path("/some/path") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := newMatcher(rs, MatchingOptionsNone)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make matcher")
	}

	r, params := m.match(&http.Request{Method: "PUT", URL: &url.URL{Path: "/some/path"}})
	if len(params) != 0 || r == nil || r.Backend != "https://example.org" {
		t.Error("failed to match request")
	}
}

func TestMakeMatcherWithWildcardPath(t *testing.T) {
	rs, err := docToRoutes(`Path("/some/:param") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := newMatcher(rs, MatchingOptionsNone)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make matcher")
	}

	r, params := m.match(&http.Request{Method: "PUT", URL: &url.URL{Path: "/some/value"}})
	if len(params) != 1 || r == nil || r.Backend != "https://example.org" || params["param"] != "value" {
		t.Error("failed to match request")
	}
}

func TestMakeMatcherErrorInLeaf(t *testing.T) {
	rs, err := docToRoutes(`testRoute: PathRegexp("**") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := newMatcher(rs, MatchingOptionsNone)
	if len(errs) != 1 || m == nil || errs[0].Index != 0 {
		t.Error("failed to make matcher with error")
	}
}

func TestMakeMatcherWithPathConflict(t *testing.T) {
	rs, err := docToRoutes(`
        testRoute0: Path("/some/path/:param0/name") -> "https://example.org";
        testRoute1: Path("/some/path/:param1/name") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := newMatcher(rs, MatchingOptionsNone)
	if len(errs) != 1 || m == nil {
		t.Error("failed to make matcher with error", len(errs), m == nil)
	}
}

func TestMatchToSlash(t *testing.T) {
	m, err := docToMatcherOpts(`Path("/some/path/") -> "https://example.org"`, IgnoreTrailingSlash)
	if err != nil {
		t.Error(err)
	}

	r, _ := m.match(&http.Request{URL: &url.URL{Path: "/some/path"}})
	if r == nil {
		t.Error("failed to match to slash")
	}
}

func TestMatchFromSlash(t *testing.T) {
	m, err := docToMatcherOpts(`Path("/some/path") -> "https://example.org"`, IgnoreTrailingSlash)
	if err != nil {
		t.Error(err)
	}

	r, _ := m.match(&http.Request{URL: &url.URL{Path: "/some/path/"}})
	if r == nil {
		t.Error("failed to match to slash")
	}
}

func TestWildcardParam(t *testing.T) {
	m, err := docToMatcher(`Path("/some/:wildcard0/path/:wildcard1") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	r, params := m.match(&http.Request{URL: &url.URL{Path: "/some/value0/path/value1"}})
	if r == nil || len(params) != 2 || params["wildcard0"] != "value0" || params["wildcard1"] != "value1" {
		t.Error("failed to match with wildcards")
	}
}

func TestWildcardParamFromSlash(t *testing.T) {
	m, err := docToMatcherOpts(`Path("/some/:wildcard0/path/:wildcard1") -> "https://example.org"`, IgnoreTrailingSlash)
	if err != nil {
		t.Error(err)
	}

	r, params := m.match(&http.Request{URL: &url.URL{Path: "/some/value0/path/value1/"}})
	if r == nil || len(params) != 2 || params["wildcard0"] != "value0" || params["wildcard1"] != "value1" {
		t.Error("failed to match with wildcards")
	}
}

func TestWildcardParamToSlash(t *testing.T) {
	m, err := docToMatcherOpts(`Path("/some/:wildcard0/path/:wildcard1/") -> "https://example.org"`, IgnoreTrailingSlash)
	if err != nil {
		t.Error(err)
	}

	r, params := m.match(&http.Request{URL: &url.URL{Path: "/some/value0/path/value1"}})
	if r == nil || len(params) != 2 || params["wildcard0"] != "value0" || params["wildcard1"] != "value1" {
		t.Error("failed to match with wildcards")
	}
}

func TestFreeWildcardParam(t *testing.T) {
	m, err := docToMatcher(`Path("/some/*wildcard") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	r, params := m.match(&http.Request{URL: &url.URL{Path: "/some/value0/value1"}})
	if r == nil || len(params) != 1 || params["wildcard"] != "/value0/value1" {
		t.Error("failed to match with wildcards", params["wildcard"])
	}
}

func TestFreeWildcardParamWithSlash(t *testing.T) {
	m, err := docToMatcherOpts(`Path("/some/*wildcard") -> "https://example.org"`, IgnoreTrailingSlash)
	if err != nil {
		t.Error(err)
	}

	r, params := m.match(&http.Request{URL: &url.URL{Path: "/some/value0/value1/"}})
	if r == nil || len(params) != 1 || params["wildcard"] != "/value0/value1" {
		t.Error("failed to match with wildcards", r == nil, len(params), params["wildcard"])
	}
}

func BenchmarkGeneric(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMatch(b, "GET", "/tessera/header", "https://header.my-department.example.org")
		testMatch(b, "GET", "/tessera/footer", "https://footer.my-department.example.org")
		testMatch(b, "GET", "/some.html", "https://pdp.layout-service.my-department.example.org")
		testMatch(b, "GET", "/nike", "https://catalog.layout-service.my-department.example.org")
		testMatch(b, "GET", "/sls-async/nike", "https://catalog-async.layout-service.my-department.example.org")
		testMatch(b, "GET", "/sc/nike", "https://catalogsc.compositor-layout-service.my-department.example.org")
		testMatch(b, "GET", "/sls/nike", "https://catalogsls.streaming-layout-service.my-department.example.org")
		testMatch(b, "GET", "/slow", "https://bugfactory.my-department.example.org")
		testMatch(b, "GET", "/debug", "https://debug.bugfactory.my-department.example.org")
		testMatch(b, "GET", "/api/cart/42", "https://cart.my-department.example.org")
		testMatch(b, "POST", "/login", "https://login-fragment.my-department.example.org")
		testMatch(b, "POST", "/logout", "https://logout.login-fragment.my-department.example.org")
		testMatch(b, "GET", "/healthcheck", "")
		testMatch(b, "GET", "/humans.txt", "")
		testMatch(b, "GET", "/assets/base-assets/some.css", "https://base-assets.my-department.example.org")
		testMatch(b, "GET", "/assets/header/some.css", "https://assets.header.my-department.example.org")
		testMatch(b, "GET", "/assets/footer/some.css", "https://assets.footer.my-department.example.org")
		testMatch(b, "GET", "/assets/cart/some.css", "https://assets.cart.my-department.example.org")
		testMatch(b, "GET", "/assets/pdp/some.css", "https://assets.pdp-fragment-alt.my-department.example.org")
		testMatch(b, "GET", "/assets/catalog/some.css", "https://assets.catalog-face.my-department.example.org")
		testMatch(b, "GET", "/assets/login/some.css", "https://assets.login-fragment.my-department.example.org")
		testMatch(b, "GET", "/herren/nike", "https://herren.layout-service.my-department.example.org")
		testMatch(b, "GET", "/damen/nike", "https://damen.layout-service.my-department.example.org")
		testMatch(b, "GET", "/sls-async/herren/nike", "https://herren-async.streaming-layout-service.my-department.example.org")
		testMatch(b, "GET", "/sls-async/damen/nike", "https://damen-async.streaming-layout-service.my-department.example.org")
		testMatch(b, "GET", "/sc/herren/nike", "https://herren-sc.compositor-layout-service.my-department.example.org")
		testMatch(b, "GET", "/sc/damen/nike", "https://damen-sc.compositor-layout-service.my-department.example.org")
		testMatch(b, "GET", "/sls/herren/nike", "https://herren-sls.streaming-layout-service.my-department.example.org")
		testMatch(b, "GET", "/sls/damen/nike", "https://damen-sls.streaming-layout-service.my-department.example.org")
		testMatch(b, "GET", "/men/nike", "https://herren-en.layout-service.my-department.example.org")
		testMatch(b, "GET", "/women/nike", "https://damen-en.layout-service.my-department.example.org")
		testMatch(b, "GET", "/sls-async/men/nike", "https://herren-en.streaming-layout-service.my-department.example.org")
		testMatch(b, "GET", "/sls-async/women/nike", "https://damen-en.streaming-layout-service.my-department.example.org")
		testMatch(b, "GET", "/sc/men/nike", "https://herren-en.compositor-layout-service.my-department.example.org")
		testMatch(b, "GET", "/sc/women/nike", "https://damen-en.compositor-layout-service.my-department.example.org")
		testMatch(b, "GET", "/sls/men/nike", "https://herren-en.streaming-layout-service.my-department.example.org")
		testMatch(b, "GET", "/sls/women/nike", "https://damen-en.streaming-layout-service.my-department.example.org")
	}
}

func BenchmarkPathTree1(b *testing.B) {
	benchmarkLookup(b, testMatcher1, benchmarkingCountPhase1)
}

func BenchmarkPathTree2(b *testing.B) {
	benchmarkLookup(b, testMatcher2, benchmarkingCountPhase2)
}

func BenchmarkPathTree3(b *testing.B) {
	benchmarkLookup(b, testMatcher3, benchmarkingCountPhase3)
}

func BenchmarkPathTree4(b *testing.B) {
	benchmarkLookup(b, testMatcher4, benchmarkingCountPhase4)
}

func BenchmarkConstructionGeneric(b *testing.B) {
	routes, err := docToRoutes(testRouteDoc)
	if err != nil {
		b.Error(err)
		return
	}

	for i := 0; i < b.N; i++ {
		_, errs := newMatcher(routes, IgnoreTrailingSlash)
		if len(errs) != 0 {
			for _, err := range errs {
				b.Log(err.Error())
			}
			b.Error("error while making matcher")
		}
	}
}

func BenchmarkConstructionMass(b *testing.B) {
	const count = 10000
	routes := make([]*Route, count)
	for i, r := range randomRoutes[:count] {
		routes[i] = r
	}

	for i := 0; i < b.N; i++ {
		_, errs := newMatcher(routes, IgnoreTrailingSlash)
		if len(errs) != 0 {
			for _, err := range errs {
				b.Log(err.Error())
			}
			b.Error("error while making matcher")
		}
	}
}
