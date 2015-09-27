package routing

import (
    "testing"
    "github.com/zalando/skipper/filters"
    "net/http"
    "time"
    "github.com/zalando/skipper/requestmatch"
)

type testDataClient struct {
    data chan string
}

func newDataClient(data string) *testDataClient {
    dc := &testDataClient{make(chan string)}
    dc.feed(data)
    return dc
}

func (dc *testDataClient) Receive() <-chan string { return dc.data }
func (dc *testDataClient) feed(data string) { go func() { dc.data <- data }() }

type testFilter struct {
    name string
    args []interface{}
}

func (spec *testFilter) Name() string { return spec.name }
func (f *testFilter) Request(ctx filters.FilterContext) {}
func (f *testFilter) Response(ctx filters.FilterContext) {}

func (spec *testFilter) CreateFilter(config []interface{}) (filters.Filter, error) {
    return &testFilter{name: spec.name, args: config}, nil
}

type matcher func(req *http.Request) (*Route, map[string]string)

func castMatcher(m *requestmatch.Matcher) matcher {
    return func(req *http.Request) (*Route, map[string]string) {
        v, p := m.Match(req)
        r, _ := v.(*Route)
        return r, p
    }
}

func testMatcherWithPath(t *testing.T, m matcher, path string, matchRoute *Route) {
    req, err := http.NewRequest("GET", "http://www.example.com" + path, nil)
    if err != nil {
        t.Error(err)
        return
    }

    rt, _ := m(req)
    if matchRoute == nil {
        if rt != nil {
            t.Error("failed not to match")
        }

        return
    }

    if matchRoute.Shunt {
        if !rt.Shunt {
            t.Error("failed to match shunt route")
        }

        return
    }

    if rt.Address != matchRoute.Address {
        t.Error("failed to match route")
        return
    }

    if len(rt.Filters) != len(matchRoute.Filters) {
        t.Error("failed to match route")
        return
    }

    for i, fv := range rt.Filters {
        f, ok := fv.(*testFilter)
        if !ok {
            t.Error("failed to match route")
            return
        }

        mf, ok := matchRoute.Filters[i].(*testFilter)
        if !ok {
            t.Error("failed to match route")
            return
        }

        if f.name != mf.name {
            t.Error("failed to match route")
            return
        }

        if len(f.args) != len(mf.args) {
            t.Error("failed to match route")
            return
        }

        for j, a := range f.args {
            if a != mf.args[j] {
                t.Error("failed to match route")
                return
            }
        }
    }
}

func testMatcher(t *testing.T, m matcher, matchRoute *Route) {
    testMatcherWithPath(t, m, "", matchRoute)
}

func delay() { time.Sleep(3 * time.Millisecond) }

func TestUsesEmptyMatcherUntilInitializedFromDataClient(t *testing.T) {
    r := New(newDataClient(`Any() -> "https://www.example.org"`), make(filters.Registry), false)
    testMatcher(t, r.Route, nil)
}

func TestUsesDataFromClientAfterInitialized(t *testing.T) {
    r := New(newDataClient(`Any() -> "https://www.example.org"`), make(filters.Registry), false)
    delay()
    testMatcher(t, r.Route, &Route{"https://www.example.org", false, nil})
}

func TestKeepUsingDataFromClient(t *testing.T) {
    r := New(newDataClient(`Any() -> "https://www.example.org"`), make(filters.Registry), false)
    testMatcher(t, r.Route, nil)
    delay()
    testMatcher(t, r.Route, &Route{"https://www.example.org", false, nil})
    testMatcher(t, r.Route, &Route{"https://www.example.org", false, nil})
    testMatcher(t, r.Route, &Route{"https://www.example.org", false, nil})
}

func TestInitialAndUpdates(t *testing.T) {
    fspec1 := &testFilter{name: "testFilter1"}
    fspec2 := &testFilter{name: "testFilter2"}
    fr := make(filters.Registry)
    fr[fspec1.Name()] = fspec1;
    fr[fspec2.Name()] = fspec2;

    doc := `
        route1: Any() -> "https://www.example.org";
        route2: Path("/some") -> testFilter1(1, "one") -> "https://some.example.org"
    `

    dc := newDataClient(doc)
    r := New(dc, fr, false)
    testMatcher(t, r.Route, nil)

    delay()

    testMatcherWithPath(t, r.Route, "", &Route{"https://www.example.org", false, nil})
    testMatcherWithPath(t, r.Route, "/some", &Route{"https://some.example.org", false,
        []filters.Filter{&testFilter{name: "testFilter1", args: []interface{}{float64(1), "one"}}}})
    testMatcherWithPath(t, r.Route, "/some-other", &Route{"https://www.example.org", false, nil})

    updatedDoc := `
        route1: Any() -> "https://www.example.org";
        route2: Path("/some") -> testFilter1(1, "one") -> "https://some-updated.example.org";
        route2: Path("/some-other") -> testFilter2(2, "two") -> "https://some-other.example.org"
    `
    dc.feed(updatedDoc)

    delay()

    testMatcherWithPath(t, r.Route, "", &Route{"https://www.example.org", false, nil})
    testMatcherWithPath(t, r.Route, "/some", &Route{"https://some-updated.example.org", false,
        []filters.Filter{&testFilter{name: "testFilter1", args: []interface{}{float64(1), "one"}}}})
    testMatcherWithPath(t, r.Route, "/some-other", &Route{"https://some-other.example.org", false,
        []filters.Filter{&testFilter{name: "testFilter2", args: []interface{}{float64(2), "two"}}}})
}
