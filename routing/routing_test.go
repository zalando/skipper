package routing

import (
    "testing"
    "github.com/zalando/skipper/filters"
    "github.com/zalando/skipper/filters/testfilter"
    "github.com/zalando/skipper/routing/testdataclient"
    "net/http"
    "time"
    "github.com/zalando/skipper/requestmatch"
)

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

    if rt.Scheme != matchRoute.Scheme || rt.Host != matchRoute.Host {
        t.Error("failed to match route")
        return
    }

    if len(rt.Filters) != len(matchRoute.Filters) {
        t.Error("failed to match route")
        return
    }

    for i, fv := range rt.Filters {
        f, ok := fv.(*testfilter.T)
        if !ok {
            t.Error("failed to match route")
            return
        }

        mf, ok := matchRoute.Filters[i].(*testfilter.T)
        if !ok {
            t.Error("failed to match route")
            return
        }

        if f.FilterName != mf.FilterName {
            t.Error("failed to match route")
            return
        }

        if len(f.Args) != len(mf.Args) {
            t.Error("failed to match route")
            return
        }

        for j, a := range f.Args {
            if a != mf.Args[j] {
                t.Error("failed to match route")
                return
            }
        }
    }
}

func testMatcher(t *testing.T, m matcher, matchRoute *Route) {
    testMatcherWithPath(t, m, "", matchRoute)
}

// used to let the data client updates be propagated
func delay() { time.Sleep(3 * time.Millisecond) }

func TestUsesDataFromClientAfterInitialized(t *testing.T) {
    r := New(testdataclient.New(`Any() -> "https://www.example.org"`), make(filters.Registry), false)
    delay()
    testMatcher(t, r.Route, &Route{&Backend{"https", "www.example.org", false}, nil})
}

func TestKeepUsingDataFromClient(t *testing.T) {
    r := New(testdataclient.New(`Any() -> "https://www.example.org"`), make(filters.Registry), false)
    delay()
    testMatcher(t, r.Route, &Route{&Backend{"https", "www.example.org", false}, nil})
    testMatcher(t, r.Route, &Route{&Backend{"https", "www.example.org", false}, nil})
    testMatcher(t, r.Route, &Route{&Backend{"https", "www.example.org", false}, nil})
}

func TestInitialAndUpdates(t *testing.T) {
    fspec1 := &testfilter.T{FilterName: "testFilter1"}
    fspec2 := &testfilter.T{FilterName: "testFilter2"}
    fr := make(filters.Registry)
    fr[fspec1.Name()] = fspec1;
    fr[fspec2.Name()] = fspec2;

    doc := `
        route1: Any() -> "https://www.example.org";
        route2: Path("/some") -> testFilter1(1, "one") -> "https://some.example.org"
    `

    dc := testdataclient.New(doc)
    r := New(dc, fr, false)
    delay()

    testMatcherWithPath(t, r.Route, "", &Route{&Backend{"https", "www.example.org", false}, nil})
    testMatcherWithPath(t, r.Route, "/some", &Route{&Backend{"https", "some.example.org", false},
        []filters.Filter{&testfilter.T{FilterName: "testFilter1", Args: []interface{}{float64(1), "one"}}}})
    testMatcherWithPath(t, r.Route, "/some-other", &Route{&Backend{"https", "www.example.org", false}, nil})

    updatedDoc := `
        route1: Any() -> "https://www.example.org";
        route2: Path("/some") -> testFilter1(1, "one") -> "https://some-updated.example.org";
        route2: Path("/some-other") -> testFilter2(2, "two") -> "https://some-other.example.org"
    `
    dc.Feed(updatedDoc)

    delay()

    testMatcherWithPath(t, r.Route, "", &Route{&Backend{"https", "www.example.org", false}, nil})
    testMatcherWithPath(t, r.Route, "/some", &Route{&Backend{"https", "some-updated.example.org", false},
        []filters.Filter{&testfilter.T{FilterName: "testFilter1", Args: []interface{}{float64(1), "one"}}}})
    testMatcherWithPath(t, r.Route, "/some-other", &Route{&Backend{"https", "some-other.example.org", false},
        []filters.Filter{&testfilter.T{FilterName: "testFilter2", Args: []interface{}{float64(2), "two"}}}})
}
