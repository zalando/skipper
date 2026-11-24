package routing

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestHostIndex(t *testing.T) {
	for _, path := range []string{"", `Path("/") && `, `PathSubtree("/") && `, `Path("/:name") && `} {
		for _, condition := range []string{
			`Host("^(a[.]example[.]org[.]?(:[0-9]+)?|svc[.]ns[.]skipper[.]cluster[.]local[.]?(:[0-9]+)?|a[.]example[.]org[.]?(:[0-9]+)?)$")`,
			`Host("^(A[.]example[.]org[.]?(:[0-9]+)?)$")`,
			`Host("^(a[.]example[.]org[.][.]?(:[0-9]+)?)$")`,
			`Host("^(a[.]example[.]org[.]?(:[0-9]+)?)$") && Method("POST")`,
			`Host("^(a[.]example[.]org[.]?(:[0-9]+)?)$") && Host("^b")`,
			`Host("^(a[.]example[.]org[.]?(:[0-9]+)?|.*[.]example[.]org[.]?(:[0-9]+)?)$")`,
			`Host("^(a.example.org[.]?(:[0-9]+)?)$")`,
			`Host(".*")`,
			`True()`,
		} {
			for _, weights := range [][2]int{{0, 0}, {1, 0}, {0, 1}} {
				for _, reverse := range []bool{false, true} {
					fallback := ".*"
					if weights[0] > weights[1] {
						fallback = "^a"
					}
					doc := fmt.Sprintf(`exact: %s%s && Weight(%d) -> <shunt>; fallback: %sHost(%q) && Weight(%d) -> <shunt>;
					padding1: %sHost("^(padding1[.]?(:[0-9]+)?)$") -> <shunt>;
					padding2: %sHost("^(padding2[.]?(:[0-9]+)?)$") -> <shunt>;`, path, condition, weights[0], path, fallback, weights[1], path, path)
					routes, err := docToRoutes(doc)
					if err != nil {
						t.Fatal(err)
					}
					if reverse {
						routes[0], routes[1] = routes[1], routes[0]
					}
					indexed, err := newTestMatcher(routes)
					if err != nil {
						t.Fatal(err)
					}
					// An empty group preserves regexp matching but forces the original scan.
					for _, r := range routes {
						for i, rx := range r.HostRegexps {
							r.HostRegexps[i] = "(?:)" + rx
						}
					}
					scan, err := newTestMatcher(routes)
					if err != nil {
						t.Fatal(err)
					}
					for _, hostname := range []string{"a.example.org", "A.example.org", "A.EXAMPLE.ORG", "svc.ns.skipper.cluster.local", "unknown", "a.example.org.", "a.example.org..", "a.example.org:80", "a.example.org.:8080", "a.example.org..:80", "a.example.org:", "a.example.org:bad", "a.example.org:80:90", ""} {
						for _, method := range []string{"GET", "POST"} {
							requestPath := "/"
							if path == `Path("/:name") && ` {
								requestPath = "/item"
							}
							req := httptest.NewRequest(method, "http://example.org"+requestPath, nil)
							req.Host = hostname
							got, gp := indexed.match(req)
							want, wp := scan.match(req)
							if got != want || !reflect.DeepEqual(gp, wp) {
								t.Fatalf("%s host %q: got %v %v, want %v %v", doc, hostname, got, gp, want, wp)
							}
						}
					}
				}
			}
		}
	}
}

type changeHost struct{ calls *int }

func (p changeHost) Match(req *http.Request) bool {
	*p.calls++
	req.Host = "after.example.org"
	return false
}

func TestHostIndexPredicateOrder(t *testing.T) {
	for _, first := range []string{`True()`, `Host("^(before[.]example[.]org[.]?(:[0-9]+)?|after[.]example[.]org[.]?(:[0-9]+)?)$")`} {
		routes, err := docToRoutes(fmt.Sprintf(`first: %s && Weight(2) -> <shunt>;
		before: Host("^(before[.]example[.]org[.]?(:[0-9]+)?)$") -> <shunt>;
		after: Host("^(after[.]example[.]org[.]?(:[0-9]+)?)$") -> <shunt>;
		padding: Host("^(padding[.]?(:[0-9]+)?)$") -> <shunt>;`, first))
		if err != nil {
			t.Fatal(err)
		}
		var calls int
		routes[0].Predicates = []Predicate{changeHost{&calls}}
		m, err := newTestMatcher(routes)
		if err != nil {
			t.Fatal(err)
		}
		for _, initial := range []string{"before.example.org", "after.example.org"} {
			calls = 0
			req := httptest.NewRequest("GET", "http://"+initial+"/", nil)
			r, _ := m.match(req)
			if r == nil || r.Id != "after" || calls != 1 {
				t.Fatalf("%s host %s: got %v and %d calls, want after and one call", first, initial, r, calls)
			}
		}
	}
}

func TestLiteralHosts(t *testing.T) {
	for _, tt := range []struct {
		rx   string
		want []string
	}{
		{`^(a[.]test[.]?(:[0-9]+)?)$`, []string{"a.test"}},
		{`^(A-9[.]test[.]?(:[0-9]+)?|svc[.]ns[.]skipper[.]cluster[.]local[.]?(:[0-9]+)?)$`, []string{"A-9.test", "svc.ns.skipper.cluster.local"}},
		{`^(a[.]test[.][.]?(:[0-9]+)?)$`, []string{"a.test."}},
		{`^(a.test[.]?(:[0-9]+)?)$`, nil},
		{`^(a[.]test[.]?(:[0-9]+)?|.*[.]?(:[0-9]+)?)$`, nil},
		{`^(a[.]test[.]?(:[0-9]+)?|)$`, nil},
		{`^([.]?(:[0-9]+)?)$`, nil},
		{`(?i)^(a[.]test[.]?(:[0-9]+)?)$`, nil},
		{`^(a[.]test)$`, nil},
		{`^(a[.]test[.]?(:[0-9]+)?)`, nil},
	} {
		if got := literalHosts(tt.rx); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("literalHosts(%q) = %v, want %v", tt.rx, got, tt.want)
		}
	}
}
