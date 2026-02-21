package routing_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/predicates/host"
	"github.com/zalando/skipper/routing"
)

// Benchmarks Host regular expression that matches optional port and trailing dot
func BenchmarkPredicateHostRegexpSingleWithDotAndPort(b *testing.B) {
	benchmarkPredicateHost(b, `Host("^(site{i}[.]example[.]org[.]?(:[0-9]+)?)$")`)
}

func BenchmarkPredicateHostRegexpMultipleWithDotAndPort(b *testing.B) {
	benchmarkPredicateHost(b, `Host("^(site{i}[.]example[.]com[.]?(:[0-9]+)?|site{i}[.]example[.]org[.]?(:[0-9]+)?)$")`)
}

// Benchmarks Host regular expression that only matches domain name
func BenchmarkPredicateHostRegexpSingleDomainOnly(b *testing.B) {
	benchmarkPredicateHost(b, `Host("^site{i}[.]example[.]org$")`)
}

func BenchmarkPredicateHostRegexpMultipleDomainOnly(b *testing.B) {
	benchmarkPredicateHost(b, `Host("^(site{i}[.]example[.]com|site{i}[.]example[.]org)$")`)
}

// Benchmarks HostAny that matches host using string comparison
func BenchmarkPredicateHostAnySingle(b *testing.B) {
	benchmarkPredicateHost(b, `HostAny("site{i}.example.org")`)
}

func BenchmarkPredicateHostAnyMultiple(b *testing.B) {
	benchmarkPredicateHost(b, `HostAny("site{i}.example.com", "site{i}.example.org")`)
}

var (
	benchmarkRouteSink *routing.Route
	benchmarkParamSink map[string]string
)

// Benchmarks multi-host setup where request matches only one host out of many using different host predicates.
// Creates R routes parametrized by sequence number from [0, R) and looks up using request that matches the last route.
func benchmarkPredicateHost(b *testing.B, predicateFmt string) {
	const R = 10000

	ha := host.NewAny()
	pr := map[string]routing.PredicateSpec{ha.Name(): ha}
	o := &routing.Options{
		FilterRegistry: make(filters.Registry),
	}

	var routes []*routing.Route
	for i := range R {
		p := strings.ReplaceAll(predicateFmt, "{i}", fmt.Sprintf("%d", i))
		def := eskip.MustParse(fmt.Sprintf(`r%d: %s -> <shunt>;`, i, p))

		route, err := routing.ExportProcessRouteDef(o, pr, def[0])
		if err != nil {
			b.Fatal(err)
		}
		routes = append(routes, route)
	}

	matcher, errs := routing.ExportNewMatcher(routes, routing.MatchingOptionsNone)
	if len(errs) > 0 {
		b.Fatal(errs)
	}

	testUrl := fmt.Sprintf(`https://site%d.example.org`, R-1)
	req, _ := http.NewRequest("GET", testUrl, nil)

	route, param := routing.ExportMatch(matcher, req)
	if route != routes[R-1] {
		b.Fatalf("expected to match last route %v", routes[R-1])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		route, param = routing.ExportMatch(matcher, req)
	}
	benchmarkRouteSink = route
	benchmarkParamSink = param
}
