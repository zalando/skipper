package settings

import (
    "github.com/zalando/skipper/skipper"
    "github.com/zalando/eskip"
    "net/url"
    "net/http"
    "fmt"
    "testing"
)

const routeDoc = `
    header:
        Path("/tessera/header") -> xalando() -> pathRewrite(/.*/, "/") -> requestHeader("Host",
        "header.mop-taskforce.zalan.do") -> "https://header.mop-taskforce.zalan.do";

    footer:
        Path("/tessera/footer") -> xalando() -> pathRewrite(/.*/, "/") -> requestHeader("Host",
        "footer.mop-taskforce.zalan.do") -> "https://footer.mop-taskforce.zalan.do";

    pdp:
        PathRegexp(/.*\.html/) -> xalando() -> requestHeader("Host",
        "layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/pdp") ->
        "https://layout-service.mop-taskforce.zalan.do";

    pdpAsync:
        PathRegexp(/\/sls-async\/.*\.html/) -> xalando() -> requestHeader("Host",
        "layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/pdp-async") ->
        "https://streamin-layout-service.mop-taskforce.zalan.do";

    pdpsc:
        PathRegexp(/\/sc\/.*\.html/) -> xalando() -> requestHeader("Host", "compositor-layout-service.mop-taskforce.zalan.do")
        -> pathRewrite(/.*/, "/pdp") -> "https://compositor-layout-service.mop-taskforce.zalan.do";

    pdpsls:
        PathRegexp(/\/sls\/.*\.html/) -> xalando() -> requestHeader("Host",
        "streaming-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/pdp") ->
        "https://streaming-layout-service.mop-taskforce.zalan.do";

    catalog:
        Path("/<string>") -> xalando() -> requestHeader("Host",
        "layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://layout-service.mop-taskforce.zalan.do";

    catalogAsync:
        Path("/sls-async/<string>") -> xalando() -> requestHeader("Host",
        "layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog-async") ->
        "https://layout-service.mop-taskforce.zalan.do";

    catalogsc:
        Path("/sc/<string>") -> xalando() -> requestHeader("Host", "compositor-layout-service.mop-taskforce.zalan.do") ->
        pathRewrite(/.*/, "/catalog") -> "https://compositor-layout-service.mop-taskforce.zalan.do";

    catalogsls:
        Path("/sls/<string>") -> xalando() -> requestHeader("Host", "streaming-layout-service.mop-taskforce.zalan.do")
        -> pathRewrite(/.*/, "/catalog") -> "https://streaming-layout-service.mop-taskforce.zalan.do";

    slow:
        Path("/slow") -> xalando() -> requestHeader("Host", "bugfactory.mop-taskforce.zalan.do") ->
        "https://bugfactory.mop-taskforce.zalan.do";

    debug:
        Path("/debug") -> xalando() -> pathRewrite(/.*/, "/") -> requestHeader("Host",
        "bugfactory.mop-taskforce.zalan.do") -> "https://bugfactory.mop-taskforce.zalan.do";

    cart:
        PathRegexp(/\/api\/cart\/.*/) -> xalando() -> requestHeader("Host", "cart-taskforce.zalan.do") ->
        "https://cart.mop-taskforce.zalan.do";

    login:
        Path("/login") && Method("POST") -> xalando() ->
        "https://login-fragment.mop-taskforce.zalan.do";

    logout:
        Path("/logout") && Method("POST") -> xalando() ->
        "https://login-fragment.mop-taskforce.zalan.do";

    healthcheck:
        Path("/healthcheck") -> healthcheck() -> <shunt>;

    humanstxt:
        Path("/humans.txt") -> humanstxt() -> <shunt>;

    baseAssetsAssets:
        Path("/assets/base-assets/<string>") -> pathRewrite("^/assets/base-assets", "/assets") ->
        requestHeader("Host", "base-assets.mop-taskforce.zalan.do") ->
        "https://base-assets.mop-taskforce.zalan.do";

    headerAssets:
        Path("/assets/header/<string>") -> pathRewrite("^/assets/header", "/assets") ->
        requestHeader("Host", "header.mop-taskforce.zalan.do") -> "https://header.mop-taskforce.zalan.do";

    footerAssets:
        Path("/assets/footer/<string>") -> pathRewrite("^/assets/footer", "/assets") ->
        requestHeader("Host", "footer.mop-taskforce.zalan.do") -> "https://footer.mop-taskforce.zalan.do";

    cartAssets:
        Path("/assets/cart/<string>") -> pathRewrite("^/assets/cart", "/assets") ->
        requestHeader("Host", "cart.mop-taskforce.zalan.do") -> "https://cart.mop-taskforce.zalan.do";

    pdpAssets:
        Path("/assets/pdp/<string>") -> pathRewrite("^/assets/pdp", "") -> requestHeader("Host",
        "pdp-fragment-alt.mop-taskforce.zalan.do") -> "https://pdp-fragment-alt.mop-taskforce.zalan.do";

    catalogAssets:
        Path("/assets/catalog/<string>") -> pathRewrite("^/assets/catalog", "/static") ->
        requestHeader("Host", "catalog-face.mop-taskforce.zalan.do") ->
        "https://catalog-face.mop-taskforce.zalan.do";

    loginAssets:
        Path("/assets/login/<string>") -> pathRewrite("^/assets/login", "/") -> requestHeader("Host",
        "login-fragment.mop-taskforce.zalan.do") -> "https://login-fragment.mop-taskforce.zalan.do";

    // some demo hack:

    // de
    catalogHerren:
        Path("/herren/<string>") -> xalando() -> requestHeader("Host",
        "layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://layout-service.mop-taskforce.zalan.do";

    catalogDamen:
        Path("/damen/<string>") -> xalando() -> requestHeader("Host",
        "layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://layout-service.mop-taskforce.zalan.do";

    catalogAsyncHerren:
        Path("/sls-async/herren/<string>") -> xalando() -> requestHeader("Host",
        "streamin-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog-async") ->
        "https://streamin-layout-service.mop-taskforce.zalan.do";

    catalogAsyncDamen:
        Path("/sls-async/damen/<string>") -> xalando() -> requestHeader("Host",
        "streamin-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog-async") ->
        "https://streamin-layout-service.mop-taskforce.zalan.do";

    catalogscHerren:
        Path("/sc/herren/<string>") -> xalando() -> requestHeader("Host",
        "compositor-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://compositor-layout-service.mop-taskforce.zalan.do";

    catalogscDamen:
        Path("/sc/damen/<string>") -> xalando() -> requestHeader("Host",
        "compositor-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://compositor-layout-service.mop-taskforce.zalan.do";

    catalogslsHerren:
        Path("/sls/herren/<string>") -> xalando() -> requestHeader("Host",
        "streamin-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://streamin-layout-service.mop-taskforce.zalan.do";

    catalogslsDamen:
        Path("/sls/damen/<string>") -> xalando() -> requestHeader("Host",
        "streaming-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://streaming-layout-service.mop-taskforce.zalan.do";

    // en
    catalogHerrenEn:
        Path("/men/<string>") -> xalando() -> requestHeader("Host",
        "layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://layout-service.mop-taskforce.zalan.do";

    catalogDamenEn:
        Path("/women/<string>") -> xalando() -> requestHeader("Host",
        "layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://layout-service.mop-taskforce.zalan.do";

    catalogAsyncHerrenEn:
        Path("/sls-async/men/<string>") -> xalando() -> requestHeader("Host",
        "streaming-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog-async") ->
        "https://streaming-layout-service.mop-taskforce.zalan.do";

    catalogAsyncDamenEn:
        Path("/sls-async/women/<string>") -> xalando() -> requestHeader("Host",
        "streaming-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog-async") ->
        "https://streaming-layout-service.mop-taskforce.zalan.do";

    catalogscHerrenEn:
        Path("/sc/men/<string>") -> xalando() -> requestHeader("Host",
        "compositor-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://compositor-layout-service.mop-taskforce.zalan.do";

    catalogscDamenEn:
        Path("/sc/women/<string>") -> xalando() -> requestHeader("Host",
        "compositor-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://compositor-layout-service.mop-taskforce.zalan.do";

    catalogslsHerrenEn:
        Path("/sls/men/<string>") -> xalando() -> requestHeader("Host",
        "streaming-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://streaming-layout-service.mop-taskforce.zalan.do";

    catalogslsDamenEn:
        Path("/sls/women/<string>") -> xalando() -> requestHeader("Host",
        "streaming-layout-service.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") ->
        "https://streaming-layout-service.mop-taskforce.zalan.do";
`

type testFilter struct {
    name string
    args []string
}

func (tf *testFilter) Id() string { return "" }
func (tf *testFilter) Request(ctx skipper.FilterContext) {}
func (tf *testFilter) Response(ctx skipper.FilterContext) {}

type testBackend struct {
    scheme string
    host string
    isShunt bool
}

func (tb *testBackend) Scheme() string { return "https" }
func (tb *testBackend) Host() string { return tb.host }
func (tb *testBackend) IsShunt() bool { return false }

type routeDefinition struct {
    eskipRoute *eskip.Route
}

func (rd *routeDefinition) Id() string {
    return rd.eskipRoute.Id
}

func (rd *routeDefinition) Path() string {
    for _, m := range rd.eskipRoute.Matchers {
        if (m.Name == "Path" || m.Name == "PathRegexp") && len(m.Args) > 0 {
            p, _ := m.Args[0].(string)
            return p
        }
    }

    return ""
}

func (rd *routeDefinition) IsPathRegexp() bool {
    for _, m := range rd.eskipRoute.Matchers {
        if m.Name == "PathRegexp" {
            return true
        }
    }

    return false
}

func (rd *routeDefinition) HostRegexps() []string {
    var hostRxs []string
    for _, m := range rd.eskipRoute.Matchers {
        if m.Name == "HostRegexp" && len(m.Args) > 0 {
            rx, _ := m.Args[0].(string)
            hostRxs = append(hostRxs, rx)
        }
    }

    return hostRxs
}

func (rd *routeDefinition) Method() string {
    for _, m := range rd.eskipRoute.Matchers {
        if m.Name == "Method" && len(m.Args) > 0 {
            method, _ := m.Args[0].(string)
            return method
        }
    }

    return ""
}

func (rd *routeDefinition) Headers() map[string]string {
    headers := make(map[string]string)
    for _, m := range rd.eskipRoute.Matchers {
        if m.Name == "Header" && len(m.Args) >= 2 {
            k, _ := m.Args[0].(string)
            v, _ := m.Args[1].(string)
            headers[k] = v
        }
    }

    return headers
}

func (rd *routeDefinition) HeaderRegexps() map[string]string {
    headers := make(map[string]string)
    for _, m := range rd.eskipRoute.Matchers {
        if m.Name == "HeaderRegexp" && len(m.Args) >= 2 {
            k, _ := m.Args[0].(string)
            v, _ := m.Args[1].(string)
            headers[k] = v
        }
    }

    return headers
}

func (rd *routeDefinition) Filters() []skipper.Filter {
    var filters []skipper.Filter
    for _, f := range rd.eskipRoute.Filters {
        args := make([]string, len(f.Args))
        for i, a := range f.Args {
            s, _ := a.(string)
            args[i] = s
        }

        filters = append(filters, &testFilter{f.Name, args})
    }

    return filters
}

func (rd *routeDefinition) Backend() skipper.Backend {
    b := &testBackend{}
    if rd.eskipRoute.Shunt {
        b.isShunt = true
    } else {
        u, err := url.Parse(rd.eskipRoute.Backend)
        if err != nil {
            // fine for now, in test:
            panic(err)
        }

        b.scheme = u.Scheme
        b.host = u.Host
    }

    return b
}

var (
    definitions []RouteDefinition
    rtskipper skipper.Router
    rtmailgun skipper.Router
)

func init() {
    routes, err := eskip.Parse(routeDoc)
    if err != nil {
        panic(err)
    }

    definitions = make([]RouteDefinition, len(routes))
    for i, r := range routes {
        definitions[i] = &routeDefinition{r}
    }

    rt, err := Make(definitions)
    if err != nil {
        panic(err)
    }

    rtskipper = rt
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

func TestSkipperHeader(t *testing.T) {
    req, err := makeRequest("GET", "/tessera/header")
    if err != nil {
        t.Error(err)
    }

    rt, _, err := rtskipper.Route(req)
    if err != nil {
        t.Error(err)
    }

    if rt == nil {
        t.Error("failed to match path")
    }

    if rt.Backend().Host() != "header.mop-taskforce.zalan.do" {
        t.Error("failed to find the correct route")
    }
}
