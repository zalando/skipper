package routing_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/predicates/host"
	"github.com/zalando/skipper/routing"
)

// Hide MatchHosts from the reference matcher so it uses the original scan.
type scanHostSpec struct{ routing.PredicateSpec }

func (s scanHostSpec) Create(args []any) (routing.Predicate, error) {
	p, err := s.PredicateSpec.Create(args)
	return struct{ routing.Predicate }{p}, err
}

func TestHostIndex(t *testing.T) {
	for _, path := range []string{"", `Path("/") && `, `PathSubtree("/") && `} {
		for _, conditions := range []string{
			`HostAny("a.example.org", "svc.ns.skipper.cluster.local", "a.example.org")`,
			`HostAny("a.example.org") && Method("POST")`,
			`HostAny("a.example.org") && Host("^b")`,
			`Host(".*")`,
		} {
			for _, weights := range [][2]int{{0, 0}, {1, 0}, {0, 1}} {
				for _, exactFirst := range []bool{false, true} {
					doc := fmt.Sprintf(`exact: %s%s && Weight(%d) -> <shunt>; fallback: %sHost(".*") && Weight(%d) -> <shunt>;`, path, conditions, weights[0], path, weights[1])
					defs := eskip.MustParse(doc + `padding1: HostAny("padding1") -> <shunt>; padding2: HostAny("padding2") -> <shunt>;`)
					if !exactFirst {
						defs[0], defs[1] = defs[1], defs[0]
					}
					h := host.NewAny()
					indexed, err := routing.NewRoutesMatcher(&routing.Options{FilterRegistry: make(filters.Registry), Predicates: []routing.PredicateSpec{h}}, defs)
					if err != nil {
						t.Fatal(err)
					}
					scan, err := routing.NewRoutesMatcher(&routing.Options{FilterRegistry: make(filters.Registry), Predicates: []routing.PredicateSpec{scanHostSpec{h}}}, defs)
					if err != nil {
						t.Fatal(err)
					}
					for _, hostname := range []string{"a.example.org", "svc.ns.skipper.cluster.local", "unknown", "a.example.org.", "a.example.org:80", ""} {
						req := httptest.NewRequest("GET", "http://example.org/", nil)
						req.Host = hostname
						got, gp := indexed.Match(req)
						want, wp := scan.Match(req)
						if got.Id != want.Id || !reflect.DeepEqual(gp, wp) {
							t.Fatalf("%s host %q: got %s %v, want %s %v", doc, hostname, got.Id, gp, want.Id, wp)
						}
					}
				}
			}
		}
	}
}

type changeHostSpec struct{ calls *int }
type changeHost struct{ calls *int }

func (changeHostSpec) Name() string                              { return "ChangeHost" }
func (s changeHostSpec) Create([]any) (routing.Predicate, error) { return changeHost(s), nil }
func (p changeHost) Match(req *http.Request) bool {
	*p.calls++
	req.Host = "after.example.org"
	return false
}

func TestHostIndexPredicateOrder(t *testing.T) {
	for _, first := range []string{
		`ChangeHost()`,
		`ChangeHost() && HostAny("unknown")`,
		`HostAny("before.example.org", "before.example.org", "after.example.org", "after.example.org") && ChangeHost()`,
	} {
		defs := eskip.MustParse(fmt.Sprintf(`first: %s && Weight(2) -> <shunt>;
		before: HostAny("before.example.org") -> <shunt>;
		after: HostAny("after.example.org") -> <shunt>; padding: HostAny("padding") -> <shunt>;`, first))
		var calls int
		m, err := routing.NewRoutesMatcher(&routing.Options{
			FilterRegistry: make(filters.Registry),
			Predicates:     []routing.PredicateSpec{host.NewAny(), changeHostSpec{&calls}},
		}, defs)
		if err != nil {
			t.Fatal(err)
		}
		for _, initial := range []string{"before.example.org", "after.example.org"} {
			calls = 0
			req := httptest.NewRequest("GET", "http://"+initial+"/", nil)
			r, _ := m.Match(req)
			if r == nil || r.Id != "after" || calls != 1 {
				t.Fatalf("%s host %s: got %v and %d calls, want after and one call", first, initial, r, calls)
			}
		}
	}
}
