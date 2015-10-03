package routing

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/routing/testdataclient"
	"net/http"
	"testing"
	"time"
)

type matcherFunc func(req *http.Request) (*Route, map[string]string)

func testMatcherWithPath(t *testing.T, m matcherFunc, path string, matchRoute *Route) {
	req, err := http.NewRequest("GET", "http://www.example.com"+path, nil)
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
		f, ok := fv.(*filtertest.Filter)
		if !ok {
			t.Error("failed to match route")
			return
		}

		mf, ok := matchRoute.Filters[i].(*filtertest.Filter)
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

func testMatcherNoPath(t *testing.T, m matcherFunc, matchRoute *Route) {
	testMatcherWithPath(t, m, "", matchRoute)
}

// used to let the data client updates be propagated
func delay() { time.Sleep(3 * time.Millisecond) }

func TestUsesDataFromClientAfterInitialized(t *testing.T) {
	r := New(testdataclient.New(`Any() -> "https://www.example.org"`), make(filters.Registry), false)
	delay()
	testMatcherNoPath(t, r.Route, &Route{Scheme: "https", Host: "www.example.org"})
}

func TestKeepUsingDataFromClient(t *testing.T) {
	r := New(testdataclient.New(`Any() -> "https://www.example.org"`), make(filters.Registry), false)
	delay()
	testMatcherNoPath(t, r.Route, &Route{Scheme: "https", Host: "www.example.org"})
	testMatcherNoPath(t, r.Route, &Route{Scheme: "https", Host: "www.example.org"})
	testMatcherNoPath(t, r.Route, &Route{Scheme: "https", Host: "www.example.org"})
}

func TestInitialAndUpdates(t *testing.T) {
	fspec1 := &filtertest.Filter{FilterName: "testFilter1"}
	fspec2 := &filtertest.Filter{FilterName: "testFilter2"}
	fr := make(filters.Registry)
	fr[fspec1.Name()] = fspec1
	fr[fspec2.Name()] = fspec2

	doc := `
        route1: Any() -> "https://www.example.org";
        route2: Path("/some") -> testFilter1(1, "one") -> "https://some.example.org"
    `

	dc := testdataclient.New(doc)
	r := New(dc, fr, false)
	delay()

	testMatcherWithPath(t, r.Route, "", &Route{Scheme: "https", Host: "www.example.org"})
	testMatcherWithPath(t, r.Route, "/some", &Route{Scheme: "https", Host: "some.example.org",
		Filters: []filters.Filter{&filtertest.Filter{FilterName: "testFilter1", Args: []interface{}{float64(1), "one"}}}})
	testMatcherWithPath(t, r.Route, "/some-other", &Route{Scheme: "https", Host: "www.example.org"})

	updatedDoc := `
        route1: Any() -> "https://www.example.org";
        route2: Path("/some") -> testFilter1(1, "one") -> "https://some-updated.example.org";
        route2: Path("/some-other") -> testFilter2(2, "two") -> "https://some-other.example.org"
    `
	dc.Feed(updatedDoc)

	delay()

	testMatcherWithPath(t, r.Route, "", &Route{Scheme: "https", Host: "www.example.org"})
	testMatcherWithPath(t, r.Route, "/some", &Route{Scheme: "https", Host: "some-updated.example.org",
		Filters: []filters.Filter{&filtertest.Filter{FilterName: "testFilter1", Args: []interface{}{float64(1), "one"}}}})
	testMatcherWithPath(t, r.Route, "/some-other", &Route{Scheme: "https", Host: "some-other.example.org",
		Filters: []filters.Filter{&filtertest.Filter{FilterName: "testFilter2", Args: []interface{}{float64(2), "two"}}}})
}

func TestFailToParse(t *testing.T) {
	_, err := processData(nil, false, "invalid eskip document")
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestCreateShuntBackend(t *testing.T) {
	m, err := processData(nil, false, `Any() -> <shunt>`)
	if err != nil {
		t.Error(err)
	}

	testMatcherNoPath(t, m.match, &Route{Route: eskip.Route{Shunt: true}})
}

func TestFailToParseBackend(t *testing.T) {
	m, err := processData(nil, false, `Any() -> "invalid backend"`)
	if err != nil {
		t.Error(err)
	}

	testMatcherNoPath(t, m.match, nil)
}

func TestParseBackend(t *testing.T) {
	m, err := processData(nil, false, `Any() -> "https://www.example.org"`)
	if err != nil {
		t.Error(err)
	}

	testMatcherNoPath(t, m.match, &Route{Scheme: "https", Host: "www.example.org"})
}

func TestFilterNotFound(t *testing.T) {
	spec1 := &filtertest.Filter{FilterName: "testFilter1"}
	spec2 := &filtertest.Filter{FilterName: "testFilter2"}
	fr := make(filters.Registry)
	fr[spec1.Name()] = spec1
	fr[spec2.Name()] = spec2

	m, err := processData(fr, false, `Any() -> testFilter3() -> "https://www.example.org"`)
	if err != nil {
		t.Error(err)
	}

	testMatcherNoPath(t, m.match, nil)
}

func TestCreateFilters(t *testing.T) {
	spec1 := &filtertest.Filter{FilterName: "testFilter1"}
	spec2 := &filtertest.Filter{FilterName: "testFilter2"}
	fr := make(filters.Registry)
	fr[spec1.Name()] = spec1
	fr[spec2.Name()] = spec2

	m, err := processData(fr, false, `Any() -> testFilter1(1, "one") -> testFilter2(2, "two") -> "https://www.example.org"`)
	if err != nil {
		t.Error(err)
	}

	testMatcherNoPath(t, m.match, &Route{Scheme: "https", Host: "www.example.org", Filters: []filters.Filter{
		&filtertest.Filter{FilterName: "testFilter1", Args: []interface{}{float64(1), "one"}},
		&filtertest.Filter{FilterName: "testFilter2", Args: []interface{}{float64(2), "two"}}}})
}
