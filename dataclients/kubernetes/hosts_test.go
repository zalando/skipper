package kubernetes

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/zalando/skipper/eskip"
	skippernet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/predicates/host"
)

func TestHostPredicate(t *testing.T) {
	patch := skippernet.HostPatch{RemovePort: true, RemoteTrailingDot: true, ToLower: true}
	for _, tt := range []struct {
		hosts []string
		name  string
	}{
		{[]string{"a.example.org"}, "HostAny"},
		{[]string{"svc.ns.skipper.cluster.local", "a.example.org"}, "HostAny"},
		{[]string{"A-1.example.", "."}, "HostAny"},
		{[]string{"a[0-9].example.org"}, "Host"},
		{[]string{"a.example.org", "a.*"}, "Host"},
		{[]string{""}, "Host"},
		{[]string{"é.example.org"}, "Host"},
	} {
		t.Run(createHostRx(tt.hosts...), func(t *testing.T) {
			p := createHostPredicate(tt.hosts...)
			if p.Name != tt.name {
				t.Fatalf("got %s, want %s", p.Name, tt.name)
			}
			old := regexp.MustCompile(createHostRx(tt.hosts...))
			match := func(req *http.Request) bool { return regexp.MustCompile(p.Args[0].(string)).MatchString(req.Host) }
			if p.Name == "HostAny" {
				predicate, err := host.NewAny().Create(p.Args)
				if err != nil {
					t.Fatal(err)
				}
				match = predicate.Match
			}
			for _, hostname := range []string{"a.example.org", "a1.example.org", "svc.ns.skipper.cluster.local", "A-1.example.", ".", "", "unknown", "é.example.org"} {
				for _, suffix := range []string{"", ".", ":80", ".:443", "..", "..:443", ":", ":abc", ":1:2"} {
					req := &http.Request{Host: patch.Apply(hostname + suffix)}
					if got, want := match(req), old.MatchString(req.Host); got != want {
						t.Fatalf("host %q normalized to %q: got %t, want %t", hostname+suffix, req.Host, got, want)
					}
				}
			}
		})
	}
	if createHostPredicate() != nil {
		t.Fatal("no hosts must not add a host condition")
	}
	if p := createHostPredicate("*.example.org"); p.Name != "Host" || p.Args[0] != createHostRx("*.example.org") {
		t.Fatal("malformed expressions must retain their original routing error")
	}
}

func TestHostAnyCatchAll(t *testing.T) {
	for _, extra := range []bool{false, true} {
		r := &eskip.Route{Predicates: []*eskip.Predicate{createHostPredicate("a.example.org")}}
		if extra {
			r.Predicates = append(r.Predicates, &eskip.Predicate{Name: "HostAny", Args: []any{"other.example.org"}})
		}
		rs := hostCatchAllRoutes(map[string][]*eskip.Route{"a.example.org": {r}}, func(string) string { return "catchall" })
		if (len(rs) == 1) != extra {
			t.Fatalf("extra host condition %t: got %d catch-all routes", extra, len(rs))
		}
	}
}
