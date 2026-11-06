package routing

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestHostIndex(t *testing.T) {
	for _, exprs := range [][]string{
		{`^(a[.]example[.]org[.]?(:[0-9]+)?)$`},
		{`^(a[.]example[.]org[.]?(:[0-9]+)?|svc[.]ns[.]skipper[.]cluster[.]local[.]?(:[0-9]+)?|a[.]example[.]org[.]?(:[0-9]+)?)$`},
		{`^(A-1[.]example[.][.]?(:[0-9]+)?|[.][.]?(:[0-9]+)?)$`},
		{`a.example.org`},
		{`(?i)^(a[.]example[.]org[.]?(:[0-9]+)?)$`},
		{`^(a.example.org[.]?(:[0-9]+)?)$`},
		{`^(a[.]example[.]org[.]?(:[0-9]+)?|.*)$`},
		{`^(a[.]example[.]org[.]?(:[0-9]+)?)$`, `^a`},
		{`^(a[.]example[.]org)$`},
		{`^([.]?(:[0-9]+)?)$`},
		{`^(é[.]example[.]org[.]?(:[0-9]+)?)$`},
	} {
		t.Run(strings.Join(exprs, ","), func(t *testing.T) {
			leaf := &leafMatcher{}
			for _, expr := range exprs {
				leaf.hostRxs = append(leaf.hostRxs, regexp.MustCompile(expr))
			}
			// Try the fallback both before and after the indexed leaf, and
			// also require a later candidate when another condition fails.
			for _, leaves := range []leafMatchers{
				{leaf},
				{&leafMatcher{hostRxs: []*regexp.Regexp{regexp.MustCompile(".*")}}, leaf},
				{leaf, &leafMatcher{}},
				{&leafMatcher{hostRxs: leaf.hostRxs, method: "POST"}, leaf, &leafMatcher{}},
			} {
				m := pathMatcher{leaves: leaves}
				m.indexHosts()
				for _, host := range []string{"a.example.org", "svc.ns.skipper.cluster.local", "A-1.example.", ".", "", "unknown", "A.example.org", "axbexamplexorg", "é.example.org"} {
					for _, suffix := range []string{"", ".", ":80", ".:443", ":", ":abc", ":1:2", "..", ":１２"} {
						req := &http.Request{Method: "GET", Host: host + suffix, URL: &url.URL{Path: "/"}}
						got := m.match(req, "/", "/")
						want := matchLeaves(leaves, req, "/", "/")
						if got != want {
							t.Fatalf("host %q: got %p, want %p", req.Host, got, want)
						}
					}
				}
			}
		})
	}
}

func TestHostIndexPriority(t *testing.T) {
	for _, path := range []string{``, `Path("/") && `, `PathSubtree("/") && `} {
		for _, doc := range []string{
			`fallback: %sHost(".*") && Weight(2) -> <shunt>; exact: %sHost("^(svc[.]ns[.]skipper[.]cluster[.]local[.]?(:[0-9]+)?)$") -> <shunt>;`,
			`exact: %sHost("^(svc[.]ns[.]skipper[.]cluster[.]local[.]?(:[0-9]+)?)$") && Weight(2) -> <shunt>; fallback: %sHost(".*") -> <shunt>;`,
			`fallback: %sHost(".*") -> <shunt>; exact: %sHost("^(svc[.]ns[.]skipper[.]cluster[.]local[.]?(:[0-9]+)?)$") -> <shunt>;`,
			`exact: %sHost("^(svc[.]ns[.]skipper[.]cluster[.]local[.]?(:[0-9]+)?)$") -> <shunt>; fallback: %sHost(".*") -> <shunt>;`,
		} {
			routes, err := docToRoutes(fmt.Sprintf(doc, path, path))
			if err != nil {
				t.Fatal(err)
			}
			m, errs := newMatcher(routes, MatchingOptionsNone)
			if len(errs) > 0 {
				t.Fatal(errs)
			}
			leaves := make(leafMatchers, len(routes))
			for i, r := range routes {
				leaves[i], err = newLeaf(r, make(map[string]*regexp.Regexp))
				if err != nil {
					t.Fatal(err)
				}
			}
			sort.Stable(leaves)
			for _, suffix := range []string{"", ".", ":80", ".:443"} {
				req := &http.Request{Method: "GET", Host: "svc.ns.skipper.cluster.local" + suffix, URL: &url.URL{Path: "/"}}
				got, _ := m.match(req)
				want := matchLeaves(leaves, req, "/", "/").route
				if got != want {
					t.Fatalf("%s: host %s: got %s, want %s", doc, req.Host, got.Id, want.Id)
				}
			}
		}
	}
}

func TestHostIndexFallback(t *testing.T) {
	m := pathMatcher{leaves: leafMatchers{{}, {hostRxs: []*regexp.Regexp{regexp.MustCompile(".*")}}}}
	m.indexHosts()
	if m.hostLeaves != nil || m.otherLeaves != nil {
		t.Fatal("list without literal hosts must retain the scan")
	}
}

type changeHostPredicate string

func (p changeHostPredicate) Match(req *http.Request) bool {
	req.Host = string(p)
	return false
}

func TestHostIndexChangedHost(t *testing.T) {
	for _, initial := range []string{"before.example.org", "after.example.org"} {
		leaves := leafMatchers{
			{predicates: []Predicate{changeHostPredicate("after.example.org")}},
			{hostRxs: []*regexp.Regexp{regexp.MustCompile(`^(before[.]example[.]org[.]?(:[0-9]+)?)$`)}},
			{hostRxs: []*regexp.Regexp{regexp.MustCompile(`^(after[.]example[.]org[.]?(:[0-9]+)?)$`)}},
		}
		m := pathMatcher{leaves: leaves}
		m.indexHosts()
		req := &http.Request{Host: initial}
		got := m.match(req, "/", "/")
		req.Host = initial
		want := matchLeaves(leaves, req, "/", "/")
		if got != want {
			t.Fatalf("host %s: got %p, want %p", initial, got, want)
		}
	}
}

type countHostPredicate int

func (p *countHostPredicate) Match(*http.Request) bool {
	*p++
	return false
}

func TestHostIndexDuplicateAlternatives(t *testing.T) {
	var calls countHostPredicate
	leaves := leafMatchers{
		{
			hostRxs:    []*regexp.Regexp{regexp.MustCompile(`^(a[.]example[.]org[.]?(:[0-9]+)?|a[.]example[.]org[.]?(:[0-9]+)?)$`)},
			predicates: []Predicate{&calls},
		},
		{},
	}
	m := pathMatcher{leaves: leaves}
	m.indexHosts()
	got := m.match(&http.Request{Host: "a.example.org"}, "/", "/")
	if got != leaves[1] || calls != 1 {
		t.Fatalf("got leaf %p and %d predicate calls, want %p and one call", got, calls, leaves[1])
	}
}
