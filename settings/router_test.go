package settings

import (
	"fmt"
	"github.bus.zalan.do/spearheads/pathmux"
	"github.com/mailgun/route"
	"github.com/zalando/eskip"
	"github.com/zalando/skipper/mock"
	"github.com/zalando/skipper/skipper"
	"net/http"
	"net/url"
	"regexp"
	"testing"
)

const routeDoc = `
    header: Path("/tessera/header") ->
        xalando() -> pathRewrite(/.*/, "/") ->
        requestHeader("Host", "header.mop-taskforce.zalan.do") ->
        "https://header.mop-taskforce.zalan.do";

    footer: Path("/tessera/footer") ->
        xalando() -> pathRewrite(/.*/, "/") ->
        requestHeader("Host", "footer.mop-taskforce.zalan.do") ->
        "https://footer.mop-taskforce.zalan.do";

    pdp: PathRegexp(/.*\.html$/) ->
        xalando() -> pathRewrite(/.*/, "/pdp") ->
        requestHeader("Host", "layout-service.mop-taskforce.zalan.do") ->
        "https://pdp.layout-service.mop-taskforce.zalan.do";

    pdpAsync: Path("/sls-async/*_") && PathRegexp(/.*\.html$/) ->
        xalando() -> pathRewrite(/.*/, "/pdp-async") ->
        requestHeader("Host", "layout-service.mop-taskforce.zalan.do") ->
        "https://async.pdp.streaming-layout-service.mop-taskforce.zalan.do";

    pdpsc: Path("/sc/*_") && PathRegexp(/.*\.html$/) ->
        xalando() -> pathRewrite(/.*/, "/pdp") ->
        requestHeader("Host", "compositor-layout-service.mop-taskforce.zalan.do") ->
        "https://pdpsc.compositor-layout-service.mop-taskforce.zalan.do";

    pdpsls: Path("/sls/*_") && PathRegexp(/.*\.html$/) ->
        xalando() -> pathRewrite(/.*/, "/pdp") ->
        requestHeader("Host", "streaming-layout-service.mop-taskforce.zalan.do") ->
        "https://pdpsls.streaming-layout-service.mop-taskforce.zalan.do";

    catalog: Any() ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "layout-service.mop-taskforce.zalan.do") ->
        "https://catalog.layout-service.mop-taskforce.zalan.do";

    catalogAsync: Path("/sls-async/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog-async") ->
        requestHeader("Host", "layout-service.mop-taskforce.zalan.do") ->
        "https://catalog-async.layout-service.mop-taskforce.zalan.do";

    catalogsc: Path("/sc/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "compositor-layout-service.mop-taskforce.zalan.do") ->
        "https://catalogsc.compositor-layout-service.mop-taskforce.zalan.do";

    catalogsls: Path("/sls/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "streaming-layout-service.mop-taskforce.zalan.do") ->
        "https://catalogsls.streaming-layout-service.mop-taskforce.zalan.do";

    slow: Path("/slow") ->
        xalando() -> requestHeader("Host", "bugfactory.mop-taskforce.zalan.do") ->
        "https://bugfactory.mop-taskforce.zalan.do";

    debug: Path("/debug") ->
        xalando() -> pathRewrite(/.*/, "/") ->
        requestHeader("Host", "bugfactory.mop-taskforce.zalan.do") ->
        "https://debug.bugfactory.mop-taskforce.zalan.do";

    cart: Path("/api/cart/*_") ->
        xalando() -> requestHeader("Host", "cart-taskforce.zalan.do") ->
        "https://cart.mop-taskforce.zalan.do";

    login: Path("/login") && Method("POST") ->
        xalando() ->
        "https://login-fragment.mop-taskforce.zalan.do";

    logout: Path("/logout") && Method("POST") ->
        xalando() ->
        "https://logout.login-fragment.mop-taskforce.zalan.do";

    healthcheck: Path("/healthcheck") -> healthcheck() -> <shunt>;

    humanstxt: Path("/humans.txt") -> humanstxt() -> <shunt>;

    baseAssetsAssets: Path("/assets/base-assets/*_") ->
        pathRewrite("^/assets/base-assets", "/assets") ->
        requestHeader("Host", "base-assets.mop-taskforce.zalan.do") ->
        "https://base-assets.mop-taskforce.zalan.do";

    headerAssets: Path("/assets/header/*_") ->
        pathRewrite("^/assets/header", "/assets") ->
        requestHeader("Host", "header.mop-taskforce.zalan.do") ->
        "https://assets.header.mop-taskforce.zalan.do";

    footerAssets: Path("/assets/footer/*_") ->
        pathRewrite("^/assets/footer", "/assets") ->
        requestHeader("Host", "footer.mop-taskforce.zalan.do") ->
        "https://assets.footer.mop-taskforce.zalan.do";

    cartAssets: Path("/assets/cart/*_") ->
        pathRewrite("^/assets/cart", "/assets") ->
        requestHeader("Host", "cart.mop-taskforce.zalan.do") ->
        "https://assets.cart.mop-taskforce.zalan.do";

    pdpAssets: Path("/assets/pdp/*_") ->
        pathRewrite("^/assets/pdp", "") ->
        requestHeader("Host", "pdp-fragment-alt.mop-taskforce.zalan.do") ->
        "https://assets.pdp-fragment-alt.mop-taskforce.zalan.do";

    catalogAssets: Path("/assets/catalog/*_") ->
        pathRewrite("^/assets/catalog", "/static") ->
        requestHeader("Host", "catalog-face.mop-taskforce.zalan.do") ->
        "https://assets.catalog-face.mop-taskforce.zalan.do";

    loginAssets: Path("/assets/login/*_") ->
        pathRewrite("^/assets/login", "/") ->
        requestHeader("Host", "login-fragment.mop-taskforce.zalan.do") ->
        "https://assets.login-fragment.mop-taskforce.zalan.do";

    // some demo hack:

    // de
    catalogHerren: Path("/herren/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "layout-service.mop-taskforce.zalan.do") ->
        "https://herren.layout-service.mop-taskforce.zalan.do";

    catalogDamen: Path("/damen/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "layout-service.mop-taskforce.zalan.do") ->
        "https://damen.layout-service.mop-taskforce.zalan.do";

    catalogAsyncHerren: Path("/sls-async/herren/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog-async") ->
        requestHeader("Host", "streaming-layout-service.mop-taskforce.zalan.do") ->
        "https://herren-async.streaming-layout-service.mop-taskforce.zalan.do";

    catalogAsyncDamen: Path("/sls-async/damen/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog-async") ->
        requestHeader("Host", "streaming-layout-service.mop-taskforce.zalan.do") ->
        "https://damen-async.streaming-layout-service.mop-taskforce.zalan.do";

    catalogscHerren: Path("/sc/herren/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "compositor-layout-service.mop-taskforce.zalan.do") ->
        "https://herren-sc.compositor-layout-service.mop-taskforce.zalan.do";

    catalogscDamen: Path("/sc/damen/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "compositor-layout-service.mop-taskforce.zalan.do") ->
        "https://damen-sc.compositor-layout-service.mop-taskforce.zalan.do";

    catalogslsHerren: Path("/sls/herren/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "streaming-layout-service.mop-taskforce.zalan.do") ->
        "https://herren-sls.streaming-layout-service.mop-taskforce.zalan.do";

    catalogslsDamen: Path("/sls/damen/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "streaming-layout-service.mop-taskforce.zalan.do") ->
        "https://damen-sls.streaming-layout-service.mop-taskforce.zalan.do";

    // en
    catalogHerrenEn: Path("/men/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "layout-service.mop-taskforce.zalan.do") ->
        "https://herren-en.layout-service.mop-taskforce.zalan.do";

    catalogDamenEn: Path("/women/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "layout-service.mop-taskforce.zalan.do") ->
        "https://damen-en.layout-service.mop-taskforce.zalan.do";

    catalogAsyncHerrenEn: Path("/sls-async/men/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog-async") ->
        requestHeader("Host", "streaming-layout-service.mop-taskforce.zalan.do") ->
        "https://herren-en.streaming-layout-service.mop-taskforce.zalan.do";

    catalogAsyncDamenEn: Path("/sls-async/women/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog-async") ->
        requestHeader("Host", "streaming-layout-service.mop-taskforce.zalan.do") ->
        "https://damen-en.streaming-layout-service.mop-taskforce.zalan.do";

    catalogscHerrenEn: Path("/sc/men/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "compositor-layout-service.mop-taskforce.zalan.do") ->
        "https://herren-en.compositor-layout-service.mop-taskforce.zalan.do";

    catalogscDamenEn: Path("/sc/women/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "compositor-layout-service.mop-taskforce.zalan.do") ->
        "https://damen-en.compositor-layout-service.mop-taskforce.zalan.do";

    catalogslsHerrenEn: Path("/sls/men/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "streaming-layout-service.mop-taskforce.zalan.do") ->
        "https://herren-en.streaming-layout-service.mop-taskforce.zalan.do";

    catalogslsDamenEn: Path("/sls/women/*_") ->
        xalando() -> pathRewrite(/.*/, "/catalog") ->
        requestHeader("Host", "streaming-layout-service.mop-taskforce.zalan.do") ->
        "https://damen-en.streaming-layout-service.mop-taskforce.zalan.do";
`

// type testFilter struct {
// 	name string
// 	args []string
// }
//
// func (tf *testFilter) Id() string                         { return "" }
// func (tf *testFilter) Request(ctx skipper.FilterContext)  {}
// func (tf *testFilter) Response(ctx skipper.FilterContext) {}
//
// type testBackend struct {
// 	scheme  string
// 	host    string
// 	isShunt bool
// }
//
// func (tb *testBackend) Scheme() string { return "https" }
// func (tb *testBackend) Host() string   { return tb.host }
// func (tb *testBackend) IsShunt() bool  { return false }

var (
	definitions []RouteDefinition
	rtskipper   skipper.Router
	rtmailgun   skipper.Router
)

func init() {
	routes, err := eskip.Parse(routeDoc)
	if err != nil {
		panic(err)
	}

	definitions = make([]RouteDefinition, len(routes))
	for i, r := range routes {
		definitions[i] = &routeDefinition{r, &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}
	}

	rt, errs := makeMatcher(definitions, true)
	if len(errs) != 0 {
		for _, err := range errs {
			println(err.Error())
		}
		panic("error while making matcher")
	}

	rtskipper = rt

	mrt := route.New()
	for _, def := range definitions {
		m := formatMailgunMatchers(def.(*routeDefinition).eskipRoute.Matchers)
		mrt.AddRoute(m, &routedef{def.Backend(), def.Filters()})
	}

	rtmailgun = &mailgunRouter{mrt}
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

func checkRoute(t testing.TB, rt skipper.Route, err error, host string) {
	if err != nil {
		t.Error(err)
		return
	}

	if rt == nil {
		t.Error("failed to match request", host)
		return
	}

	if rt.Backend().Host() != host {
		t.Error("failed to match the right route", rt.Backend().Host(), host)
	}
}

func testRoute(t testing.TB, router skipper.Router, req *http.Request, host string) {
	rt, _, err := router.Route(req)
	checkRoute(t, rt, err, host)
}

func testRouteIn(t testing.TB, method, path, host string, router skipper.Router) {
	req, err := makeRequest(method, path)
	if err != nil {
		t.Error(err)
	}

	testRoute(t, router, req, host)
}

func testRouteInMailgun(t testing.TB, method, path, host string) {
	testRouteIn(t, method, path, host, rtmailgun)
}

func testRouteInSkipper(t testing.TB, method, path, host string) {
	testRouteIn(t, method, path, host, rtskipper)
}

func testRouteInBoth(t testing.TB, method, path, host string) {
	testRouteInMailgun(t, method, path, host)
	testRouteInSkipper(t, method, path, host)
}

func TestValidRoutes(t *testing.T) {
	testRouteInBoth(t, "GET", "/tessera/header", "header.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/tessera/footer", "footer.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/some.html", "pdp.layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/path/to/some.html", "pdp.layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "", "catalog.layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/", "catalog.layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/nike", "catalog.layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sls-async/nike", "catalog-async.layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sc/nike", "catalogsc.compositor-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sls/nike", "catalogsls.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/slow", "bugfactory.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/debug", "debug.bugfactory.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/api/cart/42", "cart.mop-taskforce.zalan.do")
	testRouteInBoth(t, "POST", "/login", "login-fragment.mop-taskforce.zalan.do")
	testRouteInBoth(t, "POST", "/logout", "logout.login-fragment.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/healthcheck", "")
	testRouteInBoth(t, "GET", "/humans.txt", "")
	testRouteInBoth(t, "GET", "/assets/base-assets/some.css", "base-assets.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/assets/header/some.css", "assets.header.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/assets/footer/some.css", "assets.footer.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/assets/cart/some.css", "assets.cart.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/assets/pdp/some.css", "assets.pdp-fragment-alt.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/assets/catalog/some.css", "assets.catalog-face.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/assets/login/some.css", "assets.login-fragment.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/herren/nike", "herren.layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/damen/nike", "damen.layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sls-async/herren/nike", "herren-async.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sls-async/damen/nike", "damen-async.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sc/herren/nike", "herren-sc.compositor-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sc/damen/nike", "damen-sc.compositor-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sls/herren/nike", "herren-sls.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sls/damen/nike", "damen-sls.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/men/nike", "herren-en.layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/women/nike", "damen-en.layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sls-async/men/nike", "herren-en.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sls-async/women/nike", "damen-en.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sc/men/nike", "herren-en.compositor-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sc/women/nike", "damen-en.compositor-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sls/men/nike", "herren-en.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInBoth(t, "GET", "/sls/women/nike", "damen-en.streaming-layout-service.mop-taskforce.zalan.do")

	testRouteInSkipper(t, "GET", "/sls-async/some.html", "async.pdp.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sc/some.html", "pdpsc.compositor-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sls/some.html", "pdpsls.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/nike/sports", "catalog.layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sls-async/nike/sports", "catalog-async.layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sc/nike/sports", "catalogsc.compositor-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sls/nike/sports", "catalogsls.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/api/cart/42/all", "cart.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/assets/base-assets/dir/some.css", "base-assets.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/assets/header/dir/some.css", "assets.header.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/assets/footer/dir/some.css", "assets.footer.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/assets/cart/dir/some.css", "assets.cart.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/assets/pdp/dir/some.css", "assets.pdp-fragment-alt.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/assets/catalog/dir/some.css", "assets.catalog-face.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/assets/login/dir/some.css", "assets.login-fragment.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/herren/nike/sports", "herren.layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/damen/nike/sports", "damen.layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sls-async/herren/nike/sports", "herren-async.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sls-async/damen/nike/sports", "damen-async.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sc/herren/nike/sports", "herren-sc.compositor-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sc/damen/nike/sports", "damen-sc.compositor-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sls/herren/nike/sports", "herren-sls.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sls/damen/nike/sports", "damen-sls.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/men/nike/sports", "herren-en.layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/women/nike/sports", "damen-en.layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sls-async/men/nike/sports", "herren-en.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sls-async/women/nike/sports", "damen-en.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sc/men/nike/sports", "herren-en.compositor-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sc/women/nike/sports", "damen-en.compositor-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sls/men/nike/sports", "herren-en.streaming-layout-service.mop-taskforce.zalan.do")
	testRouteInSkipper(t, "GET", "/sls/women/nike/sports", "damen-en.streaming-layout-service.mop-taskforce.zalan.do")
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
	if findHeader(h, "Some-Header", func(v string) bool { return v == "some-wrong-value" }) {
		t.Error("failed not to find header")
	}
}

func TestFindHeaderTrue(t *testing.T) {
	h := make(http.Header)
	h["Some-Header"] = []string{"some-value"}
	h["Some-Other-Header"] = []string{"some-other-value-0", "some-other-value-1"}
	if !findHeader(h, "Some-Header", func(v string) bool { return v == "some-value" }) {
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
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{route: &routedef{}}}}
	tree := &pathmux.Tree{}
	err := tree.Add("/some/path", pm0)
	if err != nil {
		t.Error(err)
	}
	m := &rootMatcher{paths: tree}
	req := &http.Request{URL: &url.URL{Path: "/some/path"}}
	r, p := match(m, req)
	if r != pm0.leaves[0].route || len(p) != 0 {
		t.Error("failed to match path", r == nil, len(p))
	}
}

func TestMatchPathResolved(t *testing.T) {
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{route: &routedef{}}}}
	tree := &pathmux.Tree{}
	err := tree.Add("/some/path", pm0)
	if err != nil {
		t.Error(err)
	}
	m := &rootMatcher{paths: tree}
	req := &http.Request{URL: &url.URL{Path: "/some/some-other/../path"}}
	r, p := match(m, req)
	if r != pm0.leaves[0].route || len(p) != 0 {
		t.Error("failed to match path", r == nil, len(p))
	}
}

func TestMatchWrongMethod(t *testing.T) {
	pm0 := &pathMatcher{leaves: []*leafMatcher{&leafMatcher{method: "PUT", route: &routedef{}}}}
	tree := &pathmux.Tree{}
	err := tree.Add("/some/path/*_", pm0)
	if err != nil {
		t.Error(err)
	}
	m := &rootMatcher{paths: tree}
	req := &http.Request{Method: "GET", URL: &url.URL{Path: "/some/some-other/../path"}}
	r, p := match(m, req)
	if r != nil || len(p) != 0 {
		t.Error("failed to match path", r == nil, len(p))
	}
}

func TestMatchTopLeaves(t *testing.T) {
	tree := &pathmux.Tree{}
	l := &leafMatcher{method: "PUT", route: &routedef{}}
	pm := &pathMatcher{leaves: leafMatchers{l}}
	err := tree.Add("/*", pm)
	if err != nil {
		t.Error(err)
	}
	m := &rootMatcher{paths: tree}
	req := &http.Request{Method: "PUT", URL: &url.URL{Path: "/some/some-other/../path"}}
	r, _ := match(m, req)
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
	rm := &rootMatcher{paths: tree}
	req := &http.Request{URL: &url.URL{Path: "/some/path/and/params"}}
	r, p := match(rm, req)
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
	routes, err := eskip.Parse(routeExp)
	if err != nil {
		t.Error(err)
		return
	}

	rd := &routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}
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

	rd := &routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}
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

	rd := &routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}
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

	rd := &routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}
	l, err := makeLeaf(rd)
	if err != nil || l.method != "PUT" ||
		len(l.hostRxs) != 1 || len(l.pathRxs) != 1 ||
		len(l.headersExact) != 1 || len(l.headersRegexps) != 1 ||
		l.route.Backend().Scheme() != "https" ||
		l.route.Backend().Host() != "example.org" {
		t.Error("failed to create leaf")
	}
}

func TestMakeMatcherEmpty(t *testing.T) {
	m, errs := makeMatcher(nil, false)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make empty matcher")
	}

	r, params := match(m, &http.Request{Method: "PUT", URL: &url.URL{Path: "/some/path"}})
	if len(params) != 0 || r != nil {
		t.Error("failed not to match request")
	}
}

func TestMakeMatcherRootLeavesOnly(t *testing.T) {
	routes, err := eskip.Parse(`Method("PUT") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := makeMatcher([]RouteDefinition{&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, false)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make matcher")
	}

	r, _ := match(m, &http.Request{Method: "PUT", URL: &url.URL{Path: "/some/path"}})
	if r == nil || r.Backend().Host() != "example.org" {
		t.Error("failed to match request")
	}
}

func TestMakeMatcherExactPathOnly(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/path") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := makeMatcher([]RouteDefinition{&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, false)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make matcher")
	}

	r, params := match(m, &http.Request{Method: "PUT", URL: &url.URL{Path: "/some/path"}})
	if len(params) != 0 || r == nil || r.Backend().Host() != "example.org" {
		t.Error("failed to match request")
	}
}

func TestMakeMatcherWithWildcardPath(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/:param") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := makeMatcher([]RouteDefinition{&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, false)
	if len(errs) != 0 || m == nil {
		t.Error("failed to make matcher")
	}

	r, params := match(m, &http.Request{Method: "PUT", URL: &url.URL{Path: "/some/value"}})
	if len(params) != 1 || r == nil || r.Backend().Host() != "example.org" || params["param"] != "value" {
		t.Error("failed to match request")
	}
}

func TestMakeMatcherErrorInLeaf(t *testing.T) {
	routes, err := eskip.Parse(`testRoute: PathRegexp("**") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := makeMatcher([]RouteDefinition{&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, false)
	if len(errs) != 1 || m == nil || errs[0].Id != "testRoute" {
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

	m, errs := makeMatcher([]RouteDefinition{
		&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}},
		&routeDefinition{routes[1], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, false)
	if len(errs) != 1 || m == nil {
		t.Error("failed to make matcher with error", len(errs), m == nil)
	}
}

func TestMatchToSlash(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/path/") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := makeMatcher([]RouteDefinition{&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, true)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, _ := match(m, &http.Request{URL: &url.URL{Path: "/some/path"}})
	if r == nil {
		t.Error("failed to match to slash")
	}
}

func TestMatchFromSlash(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/path") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := makeMatcher([]RouteDefinition{&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, true)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, _ := match(m, &http.Request{URL: &url.URL{Path: "/some/path/"}})
	if r == nil {
		t.Error("failed to match to slash")
	}
}

func TestWildcardParam(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/:wildcard0/path/:wildcard1") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := makeMatcher([]RouteDefinition{&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, false)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, params := match(m, &http.Request{URL: &url.URL{Path: "/some/value0/path/value1"}})
	if r == nil || len(params) != 2 || params["wildcard0"] != "value0" || params["wildcard1"] != "value1" {
		t.Error("failed to match with wildcards")
	}
}

func TestWildcardParamFromSlash(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/:wildcard0/path/:wildcard1") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := makeMatcher([]RouteDefinition{&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, true)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, params := match(m, &http.Request{URL: &url.URL{Path: "/some/value0/path/value1/"}})
	if r == nil || len(params) != 2 || params["wildcard0"] != "value0" || params["wildcard1"] != "value1" {
		t.Error("failed to match with wildcards")
	}
}

func TestWildcardParamToSlash(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/:wildcard0/path/:wildcard1/") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := makeMatcher([]RouteDefinition{&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, true)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, params := match(m, &http.Request{URL: &url.URL{Path: "/some/value0/path/value1"}})
	if r == nil || len(params) != 2 || params["wildcard0"] != "value0" || params["wildcard1"] != "value1" {
		t.Error("failed to match with wildcards")
	}
}

func TestFreeWildcardParam(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/*wildcard") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := makeMatcher([]RouteDefinition{&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, false)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, params := match(m, &http.Request{URL: &url.URL{Path: "/some/value0/value1"}})
	if r == nil || len(params) != 1 || params["wildcard"] != "/value0/value1" {
		t.Error("failed to match with wildcards", params["wildcard"])
	}
}

func TestFreeWildcardParamWithSlash(t *testing.T) {
	routes, err := eskip.Parse(`Path("/some/*wildcard") -> "https://example.org"`)
	if err != nil {
		t.Error(err)
	}

	m, errs := makeMatcher([]RouteDefinition{&routeDefinition{routes[0], &mock.FilterRegistry{make(map[string]skipper.FilterSpec)}}}, true)
	if len(errs) != 0 {
		t.Error("failed to make matcher")
	}

	r, params := match(m, &http.Request{URL: &url.URL{Path: "/some/value0/value1/"}})
	if r == nil || len(params) != 1 || params["wildcard"] != "/value0/value1" {
		t.Error("failed to match with wildcards", r == nil, len(params), params["wildcard"])
	}
}

func BenchmarkGenericMailgun(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testRouteInMailgun(b, "GET", "/tessera/header", "header.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/tessera/footer", "footer.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/some.html", "pdp.layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/nike", "catalog.layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sls-async/nike", "catalog-async.layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sc/nike", "catalogsc.compositor-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sls/nike", "catalogsls.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/slow", "bugfactory.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/debug", "debug.bugfactory.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/api/cart/42", "cart.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "POST", "/login", "login-fragment.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "POST", "/logout", "logout.login-fragment.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/healthcheck", "")
		testRouteInMailgun(b, "GET", "/humans.txt", "")
		testRouteInMailgun(b, "GET", "/assets/base-assets/some.css", "base-assets.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/assets/header/some.css", "assets.header.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/assets/footer/some.css", "assets.footer.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/assets/cart/some.css", "assets.cart.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/assets/pdp/some.css", "assets.pdp-fragment-alt.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/assets/catalog/some.css", "assets.catalog-face.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/assets/login/some.css", "assets.login-fragment.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/herren/nike", "herren.layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/damen/nike", "damen.layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sls-async/herren/nike", "herren-async.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sls-async/damen/nike", "damen-async.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sc/herren/nike", "herren-sc.compositor-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sc/damen/nike", "damen-sc.compositor-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sls/herren/nike", "herren-sls.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sls/damen/nike", "damen-sls.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/men/nike", "herren-en.layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/women/nike", "damen-en.layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sls-async/men/nike", "herren-en.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sls-async/women/nike", "damen-en.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sc/men/nike", "herren-en.compositor-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sc/women/nike", "damen-en.compositor-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sls/men/nike", "herren-en.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInMailgun(b, "GET", "/sls/women/nike", "damen-en.streaming-layout-service.mop-taskforce.zalan.do")
	}
}

func BenchmarkGenericSkipper(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testRouteInSkipper(b, "GET", "/tessera/header", "header.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/tessera/footer", "footer.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/some.html", "pdp.layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/nike", "catalog.layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sls-async/nike", "catalog-async.layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sc/nike", "catalogsc.compositor-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sls/nike", "catalogsls.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/slow", "bugfactory.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/debug", "debug.bugfactory.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/api/cart/42", "cart.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "POST", "/login", "login-fragment.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "POST", "/logout", "logout.login-fragment.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/healthcheck", "")
		testRouteInSkipper(b, "GET", "/humans.txt", "")
		testRouteInSkipper(b, "GET", "/assets/base-assets/some.css", "base-assets.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/assets/header/some.css", "assets.header.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/assets/footer/some.css", "assets.footer.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/assets/cart/some.css", "assets.cart.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/assets/pdp/some.css", "assets.pdp-fragment-alt.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/assets/catalog/some.css", "assets.catalog-face.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/assets/login/some.css", "assets.login-fragment.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/herren/nike", "herren.layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/damen/nike", "damen.layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sls-async/herren/nike", "herren-async.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sls-async/damen/nike", "damen-async.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sc/herren/nike", "herren-sc.compositor-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sc/damen/nike", "damen-sc.compositor-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sls/herren/nike", "herren-sls.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sls/damen/nike", "damen-sls.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/men/nike", "herren-en.layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/women/nike", "damen-en.layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sls-async/men/nike", "herren-en.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sls-async/women/nike", "damen-en.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sc/men/nike", "herren-en.compositor-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sc/women/nike", "damen-en.compositor-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sls/men/nike", "herren-en.streaming-layout-service.mop-taskforce.zalan.do")
		testRouteInSkipper(b, "GET", "/sls/women/nike", "damen-en.streaming-layout-service.mop-taskforce.zalan.do")
	}
}
