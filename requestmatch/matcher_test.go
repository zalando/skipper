package requestmatch

import (
	"errors"
	"fmt"
	"github.bus.zalan.do/spearheads/pathmux"
	"github.bus.zalan.do/spearheads/randpath"
	"github.com/zalando/eskip"
	"net/http"
	"net/url"
	"regexp"
	"testing"
)

const (
	defsCountPhase1 = 1
	defsCountPhase2 = 100
	defsCountPhase3 = 10000
	defsCountPhase4 = 1000000
)

// defs in eskip format
const routeDoc = `
    header: Path("/tessera/header") -> "https://header.mop-taskforce.zalan.do";
    footer: Path("/tessera/footer") -> "https://footer.mop-taskforce.zalan.do";
    pdp: PathRegexp(/.*\.html$/) -> "https://pdp.layout-service.mop-taskforce.zalan.do";
    pdpAsync: Path("/sls-async/*_") && PathRegexp(/.*\.html$/) -> "https://async.pdp.streaming-layout-service.mop-taskforce.zalan.do";
    pdpsc: Path("/sc/*_") && PathRegexp(/.*\.html$/) -> "https://pdpsc.compositor-layout-service.mop-taskforce.zalan.do";
    pdpsls: Path("/sls/*_") && PathRegexp(/.*\.html$/) -> "https://pdpsls.streaming-layout-service.mop-taskforce.zalan.do";
    catalog: Any() -> "https://catalog.layout-service.mop-taskforce.zalan.do";
    catalogAsync: Path("/sls-async/*_") -> "https://catalog-async.layout-service.mop-taskforce.zalan.do";
    catalogsc: Path("/sc/*_") -> "https://catalogsc.compositor-layout-service.mop-taskforce.zalan.do";
    catalogsls: Path("/sls/*_") -> "https://catalogsls.streaming-layout-service.mop-taskforce.zalan.do";
    slow: Path("/slow") -> "https://bugfactory.mop-taskforce.zalan.do";
    debug: Path("/debug") -> "https://debug.bugfactory.mop-taskforce.zalan.do";
    cart: Path("/api/cart/*_") -> "https://cart.mop-taskforce.zalan.do";
    login: Path("/login") && Method("POST") -> "https://login-fragment.mop-taskforce.zalan.do";
    logout: Path("/logout") && Method("POST") -> "https://logout.login-fragment.mop-taskforce.zalan.do";
    healthcheck: Path("/healthcheck") -> <shunt>;
    humanstxt: Path("/humans.txt") -> <shunt>;
    baseAssetsAssets: Path("/assets/base-assets/*_") -> "https://base-assets.mop-taskforce.zalan.do";
    headerAssets: Path("/assets/header/*_") -> "https://assets.header.mop-taskforce.zalan.do";
    footerAssets: Path("/assets/footer/*_") -> "https://assets.footer.mop-taskforce.zalan.do";
    cartAssets: Path("/assets/cart/*_") -> "https://assets.cart.mop-taskforce.zalan.do";
    pdpAssets: Path("/assets/pdp/*_") -> "https://assets.pdp-fragment-alt.mop-taskforce.zalan.do";
    catalogAssets: Path("/assets/catalog/*_") -> "https://assets.catalog-face.mop-taskforce.zalan.do";
    loginAssets: Path("/assets/login/*_") -> "https://assets.login-fragment.mop-taskforce.zalan.do";

    catalogHerren: Path("/herren/*_") -> "https://herren.layout-service.mop-taskforce.zalan.do";
    catalogDamen: Path("/damen/*_") -> "https://damen.layout-service.mop-taskforce.zalan.do";
    catalogAsyncHerren: Path("/sls-async/herren/*_") -> "https://herren-async.streaming-layout-service.mop-taskforce.zalan.do";
    catalogAsyncDamen: Path("/sls-async/damen/*_") -> "https://damen-async.streaming-layout-service.mop-taskforce.zalan.do";
    catalogscHerren: Path("/sc/herren/*_") -> "https://herren-sc.compositor-layout-service.mop-taskforce.zalan.do";
    catalogscDamen: Path("/sc/damen/*_") -> "https://damen-sc.compositor-layout-service.mop-taskforce.zalan.do";
    catalogslsHerren: Path("/sls/herren/*_") -> "https://herren-sls.streaming-layout-service.mop-taskforce.zalan.do";
    catalogslsDamen: Path("/sls/damen/*_") -> "https://damen-sls.streaming-layout-service.mop-taskforce.zalan.do";

    catalogHerrenEn: Path("/men/*_") -> "https://herren-en.layout-service.mop-taskforce.zalan.do";
    catalogDamenEn: Path("/women/*_") -> "https://damen-en.layout-service.mop-taskforce.zalan.do";
    catalogAsyncHerrenEn: Path("/sls-async/men/*_") -> "https://herren-en.streaming-layout-service.mop-taskforce.zalan.do";
    catalogAsyncDamenEn: Path("/sls-async/women/*_") -> "https://damen-en.streaming-layout-service.mop-taskforce.zalan.do";
    catalogscHerrenEn: Path("/sc/men/*_") -> "https://herren-en.compositor-layout-service.mop-taskforce.zalan.do";
    catalogscDamenEn: Path("/sc/women/*_") -> "https://damen-en.compositor-layout-service.mop-taskforce.zalan.do";
    catalogslsHerrenEn: Path("/sls/men/*_") -> "https://herren-en.streaming-layout-service.mop-taskforce.zalan.do";
    catalogslsDamenEn: Path("/sls/women/*_") -> "https://damen-en.streaming-layout-service.mop-taskforce.zalan.do";
`

type def struct {
	eskipRoute *eskip.Route
}

func (rd *def) Path() string                       { return rd.eskipRoute.Path }
func (rd *def) Method() string                     { return rd.eskipRoute.Method }
func (rd *def) HostRegexps() []string              { return rd.eskipRoute.HostRegexps }
func (rd *def) PathRegexps() []string              { return rd.eskipRoute.PathRegexps }
func (rd *def) Headers() map[string]string         { return rd.eskipRoute.Headers }
func (rd *def) HeaderRegexps() map[string][]string { return rd.eskipRoute.HeaderRegexps }
func (rd *def) Value() interface{}                 { return rd.eskipRoute.Backend }

var (
	definitions []Definition
	matcher     *Matcher

	randomPaths    []string
	randomRoutes   []*eskip.Route
	randomRequests []*http.Request

	matcher1 *Matcher
	matcher2 *Matcher
	matcher3 *Matcher
	matcher4 *Matcher
)

func initMatcher() {
	routes, err := eskip.Parse(routeDoc)
	if err != nil {
		panic(err)
	}

	definitions = make([]Definition, len(routes))
	for i, r := range routes {
		definitions[i] = &def{r}
	}

	m, errs := Make(definitions, true)
	if len(errs) != 0 {
		for _, err := range errs {
			println(err.Error())
		}
		panic("error while making matcher")
	}

	matcher = m
}

func generatePaths(pg randpath.PathGenerator, count int) []string {
	paths := make([]string, count)

	for i := 0; i < count; i++ {
		paths[i] = pg.Next()
	}

	return paths
}

func generateRoutes(paths []string) []*eskip.Route {
	routes := make([]*eskip.Route, len(paths))
	for i, p := range paths {

		// the path for the backend is fine here,
		// because it is only used for checking the
		// found routes
		routes[i] = &eskip.Route{Path: p, Backend: p}
	}

	return routes
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

func makeMatcher(routes []*eskip.Route) (*Matcher, error) {
	if len(routes) == 0 {
		return nil, errors.New("we need at least one route for this test")
	}

	definitions := make([]Definition, len(routes))
	for i, r := range routes {
		definitions[i] = &def{r}
	}

	matcher, errs := Make(definitions, false)
	if len(errs) != 0 {
		return nil, errors.New("failed to create matcher")
	}

	return matcher, nil
}

func initRandomPaths() {
	const count = defsCountPhase4

	// we need to avoid '/' paths here, because we are not testing conflicting cases
	// here, and with 0 or 1 MinNamesInPath, there would be multiple '/'s.
	pg := randpath.Make(randpath.Options{
		MinNamesInPath: 2,
		MaxNamesInPath: 15})

	var err error

	randomPaths = generatePaths(pg, count)
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

	mkmatcher := func(paths []string, routes []*eskip.Route) *Matcher {
		if err != nil {
			return nil
		}

		r, e := makeMatcher(routes)
		err = e
		return r
	}

	defer func() {
		if err != nil {
			panic(err)
		}
	}()

	matcher1 = mkmatcher(randomPaths[0:defsCountPhase1], randomRoutes[0:defsCountPhase1])
	matcher2 = mkmatcher(randomPaths[0:defsCountPhase2], randomRoutes[0:defsCountPhase2])
	matcher3 = mkmatcher(randomPaths[0:defsCountPhase3], randomRoutes[0:defsCountPhase3])
	matcher4 = mkmatcher(randomPaths[0:defsCountPhase4], randomRoutes[0:defsCountPhase4])
}

func init() {
	initMatcher()
	initRandomPaths()
}

func makeRequest(method, path string) (*http.Request, error) {
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

func checkMatch(t testing.TB, v interface{}, err error, host string) {
	if err != nil {
		t.Error(err)
		return
	}

	b, ok := v.(string)
	if !ok {
		t.Error("failed to match request", host)
		return
	}

	if b != host {
		t.Error("failed to match the right value", b, host)
	}
}

func testMatch(t testing.TB, method, path, host string) {
	req, err := makeRequest(method, path)
	if err != nil {
		t.Error(err)
	}

	v, _ := matcher.Match(req)
	checkMatch(t, v, err, host)
}

func benchmarkLookup(b *testing.B, matcher *Matcher, phaseCount int) {
	// see init, double as much requests as routes
	requestCount := phaseCount * 2

	var index int
	for i := 0; i < b.N; i++ {

		// b.N comes from the test vault, doesn't matter if it matches the available
		// number of requests or routes, because in successful case, b.N will be far bigger
		index = i % requestCount

		r, _ := matcher.Match(randomRequests[index])

		if (index < phaseCount && r.(string) != randomRoutes[index].Backend) ||
			(index >= phaseCount && r != nil) {
			b.Log("benchmark failed", r == nil, fmt.Sprintf("(%s != %s)", r.(string), randomRoutes[index].Backend),
				index, i, b.N, randomRequests[index].URL.Path)
			b.FailNow()
		}
	}
}

func TestGeneric(t *testing.T) {
	testMatch(t, "GET", "/tessera/header", "https://header.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/tessera/footer", "https://footer.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/some.html", "https://pdp.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/path/to/some.html", "https://pdp.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls-async/some.html", "https://async.pdp.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sc/some.html", "https://pdpsc.compositor-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls/some.html", "https://pdpsls.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "", "https://catalog.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/", "https://catalog.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/nike", "https://catalog.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls-async/nike", "https://catalog-async.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sc/nike", "https://catalogsc.compositor-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls/nike", "https://catalogsls.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/nike/sports", "https://catalog.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls-async/nike/sports", "https://catalog-async.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sc/nike/sports", "https://catalogsc.compositor-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls/nike/sports", "https://catalogsls.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/slow", "https://bugfactory.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/debug", "https://debug.bugfactory.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/api/cart/42", "https://cart.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/api/cart/42/all", "https://cart.mop-taskforce.zalan.do")
	testMatch(t, "POST", "/login", "https://login-fragment.mop-taskforce.zalan.do")
	testMatch(t, "POST", "/logout", "https://logout.login-fragment.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/healthcheck", "")
	testMatch(t, "GET", "/humans.txt", "")
	testMatch(t, "GET", "/assets/base-assets/some.css", "https://base-assets.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/header/some.css", "https://assets.header.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/footer/some.css", "https://assets.footer.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/cart/some.css", "https://assets.cart.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/pdp/some.css", "https://assets.pdp-fragment-alt.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/catalog/some.css", "https://assets.catalog-face.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/login/some.css", "https://assets.login-fragment.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/base-assets/dir/some.css", "https://base-assets.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/header/dir/some.css", "https://assets.header.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/footer/dir/some.css", "https://assets.footer.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/cart/dir/some.css", "https://assets.cart.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/pdp/dir/some.css", "https://assets.pdp-fragment-alt.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/catalog/dir/some.css", "https://assets.catalog-face.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/assets/login/dir/some.css", "https://assets.login-fragment.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/herren/nike", "https://herren.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/damen/nike", "https://damen.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls-async/herren/nike", "https://herren-async.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls-async/damen/nike", "https://damen-async.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sc/herren/nike", "https://herren-sc.compositor-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sc/damen/nike", "https://damen-sc.compositor-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls/herren/nike", "https://herren-sls.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls/damen/nike", "https://damen-sls.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/men/nike", "https://herren-en.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/women/nike", "https://damen-en.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls-async/men/nike", "https://herren-en.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls-async/women/nike", "https://damen-en.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sc/men/nike", "https://herren-en.compositor-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sc/women/nike", "https://damen-en.compositor-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls/men/nike", "https://herren-en.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls/women/nike", "https://damen-en.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/herren/nike/sports", "https://herren.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/damen/nike/sports", "https://damen.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls-async/herren/nike/sports", "https://herren-async.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls-async/damen/nike/sports", "https://damen-async.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sc/herren/nike/sports", "https://herren-sc.compositor-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sc/damen/nike/sports", "https://damen-sc.compositor-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls/herren/nike/sports", "https://herren-sls.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls/damen/nike/sports", "https://damen-sls.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/men/nike/sports", "https://herren-en.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/women/nike/sports", "https://damen-en.layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls-async/men/nike/sports", "https://herren-en.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls-async/women/nike/sports", "https://damen-en.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sc/men/nike/sports", "https://herren-en.compositor-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sc/women/nike/sports", "https://damen-en.compositor-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls/men/nike/sports", "https://herren-en.streaming-layout-service.mop-taskforce.zalan.do")
	testMatch(t, "GET", "/sls/women/nike/sports", "https://damen-en.streaming-layout-service.mop-taskforce.zalan.do")
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
		method:         "PUT",
		hostRxs:        []*regexp.Regexp{rxh},
		pathRxs:        []*regexp.Regexp{rxp},
		headersExact:   map[string]string{"Some-Header": "some-value"},
		headersRegexps: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
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
		method:         "PUT",
		hostRxs:        []*regexp.Regexp{rxh},
		pathRxs:        []*regexp.Regexp{rxp},
		headersExact:   map[string]string{"Some-Header": "some-value"},
		headersRegexps: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
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
		method:         "PUT",
		hostRxs:        []*regexp.Regexp{rxh},
		pathRxs:        []*regexp.Regexp{rxp},
		headersExact:   map[string]string{"Some-Header": "some-value"},
		headersRegexps: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
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
		method:         "PUT",
		hostRxs:        []*regexp.Regexp{rxh},
		pathRxs:        []*regexp.Regexp{rxp},
		headersExact:   map[string]string{"Some-Header": "some-value"},
		headersRegexps: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
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
		method:         "PUT",
		hostRxs:        []*regexp.Regexp{rxh},
		pathRxs:        []*regexp.Regexp{rxp},
		headersExact:   map[string]string{"Some-Header": "some-value"},
		headersRegexps: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
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
		method:         "PUT",
		hostRxs:        []*regexp.Regexp{rxh},
		pathRxs:        []*regexp.Regexp{rxp},
		headersExact:   map[string]string{"Some-Header": "some-value"},
		headersRegexps: map[string][]*regexp.Regexp{"Some-Other-Header": []*regexp.Regexp{rxhd}}}
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
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{value: &def{}}}}
	tree := &pathmux.Tree{}
	err := tree.Add("/some/path", pm0)
	if err != nil {
		t.Error(err)
	}
	m := &Matcher{paths: tree}
	req := &http.Request{URL: &url.URL{Path: "/some/path"}}
	r, p := m.Match(req)
	if r != pm0.leaves[0].value || len(p) != 0 {
		t.Error("failed to match path", r == nil, len(p))
	}
}

func TestMatchPathResolved(t *testing.T) {
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{value: &def{}}}}
	tree := &pathmux.Tree{}
	err := tree.Add("/some/path", pm0)
	if err != nil {
		t.Error(err)
	}
	m := &Matcher{paths: tree}
	req := &http.Request{URL: &url.URL{Path: "/some/some-other/../path"}}
	r, p := m.Match(req)
	if r != pm0.leaves[0].value || len(p) != 0 {
		t.Error("failed to match path", r == nil, len(p))
	}
}

func TestMatchWrongMethod(t *testing.T) {
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{method: "PUT", value: &def{}}}}
	tree := &pathmux.Tree{}
	err := tree.Add("/some/path/*_", pm0)
	if err != nil {
		t.Error(err)
	}
	m := &Matcher{paths: tree}
	req := &http.Request{Method: "GET", URL: &url.URL{Path: "/some/some-other/../path"}}
	r, p := m.Match(req)
	if r != nil || len(p) != 0 {
		t.Error("failed to match path", r == nil, len(p))
	}
}

func TestMatchTopLeaves(t *testing.T) {
	tree := &pathmux.Tree{}
	l := &leafMatcher{method: "PUT", value: &def{}}
	pm := &pathMatcher{leaves: leafMatchers{l}}
	err := tree.Add("/*", pm)
	if err != nil {
		t.Error(err)
	}
	m := &Matcher{paths: tree}
	req := &http.Request{Method: "PUT", URL: &url.URL{Path: "/some/some-other/../path"}}
	r, _ := m.Match(req)
	if r != l.value {
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
	rm := &Matcher{paths: tree}
	req := &http.Request{URL: &url.URL{Path: "/some/path/and/params"}}
	r, p := rm.Match(req)
	if r != pm0.leaves[0].value || len(p) != 2 ||
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
	routes, err := eskip.Parse(routeExp)
	if err != nil {
		t.Error(err)
		return
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	_, err = makeLeaf(rd)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestMakeLeafInvalidPathRx(t *testing.T) {
	const routeExp = "PathRegexp(\"**\") -> \"https://example.org\""
	routes, err := eskip.Parse(routeExp)
	if err != nil {
		t.Error(err)
		return
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	_, err = makeLeaf(rd)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestMakeLeafInvalidHeaderRegexp(t *testing.T) {
	const routeExp = "HeaderRegexp(\"Some-Header\", \"**\") -> \"https://example.org\""
	routes, err := eskip.Parse(routeExp)
	if err != nil {
		t.Error(err)
		return
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	_, err = makeLeaf(rd)
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
	routes, err := eskip.Parse(routeExp)
	if err != nil {
		t.Error(err)
		return
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	l, err := makeLeaf(rd)
	if err != nil || l.method != "PUT" ||
		len(l.hostRxs) != 1 || len(l.pathRxs) != 1 ||
		len(l.headersExact) != 1 || len(l.headersRegexps) != 1 ||
		l.value.(string) != "https://example.org" {
		t.Error("failed to create leaf")
	}
}

func TestMakeMatcherEmpty(t *testing.T) {
	m, errs := Make(nil, false)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make empty matcher")
	}

	r, params := m.Match(&http.Request{Method: "PUT", URL: &url.URL{Path: "/some/path"}})
	if len(params) != 0 || r != nil {
		t.Error("failed not to match request")
	}
}

func TestMakeMatcherRootLeavesOnly(t *testing.T) {
	routes, err := eskip.Parse(`Method("PUT") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd}, false)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make matcher")
	}

	r, _ := m.Match(&http.Request{Method: "PUT", URL: &url.URL{Path: "/some/path"}})
	if r == nil || r.(string) != "https://example.org" {
		t.Error("failed to match request")
	}
}

func TestMakeMatcherExactPathOnly(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/path") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd}, false)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make matcher")
	}

	r, params := m.Match(&http.Request{Method: "PUT", URL: &url.URL{Path: "/some/path"}})
	if len(params) != 0 || r == nil || r.(string) != "https://example.org" {
		t.Error("failed to match request")
	}
}

func TestMakeMatcherWithWildcardPath(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/:param") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd}, false)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make matcher")
	}

	r, params := m.Match(&http.Request{Method: "PUT", URL: &url.URL{Path: "/some/value"}})
	if len(params) != 1 || r == nil || r.(string) != "https://example.org" || params["param"] != "value" {
		t.Error("failed to match request")
	}
}

func TestMakeMatcherErrorInLeaf(t *testing.T) {
	routes, err := eskip.Parse(`testRoute: PathRegexp("**") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd}, false)
	if len(errs) != 1 || m == nil || errs[0].Index != 0 {
		t.Error("failed to make matcher with error")
	}
}

func TestMakeMatcherWithPathConflict(t *testing.T) {
	routes, err := eskip.Parse(`
        testRoute0: Path("/some/path/:param0/name") -> "https://example.org";
        testRoute1: Path("/some/path/:param1/name") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd0 := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	rd1 := &def{routes[1]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd0, rd1}, false)
	if len(errs) != 1 || m == nil {
		t.Error("failed to make matcher with error", len(errs), m == nil)
	}
}

func TestMatchToSlash(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/path/") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd}, true)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, _ := m.Match(&http.Request{URL: &url.URL{Path: "/some/path"}})
	if r == nil {
		t.Error("failed to match to slash")
	}
}

func TestMatchFromSlash(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/path") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd}, true)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, _ := m.Match(&http.Request{URL: &url.URL{Path: "/some/path/"}})
	if r == nil {
		t.Error("failed to match to slash")
	}
}

func TestWildcardParam(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/:wildcard0/path/:wildcard1") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd}, false)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, params := m.Match(&http.Request{URL: &url.URL{Path: "/some/value0/path/value1"}})
	if r == nil || len(params) != 2 || params["wildcard0"] != "value0" || params["wildcard1"] != "value1" {
		t.Error("failed to match with wildcards")
	}
}

func TestWildcardParamFromSlash(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/:wildcard0/path/:wildcard1") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd}, true)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, params := m.Match(&http.Request{URL: &url.URL{Path: "/some/value0/path/value1/"}})
	if r == nil || len(params) != 2 || params["wildcard0"] != "value0" || params["wildcard1"] != "value1" {
		t.Error("failed to match with wildcards")
	}
}

func TestWildcardParamToSlash(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/:wildcard0/path/:wildcard1/") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd}, true)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, params := m.Match(&http.Request{URL: &url.URL{Path: "/some/value0/path/value1"}})
	if r == nil || len(params) != 2 || params["wildcard0"] != "value0" || params["wildcard1"] != "value1" {
		t.Error("failed to match with wildcards")
	}
}

func TestFreeWildcardParam(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/*wildcard") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd}, false)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, params := m.Match(&http.Request{URL: &url.URL{Path: "/some/value0/value1"}})
	if r == nil || len(params) != 1 || params["wildcard"] != "/value0/value1" {
		t.Error("failed to match with wildcards", params["wildcard"])
	}
}

func TestFreeWildcardParamWithSlash(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/*wildcard") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	rd := &def{routes[0]}
	if err != nil {
		t.Error(err)
	}

	m, errs := Make([]Definition{rd}, true)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, params := m.Match(&http.Request{URL: &url.URL{Path: "/some/value0/value1/"}})
	if r == nil || len(params) != 1 || params["wildcard"] != "/value0/value1" {
		t.Error("failed to match with wildcards", r == nil, len(params), params["wildcard"])
	}
}

func BenchmarkGeneric(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMatch(b, "GET", "/tessera/header", "https://header.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/tessera/footer", "https://footer.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/some.html", "https://pdp.layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/nike", "https://catalog.layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sls-async/nike", "https://catalog-async.layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sc/nike", "https://catalogsc.compositor-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sls/nike", "https://catalogsls.streaming-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/slow", "https://bugfactory.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/debug", "https://debug.bugfactory.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/api/cart/42", "https://cart.mop-taskforce.zalan.do")
		testMatch(b, "POST", "/login", "https://login-fragment.mop-taskforce.zalan.do")
		testMatch(b, "POST", "/logout", "https://logout.login-fragment.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/healthcheck", "")
		testMatch(b, "GET", "/humans.txt", "")
		testMatch(b, "GET", "/assets/base-assets/some.css", "https://base-assets.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/assets/header/some.css", "https://assets.header.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/assets/footer/some.css", "https://assets.footer.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/assets/cart/some.css", "https://assets.cart.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/assets/pdp/some.css", "https://assets.pdp-fragment-alt.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/assets/catalog/some.css", "https://assets.catalog-face.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/assets/login/some.css", "https://assets.login-fragment.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/herren/nike", "https://herren.layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/damen/nike", "https://damen.layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sls-async/herren/nike", "https://herren-async.streaming-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sls-async/damen/nike", "https://damen-async.streaming-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sc/herren/nike", "https://herren-sc.compositor-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sc/damen/nike", "https://damen-sc.compositor-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sls/herren/nike", "https://herren-sls.streaming-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sls/damen/nike", "https://damen-sls.streaming-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/men/nike", "https://herren-en.layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/women/nike", "https://damen-en.layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sls-async/men/nike", "https://herren-en.streaming-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sls-async/women/nike", "https://damen-en.streaming-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sc/men/nike", "https://herren-en.compositor-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sc/women/nike", "https://damen-en.compositor-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sls/men/nike", "https://herren-en.streaming-layout-service.mop-taskforce.zalan.do")
		testMatch(b, "GET", "/sls/women/nike", "https://damen-en.streaming-layout-service.mop-taskforce.zalan.do")
	}
}

func BenchmarkPathTree1(b *testing.B) {
	benchmarkLookup(b, matcher1, defsCountPhase1)
}

func BenchmarkPathTree2(b *testing.B) {
	benchmarkLookup(b, matcher2, defsCountPhase2)
}

func BenchmarkPathTree3(b *testing.B) {
	benchmarkLookup(b, matcher3, defsCountPhase3)
}

func BenchmarkPathTree4(b *testing.B) {
	benchmarkLookup(b, matcher4, defsCountPhase4)
}

func BenchmarkConstructionGeneric(b *testing.B) {
	routes, err := eskip.Parse(routeDoc)
	if err != nil {
		b.Error(err)
		return
	}

	definitions = make([]Definition, len(routes))
	for i, r := range routes {
		definitions[i] = &def{r}
	}

	for i := 0; i < b.N; i++ {
		_, errs := Make(definitions, true)
		if len(errs) != 0 {
			for _, err := range errs {
				println(err.Error())
			}
			b.Error("error while making matcher")
		}
	}
}

func BenchmarkConstructionMass(b *testing.B) {
	const count = 10000
	definitions := make([]Definition, count)
	for i, r := range randomRoutes[:count] {
		definitions[i] = &def{r}
	}

	for i := 0; i < b.N; i++ {
		_, errs := Make(definitions, true)
		if len(errs) != 0 {
			for _, err := range errs {
				println(err.Error())
			}
			b.Error("error while making matcher")
		}
	}
}
