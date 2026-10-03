package routing

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
)

// testHostAnySpec is a local replica of predicates/host.AnyPredicate for use
// in routing package tests. It avoids the import cycle
// routing → predicates/host → routing.
type testHostAnySpec struct{}
type testHostAnyPredicate struct{ hosts []string }

func (s *testHostAnySpec) Name() string { return "HostAny" }
func (s *testHostAnySpec) Create(args []any) (Predicate, error) {
	p := &testHostAnyPredicate{}
	for _, a := range args {
		h, ok := a.(string)
		if !ok {
			return nil, fmt.Errorf("HostAny: expected string argument")
		}
		p.hosts = append(p.hosts, h)
	}
	if len(p.hosts) == 0 {
		return nil, fmt.Errorf("HostAny: at least one host required")
	}
	return p, nil
}
func (p *testHostAnyPredicate) Match(r *http.Request) bool { return slices.Contains(p.hosts, r.Host) }
func (p *testHostAnyPredicate) MatchHosts() []string       { return p.hosts }

func docToMatcherHostTree(doc string) (*matcher, error) {
	defs, err := eskip.Parse(doc)
	if err != nil {
		return nil, err
	}

	routes, _, _ := processRouteDefs(&Options{
		Predicates: []PredicateSpec{&testHostAnySpec{}},
	}, defs)

	m, errs := newMatcher(routes, UseHostTree)
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("newMatcher errors: %s", strings.Join(msgs, "; "))
	}
	return m, nil
}

func reqHostPath(host, path string) *http.Request {
	r, _ := http.NewRequest("GET", "http://"+host+path, nil)
	r.Host = host
	return r
}

func TestUseHostTree_BasicMatch(t *testing.T) {
	m, err := docToMatcherHostTree(`r: HostAny("example.org") && Path("/foo") -> <shunt>;`)
	if err != nil {
		t.Fatal(err)
	}

	route, _ := m.match(reqHostPath("example.org", "/foo"))
	if route == nil {
		t.Fatal("want route match, got nil")
	}
	if route.Id != "r" {
		t.Errorf("want route id r, got %s", route.Id)
	}
}

func TestUseHostTree_NoMatchWrongHost(t *testing.T) {
	m, err := docToMatcherHostTree(`r: HostAny("example.org") && Path("/foo") -> <shunt>;`)
	if err != nil {
		t.Fatal(err)
	}

	route, _ := m.match(reqHostPath("other.org", "/foo"))
	if route != nil {
		t.Errorf("want no match for wrong host, got %s", route.Id)
	}
}

func TestUseHostTree_NoMatchWrongPath(t *testing.T) {
	m, err := docToMatcherHostTree(`r: HostAny("example.org") && Path("/foo") -> <shunt>;`)
	if err != nil {
		t.Fatal(err)
	}

	route, _ := m.match(reqHostPath("example.org", "/bar"))
	if route != nil {
		t.Errorf("want no match for wrong path, got %s", route.Id)
	}
}

func TestUseHostTree_MultipleHostsInHostAny(t *testing.T) {
	m, err := docToMatcherHostTree(`r: HostAny("a.org", "b.org") && Path("/foo") -> <shunt>;`)
	if err != nil {
		t.Fatal(err)
	}

	for _, h := range []string{"a.org", "b.org"} {
		route, _ := m.match(reqHostPath(h, "/foo"))
		if route == nil {
			t.Errorf("want match for host %s, got nil", h)
		}
	}
}

func TestUseHostTree_RouteWithoutHostAnyFallsBackToPathTree(t *testing.T) {
	m, err := docToMatcherHostTree(`
		r_host: HostAny("example.org") && Path("/api") -> <shunt>;
		r_any:  Path("/api") -> <shunt>;
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Request with a host that doesn't match HostAny; r_any has no host constraint
	// and should be found via the fallback path tree.
	route, _ := m.match(reqHostPath("other.org", "/api"))
	if route == nil {
		t.Fatal("want fallback match from path tree, got nil")
	}
	if route.Id != "r_any" {
		t.Errorf("want r_any, got %s", route.Id)
	}
}

func TestUseHostTree_HostAnyPredicateNotDoubleEvaluated(t *testing.T) {
	// A route with only HostAny+Path; the predicate is extracted as the outer
	// key, so the leaf's predicates slice should be empty. The route still matches.
	m, err := docToMatcherHostTree(`r: HostAny("example.org") && Path("/check") -> <shunt>;`)
	if err != nil {
		t.Fatal(err)
	}

	route, _ := m.match(reqHostPath("example.org", "/check"))
	if route == nil {
		t.Fatal("want match, got nil")
	}
}

func TestUseHostTree_PathSubtree(t *testing.T) {
	m, err := docToMatcherHostTree(`r: HostAny("example.org") && PathSubtree("/api") -> <shunt>;`)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{"/api", "/api/", "/api/v1/users"} {
		route, _ := m.match(reqHostPath("example.org", p))
		if route == nil {
			t.Errorf("want match for path %s, got nil", p)
		}
	}

	route, _ := m.match(reqHostPath("example.org", "/other"))
	if route != nil {
		t.Errorf("want no match for /other, got %s", route.Id)
	}
}

func TestUseHostTree_WildcardPathParam(t *testing.T) {
	m, err := docToMatcherHostTree(`r: HostAny("example.org") && Path("/users/:id") -> <shunt>;`)
	if err != nil {
		t.Fatal(err)
	}

	route, params := m.match(reqHostPath("example.org", "/users/42"))
	if route == nil {
		t.Fatal("want match, got nil")
	}
	if params["id"] != "42" {
		t.Errorf("want params[id]=42, got %v", params)
	}
}

func TestUseHostTree_DefaultOptionUnchanged(t *testing.T) {
	// Without UseHostTree, HostAny routes still work via the regular path tree.
	defs, err := eskip.Parse(`r: HostAny("example.org") && Path("/foo") -> <shunt>;`)
	if err != nil {
		t.Fatal(err)
	}
	routes, _, _ := processRouteDefs(&Options{
		Predicates: []PredicateSpec{&testHostAnySpec{}},
	}, defs)

	m, errs := newMatcher(routes, MatchingOptionsNone)
	if len(errs) > 0 {
		t.Fatalf("unexpected matcher errors: %v", errs)
	}

	route, _ := m.match(reqHostPath("example.org", "/foo"))
	if route == nil {
		t.Fatal("want match without UseHostTree, got nil")
	}
}

func TestUseHostTree_CombineWithIgnoreTrailingSlash(t *testing.T) {
	defs, err := eskip.Parse(`r: HostAny("example.org") && Path("/foo") -> <shunt>;`)
	if err != nil {
		t.Fatal(err)
	}
	routes, _, _ := processRouteDefs(&Options{
		Predicates: []PredicateSpec{&testHostAnySpec{}},
	}, defs)

	m, errs := newMatcher(routes, UseHostTree|IgnoreTrailingSlash)
	if len(errs) > 0 {
		t.Fatalf("unexpected matcher errors: %v", errs)
	}

	route, _ := m.match(reqHostPath("example.org", "/foo/"))
	if route == nil {
		t.Fatal("want match with trailing slash ignored, got nil")
	}
}

func BenchmarkUseHostTreeMatch_10kSameHost(b *testing.B) {
	// 10 000 routes all with HostAny("bench.test") and different paths.
	// The matching route is the catch-all r_last.
	var sb strings.Builder
	for i := range 9999 {
		fmt.Fprintf(&sb, "r%d: HostAny(\"bench.test\") && Path(\"/r%d\") -> <shunt>;\n", i, i)
	}
	sb.WriteString(`r_last: * -> <shunt>;`)

	defs, err := eskip.Parse(sb.String())
	if err != nil {
		b.Fatal(err)
	}
	routes, _, _ := processRouteDefs(&Options{
		Predicates: []PredicateSpec{&testHostAnySpec{}},
	}, defs)

	mBase, errs := newMatcher(routes, MatchingOptionsNone)
	if len(errs) > 0 {
		b.Fatalf("base matcher errors: %v", errs)
	}
	mHost, errs := newMatcher(routes, UseHostTree)
	if len(errs) > 0 {
		b.Fatalf("host tree matcher errors: %v", errs)
	}

	req := reqHostPath("bench.test", "/")

	b.Run("baseline", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mBase.match(req)
		}
	})
	b.Run("UseHostTree", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mHost.match(req)
		}
	})
}

func BenchmarkUseHostTreeMatch_10kDiffHost(b *testing.B) {
	// 10 000 routes each with a unique host and path.
	// 3 extra routes for "bench.test"; the request matches the last one.
	var sb strings.Builder
	for i := range 10000 {
		fmt.Fprintf(&sb, "r%d: HostAny(\"host%d.test\") && Path(\"/r%d\") -> <shunt>;\n", i, i, i)
	}
	sb.WriteString(`ra: HostAny("bench.test") && Path("/a") -> <shunt>;`)
	sb.WriteString(`rb: HostAny("bench.test") && Path("/b") -> <shunt>;`)
	sb.WriteString(`r_last: HostAny("bench.test") && Path("/") -> <shunt>;`)

	defs, err := eskip.Parse(sb.String())
	if err != nil {
		b.Fatal(err)
	}
	routes, _, _ := processRouteDefs(&Options{
		Predicates: []PredicateSpec{&testHostAnySpec{}},
	}, defs)

	mBase, errs := newMatcher(routes, MatchingOptionsNone)
	if len(errs) > 0 {
		b.Fatalf("base matcher errors: %v", errs)
	}
	mHost, errs := newMatcher(routes, UseHostTree)
	if len(errs) > 0 {
		b.Fatalf("host tree matcher errors: %v", errs)
	}

	req := reqHostPath("bench.test", "/")

	b.Run("baseline", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mBase.match(req)
		}
	})
	b.Run("UseHostTree", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mHost.match(req)
		}
	})
}
